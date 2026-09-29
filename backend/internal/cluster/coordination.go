package cluster

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CoordinationState is the dormant session adapter. Its store may be a local
// writer in tests or the owner router; the application still uses PostgreSQL.
type CoordinationState struct{ Store LifecycleStore }

func (r CoordinationState) Execute(ctx context.Context, request *pb.CommandRequest) (*pb.CommandReply, error) {
	if r.Store == nil || request == nil || request.GetAggregate().GetSession() == nil || request.GetClient().GetCoordination() == nil {
		return nil, fmt.Errorf("coordination command requires a session store")
	}
	reply := r.Store.Execute(ctx, request)
	if reply == nil {
		return nil, fmt.Errorf("coordination command received no reply")
	}
	if reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() == pb.CommandOutcome_FAILED {
		return reply, fmt.Errorf("coordination command: %s: %s", reply.Status, reply.GetOutcome().GetDetail())
	}
	return reply, nil
}

// Action accepts the boundary's command ID unchanged across retries.
func (r CoordinationState) Action(ctx context.Context, session int32, cid, commandID string, expectedStripRevision uint64, action *pb.CoordinationAction) (*pb.CommandReply, error) {
	if action == nil {
		return nil, fmt.Errorf("missing coordination action")
	}
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: commandID, Aggregate: sessionRef(session), Actor: &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: cid, SessionId: &session}, ExpectedEntityRevision: &expectedStripRevision, Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Coordination{Coordination: proto.Clone(action).(*pb.CoordinationAction)}}}}
	return r.Execute(ctx, request)
}

// NewAction is useful for internal callers with no boundary request ID.
func (r CoordinationState) NewAction(ctx context.Context, session int32, cid string, expected uint64, action *pb.CoordinationAction) (*pb.CommandReply, error) {
	return r.Action(ctx, session, cid, uuid.NewString(), expected, action)
}

// ObserveTransfer accepts an already validated internal EuroScope handover.
// Its zero ID is allocated by the owner; expected is the strip revision.
func (r CoordinationState) ObserveTransfer(ctx context.Context, session int32, commandID string, expected uint64, candidate *pb.Coordination) (*pb.CommandReply, error) {
	if r.Store == nil || candidate == nil {
		return nil, fmt.Errorf("coordination store or candidate is missing")
	}
	copy := proto.Clone(candidate).(*pb.Coordination)
	copy.Callsign = strings.ToUpper(strings.TrimSpace(copy.Callsign))
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: commandID, Aggregate: sessionRef(session), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "euroscope-coordination"}, ExpectedEntityRevision: &expected, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: copy.Callsign, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Coordination{Coordination: copy}}}}}}}
	reply := r.Store.Execute(ctx, request)
	if reply == nil {
		return nil, fmt.Errorf("coordination command received no reply")
	}
	if reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() == pb.CommandOutcome_FAILED {
		return reply, fmt.Errorf("coordination observation: %s: %s", reply.Status, reply.GetOutcome().GetDetail())
	}
	return reply, nil
}

func (r CoordinationState) read(ctx context.Context, session int32) (*Aggregate, error) {
	if r.Store == nil || session < 1 {
		return nil, fmt.Errorf("coordination store or session is invalid")
	}
	return r.Store.Read(ctx, sessionRef(session))
}

func (r CoordinationState) ByID(ctx context.Context, session int32, id uint64) (*pb.Coordination, uint64, error) {
	a, err := r.read(ctx, session)
	if err != nil {
		return nil, 0, err
	}
	if e := a.Indexes[pb.EntityKind_COORDINATION][strconv.FormatUint(id, 10)]; e != nil {
		return proto.Clone(e.GetValue().GetCoordination()).(*pb.Coordination), a.Revision, nil
	}
	return nil, a.Revision, fmt.Errorf("coordination %d not found", id)
}

func (r CoordinationState) ByStrip(ctx context.Context, session int32, callsign string) (*pb.Coordination, uint64, error) {
	a, err := r.read(ctx, session)
	if err != nil {
		return nil, 0, err
	}
	for _, e := range a.EntitiesByKind(pb.EntityKind_COORDINATION) {
		if c := e.GetValue().GetCoordination(); c != nil && c.Callsign == strings.ToUpper(strings.TrimSpace(callsign)) {
			return c, a.Revision, nil
		}
	}
	return nil, a.Revision, fmt.Errorf("coordination for %s not found", callsign)
}

