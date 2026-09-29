package cluster

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

const stripOrderSpacing uint64 = 1000
const stripMinOrderGap uint64 = 5

// StripState is an opt-in projection/command adapter. The SQL application does
// not construct it. Store.Execute supplies the owner, ledger and subject CAS.
type StripState struct{ Store LifecycleStore }

// Execute forwards a boundary-assigned UUID unchanged, including on retries.
// Only strip commands enter this adapter; the aggregate writer owns deduplication.
func (r StripState) Execute(ctx context.Context, request *pb.CommandRequest) (*pb.CommandReply, error) {
	if r.Store == nil || request == nil || request.GetAggregate().GetSession() == nil {
		return nil, fmt.Errorf("strip command requires a session store")
	}
	isStrip := request.GetClient().GetStrip() != nil
	if update := request.GetSystem().GetUpdateEntity(); update != nil && update.GetValue().GetStrip() != nil {
		isStrip = true
	}
	if deletion := request.GetSystem().GetRemoveEntity(); deletion != nil && deletion.Kind == pb.EntityKind_STRIP {
		isStrip = true
	}
	if !isStrip {
		return nil, fmt.Errorf("command is not a strip transition")
	}
	reply := r.Store.Execute(ctx, request)
	if reply == nil {
		return nil, fmt.Errorf("strip command received no reply")
	}
	if reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() == pb.CommandOutcome_FAILED {
		return reply, fmt.Errorf("strip command: %s: %s", reply.Status, reply.GetOutcome().GetDetail())
	}
	return reply, nil
}

func (r StripState) read(ctx context.Context, session int32) (*Aggregate, error) {
	if r.Store == nil || session < 1 {
		return nil, fmt.Errorf("strip store or session is invalid")
	}
	return r.Store.Read(ctx, sessionRef(session))
}

func (r StripState) ByCallsign(ctx context.Context, session int32, callsign string) (*pb.Strip, uint64, error) {
	a, err := r.read(ctx, session)
	if err != nil {
		return nil, 0, err
	}
	key := strings.ToUpper(strings.TrimSpace(callsign))
	if e := a.Entities[key]; e != nil && e.GetValue().GetStrip() != nil {
		return proto.Clone(e.GetValue().GetStrip()).(*pb.Strip), a.Revision, nil
	}
	return nil, a.Revision, fmt.Errorf("strip %s not found", key)
}

func (r StripState) ByID(ctx context.Context, session int32, id uint64) (*pb.Strip, uint64, error) {
	a, err := r.read(ctx, session)
	if err != nil {
		return nil, 0, err
	}
	if e := a.StripIDs[id]; e != nil {
		return proto.Clone(e.GetValue().GetStrip()).(*pb.Strip), a.Revision, nil
	}
	return nil, a.Revision, fmt.Errorf("strip %d not found", id)
}

// List is a coherent projection read for HTTP, PDC, CDM, EFB and initial
// frontend adapters. Its aggregate revision is the source checkpoint.
func (r StripState) List(ctx context.Context, session int32) ([]*pb.Strip, uint64, error) {
	a, err := r.read(ctx, session)
	if err != nil {
		return nil, 0, err
	}
	result := make([]*pb.Strip, 0)
	for _, e := range a.EntitiesByKind(pb.EntityKind_STRIP) {
		result = append(result, e.GetValue().GetStrip())
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Bay != result[j].Bay {
			return result[i].Bay < result[j].Bay
		}
		if result[i].Sequence != result[j].Sequence {
			return result[i].Sequence < result[j].Sequence
		}
		return result[i].Callsign < result[j].Callsign
	})
	return result, a.Revision, nil
}

func (r StripState) execute(ctx context.Context, session int32, actor *pb.Actor, expected uint64, system *pb.SystemCommand, action *pb.StripAction) (*pb.CommandReply, error) {
	if r.Store == nil {
		return nil, fmt.Errorf("strip store is unavailable")
	}
	req := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(session), Actor: actor, ExpectedEntityRevision: &expected}
	if system != nil {
		req.Command = &pb.CommandRequest_System{System: system}
	} else {
		req.Command = &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Strip{Strip: action}}}
	}
	return r.Execute(ctx, req)
}

