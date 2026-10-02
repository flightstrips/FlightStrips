package cluster

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"FlightStrips/internal/sat"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// StandState plans stand allocation from accepted session state and the local
// position projection. The session owner conditionally publishes the result.
type StandState struct {
	Store      LifecycleStore
	Projection *Projection // current FS_POSITIONS view for physical occupancy checks
	Stands     *sat.StandCapabilityRegistry
	Policy     *sat.AirlineAssignmentConfig
	Aircraft   *sat.AircraftRegistry
	Engines    *sat.AircraftEngineRegistry
	Borders    *sat.AirportCountryRegistry
	Random     func() float64
	Now        func() time.Time
}

func (s StandState) Execute(ctx context.Context, request *pb.CommandRequest) *pb.CommandReply {
	if s.Store == nil {
		return &pb.CommandReply{Status: pb.CommandReply_UNAVAILABLE, Detail: "stand store unavailable"}
	}
	if request == nil || request.GetAggregate().GetSession() == nil || (request.GetClient().GetStand() == nil && (request.GetSystem().GetRemoveEntity() == nil || (request.GetSystem().GetRemoveEntity().Kind != pb.EntityKind_STAND_ASSIGNMENT && request.GetSystem().GetRemoveEntity().Kind != pb.EntityKind_STAND_BLOCK))) {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT, Detail: "stand action requires a session"}
	}
	return s.Store.Execute(ctx, request)
}

func (s StandState) Assignments(ctx context.Context, session int32) ([]*pb.StandAssignment, uint64, error) {
	if s.Store == nil {
		return nil, 0, fmt.Errorf("stand store unavailable")
	}
	a, err := s.Store.Read(ctx, sessionRef(session))
	if err != nil {
		return nil, 0, err
	}
	out := make([]*pb.StandAssignment, 0)
	for _, e := range a.EntitiesByKind(pb.EntityKind_STAND_ASSIGNMENT) {
		out = append(out, e.GetValue().GetStandAssignment())
	}
	return out, a.Revision, nil
}

func (s StandState) Assignment(ctx context.Context, session int32, callsign string) (*pb.StandAssignment, uint64, error) {
	if s.Store == nil {
		return nil, 0, fmt.Errorf("stand store unavailable")
	}
	a, err := s.Store.Read(ctx, sessionRef(session))
	if err != nil {
		return nil, 0, err
	}
	e := a.Indexes[pb.EntityKind_STAND_ASSIGNMENT][standToken(callsign)]
	if e == nil {
		return nil, a.Revision, fmt.Errorf("stand assignment not found")
	}
	return proto.Clone(e.GetValue().GetStandAssignment()).(*pb.StandAssignment), a.Revision, nil
}

func (s StandState) AssignmentsAtStand(ctx context.Context, session int32, stand string) ([]*pb.StandAssignment, uint64, error) {
	if s.Store == nil {
		return nil, 0, fmt.Errorf("stand store unavailable")
	}
	a, err := s.Store.Read(ctx, sessionRef(session))
	if err != nil {
		return nil, 0, err
	}
	out := make([]*pb.StandAssignment, 0, len(a.StandAssignmentsByStand[standToken(stand)]))
	for _, e := range a.StandAssignmentsByStand[standToken(stand)] {
		out = append(out, proto.Clone(e.GetValue().GetStandAssignment()).(*pb.StandAssignment))
	}
	return out, a.Revision, nil
}

func (s StandState) Block(ctx context.Context, session int32, stand string) (*pb.StandBlock, uint64, error) {
	if s.Store == nil {
		return nil, 0, fmt.Errorf("stand store unavailable")
	}
	a, err := s.Store.Read(ctx, sessionRef(session))
	if err != nil {
		return nil, 0, err
	}
	e := a.Indexes[pb.EntityKind_STAND_BLOCK][standToken(stand)]
	if e == nil {
		return nil, a.Revision, fmt.Errorf("stand block not found")
	}
	return proto.Clone(e.GetValue().GetStandBlock()).(*pb.StandBlock), a.Revision, nil
}

func (s StandState) Blocks(ctx context.Context, session int32) ([]*pb.StandBlock, uint64, error) {
	if s.Store == nil {
		return nil, 0, fmt.Errorf("stand store unavailable")
	}
	a, err := s.Store.Read(ctx, sessionRef(session))
	if err != nil {
		return nil, 0, err
	}
	out := make([]*pb.StandBlock, 0)
	for _, e := range a.EntitiesByKind(pb.EntityKind_STAND_BLOCK) {
		out = append(out, e.GetValue().GetStandBlock())
	}
	return out, a.Revision, nil
}

