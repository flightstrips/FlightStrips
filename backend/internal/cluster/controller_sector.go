package cluster

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

// ControllerSector is the opt-in session projection adapter. Socket liveness is
// deliberately absent from its durable records; callers join FS_PRESENCE when
// presenting operational controllers.
type ControllerSector struct{ Store LifecycleStore }

func (r ControllerSector) state(ctx context.Context, sessionID int32) (*Aggregate, error) {
	if r.Store == nil || sessionID < 1 {
		return nil, fmt.Errorf("controller/sector store or session is invalid")
	}
	return r.Store.Read(ctx, sessionRef(sessionID))
}

func (r ControllerSector) Session(ctx context.Context, sessionID int32) (*pb.Session, uint64, error) {
	a, err := r.state(ctx, sessionID)
	if err != nil {
		return nil, 0, err
	}
	e := a.Entities[fmt.Sprint(sessionID)]
	if e == nil || e.GetValue().GetSession() == nil || e.GetValue().GetSession().GetTombstoned() {
		return nil, 0, fmt.Errorf("session %d is not active", sessionID)
	}
	return proto.Clone(e.GetValue().GetSession()).(*pb.Session), e.Revision, nil
}

func (r ControllerSector) Controllers(ctx context.Context, sessionID int32) ([]*pb.Controller, error) {
	a, err := r.state(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	result := make([]*pb.Controller, 0)
	for _, e := range a.EntitiesByKind(pb.EntityKind_CONTROLLER) {
		result = append(result, proto.Clone(e.GetValue().GetController()).(*pb.Controller))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Callsign < result[j].Callsign })
	return result, nil
}

// OperationalControllers joins a caller's fresh, typed FS_PRESENCE view with
// durable identities. A restored controller alone never counts as online.
func (r ControllerSector) OperationalControllers(ctx context.Context, sessionID int32, presence []KVPresence, now time.Time) ([]*pb.Controller, error) {
	controllers, err := r.Controllers(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	nodes := make(map[string]bool)
	for _, entry := range presence {
		if node := entry.Value.GetNode(); node != nil && node.Ready && !entry.Observed.After(now) && now.Sub(entry.Observed) < 10*time.Second {
			nodes[node.NodeId] = true
		}
	}
	live := make(map[string]*pb.ClientPresence)
	for _, entry := range presence {
		client := entry.Value.GetClient()
		if client != nil && client.Kind == pb.ClientPresence_EUROSCOPE && client.SessionId == sessionID && nodes[client.NodeId] && !entry.Observed.After(now) && now.Sub(entry.Observed) < 10*time.Second {
			live[client.Cid] = client
		}
	}
	result := make([]*pb.Controller, 0)
	for _, controller := range controllers {
		if client := live[controller.Cid]; client != nil && client.Callsign == controller.Callsign && client.Position == controller.Position {
			result = append(result, controller)
		}
	}
	return result, nil
}

func (r ControllerSector) ControllerByCID(ctx context.Context, sessionID int32, cid string) (*pb.Controller, uint64, error) {
	a, err := r.state(ctx, sessionID)
	if err != nil {
		return nil, 0, err
	}
	e := a.Entities[cid]
	if e == nil || e.GetValue().GetController() == nil {
		return nil, 0, fmt.Errorf("controller %s not found", cid)
	}
	return proto.Clone(e.GetValue().GetController()).(*pb.Controller), e.Revision, nil
}

func (r ControllerSector) ControllerByCallsign(ctx context.Context, sessionID int32, callsign string) (*pb.Controller, uint64, error) {
	a, err := r.state(ctx, sessionID)
	if err != nil {
		return nil, 0, err
	}
	for _, e := range a.EntitiesByKind(pb.EntityKind_CONTROLLER) {
		if e.GetValue().GetController().Callsign == callsign {
			return proto.Clone(e.GetValue().GetController()).(*pb.Controller), e.Revision, nil
		}
	}
	return nil, 0, fmt.Errorf("controller %s not found", callsign)
}

func (r ControllerSector) ControllersByPosition(ctx context.Context, sessionID int32, position string) ([]*pb.Controller, error) {
	all, err := r.Controllers(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	result := make([]*pb.Controller, 0)
	for _, c := range all {
		if c.Position == position {
			result = append(result, c)
		}
	}
	return result, nil
}

func (r ControllerSector) SectorOwners(ctx context.Context, sessionID int32) ([]*pb.SectorOwner, error) {
	a, err := r.state(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	result := make([]*pb.SectorOwner, 0)
	for _, e := range a.EntitiesByKind(pb.EntityKind_SECTOR_OWNER) {
		result = append(result, proto.Clone(e.GetValue().GetSectorOwner()).(*pb.SectorOwner))
	}
	return result, nil
}

// MasterPositionOrder is an airport projection read, independent of any
// session's controller presence or hub-local maps.
func (r ControllerSector) MasterPositionOrder(ctx context.Context, airport string) ([]string, error) {
	if r.Store == nil {
		return nil, fmt.Errorf("controller/sector store is unavailable")
	}
	airport = strings.ToUpper(strings.TrimSpace(airport))
	a, err := r.Store.Read(ctx, &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: airport}}})
	if err != nil {
		return nil, err
	}
	e := a.Entities[airport]
	if e == nil || e.GetValue().GetAirportPolicy() == nil {
		return nil, fmt.Errorf("airport policy %s not found", airport)
	}
	return append([]string(nil), e.GetValue().GetAirportPolicy().MasterPositionOrder...), nil
}