// Put writes a complete candidate strip. A zero ID allocates from the session
// high-water mark in the same event. Expected is the entity revision (zero for
// creation). EuroScope observations use Observe and retain controller-owned data.
func (r StripState) Put(ctx context.Context, session int32, strip *pb.Strip, expected uint64) (*pb.CommandReply, error) {
	if strip == nil {
		return nil, fmt.Errorf("missing strip")
	}
	copy := proto.Clone(strip).(*pb.Strip)
	copy.Callsign = strings.ToUpper(strings.TrimSpace(copy.Callsign))
	return r.execute(ctx, session, &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "strip"}, expected, &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: copy.Callsign, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: copy}}}}}, nil)
}

func (r StripState) Observe(ctx context.Context, session int32, strip *pb.Strip, expected uint64) (*pb.CommandReply, error) {
	if strip == nil {
		return nil, fmt.Errorf("missing strip")
	}
	copy := proto.Clone(strip).(*pb.Strip)
	copy.Callsign = strings.ToUpper(strings.TrimSpace(copy.Callsign))
	return r.execute(ctx, session, &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "euroscope-strip"}, expected, &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: copy.Callsign, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: copy}}}}}, nil)
}

func (r StripState) Delete(ctx context.Context, session int32, callsign string, expected uint64) (*pb.CommandReply, error) {
	return r.execute(ctx, session, &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "strip"}, expected, &pb.SystemCommand{Action: &pb.SystemCommand_RemoveEntity{RemoveEntity: &pb.RemoveEntity{Key: strings.ToUpper(strings.TrimSpace(callsign)), Kind: pb.EntityKind_STRIP}}}, nil)
}

func (r StripState) Edit(ctx context.Context, session int32, cid string, expected uint64, action *pb.StripAction) (*pb.CommandReply, error) {
	if action == nil {
		return nil, fmt.Errorf("missing strip action")
	}
	copy := proto.Clone(action).(*pb.StripAction)
	copy.Callsign = strings.ToUpper(strings.TrimSpace(copy.Callsign))
	return r.execute(ctx, session, &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: cid, SessionId: &session}, expected, nil, copy)
}

// PlanStrip derives all strip and related session replacements from one loaded
// revision. The writer publishes the returned DomainChange as one event.
func PlanStrip(ctx context.Context, request *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if request.GetClient().GetCoordination() != nil {
		return PlanCoordination(ctx, request, state)
	}
	if update := request.GetSystem().GetUpdateEntity(); update != nil && update.GetValue().GetCoordination() != nil {
		return PlanSystemCoordination(request, state, update)
	}
	if request.GetClient().GetPdc() != nil || request.GetClient().GetTactical() != nil {
		return PlanPdcTactical(ctx, request, state)
	}
	if action := request.GetClient().GetStrip(); action != nil {
		return planStripEdit(request, state, action)
	}
	if update := request.GetSystem().GetUpdateEntity(); update != nil && update.GetValue().GetStrip() != nil {
		return planStripPut(request, state, update)
	}
	if deletion := request.GetSystem().GetRemoveEntity(); deletion != nil && deletion.Kind == pb.EntityKind_STRIP {
		return planStripDelete(request, state, deletion)
	}
	return PlanControllerSector(ctx, request, state)
}

func stripSession(request *pb.CommandRequest, state *Aggregate) (*pb.EntitySnapshot, error) {
	ref := request.GetAggregate().GetSession()
	if ref == nil || state == nil {
		return nil, fmt.Errorf("strip command requires a session")
	}
	e := state.Entities[fmt.Sprint(ref.Id)]
	if e == nil || e.GetValue().GetSession() == nil || e.GetValue().GetSession().Tombstoned {
		return nil, fmt.Errorf("session is not active")
	}
	return e, nil
}