// PlanStand runs on each writer attempt, including after a subject CAS miss.
// It never accepts a precomputed stand choice from an automatic command.
func (s StandState) PlanStand(ctx context.Context, req *pb.CommandRequest, a *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if action := req.GetClient().GetStand(); action != nil {
		return s.planAction(req, a, action)
	}
	if deletion := req.GetSystem().GetRemoveEntity(); deletion != nil && (deletion.Kind == pb.EntityKind_STAND_ASSIGNMENT || deletion.Kind == pb.EntityKind_STAND_BLOCK) {
		return s.planExpiry(req, a, deletion)
	}
	if update := req.GetSystem().GetUpdateEntity(); update != nil && (update.Value.GetStandAssignment() != nil || update.Value.GetStandBlock() != nil) {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("stand records require a validated stand action")
	}
	return PlanStrip(ctx, req, a)
}

func (s StandState) clock() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func standToken(v string) string { return strings.ToUpper(strings.TrimSpace(v)) }
func standRevision(e *pb.EntitySnapshot) uint64 {
	if e == nil {
		return 0
	}
	return e.Revision
}
func standFailure(status pb.CommandReply_Status, revision uint64, msg string) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	return nil, status, revision, fmt.Errorf("%s", msg)
}
func standChange(kind pb.EntityKind, key string, old *pb.EntitySnapshot, record *pb.EntityRecord) *pb.EntityChange {
	c := &pb.EntityChange{Key: key, Revision: standRevision(old) + 1}
	if record != nil {
		c.Operation = &pb.EntityChange_Upsert{Upsert: record}
	} else {
		c.Operation = &pb.EntityChange_Delete{Delete: &pb.DeleteEntity{Kind: kind}}
	}
	return c
}
func standSort(changes []*pb.EntityChange) {
	sort.Slice(changes, func(i, j int) bool {
		x, _ := changeKind(changes[i])
		y, _ := changeKind(changes[j])
		if x != y {
			return x < y
		}
		return changes[i].Key < changes[j].Key
	})
}

