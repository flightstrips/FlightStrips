package ecfmp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/models"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CandidateFetch is constructed only by the future NATS runtime. The global
// owner persists a typed provider page and checkpoint before any session uses
// the generation; the current SQL service does not construct this adapter.
type CandidateFetch struct {
	Client *Client
	State  cluster.NavigationWeather
	Worker cluster.ExternalCallWorker
}

func (a CandidateFetch) FetchGlobal(ctx context.Context, workflowID string) (bool, error) {
	if a.Client == nil {
		return false, fmt.Errorf("ECFMP client unavailable")
	}
	return a.State.FetchProviderPageFor(ctx, a.Worker, workflowID, &pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}, "ecfmp", "flow-measure/active",
		func(ctx context.Context, _ *pb.ProviderCheckpoint, _ *pb.ProviderPage) (*pb.ProviderPage, *pb.ProviderCheckpoint, error) {
			measures, err := a.Client.FlowMeasures(ctx)
			if err != nil {
				return nil, nil, err
			}
			page, err := TypedPage(measures, time.Now().UTC())
			if err != nil {
				return nil, nil, err
			}
			return &pb.ProviderPage{Provider: "ecfmp", Resource: "flow-measure/active", Parsed: &pb.ProviderPage_Ecfmp{Ecfmp: page}},
				&pb.ProviderCheckpoint{Provider: "ecfmp", Resource: "flow-measure/active"}, nil
		})
}

// CandidateApply runs on the session owner after a committed global page is
// visible. Every flight update has a stable command ID derived from that page
// and the evaluated strip revision.
type CandidateApply struct {
	Source  cluster.NavigationWeather
	Session cluster.EcfmpSessionAdapter
}

// AcceptedMeasures joins the provider generation and the existing test overlay
// after both have been accepted by the global owner. Local client caches are
// never an authority for the HTTP or session application paths.
func AcceptedMeasures(ctx context.Context, source cluster.NavigationWeather) ([]FlowMeasure, error) {
	out := make([]FlowMeasure, 0)
	for _, resource := range []string{"flow-measure/active", "flow-measure/test"} {
		_, page, err := source.CheckpointFor(ctx, &pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}, "ecfmp", resource)
		if err != nil {
			return nil, err
		}
		if page == nil {
			continue
		}
		values, err := MeasuresFromPage(page.GetEcfmp())
		if err != nil {
			return nil, err
		}
		out = append(out, values...)
	}
	return out, nil
}

func (a CandidateApply) ApplySession(ctx context.Context, sessionID int32, at time.Time) error {
	if at.IsZero() {
		return fmt.Errorf("missing ECFMP application time")
	}
	ref := &pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}
	checkpoint, page, revision, err := a.Source.CheckpointRevisionFor(ctx, ref, "ecfmp", "flow-measure/active")
	if err != nil {
		return err
	}
	if checkpoint == nil || page == nil || page.GetEcfmp() == nil || revision == 0 {
		return fmt.Errorf("committed ECFMP page unavailable")
	}
	measures, err := AcceptedMeasures(ctx, a.Source)
	if err != nil {
		return err
	}
	strips, err := a.Session.Strips(ctx, sessionID)
	if err != nil {
		return err
	}
	sourceVersion := fmt.Sprintf("%020d:%s", revision, checkpoint.Sha256)
	if test, _, testRevision, e := a.Source.CheckpointRevisionFor(ctx, ref, "ecfmp", "flow-measure/test"); e != nil {
		return e
	} else if test != nil {
		sourceVersion += fmt.Sprintf(":%020d:%s", testRevision, test.Sha256)
	}
	for _, strip := range strips {
		legacy := &models.Strip{Callsign: strip.Callsign, Origin: strip.Departure, Destination: strip.Destination,
			RequestedAltitude: strip.RequestedAltitude}
		legacy.Route = &strip.Route
		matched := MatchingRestrictions(legacy, measures, at)
		restrictions := make([]*pb.EcfmpRestriction, 0, len(matched))
		for _, item := range matched {
			if item.MeasureID < 0 {
				return fmt.Errorf("negative ECFMP measure ID")
			}
			if item.MinLevel != nil && (*item.MinLevel < math.MinInt32 || *item.MinLevel > math.MaxInt32) || item.MaxLevel != nil && (*item.MaxLevel < math.MinInt32 || *item.MaxLevel > math.MaxInt32) {
				return fmt.Errorf("ECFMP level outside int32 range")
			}
			r := &pb.EcfmpRestriction{MeasureId: uint64(item.MeasureID), Ident: item.Ident, Kind: item.Type,
				Reason: item.Reason, Routes: append([]string(nil), item.Routes...), Destination: item.Destination, HasCtot: item.HasCtot}
			if item.MinLevel != nil {
				value := int32(*item.MinLevel)
				r.MinLevel = &value
			}
			if item.MaxLevel != nil {
				value := int32(*item.MaxLevel)
				r.MaxLevel = &value
			}
			for _, level := range item.ExactLevels {
				if level < math.MinInt32 || level > math.MaxInt32 {
					return fmt.Errorf("ECFMP exact level outside int32 range")
				}
				r.ExactLevels = append(r.ExactLevels, int32(level))
			}
			restrictions = append(restrictions, r)
		}
		reply := a.Session.Apply(ctx, sessionID, strip, sourceVersion, restrictions)
		if reply == nil || reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
			return fmt.Errorf("ECFMP application for %s not committed: %v", strip.Callsign, reply)
		}
	}
	return nil
}