func stripChange(old *pb.EntitySnapshot, strip *pb.Strip) *pb.EntityChange {
	rev := uint64(1)
	if old != nil {
		rev = old.Revision + 1
	}
	strip.Revision = rev
	return &pb.EntityChange{Key: strip.Callsign, Revision: rev, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: strip}}}}
}

func planStripPut(request *pb.CommandRequest, state *Aggregate, update *pb.UpdateEntity) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	session, err := stripSession(request, state)
	if err != nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, err
	}
	if request.GetActor().GetKind() != pb.Actor_SYSTEM || request.ExpectedEntityRevision == nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("system strip revision required")
	}
	incoming := proto.Clone(update.Value.GetStrip()).(*pb.Strip)
	key := strings.ToUpper(strings.TrimSpace(incoming.Callsign))
	if key == "" || update.Key != key {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("strip callsign/key mismatch")
	}
	incoming.Callsign = key
	old := state.Entities[key]
	current := uint64(0)
	if old != nil {
		current = old.Revision
	}
	if *request.ExpectedEntityRevision != current {
		return nil, pb.CommandReply_REVISION_CONFLICT, current, fmt.Errorf("stale strip revision")
	}
	if old != nil && old.GetValue().GetStrip() == nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("callsign key is occupied")
	}
	changes := make([]*pb.EntityChange, 0, 2)
	if old == nil {
		s := proto.Clone(session.GetValue().GetSession()).(*pb.Session)
		if s.NextStripId == 0 || s.NextStripId == math.MaxUint64 || incoming.Id != 0 {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid strip allocation")
		}
		incoming.Id = s.NextStripId
		s.NextStripId++
		changes = append(changes, &pb.EntityChange{Key: session.Key, Revision: session.Revision + 1, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: s}}}})
	} else {
		previous := old.GetValue().GetStrip()
		if request.Actor.Id == "euroscope-strip" && incoming.Id == 0 {
			incoming.Id = previous.Id
		}
		if incoming.Id != previous.Id {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("strip ID is immutable")
		}
		if request.Actor.Id == "euroscope-strip" {
			incoming = mergeObservedStrip(previous, incoming)
		}
	}
	if !validStripBay(incoming.Bay) {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid strip bay")
	}
	if old == nil || incoming.Bay != old.GetValue().GetStrip().Bay {
		sequence, err := endOfStripBay(state, incoming.Bay, key)
		if err != nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
		}
		incoming.Sequence = sequence
	} else if incoming.Sequence == 0 {
		incoming.Sequence = old.GetValue().GetStrip().Sequence
	}
	if old != nil && equalStripWithoutRevision(old.GetValue().GetStrip(), incoming) {
		return &pb.DomainChange{}, pb.CommandReply_COMMITTED, current, nil
	}
	changes = append(changes, stripChange(old, incoming))
	if err := checkStripChanges(state, changes); err != nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
	}
	return &pb.DomainChange{Changes: changes}, pb.CommandReply_COMMITTED, current, nil
}

func planStripDelete(request *pb.CommandRequest, state *Aggregate, deletion *pb.RemoveEntity) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if _, err := stripSession(request, state); err != nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, err
	}
	if request.GetActor().GetKind() != pb.Actor_SYSTEM || request.ExpectedEntityRevision == nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("system strip revision required")
	}
	old := state.Entities[deletion.Key]
	if old == nil || old.GetValue().GetStrip() == nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, fmt.Errorf("strip not found")
	}
	if *request.ExpectedEntityRevision != old.Revision {
		return nil, pb.CommandReply_REVISION_CONFLICT, old.Revision, fmt.Errorf("stale strip revision")
	}
	change := &pb.EntityChange{Key: old.Key, Revision: old.Revision + 1, Operation: &pb.EntityChange_Delete{Delete: &pb.DeleteEntity{Kind: pb.EntityKind_STRIP}}}
	changes := []*pb.EntityChange{change}
	if pdc := state.Indexes[pb.EntityKind_PDC_SEQUENCE][old.Key]; pdc != nil {
		changes = append(changes, candidateDelete(old.Key, pdc, pb.EntityKind_PDC_SEQUENCE))
	}
	if deadline := state.Indexes[pb.EntityKind_SESSION_DEADLINE]["pdc."+old.Key]; deadline != nil {
		changes = append(changes, candidateDelete(deadline.Key, deadline, pb.EntityKind_SESSION_DEADLINE))
	}
	sortCandidateChanges(changes)
	return &pb.DomainChange{Changes: changes}, pb.CommandReply_COMMITTED, old.Revision, nil
}