func (s StandState) planAction(req *pb.CommandRequest, a *Aggregate, action *pb.StandAction) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	seed, err := stripSession(req, a)
	if err != nil {
		return standFailure(pb.CommandReply_NOT_FOUND, 0, err.Error())
	}
	airport := seed.GetValue().GetSession().Airport
	actor := req.GetActor()
	if actor == nil || (actor.Kind != pb.Actor_CONTROLLER && actor.Kind != pb.Actor_PILOT && actor.Kind != pb.Actor_SYSTEM) || strings.TrimSpace(actor.Id) == "" {
		return standFailure(pb.CommandReply_UNAUTHORIZED, 0, "stand action requires an actor")
	}
	if actor.Kind != pb.Actor_SYSTEM && (action.Stage != nil || action.Eta != nil || action.EtaSource != nil || action.ExpiresAt != nil || action.VatsimCid != nil || action.VatsimRevision != nil || action.ObservedStand != nil) {
		return standFailure(pb.CommandReply_UNAUTHORIZED, 0, "lifecycle facts require a system actor")
	}
	if s.Stands == nil {
		return standFailure(pb.CommandReply_UNAVAILABLE, 0, "stand registry unavailable")
	}
	now := s.clock()
	key, stand := standToken(action.Callsign), standToken(action.Stand)
	if action.GetCreateBlock() != nil || action.GetRemoveBlock() != nil {
		if actor.Kind != pb.Actor_CONTROLLER || stand == "" || key != "" {
			return standFailure(pb.CommandReply_INVALID_ARGUMENT, 0, "invalid stand block action")
		}
		return s.planBlock(req, a, airport, stand, now, action)
	}
	if key == "" {
		return standFailure(pb.CommandReply_INVALID_ARGUMENT, 0, "missing callsign")
	}
	old := a.Indexes[pb.EntityKind_STAND_ASSIGNMENT][key]
	current := standRevision(old)
	if req.ExpectedEntityRevision == nil || *req.ExpectedEntityRevision != current {
		return standFailure(pb.CommandReply_REVISION_CONFLICT, current, "stale stand assignment revision")
	}
	prior := old.GetValue().GetStandAssignment()
	stripEntity := a.Indexes[pb.EntityKind_STRIP][key]
	if stripEntity == nil {
		return standFailure(pb.CommandReply_NOT_FOUND, current, "stand action strip missing")
	}
	strip := stripEntity.GetValue().GetStrip()
	if action.GetAcknowledge() != nil {
		if prior == nil {
			return standFailure(pb.CommandReply_NOT_FOUND, current, "stand assignment missing")
		}
		if prior.Acknowledged {
			return &pb.DomainChange{}, pb.CommandReply_COMMITTED, current, nil
		}
		next := proto.Clone(prior).(*pb.StandAssignment)
		next.Acknowledged = true
		next.AcknowledgedAt = timestamppb.New(now)
		next.AcknowledgedBy = &actor.Id
		next.Revision = current + 1
		next.UpdatedAt = timestamppb.New(now)
		return &pb.DomainChange{Changes: []*pb.EntityChange{standChange(pb.EntityKind_STAND_ASSIGNMENT, key, old, &pb.EntityRecord{Value: &pb.EntityRecord_StandAssignment{StandAssignment: next}})}}, pb.CommandReply_COMMITTED, current, nil
	}
	if action.GetConfirmOverride() != nil {
		if prior == nil || prior.Source != "MANUAL_OVERRIDE" || prior.Confirmed {
			return standFailure(pb.CommandReply_INVALID_ARGUMENT, current, "no pending manual override")
		}
		next := proto.Clone(prior).(*pb.StandAssignment)
		next.Confirmed = true
		next.Revision = current + 1
		next.UpdatedAt = timestamppb.New(now)
		return &pb.DomainChange{Changes: []*pb.EntityChange{standChange(pb.EntityKind_STAND_ASSIGNMENT, key, old, &pb.EntityRecord{Value: &pb.EntityRecord_StandAssignment{StandAssignment: next}})}}, pb.CommandReply_COMMITTED, current, nil
	}
	if action.GetVacate() != nil {
		if prior == nil {
			return standFailure(pb.CommandReply_NOT_FOUND, current, "stand assignment missing")
		}
		changes := []*pb.EntityChange{standChange(pb.EntityKind_STAND_ASSIGNMENT, key, old, nil)}
		if strip.Stand == prior.Stand && prior.Direction == "ARRIVAL" {
			copy := proto.Clone(strip).(*pb.Strip)
			copy.Stand = ""
			changes = append(changes, stripChange(stripEntity, copy))
		}
		standSort(changes)
		return &pb.DomainChange{Changes: changes}, pb.CommandReply_COMMITTED, current, nil
	}
	if action.GetAutomatic() == nil && action.GetManual() == nil && action.GetOccupy() == nil {
		return standFailure(pb.CommandReply_INVALID_ARGUMENT, current, "unknown stand transition")
	}
	direction, stage := "ARRIVAL", "CONFIRMED"
	if strip.Departure == airport {
		direction, stage = "DEPARTURE", "RESERVED"
	}
	if action.Stage != nil {
		stage = *action.Stage
	}
	if !validStandStage(direction, stage) {
		return standFailure(pb.CommandReply_INVALID_ARGUMENT, current, "invalid stand lifecycle stage")
	}
	if prior != nil && prior.Manual && action.GetAutomatic() != nil {
		return standFailure(pb.CommandReply_INVALID_ARGUMENT, current, "manual assignment has precedence")
	}
	if action.VatsimRevision != nil && action.VatsimCid == nil {
		return standFailure(pb.CommandReply_INVALID_ARGUMENT, current, "VATSIM revision requires identity")
	}
	if prior != nil && prior.VatsimCid != nil && action.VatsimCid != nil && *prior.VatsimCid != *action.VatsimCid {
		return standFailure(pb.CommandReply_REVISION_CONFLICT, current, "VATSIM identity changed")
	}
	if prior != nil && prior.VatsimRevision != nil && action.VatsimRevision != nil && *action.VatsimRevision < *prior.VatsimRevision {
		return standFailure(pb.CommandReply_REVISION_CONFLICT, current, "stale VATSIM observation")
	}
	facts := sat.ResolveFlightCompatibilityFacts(sat.FlightCompatibilityInput{Direction: standFlightDirection(direction), Origin: strip.Departure, Destination: strip.Destination, AircraftType: strip.AircraftType}, s.Aircraft, s.Engines, s.Borders)
	projected := projectedStandRelease(strip, now)
	eta, expires := action.Eta, action.ExpiresAt
	if prior != nil {
		if eta == nil {
			eta = prior.Eta
		}
		if expires == nil {
			expires = prior.ExpiresAt
		}
		if projected == nil {
			projected = prior.ProjectedReleaseAt
		}
	}
	if expires == nil && direction == "DEPARTURE" {
		expires = timestamppb.New(now.Add(15 * time.Minute))
	}
	manual := action.GetManual() != nil || action.GetOccupy() != nil
	matches := s.Stands.EvaluateCompatibility(airport, facts).Matches
	if manual {
		matches = s.Stands.EvaluateManualCompatibility(airport, facts).Matches
	}
	byStand := map[string]sat.StandCompatibilityMatch{}
	for _, m := range matches {
		byStand[m.Stand.Name] = m
	}
	source, reason, confirmed := "AUTOMATIC", "", true
	var selection *sat.StandSelection
	var displaced []*pb.EntitySnapshot
	if manual {
		if stand == "" {
			return standFailure(pb.CommandReply_INVALID_ARGUMENT, current, "manual stand is empty")
		}
		if _, known := s.Stands.Lookup(airport, stand); !known {
			return standFailure(pb.CommandReply_INVALID_ARGUMENT, current, "stand is not configured")
		}
		if action.GetOccupy() != nil {
			source = "PHYSICAL"
		} else {
			source = "MANUAL"
		}
		if action.GetManual() != nil {
			reason = strings.TrimSpace(action.GetManual().Reason)
		}
		if _, fits := byStand[stand]; !fits && reason == "" && action.GetOccupy() == nil {
			return standFailure(pb.CommandReply_INVALID_ARGUMENT, current, "manual stand is incompatible")
		}
	} else {
		if s.Policy == nil {
			return standFailure(pb.CommandReply_UNAVAILABLE, current, "stand policy unavailable")
		}
		available := make([]string, 0, len(matches))
		relaxed := make([]string, 0, len(matches))
		for _, m := range matches {
			if !s.conflicts(a, airport, m.Stand.Name, m.Blocks, key, direction, stage, eta, expires, projected, now) {
				available = append(available, m.Stand.Name)
				relaxed = append(relaxed, m.Stand.Name)
			} else if stage != "ESTIMATED" {
				if _, ok := s.estimatedDisplacements(a, m.Stand.Name, m.Blocks, key, direction, stage, eta, expires, projected, now); ok {
					relaxed = append(relaxed, m.Stand.Name)
				}
			}
		}
		sort.Strings(available)
		sort.Strings(relaxed)
		assignmentFacts := sat.AssignmentFlightFacts{Callsign: key, AircraftType: strip.AircraftType, AircraftUse: facts.Aircraft.UseCode, BorderStatus: facts.BorderStatus, Direction: sat.AssignmentDirection(direction)}
		random := s.Random
		if random == nil {
			random = func() float64 { return 0 }
		}
		var selectErr error
		selection, selectErr = s.Policy.SelectStand(assignmentFacts, available, random)
		if selectErr != nil {
			return standFailure(pb.CommandReply_INVALID_ARGUMENT, current, selectErr.Error())
		}
		if len(relaxed) > len(available) {
			alternative, err := s.Policy.SelectStand(assignmentFacts, relaxed, random)
			if err != nil {
				return standFailure(pb.CommandReply_INVALID_ARGUMENT, current, err.Error())
			}
			if alternative != nil && (selection == nil || (selection.FallbackUsed && !alternative.FallbackUsed) || (selection.FallbackUsed == alternative.FallbackUsed && alternative.RuleID == selection.RuleID && alternative.Tier < selection.Tier)) {
				selection = alternative
			}
		}
		if selection == nil {
			return standFailure(pb.CommandReply_INVALID_ARGUMENT, current, "no compatible available policy stand")
		}
		stand = selection.Stand
		if stage != "ESTIMATED" {
			displaced, _ = s.estimatedDisplacements(a, stand, byStand[stand].Blocks, key, direction, stage, eta, expires, projected, now)
		}
	}
	match := byStand[stand]
	blocked := append([]string(nil), match.Blocks...)
	if len(blocked) == 0 {
		physical, _ := s.Stands.Lookup(airport, stand)
		blocked = append(blocked, physical.Blocks...)
	}
	for i := range blocked {
		blocked[i] = standToken(blocked[i])
	}
	sort.Strings(blocked)
	blocked = slices.Compact(blocked)
	if s.conflicts(a, airport, stand, blocked, key, direction, stage, eta, expires, projected, now) && len(displaced) == 0 {
		if reason == "" && action.GetOccupy() == nil {
			return standFailure(pb.CommandReply_INVALID_ARGUMENT, current, "stand is reserved or blocked")
		}
		if reason == "" {
			reason = "physical occupancy conflicts with reservation"
		}
	}
	if reason != "" && action.GetManual() != nil {
		source = "MANUAL_OVERRIDE"
		confirmed = false
	}
	next := &pb.StandAssignment{Callsign: key, Stand: stand, Source: source, Revision: current + 1, Confirmed: confirmed, Actor: actor.Id, Direction: direction, Stage: stage, AssignedAt: timestamppb.New(now), Manual: manual, UpdatedAt: timestamppb.New(now), BlockedStands: blocked}
	if prior != nil {
		next.CreatedAt = prior.CreatedAt
	} else {
		next.CreatedAt = timestamppb.New(now)
	}
	if reason != "" {
		next.ConflictReason = &reason
	}
	if action.ObservedStand != nil {
		v := standToken(*action.ObservedStand)
		next.ObservedStand = &v
	}
	next.Eta = eta
	next.EtaSource = action.EtaSource
	next.ExpiresAt = expires
	next.VatsimCid = action.VatsimCid
	next.VatsimRevision = action.VatsimRevision
	if prior != nil {
		if action.Eta == nil {
			next.EtaSource = prior.EtaSource
		}
		if next.VatsimCid == nil {
			next.VatsimCid = prior.VatsimCid
			next.VatsimRevision = prior.VatsimRevision
		}
	}
	if selection != nil {
		id := selection.RuleID
		tier := int32(selection.Tier)
		next.RuleId = &id
		next.Tier = &tier
	}
	if match.Variant.Line > 0 {
		v := fmt.Sprintf("%s:%s:%d", airport, stand, match.Variant.Line)
		next.MatchedVariant = &v
	}
	if direction == "DEPARTURE" {
		next.ProjectedReleaseAt = projected
	}
	changes := []*pb.EntityChange{standChange(pb.EntityKind_STAND_ASSIGNMENT, key, old, &pb.EntityRecord{Value: &pb.EntityRecord_StandAssignment{StandAssignment: next}})}
	for _, entity := range displaced {
		changes = append(changes, standChange(pb.EntityKind_STAND_ASSIGNMENT, entity.Key, entity, nil))
		if oldStrip := a.Indexes[pb.EntityKind_STRIP][entity.Key]; oldStrip != nil && oldStrip.GetValue().GetStrip().Stand == entity.GetValue().GetStandAssignment().Stand {
			copy := proto.Clone(oldStrip.GetValue().GetStrip()).(*pb.Strip)
			copy.Stand = ""
			changes = append(changes, stripChange(oldStrip, copy))
		}
	}
	if strip.Stand != stand {
		copy := proto.Clone(strip).(*pb.Strip)
		copy.Stand = stand
		changes = append(changes, stripChange(stripEntity, copy))
	}
	standSort(changes)
	return &pb.DomainChange{Changes: changes}, pb.CommandReply_COMMITTED, current, nil
}