// TypedPage consumes provider JSON only in memory. An unknown value shape is
// rejected, so an immutable provider object never carries a lossy conversion.
func TypedPage(measures []FlowMeasure, at time.Time) (*pb.EcfmpPage, error) {
	if at.IsZero() {
		return nil, fmt.Errorf("missing ECFMP fetch time")
	}
	page := &pb.EcfmpPage{FetchedAt: timestamppb.New(at.UTC())}
	seen := make(map[int64]bool, len(measures))
	for _, measure := range measures {
		if measure.ID <= 0 || seen[measure.ID] || measure.StartTime.IsZero() || !measure.EndTime.After(measure.StartTime) {
			return nil, fmt.Errorf("invalid ECFMP measure identity or interval")
		}
		seen[measure.ID] = true
		item := &pb.EcfmpMeasure{Id: measure.ID, Ident: measure.Ident, EventId: measure.EventID, Reason: measure.Reason,
			StartTime: timestamppb.New(measure.StartTime.UTC()), EndTime: timestamppb.New(measure.EndTime.UTC()),
			NotifiedFlightInformationRegions: append([]int64(nil), measure.NotifiedFlightInformationRegions...), Kind: string(measure.Measure.Type)}
		if measure.WithdrawnAt != nil {
			item.WithdrawnAt = timestamppb.New(measure.WithdrawnAt.UTC())
		}
		switch measure.Measure.Type {
		case MeasureTypeMandatoryRoute:
			if err := decodeValue(measure.Measure.Value, &item.Routes); err != nil {
				return nil, err
			}
		case MeasureTypeProhibit, MeasureTypeGroundStop:
			if !isNull(measure.Measure.Value) {
				return nil, fmt.Errorf("unexpected ECFMP %s value", item.Kind)
			}
		case MeasureTypeMinimumDepartureInterval, MeasureTypeAverageDepartureInterval, MeasureTypePerHour,
			MeasureTypeMilesInTrail, MeasureTypeMaxIAS, MeasureTypeMaxMach, MeasureTypeIASReduction, MeasureTypeMachReduction:
			var number json.Number
			if err := decodeValue(measure.Measure.Value, &number); err != nil {
				return nil, err
			}
			if integer, err := number.Int64(); err == nil {
				item.NumericValue = &integer
			} else {
				decimal, err := number.Float64()
				if err != nil || math.IsInf(decimal, 0) || math.IsNaN(decimal) {
					return nil, fmt.Errorf("invalid ECFMP numeric value")
				}
				item.DecimalValue = &decimal
			}
		default:
			return nil, fmt.Errorf("unsupported ECFMP measure kind %q", item.Kind)
		}
		for _, filter := range measure.Filters {
			part := &pb.EcfmpFilter{Kind: string(filter.Type)}
			switch filter.Type {
			case FilterTypeADEP, FilterTypeADES, FilterTypeWaypoint:
				if err := decodeValue(filter.Value, &part.Names); err != nil {
					return nil, err
				}
			case FilterTypeLevel:
				if err := decodeValue(filter.Value, &part.Levels); err != nil {
					return nil, err
				}
			case FilterTypeLevelAbove, FilterTypeLevelBelow:
				var level int32
				if err := decodeValue(filter.Value, &level); err != nil {
					return nil, err
				}
				part.Level = &level
			case FilterTypeMemberEvent, FilterTypeMemberNotEvent:
				var eventID int64
				if err := decodeValue(filter.Value, &eventID); err != nil {
					return nil, err
				}
				part.EventId = &eventID
			default:
				return nil, fmt.Errorf("unsupported ECFMP filter kind %q", filter.Type)
			}
			item.Filters = append(item.Filters, part)
		}
		page.Measures = append(page.Measures, item)
	}
	return page, nil
}