func (r CoordinationState) ByStripID(ctx context.Context, session int32, stripID uint64) (*pb.Coordination, uint64, error) {
	a, err := r.read(ctx, session)
	if err != nil {
		return nil, 0, err
	}
	strip := a.StripIDs[stripID]
	if strip == nil {
		return nil, a.Revision, fmt.Errorf("strip %d not found", stripID)
	}
	for _, e := range a.EntitiesByKind(pb.EntityKind_COORDINATION) {
		if c := e.GetValue().GetCoordination(); c != nil && c.Callsign == strip.GetValue().GetStrip().Callsign {
			return c, a.Revision, nil
		}
	}
	return nil, a.Revision, fmt.Errorf("coordination for strip %d not found", stripID)
}

func (r CoordinationState) ListByStrip(ctx context.Context, session int32, stripID uint64) ([]*pb.Coordination, uint64, error) {
	c, revision, err := r.ByStripID(ctx, session, stripID)
	if err != nil {
		return nil, revision, err
	}
	return []*pb.Coordination{c}, revision, nil
}

func (r CoordinationState) List(ctx context.Context, session int32) ([]*pb.Coordination, uint64, error) {
	a, err := r.read(ctx, session)
	if err != nil {
		return nil, 0, err
	}
	result := make([]*pb.Coordination, 0)
	for _, e := range a.EntitiesByKind(pb.EntityKind_COORDINATION) {
		result = append(result, e.GetValue().GetCoordination())
	}
	return result, a.Revision, nil
}

func coordinationForStrip(state *Aggregate, callsign string) *pb.EntitySnapshot {
	for _, e := range state.Indexes[pb.EntityKind_COORDINATION] {
		if e.GetValue().GetCoordination().Callsign == callsign {
			return e
		}
	}
	return nil
}

func activeController(state *Aggregate, cid string) *pb.Controller {
	e := state.Indexes[pb.EntityKind_CONTROLLER][cid]
	if e == nil || e.GetValue().GetController().Observer {
		return nil
	}
	return e.GetValue().GetController()
}

// PlanSystemCoordination handles an internal observation that cannot be
// represented as a controller action, including an inbound EuroScope handover.
func PlanSystemCoordination(request *pb.CommandRequest, state *Aggregate, update *pb.UpdateEntity) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	session, err := stripSession(request, state)
	if err != nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, err
	}
	if request.GetActor().GetKind() != pb.Actor_SYSTEM || request.ExpectedEntityRevision == nil {
		return nil, pb.CommandReply_UNAUTHORIZED, 0, fmt.Errorf("internal coordination observation required")
	}
	incoming := update.GetValue().GetCoordination()
	if incoming == nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("missing coordination")
	}
	key := strings.ToUpper(strings.TrimSpace(incoming.Callsign))
	strip := state.Indexes[pb.EntityKind_STRIP][key]
	if strip == nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, fmt.Errorf("strip not found")
	}
	current := strip.Revision
	if *request.ExpectedEntityRevision != current {
		return nil, pb.CommandReply_REVISION_CONFLICT, current, fmt.Errorf("stale strip revision")
	}
	if incoming.Id != 0 || update.Key != key || key == "" || incoming.Status != "TRANSFER" || incoming.CreatedAt != nil || incoming.ResolvedAt != nil || incoming.FromCid == incoming.ToCid || activeController(state, incoming.ToCid) == nil || coordinationForStrip(state, key) != nil || (!incoming.FromEuroscope && incoming.EuroscopeHandoverCid != "") || (incoming.EuroscopeHandoverCid != "" && incoming.EuroscopeHandoverCid != incoming.ToCid) {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid or duplicate internal transfer")
	}
	s := proto.Clone(strip.GetValue().GetStrip()).(*pb.Strip)
	if !incoming.FromEuroscope && s.OwnerCid != incoming.FromCid {
		return nil, pb.CommandReply_UNAUTHORIZED, current, fmt.Errorf("transfer source is not strip owner")
	}
	if incoming.FromEuroscope && s.Bay != shared.BAY_ARR_HIDDEN && s.Bay != shared.BAY_FINAL && s.Bay != shared.BAY_RWY_ARR {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("EuroScope arrival strip is outside arrival bays")
	}
	copySession := proto.Clone(session.GetValue().GetSession()).(*pb.Session)
	if copySession.NextCoordinationId == 0 || copySession.NextCoordinationId == math.MaxUint64 {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("coordination ID exhausted")
	}
	coord := proto.Clone(incoming).(*pb.Coordination)
	coord.Id = copySession.NextCoordinationId
	coord.Callsign = key
	coord.CreatedAt = timestamppb.New(time.Now().UTC())
	copySession.NextCoordinationId++
	changes := []*pb.EntityChange{{Key: session.Key, Revision: session.Revision + 1, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: copySession}}}}}
	if incoming.FromEuroscope {
		if s.Bay == shared.BAY_ARR_HIDDEN {
			if err := moveStripBay(state, s, shared.BAY_FINAL, copySession.Airport, false, false); err != nil {
				return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
			}
		}
		if s.OwnerCid == coord.ToCid && coord.FromCid != coord.ToCid && s.Bay == shared.BAY_FINAL {
			s.OwnerCid = ""
		}
	}
	if !equalStripWithoutRevision(strip.GetValue().GetStrip(), s) {
		changes = append(changes, stripChange(strip, s))
	}
	changes = append(changes, &pb.EntityChange{Key: strconv.FormatUint(coord.Id, 10), Revision: 1, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_Coordination{Coordination: coord}}}})
	return &pb.DomainChange{Changes: changes}, pb.CommandReply_COMMITTED, current, nil
}