func standFlightDirection(direction string) sat.FlightDirection {
	if direction == "DEPARTURE" {
		return sat.Departure
	}
	return sat.Arrival
}
func projectedStandRelease(strip *pb.Strip, now time.Time) *timestamppb.Timestamp {
	if strip == nil {
		return nil
	}
	release := strip.Tobt
	if strip.Tsat != nil && (release == nil || strip.Tsat.AsTime().After(release.AsTime())) {
		release = strip.Tsat
	}
	if release == nil {
		return nil
	}
	at := release.AsTime()
	if !strip.StartRequested {
		at = at.Add(10 * time.Minute)
	}
	// Passing the estimate is never evidence that an aircraft vacated.
	if !at.After(now) {
		return nil
	}
	return timestamppb.New(at)
}
func validStandStage(direction, stage string) bool {
	if direction == "ARRIVAL" {
		return stage == "ESTIMATED" || stage == "ASSIGNED" || stage == "CONFIRMED"
	}
	return direction == "DEPARTURE" && (stage == "RESERVED" || stage == "DEPARTURE_BLOCK")
}

func (s StandState) planBlock(req *pb.CommandRequest, a *Aggregate, airport, stand string, now time.Time, action *pb.StandAction) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	old := a.Indexes[pb.EntityKind_STAND_BLOCK][stand]
	current := standRevision(old)
	if req.ExpectedEntityRevision == nil || *req.ExpectedEntityRevision != current {
		return standFailure(pb.CommandReply_REVISION_CONFLICT, current, "stale stand block revision")
	}
	if action.GetRemoveBlock() != nil {
		if old == nil {
			return standFailure(pb.CommandReply_NOT_FOUND, current, "stand block missing")
		}
		if old.GetValue().GetStandBlock().Actor != req.Actor.Id {
			return standFailure(pb.CommandReply_UNAUTHORIZED, current, "stand block belongs to another controller")
		}
		return &pb.DomainChange{Changes: []*pb.EntityChange{standChange(pb.EntityKind_STAND_BLOCK, stand, old, nil)}}, pb.CommandReply_COMMITTED, current, nil
	}
	if old != nil {
		return standFailure(pb.CommandReply_INVALID_ARGUMENT, current, "stand is already blocked")
	}
	physical, known := s.Stands.Lookup(airport, stand)
	if !known {
		return standFailure(pb.CommandReply_INVALID_ARGUMENT, current, "stand is not configured")
	}
	if s.conflicts(a, airport, stand, physical.Blocks, "", "ARRIVAL", "CONFIRMED", nil, nil, nil, now) {
		return standFailure(pb.CommandReply_INVALID_ARGUMENT, current, "stand is reserved or adjacency blocked")
	}
	reason := strings.TrimSpace(action.GetCreateBlock().Reason)
	if reason == "" {
		return standFailure(pb.CommandReply_INVALID_ARGUMENT, current, "stand block requires a reason")
	}
	expires := action.GetCreateBlock().ExpiresAt
	if expires != nil && !expires.AsTime().After(now) {
		return standFailure(pb.CommandReply_INVALID_ARGUMENT, current, "stand block deadline has passed")
	}
	blocked := append([]string(nil), physical.Blocks...)
	sort.Strings(blocked)
	blocked = slices.Compact(blocked)
	b := &pb.StandBlock{Stand: stand, Reason: reason, Actor: req.Actor.Id, CreatedAt: timestamppb.New(now), ExpiresAt: expires, BlockType: "MANUAL", Source: "CONTROLLER", Manual: true, Revision: 1, UpdatedAt: timestamppb.New(now), BlockedStands: blocked}
	return &pb.DomainChange{Changes: []*pb.EntityChange{standChange(pb.EntityKind_STAND_BLOCK, stand, nil, &pb.EntityRecord{Value: &pb.EntityRecord_StandBlock{StandBlock: b}})}}, pb.CommandReply_COMMITTED, current, nil
}