func mergeObservedStrip(old, incoming *pb.Strip) *pb.Strip {
	copy := proto.Clone(old).(*pb.Strip)
	// EuroScope owns flight-plan facts. Controller state and ordering stay local.
	copy.AircraftType, copy.Departure, copy.Destination = incoming.AircraftType, incoming.Departure, incoming.Destination
	copy.Route, copy.Remarks, copy.Sid, copy.Runway = incoming.Route, incoming.Remarks, incoming.Sid, incoming.Runway
	copy.AssignedSquawk, copy.Squawk = incoming.AssignedSquawk, incoming.Squawk
	copy.RequestedAltitude, copy.ClearedAltitude, copy.Heading = incoming.RequestedAltitude, incoming.ClearedAltitude, incoming.Heading
	copy.Stand, copy.Eobt, copy.Alternate, copy.Star = incoming.Stand, incoming.Eobt, incoming.Alternate, incoming.Star
	copy.HasFlightPlan = incoming.HasFlightPlan
	if incoming.Bay != "" && incoming.Bay != shared.BAY_UNKNOWN {
		copy.Bay = incoming.Bay
	}
	if incoming.HasFlightPlan {
		copy.Manual = false
	}
	for _, field := range old.ControllerModifiedFields {
		switch field {
		case "route":
			copy.Route = old.Route
		case "remarks":
			copy.Remarks = old.Remarks
		case "sid":
			copy.Sid = old.Sid
		case "runway":
			copy.Runway = old.Runway
		case "stand":
			copy.Stand = old.Stand
		case "heading":
			copy.Heading = old.Heading
		case "requested_altitude":
			copy.RequestedAltitude = old.RequestedAltitude
		case "cleared_altitude":
			copy.ClearedAltitude = old.ClearedAltitude
		case "assigned_squawk":
			copy.AssignedSquawk = old.AssignedSquawk
		}
	}
	return copy
}

func equalStripWithoutRevision(a, b *pb.Strip) bool {
	copy := proto.Clone(b).(*pb.Strip)
	copy.Revision = a.Revision
	return proto.Equal(a, copy)
}

func validStripBay(bay string) bool {
	switch bay {
	case shared.BAY_UNKNOWN, shared.BAY_NOT_CLEARED, shared.BAY_CLEARED, shared.BAY_PUSH,
		shared.BAY_TAXI, shared.BAY_TAXI_LWR, shared.BAY_TAXI_TWR, shared.BAY_DEPART,
		shared.BAY_AIRBORNE, shared.BAY_FINAL, shared.BAY_RWY_ARR, shared.BAY_TWY_ARR,
		shared.BAY_STAND, shared.BAY_HIDDEN, shared.BAY_HIDDEN_DEP, shared.BAY_DEP_HIDDEN,
		shared.BAY_ARR_HIDDEN, shared.BAY_CONTROLZONE:
		return true
	}
	return false
}

func endOfStripBay(state *Aggregate, bay, except string) (uint64, error) {
	max := uint64(0)
	for _, e := range state.Entities {
		if e.Key == except {
			continue
		}
		if s := e.GetValue().GetStrip(); s != nil && s.Bay == bay && s.Sequence > max {
			max = s.Sequence
		}
		if s := e.GetValue().GetTacticalStrip(); s != nil && s.Bay == bay && s.Sequence > max {
			max = s.Sequence
		}
	}
	if max > math.MaxUint64-stripOrderSpacing {
		return 0, fmt.Errorf("strip order exhausted")
	}
	return max + stripOrderSpacing, nil
}

