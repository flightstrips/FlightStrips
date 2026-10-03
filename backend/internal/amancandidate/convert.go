package amancandidate

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

func timestamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}
func instant(t *timestamppb.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.AsTime()
}
func optionalTimestamp(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamp(*t)
}
func optionalInstant(t *timestamppb.Timestamp) *time.Time {
	if t == nil {
		return nil
	}
	v := instant(t)
	return &v
}
func optionalDuration(t *time.Duration) *durationpb.Duration {
	if t == nil {
		return nil
	}
	return durationpb.New(*t)
}
func optionalDomainDuration(t *durationpb.Duration) *time.Duration {
	if t == nil {
		return nil
	}
	v := t.AsDuration()
	return &v
}

type scalar interface {
	~int | ~int32 | ~int64 | ~uint8 | ~uint32 | ~uint64 | ~float64
}

func optionalNumber[T scalar, S scalar](v *S) *T {
	if v == nil {
		return nil
	}
	r := T(*v)
	return &r
}
func optionalString[T ~string, S ~string](v *S) *T {
	if v == nil {
		return nil
	}
	r := T(*v)
	return &r
}

func decodeNavHolding(v *pb.NavHolding) *navdata.HoldingPattern {
	if v == nil {
		return nil
	}
	r := &navdata.HoldingPattern{ID: navdata.HoldingID(v.Id), Fix: navdata.FixID(v.Fix), InboundCourseTrueDeg: v.InboundCourseTrueDegrees, TurnDirection: navdata.TurnDirection(v.TurnDirection),
		MinimumAltitudeFt: optionalNumber[int](v.MinimumAltitudeFeet), MaximumAltitudeFt: optionalNumber[int](v.MaximumAltitudeFeet), MaximumSpeedKt: optionalNumber[int](v.MaximumSpeedKnots), Termination: navdata.HoldingTermination(v.Termination)}
	if v.Provenance != nil {
		r.Provenance = *decodeNavProvenance(v.Provenance)
	}
	switch e := v.LegExtent.(type) {
	case *pb.NavHolding_LengthNm:
		r.LegLengthNM = &e.LengthNm
	case *pb.NavHolding_TimeSeconds:
		r.LegTimeSeconds = &e.TimeSeconds
	}
	return r
}
func encodeNavHolding(v *navdata.HoldingPattern) *pb.NavHolding {
	if v == nil {
		return nil
	}
	r := &pb.NavHolding{Id: string(v.ID), Fix: string(v.Fix), InboundCourseTrueDegrees: v.InboundCourseTrueDeg, TurnDirection: string(v.TurnDirection),
		MinimumAltitudeFeet: optionalNumber[int32](v.MinimumAltitudeFt), MaximumAltitudeFeet: optionalNumber[int32](v.MaximumAltitudeFt), MaximumSpeedKnots: optionalNumber[int32](v.MaximumSpeedKt), Termination: string(v.Termination), Provenance: encodeNavProvenance(&v.Provenance)}
	if v.LegLengthNM != nil {
		r.LegExtent = &pb.NavHolding_LengthNm{LengthNm: *v.LegLengthNM}
	} else if v.LegTimeSeconds != nil {
		r.LegExtent = &pb.NavHolding_TimeSeconds{TimeSeconds: *v.LegTimeSeconds}
	}
	return r
}
func decodeNavStarFamilyPolicy(v *pb.NavStarFamilyPolicy) *navdata.STARFamilyPolicy {
	if v == nil {
		return nil
	}
	r := &navdata.STARFamilyPolicy{STARFamily: navdata.STARFamilyID(v.StarFamily), HoldingSequencePolicy: navdata.HoldingSequencePolicy(v.HoldingSequencePolicy)}
	if v.SameStarSpacing != nil {
		s := v.SameStarSpacing
		r.SameSTARSpacing = navdata.SameSTARSpacingPolicy{Enabled: s.Enabled, ActivationRatePerHour: s.ActivationRatePerHour, MinimumEmptySlots: s.MinimumEmptySlots}
	}
	return r
}
func encodeNavStarFamilyPolicy(v *navdata.STARFamilyPolicy) *pb.NavStarFamilyPolicy {
	if v == nil {
		return nil
	}
	s := v.SameSTARSpacing
	return &pb.NavStarFamilyPolicy{StarFamily: string(v.STARFamily), HoldingSequencePolicy: string(v.HoldingSequencePolicy), SameStarSpacing: &pb.AmanSameStarSpacing{Enabled: s.Enabled, ActivationRatePerHour: s.ActivationRatePerHour, MinimumEmptySlots: s.MinimumEmptySlots}}
}