// PlanCoordination validates the complete transition against one session
// revision. The writer's subject CAS retries it after a competing command.
func PlanCoordination(_ context.Context, request *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if _, err := stripSession(request, state); err != nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, err
	}
	actor := request.GetActor()
	sessionID := request.GetAggregate().GetSession().GetId()
	if actor.GetKind() != pb.Actor_CONTROLLER || actor.GetSessionId() != sessionID || activeController(state, actor.Id) == nil {
		return nil, pb.CommandReply_UNAUTHORIZED, 0, fmt.Errorf("active controller required")
	}
	action := request.GetClient().GetCoordination()
	if action == nil || action.GetChange() == nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("missing coordination change")
	}
	key := strings.ToUpper(strings.TrimSpace(action.Callsign))
	old := state.Indexes[pb.EntityKind_STRIP][key]
	if old == nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, fmt.Errorf("strip not found")
	}
	current := old.Revision
	if request.ExpectedEntityRevision == nil || *request.ExpectedEntityRevision != current {
		return nil, pb.CommandReply_REVISION_CONFLICT, current, fmt.Errorf("stale strip revision")
	}
	s := proto.Clone(old.GetValue().GetStrip()).(*pb.Strip)
	coord := coordinationForStrip(state, key)
	fail := func(status pb.CommandReply_Status, message string) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		return nil, status, current, fmt.Errorf("%s", message)
	}
	if s.Validation != nil && s.Validation.Active {
		return fail(pb.CommandReply_INVALID_ARGUMENT, "strip is locked by validation")
	}
	var changes []*pb.EntityChange
	var nextOwner string
	var claim, release, create bool
	var missedApproachTower string
	var target, tag string
	switch x := action.GetChange().(type) {
	case *pb.CoordinationAction_Transfer:
		if x.Transfer == nil {
			return fail(pb.CommandReply_INVALID_ARGUMENT, "missing transfer")
		}
		if s.OwnerCid != actor.Id {
			return fail(pb.CommandReply_UNAUTHORIZED, "strip is not owned by initiator")
		}
		if coord != nil {
			return fail(pb.CommandReply_INVALID_ARGUMENT, "strip already has a coordination")
		}
		target = strings.TrimSpace(x.Transfer.ToCid)
		if target == "" && len(s.NextControllers) > 0 {
			target = s.NextControllers[0]
		}
		if target == actor.Id || activeController(state, target) == nil {
			return fail(pb.CommandReply_INVALID_ARGUMENT, "invalid transfer target")
		}
		create = true
		s.Marked = false
		if s.Bay == "STAND" {
			s.StartRequested = false
		}
	case *pb.CoordinationAction_Tag:
		if x.Tag == nil || s.OwnerCid == "" || s.OwnerCid == actor.Id || coord != nil || x.Tag.ToCid != s.OwnerCid {
			return fail(pb.CommandReply_INVALID_ARGUMENT, "invalid or duplicate tag request")
		}
		target, tag, create = actor.Id, x.Tag.Tag, true
	case *pb.CoordinationAction_Assume:
		if x.Assume == nil {
			return fail(pb.CommandReply_INVALID_ARGUMENT, "missing assume")
		}
		if coord != nil {
			c := coord.GetValue().GetCoordination()
			if c.Status != "TRANSFER" || c.ToCid != actor.Id {
				return fail(pb.CommandReply_UNAUTHORIZED, "strip was not transferred to controller")
			}
			release = true
			if c.FromCid != "" && c.FromCid != actor.Id && !slices.Contains(s.PreviousControllers, c.FromCid) {
				s.PreviousControllers = append(s.PreviousControllers, c.FromCid)
			}
			if from := state.Indexes[pb.EntityKind_CONTROLLER][c.FromCid]; s.Bay == shared.BAY_AIRBORNE && from != nil && from.GetValue().GetController().Section == "TWR" && activeController(state, actor.Id).Section == "APP" {
				missedApproachTower = c.FromCid
			}
		} else if s.OwnerCid != "" {
			return fail(pb.CommandReply_UNAUTHORIZED, "strip is already owned")
		}
		claim, nextOwner = true, actor.Id
	case *pb.CoordinationAction_ForceAssume:
		if x.ForceAssume == nil {
			return fail(pb.CommandReply_INVALID_ARGUMENT, "missing force assume")
		}
		if x.ForceAssume.FromCid != "" && x.ForceAssume.FromCid != s.OwnerCid {
			return fail(pb.CommandReply_REVISION_CONFLICT, "owner changed")
		}
		release, claim, nextOwner = coord != nil, true, actor.Id
		// A displaced owner belongs in history only on the expected route.
		expected := slices.Contains(s.NextControllers, actor.Id)
		s.PreviousControllers = slices.DeleteFunc(s.PreviousControllers, func(cid string) bool { return cid == s.OwnerCid })
		if expected && s.OwnerCid != "" && s.OwnerCid != actor.Id {
			s.PreviousControllers = append(s.PreviousControllers, s.OwnerCid)
		}
	case *pb.CoordinationAction_AcceptTag:
		if x.AcceptTag == nil || s.OwnerCid != actor.Id || coord == nil {
			return fail(pb.CommandReply_UNAUTHORIZED, "strip owner and pending tag request required")
		}
		c := coord.GetValue().GetCoordination()
		if c.Status != "TAG" || x.AcceptTag.RequestId != strconv.FormatUint(c.Id, 10) {
			return fail(pb.CommandReply_INVALID_ARGUMENT, "tag request ID mismatch")
		}
		release, claim, nextOwner = true, true, c.ToCid
		expected := slices.Contains(s.NextControllers, nextOwner)
		s.PreviousControllers = slices.DeleteFunc(s.PreviousControllers, func(cid string) bool { return cid == actor.Id })
		if expected {
			s.PreviousControllers = append(s.PreviousControllers, actor.Id)
		}
	case *pb.CoordinationAction_Cancel:
		if x.Cancel == nil || coord == nil {
			return fail(pb.CommandReply_NOT_FOUND, "transfer not found")
		}
		c := coord.GetValue().GetCoordination()
		if c.Status != "TRANSFER" || x.Cancel.TransferId != strconv.FormatUint(c.Id, 10) {
			return fail(pb.CommandReply_INVALID_ARGUMENT, "transfer ID mismatch")
		}
		if c.FromCid != actor.Id {
			return fail(pb.CommandReply_UNAUTHORIZED, "only initiator can cancel transfer")
		}
		release = true
	case *pb.CoordinationAction_Free:
		if x.Free == nil || s.OwnerCid != actor.Id {
			return fail(pb.CommandReply_UNAUTHORIZED, "only owner can free strip")
		}
		if !slices.Contains(s.PreviousControllers, actor.Id) {
			s.PreviousControllers = append(s.PreviousControllers, actor.Id)
		}
		s.OwnerCid = ""
	default:
		return fail(pb.CommandReply_INVALID_ARGUMENT, "unsupported coordination action")
	}
	if create {
		session := state.Indexes[pb.EntityKind_SESSION][strconv.Itoa(int(sessionID))]
		copy := proto.Clone(session.GetValue().GetSession()).(*pb.Session)
		if copy.NextCoordinationId == 0 || copy.NextCoordinationId == math.MaxUint64 {
			return fail(pb.CommandReply_INVALID_ARGUMENT, "coordination ID exhausted")
		}
		id := copy.NextCoordinationId
		copy.NextCoordinationId++
		changes = append(changes, &pb.EntityChange{Key: session.Key, Revision: session.Revision + 1, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: copy}}}})
		status := "TRANSFER"
		from := actor.Id
		if _, ok := action.GetChange().(*pb.CoordinationAction_Tag); ok {
			status, from = "TAG", s.OwnerCid
		}
		c := &pb.Coordination{Id: id, Callsign: key, FromCid: from, ToCid: target, Status: status, Tag: tag, CreatedAt: timestamppb.New(time.Now().UTC())}
		changes = append(changes, &pb.EntityChange{Key: strconv.FormatUint(id, 10), Revision: 1, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_Coordination{Coordination: c}}}})
	}
	if release {
		changes = append(changes, &pb.EntityChange{Key: coord.Key, Revision: coord.Revision + 1, Operation: &pb.EntityChange_Delete{Delete: &pb.DeleteEntity{Kind: pb.EntityKind_COORDINATION}}})
	}
	if claim {
		if i := slices.Index(s.NextControllers, nextOwner); i >= 0 {
			s.NextControllers = slices.Clone(s.NextControllers[i+1:])
		}
		s.OwnerCid = nextOwner
		s.PreviousControllers = slices.DeleteFunc(s.PreviousControllers, func(cid string) bool { return slices.Contains(s.NextControllers, cid) })
		if missedApproachTower != "" {
			s.PreviousControllers = slices.DeleteFunc(s.PreviousControllers, func(cid string) bool { return cid == missedApproachTower })
			s.NextControllers = slices.DeleteFunc(s.NextControllers, func(cid string) bool { return cid == missedApproachTower })
			s.NextControllers = append([]string{missedApproachTower}, s.NextControllers...)
			if err := moveStripBay(state, s, shared.BAY_FINAL, state.Indexes[pb.EntityKind_SESSION][strconv.Itoa(int(sessionID))].GetValue().GetSession().Airport, false, false); err != nil {
				return fail(pb.CommandReply_INVALID_ARGUMENT, err.Error())
			}
		}
	}
	if !equalStripWithoutRevision(old.GetValue().GetStrip(), s) {
		changes = append(changes, stripChange(old, s))
	}
	// The reducer requires entity case order: session, strip, coordination.
	slices.SortFunc(changes, func(a, b *pb.EntityChange) int {
		ak, _ := changeKind(a)
		bk, _ := changeKind(b)
		if ak != bk {
			return int(ak) - int(bk)
		}
		return strings.Compare(a.Key, b.Key)
	})
	return &pb.DomainChange{Changes: changes}, pb.CommandReply_COMMITTED, current, nil
}

