package services

import (
	"FlightStrips/internal/cluster"
	"FlightStrips/internal/models"
	"FlightStrips/internal/sat"
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"fmt"
	"hash/fnv"
	"time"
)

// StandReadCandidate runs the existing allocation presentation policy on one
// detached accepted session snapshot; it has no transaction or write port.
type StandReadCandidate struct {
	State          cluster.StandState
	HoldDuration   time.Duration
	BlockExtension time.Duration
}

func (c StandReadCandidate) snapshot(ctx context.Context, id int32, airport, callsign string) (*StandAllocationService, StandAllocationRequest, []*models.StandAssignment, []*models.StandBlock, error) {
	state, err := c.State.Store.Read(ctx, sessionRef(id))
	if err != nil {
		return nil, StandAllocationRequest{}, nil, nil, err
	}
	seed := state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(id)].Value.GetSession()
	s := state.Indexes[pb.EntityKind_STRIP][callsign].GetValue().GetStrip()
	if seed == nil || seed.Tombstoned || seed.Airport != airport || s == nil || c.State.Stands == nil || c.State.Policy == nil {
		return nil, StandAllocationRequest{}, nil, nil, fmt.Errorf("stand presentation unavailable")
	}
	now := time.Now().UTC()
	if c.State.Now != nil {
		now = c.State.Now().UTC()
	}
	strip := CandidateModelStrip(s, id)
	positions, _, err := c.State.Projection.ObservationSnapshot(id)
	if err != nil {
		return nil, StandAllocationRequest{}, nil, nil, err
	}
	occupancy := map[string]string{}
	for _, o := range positions {
		if !o.Stale && o.Value.GetPosition() != nil {
			pos := o.Value.GetPosition()
			if o.Value.AircraftKey == callsign {
				strip.PositionLatitude = &pos.Latitude
				strip.PositionLongitude = &pos.Longitude
				strip.PositionAltitude = &pos.AltitudeFeet
			}
			observed := &models.Strip{PositionLatitude: &pos.Latitude, PositionLongitude: &pos.Longitude, PositionAltitude: &pos.AltitudeFeet}
			if stand, found := parkedArrivalStandAtPosition(c.State.Stands, observed, airport); found {
				occupancy[stand] = o.Value.AircraftKey
			}
		}
	}
	assignments := make([]*models.StandAssignment, 0)
	blocks := make([]*models.StandBlock, 0)
	assignedBlocks := map[string][]string{}
	var existing *models.StandAssignment
	for i, e := range state.EntitiesByKind(pb.EntityKind_STAND_ASSIGNMENT) {
		a := CandidateModelAssignment(e.Value.GetStandAssignment(), id, int64(i+1))
		assignments = append(assignments, a)
		assignedBlocks[a.Callsign] = e.Value.GetStandAssignment().BlockedStands
		if a.Callsign == callsign {
			existing = a
		}
	}
	for _, e := range state.EntitiesByKind(pb.EntityKind_STAND_BLOCK) {
		b := CandidateModelBlock(e.Value.GetStandBlock(), id)
		if b.Manual && !expired(b.ExpiresAt, now) {
			blocks = append(blocks, b)
		}
	}
	direction, compat := sat.AssignmentDirectionArrival, sat.Arrival
	stage := StageConfirmed
	var expires *time.Time
	if strip.Origin == airport {
		direction, compat = sat.AssignmentDirectionDeparture, sat.Departure
		stage = StageReserved
		hold := c.HoldDuration
		if hold <= 0 {
			hold = defaultDepartureHoldDuration
		}
		at := now.Add(hold)
		expires = &at
	}
	if existing != nil {
		stage = existing.Stage
		expires = existing.ExpiresAt
	}
	facts := sat.ResolveFlightCompatibilityFacts(sat.FlightCompatibilityInput{Direction: compat, Origin: strip.Origin, Destination: strip.Destination, AircraftType: s.AircraftType, LiveEngineType: s.EngineType}, c.State.Aircraft, c.State.Engines, c.State.Borders)
	request := StandAllocationRequest{SessionID: id, Airport: airport, Callsign: callsign, Direction: direction, Stage: stage, FlightFacts: facts, AssignmentFacts: sat.AssignmentFlightFacts{Callsign: callsign, AircraftType: s.AircraftType, AircraftUse: facts.Aircraft.UseCode, BorderStatus: facts.BorderStatus, Direction: direction}, ETA: arrivalETATime(strip), ETASource: existingETASource(existing), ExpiresAt: expires, DepartureTOBT: departureTobtTime(strip, now), DepartureTSAT: departureTsatTime(strip, now), DepartureReady: departureExpectedToVacate(strip)}
	if c.BlockExtension <= 0 {
		c.BlockExtension = defaultDepartureBlockExtension
	}
	policy := &StandAllocationService{stands: c.State.Stands, policy: c.State.Policy, now: func() time.Time { return now }, planningOccupancy: occupancy, planningBlocks: assignedBlocks, departureReleaseBuffer: c.BlockExtension}
	return policy, request, assignments, blocks, nil
}
func (c StandReadCandidate) Preview(ctx context.Context, id int32, airport, callsign string) (StandAllocationPreview, error) {
	policy, request, assignments, blocks, err := c.snapshot(ctx, id, airport, callsign)
	if err != nil {
		return StandAllocationPreview{}, err
	}
	evaluation := policy.stands.EvaluateCompatibility(airport, request.FlightFacts)
	matches := map[string]sat.StandCompatibilityMatch{}
	for _, m := range evaluation.Matches {
		matches[standName(m.Stand.Name)] = m
	}
	available, selection, err := policy.automaticStandPool(request, assignments, blocks, matches, nil)
	return StandAllocationPreview{Callsign: callsign, Airport: airport, FallbackUsed: selection.FallbackUsed, CompatibleStands: len(matches), AvailableStands: len(available), Selection: selection}, err
}
func (c StandReadCandidate) AvailableForPilot(ctx context.Context, id int32, airport, callsign string) ([]StandAvailability, error) {
	policy, request, assignments, blocks, err := c.snapshot(ctx, id, airport, callsign)
	if err != nil {
		return nil, err
	}
	evaluation := policy.stands.EvaluateManualCompatibility(airport, request.FlightFacts)
	matches := map[string]sat.StandCompatibilityMatch{}
	for _, m := range evaluation.Matches {
		matches[standName(m.Stand.Name)] = m
	}
	unavailable := policy.availability(request, assignments, blocks, matches)
	out := make([]StandAvailability, 0)
	for _, stand := range policy.stands.Stands(airport) {
		name := standName(stand.Name)
		value := StandAvailability{Stand: name, Available: true}
		if _, ok := matches[name]; !ok {
			value.Available = false
			value.Reason = compatibilityReason(name, evaluation.Rejections)
		} else if reasons := unavailable[name]; len(reasons) > 0 {
			value.Available = false
			value.Reason = joinAllocationReasons(reasons)
		}
		out = append(out, value)
	}
	return out, nil
}