func (s StandState) planExpiry(req *pb.CommandRequest, a *Aggregate, d *pb.RemoveEntity) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if req.Actor.GetKind() != pb.Actor_SYSTEM || req.Aggregate.GetSession() == nil {
		return standFailure(pb.CommandReply_UNAUTHORIZED, 0, "stand expiry requires a session system actor")
	}
	old := a.Indexes[d.Kind][d.Key]
	current := standRevision(old)
	if old == nil {
		return standFailure(pb.CommandReply_NOT_FOUND, 0, "stand expiry target missing")
	}
	if req.ExpectedEntityRevision == nil || *req.ExpectedEntityRevision != current {
		return standFailure(pb.CommandReply_REVISION_CONFLICT, current, "stale stand expiry")
	}
	var due *timestamppb.Timestamp
	if d.Kind == pb.EntityKind_STAND_ASSIGNMENT {
		due = old.GetValue().GetStandAssignment().ExpiresAt
	} else {
		due = old.GetValue().GetStandBlock().ExpiresAt
	}
	if due == nil || due.AsTime().After(s.clock()) {
		return standFailure(pb.CommandReply_INVALID_ARGUMENT, current, "stand deadline has not elapsed")
	}
	if d.Kind == pb.EntityKind_STAND_ASSIGNMENT {
		assignment := old.GetValue().GetStandAssignment()
		if assignment.Stage == "DEPARTURE_BLOCK" {
			stripEntity := a.Indexes[pb.EntityKind_STRIP][assignment.Callsign]
			if stripEntity != nil {
				atStand := false
				positionKnown := false
				if s.Projection != nil {
					positions, _, err := s.Projection.ObservationSnapshot(req.Aggregate.GetSession().Id)
					if err != nil {
						return nil, pb.CommandReply_UNAVAILABLE, current, err
					}
					for _, position := range positions {
						if position.Value.AircraftKey != assignment.Callsign || position.Stale || position.Value.GetPosition() == nil {
							continue
						}
						positionKnown = true
						if s.Stands != nil {
							airport := a.Indexes[pb.EntityKind_SESSION][fmt.Sprint(req.Aggregate.GetSession().Id)].GetValue().GetSession().Airport
							physical, found := s.Stands.StandAtPosition(airport, position.Value.GetPosition().Latitude, position.Value.GetPosition().Longitude)
							atStand = found && physical.Name == assignment.Stand
						}
						break
					}
				}
				if !positionKnown {
					atStand = stripEntity.GetValue().GetStrip().Stand == assignment.Stand
				}
				if atStand {
					copy := proto.Clone(assignment).(*pb.StandAssignment)
					copy.ExpiresAt = nil
					copy.Revision = current + 1
					copy.UpdatedAt = timestamppb.New(s.clock())
					return &pb.DomainChange{Changes: []*pb.EntityChange{standChange(d.Kind, d.Key, old, &pb.EntityRecord{Value: &pb.EntityRecord_StandAssignment{StandAssignment: copy}})}}, pb.CommandReply_COMMITTED, current, nil
				}
			}
		}
	}
	return &pb.DomainChange{Changes: []*pb.EntityChange{standChange(d.Kind, d.Key, old, nil)}}, pb.CommandReply_COMMITTED, current, nil
}