// validateCoordinationTransition guards replay and the generic entity writer.
func validateCoordinationTransition(state *Aggregate, changes []*pb.EntityChange, staged map[string]*pb.EntitySnapshot) error {
	if state.Ref.GetSession() == nil {
		return nil
	}
	key := strconv.Itoa(int(state.Ref.GetSession().GetId()))
	oldSession := state.Indexes[pb.EntityKind_SESSION][key]
	newSession := entitySlot(staged, pb.EntityKind_SESSION, key)
	if oldSession == nil || staged[newSession] == nil {
		return nil
	}
	oldCounter := oldSession.GetValue().GetSession().NextCoordinationId
	newCounter := staged[newSession].GetValue().GetSession().NextCoordinationId
	created := uint64(0)
	for _, c := range changes {
		if v := c.GetUpsert().GetCoordination(); v != nil {
			if state.Indexes[pb.EntityKind_COORDINATION][c.Key] != nil {
				return fmt.Errorf("coordination is immutable")
			}
			if v.Id != oldCounter+created {
				return fmt.Errorf("coordination ID was not allocated from session")
			}
			created++
		}
	}
	if oldCounter > math.MaxUint64-created || newCounter != oldCounter+created {
		return fmt.Errorf("coordination allocation counter changed outside creation")
	}
	active := make(map[string]bool)
	for _, e := range staged {
		c := e.GetValue().GetCoordination()
		if c == nil {
			continue
		}
		if c.Id == 0 || c.Id >= newCounter || c.Callsign == "" || strings.ToUpper(c.Callsign) != c.Callsign || c.FromCid == c.ToCid || c.ToCid == "" || (c.Status != "TRANSFER" && c.Status != "TAG") || c.CreatedAt == nil || c.CreatedAt.CheckValid() != nil || active[c.Callsign] {
			return fmt.Errorf("invalid or duplicate coordination")
		}
		if strip := staged[entitySlot(staged, pb.EntityKind_STRIP, c.Callsign)]; strip == nil || strip.GetValue().GetStrip() == nil {
			return fmt.Errorf("coordination strip missing")
		}
		active[c.Callsign] = true
	}
	return nil
}
