package services

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"FlightStrips/internal/cdm"
	"FlightStrips/internal/cluster"
	"FlightStrips/internal/models"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CdmCandidate remains dormant until Task 20. All reads come from accepted
// typed projections/pages; policy planning never constructs SQL services.
type CdmCandidate struct {
	Writer cluster.Writer
	Config cluster.CdmConfigCandidateWorker
	Reads  cluster.ViffReadAdapter
	Writes cluster.ViffWriteAdapter
	Now    func() time.Time
}

func NewCdmCandidate(writer cluster.Writer, config cluster.CdmConfigCandidateWorker, reads cluster.ViffReadAdapter, writes cluster.ViffWriteAdapter) (*CdmCandidate, error) {
	if writer.Store == nil || writer.Projection == nil || writer.Lease == nil || config.State.Objects == nil {
		return nil, fmt.Errorf("CDM requires accepted configuration, projection and owner writer")
	}
	if (reads.Client == nil) != (writes.Client == nil) {
		return nil, fmt.Errorf("vIFF requires both durable read and write adapters")
	}
	return &CdmCandidate{Writer: writer, Config: config, Reads: reads, Writes: writes, Now: time.Now}, nil
}

func (c *CdmCandidate) clock() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}
func (c *CdmCandidate) admission(id int32) error {
	if err := c.Writer.Projection.Ready(); err != nil {
		return err
	}
	if !lifecycleOwnerCanPlan(c.Writer.Lease, c.Writer.Projection.Async != nil, sessionRef(id)) {
		return fmt.Errorf("CDM session is not owned")
	}
	return nil
}
func cdmSeed(state *cluster.Aggregate, id int32) (*pb.Session, error) {
	s := state.Indexes[pb.EntityKind_SESSION][strconv.Itoa(int(id))].GetValue().GetSession()
	if s == nil || s.Tombstoned {
		return nil, fmt.Errorf("CDM session unavailable")
	}
	return s, nil
}
func (c *CdmCandidate) usesViff(s *pb.Session) bool {
	return cdm.ViffEnabledSession(s.Name) && c.Reads.Client != nil && c.Writes.Client != nil
}

// CDM is the concrete SessionWork.CDM callback. Persisted slots, rather than
// local tickers, define polling, validation, sequence and debounce episodes.
func (c *CdmCandidate) CDM(ctx context.Context, id int32) error {
	if err := c.admission(id); err != nil {
		return err
	}
	if c.Writes.Client != nil {
		if err := c.Writes.Resume(ctx, "", id); err != nil {
			return err
		}
	}
	if err := c.resumeExports(ctx, id); err != nil {
		return err
	}
	for _, kind := range []string{"cdm-sync", "cdm-recalculate", "cdm-validation"} {
		if err := c.ensureDeadline(ctx, id, kind); err != nil {
			return err
		}
	}
	state, err := c.Writer.Read(ctx, sessionRef(id))
	if err != nil {
		return err
	}
	deadlines := state.EntitiesByKind(pb.EntityKind_SESSION_DEADLINE)
	// The initial vIFF page must be accepted before sequence/validation reads
	// it. Entity key ordering places recalculation before sync, so order policy
	// work explicitly instead of allowing that initial dependency to starve.
	sort.SliceStable(deadlines, func(i, j int) bool {
		return deadlines[i].GetValue().GetSessionDeadline().GetKind() == "cdm-sync" && deadlines[j].GetValue().GetSessionDeadline().GetKind() != "cdm-sync"
	})
	for _, entry := range deadlines {
		d := entry.GetValue().GetSessionDeadline()
		if d == nil || !strings.HasPrefix(d.Kind, "cdm-") || d.DueAt == nil || c.clock().Before(d.DueAt.AsTime()) {
			continue
		}
		if err = c.fire(ctx, id, entry); err != nil {
			return err
		}
	}
	return c.resumeExports(ctx, id)
}