func (r ControllerSector) execute(ctx context.Context, sessionID int32, actor *pb.Actor, expected uint64, system *pb.SystemCommand, client *pb.ClientCommand) (*pb.CommandReply, error) {
	if r.Store == nil {
		return nil, fmt.Errorf("controller/sector store is unavailable")
	}
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(sessionID), Actor: actor, ExpectedEntityRevision: &expected}
	if system != nil {
		request.Command = &pb.CommandRequest_System{System: system}
	} else {
		request.Command = &pb.CommandRequest_Client{Client: client}
	}
	reply := r.Store.Execute(ctx, request)
	if reply == nil {
		return nil, fmt.Errorf("controller/sector command received no reply")
	}
	if reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() == pb.CommandOutcome_FAILED {
		return reply, fmt.Errorf("controller/sector command: %s: %s", reply.Status, reply.Detail)
	}
	return reply, nil
}

func (r ControllerSector) executeBatch(ctx context.Context, sessionID int32, system *pb.SystemCommand) (*pb.CommandReply, error) {
	if r.Store == nil {
		return nil, fmt.Errorf("controller/sector store is unavailable")
	}
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(sessionID), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "controller-sector"}, Command: &pb.CommandRequest_System{System: system}}
	reply := r.Store.Execute(ctx, request)
	if reply == nil {
		return nil, fmt.Errorf("controller/sector command received no reply")
	}
	if reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() == pb.CommandOutcome_FAILED {
		return reply, fmt.Errorf("controller/sector command: %s: %s", reply.Status, reply.GetOutcome().GetDetail())
	}
	return reply, nil
}

func (r ControllerSector) PutController(ctx context.Context, sessionID int32, controller *pb.Controller, expected uint64) (*pb.CommandReply, error) {
	if controller == nil {
		return nil, fmt.Errorf("missing controller")
	}
	copy := proto.Clone(controller).(*pb.Controller)
	copy.Revision = expected + 1
	return r.execute(ctx, sessionID, &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "controller-sector"}, expected,
		&pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: copy.Cid, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Controller{Controller: copy}}}}}, nil)
}

func (r ControllerSector) RemoveController(ctx context.Context, sessionID int32, cid string, expected uint64) (*pb.CommandReply, error) {
	return r.execute(ctx, sessionID, &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "controller-sector"}, expected,
		&pb.SystemCommand{Action: &pb.SystemCommand_RemoveEntity{RemoveEntity: &pb.RemoveEntity{Key: cid, Kind: pb.EntityKind_CONTROLLER}}}, nil)
}

func (r ControllerSector) PutSectorOwner(ctx context.Context, sessionID int32, owner *pb.SectorOwner, expected uint64) (*pb.CommandReply, error) {
	if owner == nil {
		return nil, fmt.Errorf("missing sector owner")
	}
	copy := proto.Clone(owner).(*pb.SectorOwner)
	copy.Sector = strings.ToUpper(strings.TrimSpace(copy.Sector))
	return r.execute(ctx, sessionID, &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "controller-sector"}, expected,
		&pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: copy.Sector, Value: &pb.EntityRecord{Value: &pb.EntityRecord_SectorOwner{SectorOwner: copy}}}}}, nil)
}

func (r ControllerSector) RemoveSectorOwner(ctx context.Context, sessionID int32, sector string, expected uint64) (*pb.CommandReply, error) {
	return r.execute(ctx, sessionID, &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "controller-sector"}, expected,
		&pb.SystemCommand{Action: &pb.SystemCommand_RemoveEntity{RemoveEntity: &pb.RemoveEntity{Key: strings.ToUpper(strings.TrimSpace(sector)), Kind: pb.EntityKind_SECTOR_OWNER}}}, nil)
}