func decodeBoard(board cluster.AmanBoard) (aman.AirportState, error) {
	if board.Airport == nil {
		return aman.AirportState{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "AMAN board not initialized"}
	}
	a := board.Airport
	mode := a.ConfiguredMode
	if mode == "" {
		mode = a.EffectiveMode
	}
	s := aman.AirportState{Airport: a.Airport, Revision: aman.SequenceRevision(a.Revision), GeneratedAt: instant(a.GeneratedAt), PolicyVersion: a.PolicyVersion, Mode: aman.RolloutMode(mode), Authoritative: a.Authoritative}
	for _, f := range board.Flights {
		s.Flights = append(s.Flights, *decodeAmanFlight(f))
	}
	for _, g := range a.RunwayGroups {
		s.RunwayGroups = append(s.RunwayGroups, *decodeAmanRunwayGroup(g))
	}
	if a.ActiveRunwayGroupIds != nil {
		s.ActiveRunwayGroups = make([]aman.RunwayGroupID, len(a.ActiveRunwayGroupIds))
		for i, g := range a.ActiveRunwayGroupIds {
			s.ActiveRunwayGroups[i] = aman.RunwayGroupID(g)
		}
	}
	return s, s.Validate()
}
func encodeBoard(s aman.AirportState, h aman.TechnicalHealth) cluster.AmanTransition {
	a := &pb.AmanAirport{Airport: s.Airport, Revision: uint64(s.Revision), GeneratedAt: timestamp(s.GeneratedAt), PolicyVersion: s.PolicyVersion, ConfiguredMode: string(s.Mode), EffectiveMode: string(h.EffectiveMode), Authoritative: s.Authoritative,
		Health: &pb.AmanTechnicalHealth{Status: string(h.Status), Ready: h.Ready, BlockedReasons: append([]string(nil), h.BlockedReasons...)}}
	for _, c := range []struct {
		name  string
		value aman.ComponentHealth
	}{{"observation_source", h.ObservationSource}, {"navigation", h.Navigation}, {"weather", h.Weather}, {"repository", h.Repository}, {"predictor", h.Predictor}, {"replay_validation", h.ReplayValidation}} {
		a.Health.Components = append(a.Health.Components, &pb.AmanComponentHealth{Component: c.name, Status: string(c.value.Status), Reason: c.value.Reason, UpdatedAt: optionalTimestamp(c.value.UpdatedAt), AgeSeconds: optionalNumber[float64](c.value.AgeSeconds)})
	}
	for i := range s.RunwayGroups {
		a.RunwayGroups = append(a.RunwayGroups, encodeAmanRunwayGroup(&s.RunwayGroups[i]))
	}
	if s.ActiveRunwayGroups != nil {
		a.ActiveRunwayGroupIds = make([]string, len(s.ActiveRunwayGroups))
		for i, g := range s.ActiveRunwayGroups {
			a.ActiveRunwayGroupIds[i] = string(g)
		}
	}
	t := cluster.AmanTransition{Airport: a}
	for i := range s.Flights {
		t.Flights = append(t.Flights, encodeAmanFlight(&s.Flights[i]))
	}
	return t
}

// DecodeBoard restores the coherent accepted airport policy state for typed transports.
func DecodeBoard(board cluster.AmanBoard) (aman.AirportState, error) { return decodeBoard(board) }