func cdmDeadline(id int32, kind string, at time.Time, source uint64) *pb.SessionDeadline {
	key := kind + "/session"
	return &pb.SessionDeadline{Id: key, Kind: kind, DueAt: timestamppb.New(at), SourceRevision: source, CommandId: lifecycleID(fmt.Sprintf("cdm/%d/%s/%s/%d", id, kind, at.UTC().Format(time.RFC3339Nano), source), "slot")}
}
func (c *CdmCandidate) ensureDeadline(ctx context.Context, id int32, kind string) error {
	state, err := c.Writer.Read(ctx, sessionRef(id))
	if err != nil {
		return err
	}
	if _, err = cdmSeed(state, id); err != nil {
		return err
	}
	key := kind + "/session"
	if state.Indexes[pb.EntityKind_SESSION_DEADLINE][key] != nil {
		return nil
	}
	d := cdmDeadline(id, kind, c.clock(), state.Revision)
	req := cdmSystem(id, d.CommandId, 0, &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: key, Value: &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: d}}}}})
	w := c.Writer
	w.Plan = func(ctx context.Context, req *pb.CommandRequest, current *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		if _, err := cdmSeed(current, id); err != nil {
			return nil, pb.CommandReply_UNAVAILABLE, 0, err
		}
		return cluster.PlanSystemEntity(ctx, req, current)
	}
	return cdmReply(w.Execute(ctx, req))
}
func cdmSystem(id int32, command string, expected uint64, action *pb.SystemCommand) *pb.CommandRequest {
	return &pb.CommandRequest{ProtocolRevision: 1, CommandId: command, Aggregate: sessionRef(id), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "cdm-worker"}, ExpectedEntityRevision: &expected, Command: &pb.CommandRequest_System{System: action}}
}
func cdmReply(reply *pb.CommandReply) error {
	if reply == nil || reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() == pb.CommandOutcome_FAILED {
		return fmt.Errorf("CDM acceptance failed: %v", reply)
	}
	return nil
}

func (c *CdmCandidate) fire(ctx context.Context, id int32, deadline *pb.EntitySnapshot) error {
	state, err := c.Writer.Read(ctx, sessionRef(id))
	if err != nil {
		return err
	}
	seed, err := cdmSeed(state, id)
	if err != nil {
		return err
	}
	d := deadline.GetValue().GetSessionDeadline()
	if d == nil || d.DueAt == nil {
		return fmt.Errorf("invalid CDM deadline")
	}
	switch d.Kind {
	case "cdm-sync", "cdm-recalculate", "cdm-validation", "cdm-debounce", "cdm-pushback":
	default:
		return fmt.Errorf("unsupported CDM deadline kind")
	}
	if d.Kind == "cdm-pushback" {
		return c.firePushback(ctx, id, deadline)
	}
	if d.Kind == "cdm-sync" && c.usesViff(seed) {
		// The read identity is the persisted slot, including after owner death.
		if _, err = c.Reads.Flights(ctx, seed.Airport, id, d.DueAt.AsTime()); err != nil {
			return err
		}
		state, err = c.Writer.Read(ctx, sessionRef(id))
		if err != nil {
			return err
		}
	}
	page, configRevision, err := c.Config.Read(ctx, seed.Airport)
	if err != nil {
		return err
	}
	config, err := cluster.OperationalCdmConfig(page)
	if err != nil {
		return err
	}
	var flights *pb.ViffFlightPage
	var flightRevision uint64
	if c.usesViff(seed) {
		flights, flightRevision, err = c.Reads.ReadFlights(ctx, id, "")
		if err != nil || flights == nil {
			return fmt.Errorf("accepted vIFF page unavailable: %v", err)
		}
	}
	positions, _, err := c.Writer.Projection.ObservationSnapshot(id)
	if err != nil {
		return err
	}
	tag := lifecyclePositionTag(positions)
	identity := lifecycleID(d.CommandId, fmt.Sprintf("fire/%d/%d/%d/%s", deadline.Revision, state.Revision, configRevision, tag))
	req := cdmSystem(id, identity, state.Revision, &pb.SystemCommand{Action: &pb.SystemCommand_RemoveEntity{RemoveEntity: &pb.RemoveEntity{Key: d.Id, Kind: pb.EntityKind_SESSION_DEADLINE}}})
	w := c.Writer
	w.Plan = func(ctx context.Context, request *pb.CommandRequest, current *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		if current.Revision != *request.ExpectedEntityRevision || !proto.Equal(current.Indexes[pb.EntityKind_SESSION_DEADLINE][d.Id], deadline) {
			return nil, pb.CommandReply_UNAVAILABLE, current.Revision, fmt.Errorf("CDM input/deadline changed; rederive")
		}
		fresh, rev, readErr := c.Config.Read(ctx, seed.Airport)
		if readErr != nil || rev != configRevision || !proto.Equal(fresh, page) {
			return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("CDM configuration changed; rederive")
		}
		if c.usesViff(seed) {
			fresh, rev, err := c.Reads.ReadFlights(ctx, id, "")
			if err != nil || rev != flightRevision || !proto.Equal(fresh, flights) {
				return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("vIFF page changed; rederive")
			}
		}
		observations, _, err := c.Writer.Projection.ObservationSnapshot(id)
		if err != nil || lifecyclePositionTag(observations) != tag {
			return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("CDM observations changed; rederive")
		}
		change, err := c.plan(ctx, id, current, seed, config, flights, observations, d.Kind, identity, c.clock())
		if err != nil {
			return nil, pb.CommandReply_UNAVAILABLE, 0, err
		}
		old := current.Indexes[pb.EntityKind_SESSION_DEADLINE][d.Id]
		fresh, rev, readErr = c.Config.Read(ctx, seed.Airport)
		if readErr != nil || rev != configRevision || !proto.Equal(fresh, page) {
			return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("CDM configuration changed during calculation")
		}
		if c.usesViff(seed) {
			freshFlights, rev, err := c.Reads.ReadFlights(ctx, id, "")
			if err != nil || rev != flightRevision || !proto.Equal(freshFlights, flights) {
				return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("vIFF page changed during calculation")
			}
		}
		freshPositions, _, err := c.Writer.Projection.ObservationSnapshot(id)
		if err != nil || lifecyclePositionTag(freshPositions) != tag {
			return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("CDM observations changed during calculation")
		}
		if d.Kind == "cdm-debounce" {
			change.Changes = append(change.Changes, candidateDelete(d.Id, old, pb.EntityKind_SESSION_DEADLINE))
		} else {
			interval := time.Minute
			if d.Kind == "cdm-sync" {
				interval = 30 * time.Second
			}
			next := cdmDeadline(id, d.Kind, c.clock().Add(interval), current.Revision+1)
			change.Changes = append(change.Changes, candidateUpsert(d.Id, old, &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: next}}))
		}
		cdmSort(change.Changes)
		return change, pb.CommandReply_COMMITTED, 0, nil
	}
	return cdmReply(w.Execute(ctx, req))
}