// SetPositionLayout replaces every controller at a frequency in one session
// event, matching the former SQL update's all-rows behavior.
func (r ControllerSector) SetPositionLayout(ctx context.Context, sessionID int32, position, layout string) (*pb.CommandReply, error) {
	return r.executeBatch(ctx, sessionID, &pb.SystemCommand{Action: &pb.SystemCommand_SetPositionLayout{SetPositionLayout: &pb.SetPositionLayout{Position: position, LayoutId: layout}}})
}

// ReplaceSectorOwners atomically replaces the sector routing table. A legacy
// row containing multiple sectors is expanded to one typed owner per sector.
func (r ControllerSector) ReplaceSectorOwners(ctx context.Context, sessionID int32, owners []*pb.SectorOwner) (*pb.CommandReply, error) {
	return r.executeBatch(ctx, sessionID, &pb.SystemCommand{Action: &pb.SystemCommand_ReplaceSectorOwners{ReplaceSectorOwners: &pb.ReplaceSectorOwners{Owners: owners}}})
}

func (r ControllerSector) ChangeRunways(ctx context.Context, sessionID int32, cid string, expected uint64, runways []*pb.Runway) (*pb.CommandReply, error) {
	action := &pb.SessionAction{Change: &pb.SessionAction_Runways{Runways: &pb.ChangeRunways{Runways: runways}}}
	return r.execute(ctx, sessionID, &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: cid, SessionId: &sessionID}, expected, nil, &pb.ClientCommand{Action: &pb.ClientCommand_Session{Session: action}})
}

func (r ControllerSector) UpdateRunwayStatus(ctx context.Context, sessionID int32, cid string, expected uint64, pair, status string) (*pb.CommandReply, error) {
	action := &pb.SessionAction{Change: &pb.SessionAction_UpdateRunwayStatus{UpdateRunwayStatus: &pb.UpdateRunwayStatus{Pair: pair, Status: status}}}
	return r.execute(ctx, sessionID, &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: cid, SessionId: &sessionID}, expected, nil, &pb.ClientCommand{Action: &pb.ClientCommand_Session{Session: action}})
}

func (r ControllerSector) ChangeLayout(ctx context.Context, sessionID int32, cid string, expected uint64, layout string) (*pb.CommandReply, error) {
	action := &pb.SessionAction{Change: &pb.SessionAction_Layout{Layout: &pb.ChangeLayout{LayoutId: layout}}}
	return r.execute(ctx, sessionID, &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: cid, SessionId: &sessionID}, expected, nil, &pb.ClientCommand{Action: &pb.ClientCommand_Session{Session: action}})
}

// PlanControllerSector validates the durable candidate entities before the
// writer's subject CAS. The reducer repeats cross-record checks on replay.
func PlanControllerSector(ctx context.Context, request *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if action := request.GetClient().GetSession(); action != nil {
		return planSessionDisplay(request, state, action)
	}
	if action := request.GetSystem().GetSetPositionLayout(); action != nil {
		return planPositionLayout(request, state, action)
	}
	if action := request.GetSystem().GetReplaceSectorOwners(); action != nil {
		return planReplaceSectorOwners(request, state, action)
	}
	change, status, current, err := PlanSystemEntity(ctx, request, state)
	if err != nil || change == nil || len(change.Changes) != 1 {
		return change, status, current, err
	}
	c := change.Changes[0]
	kind, _ := changeKind(c)
	if kind != pb.EntityKind_CONTROLLER && kind != pb.EntityKind_SECTOR_OWNER {
		return change, status, current, nil
	}
	if request.GetAggregate().GetSession() == nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("controller/sector requires session")
	}
	if c.GetUpsert() != nil {
		if controller := c.GetUpsert().GetController(); controller != nil && controller.Revision != c.Revision {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("controller revision mismatch")
		}
	}
	staged := make(map[string]*pb.EntitySnapshot, len(state.Entities)+1)
	for key, entity := range state.Entities {
		staged[key] = entity
	}
	if c.GetDelete() != nil {
		delete(staged, c.Key)
	} else {
		staged[c.Key] = &pb.EntitySnapshot{Key: c.Key, Revision: c.Revision, Value: c.GetUpsert()}
	}
	if err := validateControllerSectorState(state.Ref, staged); err != nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
	}
	return change, status, current, nil
}