func standTouches(left string, leftBlocks []string, right string, rightBlocks []string) bool {
	if left == right {
		return true
	}
	for _, v := range leftBlocks {
		if v == right {
			return true
		}
	}
	for _, v := range rightBlocks {
		if v == left {
			return true
		}
	}
	return false
}
func standWindow(direction, stage string, eta, expires, projected *timestamppb.Timestamp, now time.Time) (time.Time, *time.Time) {
	if expires != nil {
		end := expires.AsTime()
		return now, &end
	}
	if direction == "DEPARTURE" && projected != nil && projected.AsTime().After(now) {
		end := projected.AsTime()
		return now, &end
	}
	if direction == "ARRIVAL" && eta != nil && stage != "CONFIRMED" {
		start := eta.AsTime()
		end := start.Add(30 * time.Minute)
		if end.After(now) {
			return start, &end
		}
	}
	return now, nil
}
func standOverlaps(leftStart time.Time, leftEnd *time.Time, rightStart time.Time, rightEnd *time.Time) bool {
	return (leftEnd == nil || leftEnd.After(rightStart)) && (rightEnd == nil || rightEnd.After(leftStart))
}
func (s StandState) conflicts(a *Aggregate, airport, stand string, blocked []string, except, direction, stage string, eta, expires, projected *timestamppb.Timestamp, now time.Time) bool {
	start, end := standWindow(direction, stage, eta, expires, projected, now)
	for _, e := range a.EntitiesByKind(pb.EntityKind_STAND_ASSIGNMENT) {
		v := e.GetValue().GetStandAssignment()
		if v.Callsign == except || (v.ExpiresAt != nil && !v.ExpiresAt.AsTime().After(now)) {
			continue
		}
		if !standTouches(stand, blocked, v.Stand, v.BlockedStands) {
			continue
		}
		otherStart, otherEnd := standWindow(v.Direction, v.Stage, v.Eta, v.ExpiresAt, v.ProjectedReleaseAt, now)
		if standOverlaps(start, end, otherStart, otherEnd) {
			return true
		}
	}
	for _, e := range a.EntitiesByKind(pb.EntityKind_STAND_BLOCK) {
		v := e.GetValue().GetStandBlock()
		if v.ExpiresAt != nil && !v.ExpiresAt.AsTime().After(now) {
			continue
		}
		if standTouches(stand, blocked, v.Stand, v.BlockedStands) {
			return true
		}
	}
	return false
}