func (c *CdmCandidate) plan(ctx context.Context, id int32, state *cluster.Aggregate, seed *pb.Session, config *cdm.CdmAirportConfig, flights *pb.ViffFlightPage, positions []cluster.KVPosition, kind, command string, now time.Time) (*pb.DomainChange, error) {
	config = cdmSessionConfig(config, seed)
	strips := []*models.Strip{}
	for _, e := range state.EntitiesByKind(pb.EntityKind_STRIP) {
		m := cdmModel(state, e, id)
		for _, pos := range positions {
			if pos.Value.AircraftKey == m.Callsign && pos.Value.GetPosition() != nil {
				if pos.Stale {
					return nil, fmt.Errorf("CDM position awaits accepted owner sync")
				}
				p := pos.Value.GetPosition()
				m.PositionLatitude, m.PositionLongitude = &p.Latitude, &p.Longitude
			}
		}
		if kind == "cdm-sync" && m.Origin == seed.Airport {
			for _, f := range flights.GetFlights() {
				if f.Callsign == m.Callsign {
					m.CdmData = cdm.MergeViffFlight(m.CdmData, cdmIfps(f))
					break
				}
			}
			m.CdmData = cdm.NormalizeSessionEobt(m.CdmData, now)
		}
		strips = append(strips, m)
	}
	if kind != "cdm-validation" {
		departures := []*models.Strip{}
		for _, m := range strips {
			if m.Origin == seed.Airport {
				departures = append(departures, m)
			}
		}
		results, err := cdm.PlanSequence(ctx, id, departures, config, now)
		if err != nil {
			return nil, err
		}
		for _, m := range strips {
			if data := results[m.Callsign]; data != nil {
				m.CdmData = data
			}
		}
	}
	change := &pb.DomainChange{}
	for _, m := range strips {
		oldStrip := state.Indexes[pb.EntityKind_STRIP][m.Callsign]
		oldCdm := state.Indexes[pb.EntityKind_CDM_STATE][m.Callsign]
		s, data, err := cdmRecords(oldStrip.GetValue().GetStrip(), oldCdm.GetValue().GetCdmState(), m.CdmData, now)
		if err != nil {
			return nil, err
		}
		validation := PlanCtotValidation(m, now, lifecycleID(command, "validation/"+m.Callsign), false)
		if validation == nil {
			s.Validation = nil
		} else if validation.IssueType == ctotValidationIssueType {
			s.Validation = &pb.ValidationStatus{IssueType: validation.IssueType, Message: validation.Message, OwningPosition: validation.OwningPosition, Active: validation.Active, ActivationKey: validation.ActivationKey, Action: &pb.ValidationAction{Label: ctotValidationActionLabel, Action: &pb.ValidationAction_AssignHoldingPoint{AssignHoldingPoint: &pb.AssignHoldingPoint{}}}}
		}
		if !equalStripWithoutRevision(oldStrip.GetValue().GetStrip(), s) {
			change.Changes = append(change.Changes, stripChange(oldStrip, s))
		}
		policyChanged := !proto.Equal(oldCdm.GetValue().GetCdmState(), data)
		if policyChanged {
			data.SourceRevision = command
		}
		needsExport := policyChanged
		if kind == "cdm-sync" {
			for _, row := range flights.GetFlights() {
				if row.Callsign == m.Callsign && cdm.ViffNeedsExport(m.CdmData, cdmIfps(row)) {
					needsExport = true
				}
			}
		}
		if needsExport && c.usesViff(seed) && !m.CdmData.NeedsLocalRecalculation() {
			if err := c.queueExport(state, data, command, m, standRevision(oldCdm)+1, config); err != nil {
				return nil, err
			}
		}
		if !proto.Equal(oldCdm.GetValue().GetCdmState(), data) {
			change.Changes = append(change.Changes, candidateUpsert(m.Callsign, oldCdm, &pb.EntityRecord{Value: &pb.EntityRecord_CdmState{CdmState: data}}))
		}
	}
	return change, nil
}
func cdmValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func clusterCdmConfig(page *pb.CdmConfigPage, seed *pb.Session) (*cdm.CdmAirportConfig, error) {
	config, err := cluster.OperationalCdmConfig(page)
	if err != nil {
		return nil, err
	}
	return cdmSessionConfig(config, seed), nil
}