func planPositionLayout(request *pb.CommandRequest, state *Aggregate, action *pb.SetPositionLayout) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if request.GetAggregate().GetSession() == nil || request.GetActor().GetKind() != pb.Actor_SYSTEM || request.ExpectedEntityRevision != nil || action.Position == "" || strings.TrimSpace(action.LayoutId) == "" {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("invalid position layout command")
	}
	changes := make([]*pb.EntityChange, 0)
	for _, old := range state.EntitiesByKind(pb.EntityKind_CONTROLLER) {
		controller := old.GetValue().GetController()
		if controller.Position != action.Position || controller.LayoutId == action.LayoutId {
			continue
		}
		copy := proto.Clone(controller).(*pb.Controller)
		copy.LayoutId, copy.Revision = action.LayoutId, old.Revision+1
		changes = append(changes, &pb.EntityChange{Key: old.Key, Revision: old.Revision + 1, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_Controller{Controller: copy}}}})
	}
	return &pb.DomainChange{Changes: changes}, pb.CommandReply_COMMITTED, 0, nil
}

func planReplaceSectorOwners(request *pb.CommandRequest, state *Aggregate, action *pb.ReplaceSectorOwners) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if request.GetAggregate().GetSession() == nil || request.GetActor().GetKind() != pb.Actor_SYSTEM || request.ExpectedEntityRevision != nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("invalid sector replacement command")
	}
	wanted := make(map[string]*pb.SectorOwner, len(action.Owners))
	for _, owner := range action.Owners {
		if owner == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("nil sector owner")
		}
		copy := proto.Clone(owner).(*pb.SectorOwner)
		copy.Sector = strings.ToUpper(strings.TrimSpace(copy.Sector))
		if copy.Sector == "" || wanted[copy.Sector] != nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("duplicate or empty sector")
		}
		wanted[copy.Sector] = copy
	}
	changes := make([]*pb.EntityChange, 0)
	for _, old := range state.EntitiesByKind(pb.EntityKind_SECTOR_OWNER) {
		if wanted[old.Key] == nil {
			changes = append(changes, &pb.EntityChange{Key: old.Key, Revision: old.Revision + 1, Operation: &pb.EntityChange_Delete{Delete: &pb.DeleteEntity{Kind: pb.EntityKind_SECTOR_OWNER}}})
		}
	}
	for key, owner := range wanted {
		old := state.Entities[key]
		if old != nil && old.GetValue().GetSectorOwner() != nil && proto.Equal(old.GetValue().GetSectorOwner(), owner) {
			continue
		}
		revision := uint64(1)
		if old != nil {
			revision = old.Revision + 1
		}
		changes = append(changes, &pb.EntityChange{Key: key, Revision: revision, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_SectorOwner{SectorOwner: owner}}}})
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Key < changes[j].Key })
	staged := make(map[string]*pb.EntitySnapshot, len(state.Entities)+len(changes))
	for key, entity := range state.Entities {
		staged[key] = entity
	}
	for _, change := range changes {
		if err := validateChange(state.Ref, change, staged[change.Key]); err != nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, err
		}
		if change.GetDelete() != nil {
			delete(staged, change.Key)
		} else {
			staged[change.Key] = &pb.EntitySnapshot{Key: change.Key, Revision: change.Revision, Value: change.GetUpsert()}
		}
	}
	if err := validateControllerSectorState(state.Ref, staged); err != nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, err
	}
	return &pb.DomainChange{Changes: changes}, pb.CommandReply_COMMITTED, 0, nil
}