func (s StandState) estimatedDisplacements(a *Aggregate, stand string, blocked []string, except, direction, stage string, eta, expires, projected *timestamppb.Timestamp, now time.Time) ([]*pb.EntitySnapshot, bool) {
	start, end := standWindow(direction, stage, eta, expires, projected, now)
	for _, e := range a.EntitiesByKind(pb.EntityKind_STAND_BLOCK) {
		v := e.GetValue().GetStandBlock()
		if (v.ExpiresAt == nil || v.ExpiresAt.AsTime().After(now)) && standTouches(stand, blocked, v.Stand, v.BlockedStands) {
			return nil, false
		}
	}
	displaced := make([]*pb.EntitySnapshot, 0)
	for _, e := range a.EntitiesByKind(pb.EntityKind_STAND_ASSIGNMENT) {
		v := e.GetValue().GetStandAssignment()
		if v.Callsign == except || (v.ExpiresAt != nil && !v.ExpiresAt.AsTime().After(now)) || !standTouches(stand, blocked, v.Stand, v.BlockedStands) {
			continue
		}
		otherStart, otherEnd := standWindow(v.Direction, v.Stage, v.Eta, v.ExpiresAt, v.ProjectedReleaseAt, now)
		if !standOverlaps(start, end, otherStart, otherEnd) {
			continue
		}
		if v.Direction != "ARRIVAL" || v.Stage != "ESTIMATED" || v.Manual {
			return nil, false
		}
		displaced = append(displaced, e)
	}
	return displaced, true
}