func cdmSessionConfig(config *cdm.CdmAirportConfig, seed *pb.Session) *cdm.CdmAirportConfig {
	config = config.Clone()
	config.ActiveArrivalRunways, config.ActiveDepartureRunways = nil, nil
	for _, r := range seed.Runways {
		if r.Arrival {
			config.ActiveArrivalRunways = append(config.ActiveArrivalRunways, r.Name)
		}
		if r.Departure {
			config.ActiveDepartureRunways = append(config.ActiveDepartureRunways, r.Name)
		}
	}
	config.LvoActive = false
	for _, r := range seed.RunwayStatuses {
		if r.Status == "LOW_VIS" {
			config.LvoActive = true
		}
	}
	return config
}
func cdmIfps(f *pb.ViffFlight) cdm.IFPSData {
	d := f.GetCdmData()
	return cdm.IFPSData{Callsign: f.Callsign, Departure: f.Departure, Arrival: f.Arrival, EOBT: f.Eobt, TOBT: f.Tobt, CTOT: f.Ctot, MostPenalizingAirspace: f.MostPenalizingAirspace, CDMStatus: f.CdmStatus, CDMData: cdm.CDMData{TOBT: d.GetTobt(), TSAT: d.GetTsat(), TTOT: d.GetTtot(), CTOT: d.GetCtot(), Reason: d.GetReason(), ReqTOBT: d.GetRequestedTobt(), ReqTOBTType: d.GetRequestedTobtType(), ReqASRT: d.GetRequestedAsrt()}}
}
func cdmSort(changes []*pb.EntityChange) {
	kind := func(e *pb.EntityChange) int {
		if e.GetDelete() != nil {
			return int(e.GetDelete().Kind)
		}
		return int(e.GetUpsert().ProtoReflect().WhichOneof(e.GetUpsert().ProtoReflect().Descriptor().Oneofs().ByName("value")).Number())
	}
	sort.Slice(changes, func(i, j int) bool {
		if kind(changes[i]) != kind(changes[j]) {
			return kind(changes[i]) < kind(changes[j])
		}
		return changes[i].Key < changes[j].Key
	})
}

// Recalculate is the concrete SessionUpdate/SessionDisconnect callback for
// changes that affect sequencing. Its debounce is accepted durable work, and
// a repeated callback without newer domain inputs cannot create a new episode.
func (c *CdmCandidate) Recalculate(ctx context.Context, id int32) error {
	if err := c.admission(id); err != nil {
		return err
	}
	state, err := c.Writer.Read(ctx, sessionRef(id))
	if err != nil {
		return err
	}
	if _, err := cdmSeed(state, id); err != nil {
		return err
	}
	prior := state.Indexes[pb.EntityKind_SESSION_DEADLINE]["cdm-debounce/session"]
	if prior != nil && prior.GetValue().GetSessionDeadline().SourceRevision == state.Revision {
		return nil
	}
	d := cdmDeadline(id, "cdm-debounce", c.clock().Add(500*time.Millisecond), state.Revision+1)
	d.CommandId = lifecycleID(fmt.Sprintf("cdm/%d/recalculate/%d", id, state.Revision), "debounce")
	request := cdmSystem(id, d.CommandId, standRevision(prior), &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: d.Id, Value: &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: d}}}}})
	w := c.Writer
	w.Plan = func(ctx context.Context, req *pb.CommandRequest, current *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		if current.Revision != state.Revision {
			return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("recalculation input changed; rederive")
		}
		return cluster.PlanSystemEntity(ctx, req, current)
	}
	return cdmReply(w.Execute(ctx, request))
}