// MeasuresFromPage reconstructs the existing matcher inputs from named typed
// fields. JSON exists only inside the legacy matcher boundary, never in NATS.
func MeasuresFromPage(page *pb.EcfmpPage) ([]FlowMeasure, error) {
	if page == nil || page.FetchedAt == nil || page.FetchedAt.CheckValid() != nil {
		return nil, fmt.Errorf("invalid ECFMP page")
	}
	result := make([]FlowMeasure, 0, len(page.Measures))
	seen := make(map[int64]bool, len(page.Measures))
	for _, item := range page.Measures {
		if item == nil || item.Id <= 0 || seen[item.Id] || item.StartTime == nil || item.EndTime == nil || item.StartTime.CheckValid() != nil || item.EndTime.CheckValid() != nil || !item.EndTime.AsTime().After(item.StartTime.AsTime()) {
			return nil, fmt.Errorf("invalid ECFMP measure")
		}
		seen[item.Id] = true
		measure := FlowMeasure{ID: item.Id, Ident: item.Ident, EventID: item.EventId, Reason: item.Reason,
			StartTime: item.StartTime.AsTime(), EndTime: item.EndTime.AsTime(),
			NotifiedFlightInformationRegions: append([]int64(nil), item.NotifiedFlightInformationRegions...),
			Measure:                          FlowMeasureType{Type: MeasureType(item.Kind)}}
		if item.WithdrawnAt != nil {
			measure.WithdrawnAt = new(time.Time)
			*measure.WithdrawnAt = item.WithdrawnAt.AsTime()
		}
		var value any
		switch measure.Measure.Type {
		case MeasureTypeMandatoryRoute:
			value = item.Routes
		case MeasureTypeProhibit, MeasureTypeGroundStop:
			value = nil
		case MeasureTypeMinimumDepartureInterval, MeasureTypeAverageDepartureInterval, MeasureTypePerHour,
			MeasureTypeMilesInTrail, MeasureTypeMaxIAS, MeasureTypeMaxMach, MeasureTypeIASReduction, MeasureTypeMachReduction:
			if item.NumericValue != nil && item.DecimalValue == nil {
				value = *item.NumericValue
			} else if item.DecimalValue != nil && item.NumericValue == nil {
				value = *item.DecimalValue
			} else {
				return nil, fmt.Errorf("invalid ECFMP numeric value")
			}
		default:
			return nil, fmt.Errorf("unsupported ECFMP measure kind %q", item.Kind)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		measure.Measure.Value = encoded
		for _, part := range item.Filters {
			if part == nil {
				return nil, fmt.Errorf("nil ECFMP filter")
			}
			filter := FlowMeasureFilter{Type: FilterType(part.Kind)}
			switch filter.Type {
			case FilterTypeADEP, FilterTypeADES, FilterTypeWaypoint:
				value = part.Names
			case FilterTypeLevel:
				value = part.Levels
			case FilterTypeLevelAbove, FilterTypeLevelBelow:
				if part.Level == nil {
					return nil, fmt.Errorf("missing ECFMP level")
				}
				value = *part.Level
			case FilterTypeMemberEvent, FilterTypeMemberNotEvent:
				if part.EventId == nil {
					return nil, fmt.Errorf("missing ECFMP event ID")
				}
				value = *part.EventId
			default:
				return nil, fmt.Errorf("unsupported ECFMP filter kind %q", part.Kind)
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			filter.Value = encoded
			measure.Filters = append(measure.Filters, filter)
		}
		result = append(result, measure)
	}
	return result, nil
}

func isNull(raw json.RawMessage) bool {
	value := strings.TrimSpace(string(raw))
	return value == "" || value == "null"
}

func decodeValue(raw json.RawMessage, target any) error {
	if isNull(raw) {
		return fmt.Errorf("missing ECFMP value")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid ECFMP value: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("trailing ECFMP value")
	}
	return nil
}