// Candidate model conversions exist only at JSON/policy boundaries.
func CandidateModelStrip(s *pb.Strip, id int32) *models.Strip {
	m := lifecycleModelStrip(s, id)
	m.Route = &s.Route
	m.Remarks = &s.Remarks
	m.AssignedSquawk = &s.AssignedSquawk
	m.Sid = &s.Sid
	m.Star = &s.Star
	m.Runway = &s.Runway
	m.ReleasePoint = &s.ReleasePoint
	m.Cleared = s.Bay != "NOT_CLEARED" && s.Bay != "UNKNOWN"
	m.HasFP = s.HasFlightPlan
	m.IsManual = s.Manual
	m.PdcState = s.PdcState
	m.PdcRequestRemarks = &s.PdcRequestRemarks
	m.Owner = &s.OwnerCid
	m.RequestedAltitude = s.RequestedAltitude
	m.ClearedAltitude = s.ClearedAltitude
	m.Heading = s.Heading
	m.Hold = s.Hold
	m.HoldType = s.HoldType
	m.HoldEat = s.HoldEat
	m.TrackingController = s.TrackingController
	m.CdmData.Eobt = lifecycleClock(s.Eobt)
	m.CdmData.Ttot = lifecycleClock(s.Ttot)
	m.CdmData.Ctot = lifecycleClock(s.Ctot)
	return m
}
func CandidateModelAssignment(a *pb.StandAssignment, id int32, identity int64) *models.StandAssignment {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%d/%s/%s", id, a.Callsign, a.CreatedAt.AsTime().Format(time.RFC3339Nano))
	identity = int64(h.Sum64() & 0x7fffffffffffffff)
	return lifecycleModelAssignment(a, id, identity)
}
func CandidateModelBlock(b *pb.StandBlock, id int32) *models.StandBlock {
	h := fnv.New64a()
	_, _ = fmt.Fprintf(h, "%d/%s/%s", id, b.Stand, b.CreatedAt.AsTime().Format(time.RFC3339Nano))
	return &models.StandBlock{ID: int64(h.Sum64() & 0x7fffffffffffffff), SessionID: id, Stand: b.Stand, BlockType: b.BlockType, Source: b.Source, Reason: &b.Reason, Callsign: b.Callsign, CreatedBy: &b.Actor, ExpiresAt: lifecycleTime(b.ExpiresAt), Manual: b.Manual, Version: int32(b.Revision), CreatedAt: b.CreatedAt.AsTime(), UpdatedAt: b.UpdatedAt.AsTime()}
}