func checkStripChanges(state *Aggregate, changes []*pb.EntityChange) error {
	staged := make(map[string]*pb.EntitySnapshot, len(state.Entities)+len(changes))
	for key, e := range state.Entities {
		staged[key] = e
	}
	for _, c := range changes {
		if err := validateChange(state.Ref, c, staged[c.Key]); err != nil {
			return err
		}
		if c.GetDelete() != nil {
			delete(staged, c.Key)
		} else {
			staged[c.Key] = &pb.EntitySnapshot{Key: c.Key, Revision: c.Revision, Value: c.GetUpsert()}
		}
	}
	return validateStripState(state.Ref, staged)
}

func validateStripState(ref *pb.AggregateRef, entities map[string]*pb.EntitySnapshot) error {
	if ref.GetSession() == nil {
		return nil
	}
	session := entities[fmt.Sprint(ref.GetSession().Id)]
	var next uint64
	if session != nil && session.GetValue().GetSession() != nil {
		next = session.GetValue().GetSession().NextStripId
	}
	ids := map[uint64]bool{}
	orders := map[string]map[uint64]bool{}
	for key, e := range entities {
		if tactical := e.GetValue().GetTacticalStrip(); tactical != nil {
			if orders[tactical.Bay] == nil {
				orders[tactical.Bay] = map[uint64]bool{}
			}
			if tactical.Sequence != 0 {
				if orders[tactical.Bay][tactical.Sequence] {
					return fmt.Errorf("duplicate unified strip order in bay")
				}
				orders[tactical.Bay][tactical.Sequence] = true
			}
		}
		s := e.GetValue().GetStrip()
		if s == nil {
			continue
		}
		if s.Callsign != key || key == "" || strings.ToUpper(key) != key || strings.TrimSpace(key) != key || s.Id == 0 || s.Id >= next || ids[s.Id] || s.Revision != e.Revision || !validStripBay(s.Bay) || s.Sequence == 0 {
			return fmt.Errorf("invalid strip identity, version, bay or sequence")
		}
		ids[s.Id] = true
		if orders[s.Bay] == nil {
			orders[s.Bay] = map[uint64]bool{}
		}
		if orders[s.Bay][s.Sequence] {
			return fmt.Errorf("duplicate strip order in bay")
		}
		orders[s.Bay][s.Sequence] = true
	}
	return nil
}

// validateStripTransition closes the generic UpdateEntity escape hatch: a new
// numeric ID must consume the session counter in the same atomic event.
func validateStripTransition(state *Aggregate, changes []*pb.EntityChange, staged map[string]*pb.EntitySnapshot) error {
	if state.Ref.GetSession() == nil {
		return nil
	}
	key := fmt.Sprint(state.Ref.GetSession().Id)
	oldSession := state.Entities[key].GetValue().GetSession()
	newSession := staged[key].GetValue().GetSession()
	if oldSession == nil || newSession == nil {
		return nil
	}
	created := uint64(0)
	for _, change := range changes {
		strip := change.GetUpsert().GetStrip()
		if strip == nil {
			continue
		}
		old := state.Entities[change.Key]
		if old == nil {
			if strip.Id != oldSession.NextStripId+created {
				return fmt.Errorf("strip ID was not allocated from session")
			}
			created++
		} else if prior := old.GetValue().GetStrip(); prior != nil && strip.Id != prior.Id {
			return fmt.Errorf("strip ID changed")
		}
	}
	if oldSession.NextStripId > math.MaxUint64-created || newSession.NextStripId != oldSession.NextStripId+created {
		return fmt.Errorf("strip allocation counter changed outside strip creation")
	}
	return nil
}