// validateStandState is repeated by every projection when replaying an event.
// The event carries the chosen variant's adjacency, so all nodes can check the
// invariant without loading their local SAT configuration or consulting SQL.
func validateStandState(ref *pb.AggregateRef, entities map[string]*pb.EntitySnapshot) error {
	if ref.GetSession() == nil {
		return nil
	}
	assignments := make([]*pb.StandAssignment, 0)
	blocks := make([]*pb.StandBlock, 0)
	for _, e := range entities {
		if v := e.GetValue().GetStandAssignment(); v != nil {
			validSource := v.Source == "AUTOMATIC" || v.Source == "MANUAL" || v.Source == "MANUAL_OVERRIDE" || v.Source == "PHYSICAL"
			managedConflict := v.ConflictReason != nil && (strings.HasPrefix(*v.ConflictReason, "WRONG_STAND_PENDING: observed ") || strings.HasPrefix(*v.ConflictReason, "WRONG_STAND_AWAITING_MESSAGE: observed ") || strings.HasPrefix(*v.ConflictReason, "observed departure conflicts with confirmed arrival:") || strings.HasPrefix(*v.ConflictReason, "observed parked arrival: ") || *v.ConflictReason == "physically displaced; no compatible replacement stand available" || *v.ConflictReason == "displaced arrival relocation cycle; controller action required")
			advisory := v.Stand == "" && v.Direction == "ARRIVAL" && managedConflict
			if v.Callsign != e.Key || v.Callsign != standToken(v.Callsign) || v.Stand == "" && !advisory || v.Stand != standToken(v.Stand) || v.Revision != e.Revision || !validStandStage(v.Direction, v.Stage) || !validSource || v.Actor == "" || v.Manual != (v.Source != "AUTOMATIC") || (v.ConflictReason != nil && v.Source != "MANUAL_OVERRIDE" && v.Source != "PHYSICAL" && !managedConflict) || v.AssignedAt == nil || v.CreatedAt == nil || v.UpdatedAt == nil || (v.Acknowledged && (v.AcknowledgedAt == nil || v.AcknowledgedBy == nil)) || (!v.Acknowledged && (v.AcknowledgedAt != nil || v.AcknowledgedBy != nil)) || (v.VatsimRevision != nil && v.VatsimCid == nil) {
				return fmt.Errorf("invalid stand assignment")
			}
			if err := validBlockedStands(v.Stand, v.BlockedStands); err != nil {
				return err
			}
			assignments = append(assignments, v)
		}
		if v := e.GetValue().GetStandBlock(); v != nil {
			if v.Stand != e.Key || v.Stand == "" || v.Stand != standToken(v.Stand) || v.Revision != e.Revision || v.CreatedAt == nil || v.UpdatedAt == nil || v.Actor == "" || v.Reason == "" || v.BlockType == "" || v.Source == "" {
				return fmt.Errorf("invalid stand block")
			}
			if err := validBlockedStands(v.Stand, v.BlockedStands); err != nil {
				return err
			}
			blocks = append(blocks, v)
		}
	}
	for i, left := range assignments {
		if left.Stand == "" {
			continue
		}
		for _, right := range assignments[i+1:] {
			if right.Stand == "" {
				continue
			}
			if !standTouches(left.Stand, left.BlockedStands, right.Stand, right.BlockedStands) {
				continue
			}
			later := left.AssignedAt.AsTime()
			if right.AssignedAt.AsTime().After(later) {
				later = right.AssignedAt.AsTime()
			}
			if (left.ExpiresAt != nil && !left.ExpiresAt.AsTime().After(later)) || (right.ExpiresAt != nil && !right.ExpiresAt.AsTime().After(later)) {
				continue
			}
			ls, le := standWindow(left.Direction, left.Stage, left.Eta, left.ExpiresAt, left.ProjectedReleaseAt, later)
			rs, re := standWindow(right.Direction, right.Stage, right.Eta, right.ExpiresAt, right.ProjectedReleaseAt, later)
			if standOverlaps(ls, le, rs, re) && left.ConflictReason == nil && right.ConflictReason == nil {
				return fmt.Errorf("conflicting stand assignments")
			}
		}
		for _, block := range blocks {
			if !standTouches(left.Stand, left.BlockedStands, block.Stand, block.BlockedStands) {
				continue
			}
			later := left.AssignedAt.AsTime()
			if block.CreatedAt.AsTime().After(later) {
				later = block.CreatedAt.AsTime()
			}
			if (left.ExpiresAt == nil || left.ExpiresAt.AsTime().After(later)) && (block.ExpiresAt == nil || block.ExpiresAt.AsTime().After(later)) && left.ConflictReason == nil {
				return fmt.Errorf("assignment overlaps stand block")
			}
		}
	}
	for i, left := range blocks {
		for _, right := range blocks[i+1:] {
			if standTouches(left.Stand, left.BlockedStands, right.Stand, right.BlockedStands) && (left.ExpiresAt == nil || left.ExpiresAt.AsTime().After(right.CreatedAt.AsTime())) && (right.ExpiresAt == nil || right.ExpiresAt.AsTime().After(left.CreatedAt.AsTime())) {
				return fmt.Errorf("overlapping stand blocks")
			}
		}
	}
	return nil
}

func validBlockedStands(stand string, blocked []string) error {
	for i, v := range blocked {
		if v == "" || v == stand || v != standToken(v) || (i > 0 && blocked[i-1] >= v) {
			return fmt.Errorf("invalid stand adjacency")
		}
	}
	return nil
}