func planSessionDisplay(request *pb.CommandRequest, state *Aggregate, action *pb.SessionAction) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	id := request.GetAggregate().GetSession().GetId()
	if id < 1 || request.Actor == nil || request.Actor.Kind != pb.Actor_CONTROLLER || request.Actor.GetSessionId() != id {
		return nil, pb.CommandReply_UNAUTHORIZED, 0, fmt.Errorf("session controller required")
	}
	controller := state.Entities[request.Actor.Id]
	if controller == nil || controller.GetValue().GetController() == nil || controller.GetValue().GetController().Observer {
		return nil, pb.CommandReply_UNAUTHORIZED, 0, fmt.Errorf("active controller identity required")
	}
	key := fmt.Sprint(id)
	old := state.Entities[key]
	if old == nil || old.GetValue().GetSession() == nil || old.GetValue().GetSession().Tombstoned {
		return nil, pb.CommandReply_NOT_FOUND, 0, fmt.Errorf("session not active")
	}
	current := old.Revision
	if request.ExpectedEntityRevision == nil || *request.ExpectedEntityRevision != current {
		return nil, pb.CommandReply_REVISION_CONFLICT, current, fmt.Errorf("stale session revision")
	}
	s := proto.Clone(old.GetValue().GetSession()).(*pb.Session)
	switch x := action.GetChange().(type) {
	case *pb.SessionAction_Layout:
		if x.Layout == nil || strings.TrimSpace(x.Layout.LayoutId) == "" {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid layout")
		}
		s.LayoutId = x.Layout.LayoutId
	case *pb.SessionAction_Runways:
		if x.Runways == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("missing runways")
		}
		s.Runways = nil
		for _, runway := range x.Runways.Runways {
			if runway == nil {
				return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("nil runway")
			}
			s.Runways = append(s.Runways, proto.Clone(runway).(*pb.Runway))
		}
	case *pb.SessionAction_UpdateRunwayStatus:
		if x.UpdateRunwayStatus == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("missing runway status")
		}
		found := false
		for _, item := range s.RunwayStatuses {
			if item.Pair == x.UpdateRunwayStatus.Pair {
				item.Status = x.UpdateRunwayStatus.Status
				found = true
				break
			}
		}
		if !found {
			s.RunwayStatuses = append(s.RunwayStatuses, &pb.RunwayStatus{Pair: x.UpdateRunwayStatus.Pair, Status: x.UpdateRunwayStatus.Status})
		}
	default:
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("unsupported session action")
	}
	if err := validateSessionDisplay(s); err != nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
	}
	if proto.Equal(s, old.GetValue().GetSession()) {
		return &pb.DomainChange{}, pb.CommandReply_COMMITTED, current, nil
	}
	change := &pb.EntityChange{Key: key, Revision: current + 1, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: s}}}}
	return &pb.DomainChange{Changes: []*pb.EntityChange{change}}, pb.CommandReply_COMMITTED, current, nil
}

func validateSessionDisplay(s *pb.Session) error {
	seen := make(map[string]bool)
	for _, runway := range s.Runways {
		if runway == nil || runway.Name == "" || strings.ToUpper(runway.Name) != runway.Name || strings.TrimSpace(runway.Name) != runway.Name || (!runway.Arrival && !runway.Departure) || seen[runway.Name] {
			return fmt.Errorf("invalid or duplicate runway")
		}
		seen[runway.Name] = true
	}
	pairs := make(map[string]bool)
	for _, status := range s.RunwayStatuses {
		if status == nil || status.Pair == "" || strings.TrimSpace(status.Pair) != status.Pair || pairs[status.Pair] || (status.Status != "OPEN" && status.Status != "LOW_VIS" && status.Status != "CLOSED") {
			return fmt.Errorf("invalid or duplicate runway status")
		}
		pairs[status.Pair] = true
	}
	sort.Slice(s.Runways, func(i, j int) bool { return s.Runways[i].Name < s.Runways[j].Name })
	sort.Slice(s.RunwayStatuses, func(i, j int) bool { return s.RunwayStatuses[i].Pair < s.RunwayStatuses[j].Pair })
	return nil
}

func validateControllerSectorState(ref *pb.AggregateRef, entities map[string]*pb.EntitySnapshot) error {
	if ref.GetSession() == nil {
		return nil
	}
	callsigns := make(map[string]string)
	for key, entity := range entities {
		if c := entity.GetValue().GetController(); c != nil {
			if c.Cid != key || c.Cid == "" || c.Callsign == "" || strings.ToUpper(c.Callsign) != c.Callsign || strings.TrimSpace(c.Callsign) != c.Callsign || c.Revision != entity.Revision {
				return fmt.Errorf("invalid controller identity")
			}
			if previous := callsigns[c.Callsign]; previous != "" && previous != key {
				return fmt.Errorf("duplicate controller callsign")
			}
			callsigns[c.Callsign] = key
			sectors := make(map[string]bool)
			for _, sector := range c.OwnedSectors {
				if sector == "" || strings.ToUpper(sector) != sector || sectors[sector] {
					return fmt.Errorf("invalid owned sector")
				}
				sectors[sector] = true
			}
		}
		if owner := entity.GetValue().GetSectorOwner(); owner != nil {
			if owner.Sector != key || owner.Sector == "" || strings.ToUpper(owner.Sector) != owner.Sector || strings.TrimSpace(owner.Sector) != owner.Sector || owner.Position == "" || owner.Identifier == "" {
				return fmt.Errorf("invalid sector owner")
			}
		}
		if session := entity.GetValue().GetSession(); session != nil {
			if err := validateSessionDisplay(proto.Clone(session).(*pb.Session)); err != nil {
				return err
			}
		}
	}
	return nil
}
