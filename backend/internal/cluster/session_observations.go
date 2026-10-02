package cluster

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SessionObservations is opt-in until the NATS-only cutover. The current hub
// and PostgreSQL runtime do not construct this adapter.
type SessionObservations struct{ Store LifecycleStore }

func (s SessionObservations) List(ctx context.Context, sessionID int32) (*Aggregate, error) {
	if s.Store == nil || sessionID < 1 {
		return nil, fmt.Errorf("session observation store unavailable")
	}
	return s.Store.Read(ctx, sessionRef(sessionID))
}

func (s SessionObservations) Execute(ctx context.Context, request *pb.CommandRequest) (*pb.CommandReply, error) {
	if s.Store == nil || request == nil || request.Aggregate.GetSession() == nil || !canonicalUUID(request.CommandId) {
		return nil, fmt.Errorf("invalid session observation request")
	}
	reply := s.Store.Execute(ctx, request)
	if reply == nil {
		return nil, fmt.Errorf("session observation command received no reply")
	}
	if reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() == pb.CommandOutcome_FAILED {
		return reply, fmt.Errorf("session observation: %s: %s", reply.Status, reply.Detail)
	}
	return reply, nil
}

// ExecuteFromPosition holds the owner's position dispatcher barrier through
// the state commit. Accepted reports drain first, then the KV source revision
// is rechecked before a stand, strip or other derived session transition.
func (s SessionObservations) ExecuteFromPosition(ctx context.Context, positions *PositionWriter, request *pb.CommandRequest, aircraft string, revision uint64) (*pb.CommandReply, error) {
	if positions == nil || request == nil || request.Aggregate.GetSession() == nil || request.Aggregate.GetSession().Id != positions.SessionID {
		return nil, fmt.Errorf("position-derived command requires the session owner writer")
	}
	return positions.ExecuteDerivedContext(ctx, aircraft, revision, func(runCtx context.Context) (*pb.CommandReply, error) { return s.Execute(runCtx, request) })
}

func (s SessionObservations) SendMessage(ctx context.Context, sessionID int32, commandID, senderCID, text string, recipients []string) (*pb.CommandReply, error) {
	request := &pb.CommandRequest{ProtocolRevision: 1, Aggregate: sessionRef(sessionID), CommandId: commandID,
		Actor:   &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: senderCID, SessionId: &sessionID},
		Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Message{Message: &pb.MessageAction{Send: &pb.MessageAction_Broadcast{Broadcast: &pb.BroadcastMessage{Text: text, Recipients: recipients}}}}}}}
	return s.Execute(ctx, request)
}

func (s SessionObservations) put(ctx context.Context, sessionID int32, commandID, actor string, key string, record *pb.EntityRecord, expected uint64) (*pb.CommandReply, error) {
	request := &pb.CommandRequest{ProtocolRevision: 1, Aggregate: sessionRef(sessionID), CommandId: commandID,
		Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: actor}, ExpectedEntityRevision: &expected,
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: key, Value: record}}}}}
	return s.Execute(ctx, request)
}

func (s SessionObservations) remove(ctx context.Context, sessionID int32, commandID, actor, key string, kind pb.EntityKind, expected uint64) (*pb.CommandReply, error) {
	request := &pb.CommandRequest{ProtocolRevision: 1, Aggregate: sessionRef(sessionID), CommandId: commandID,
		Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: actor}, ExpectedEntityRevision: &expected,
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_RemoveEntity{RemoveEntity: &pb.RemoveEntity{Key: key, Kind: kind}}}}}
	return s.Execute(ctx, request)
}

// PutAtis replaces the full session presentation including both ATIS codes.
func (s SessionObservations) PutAtis(ctx context.Context, sessionID int32, commandID string, atis *pb.Atis, expected uint64) (*pb.CommandReply, error) {
	if atis == nil {
		return nil, fmt.Errorf("missing ATIS")
	}
	copy := proto.Clone(atis).(*pb.Atis)
	return s.put(ctx, sessionID, commandID, "atis", copy.Airport, &pb.EntityRecord{Value: &pb.EntityRecord_Atis{Atis: copy}}, expected)
}

func (s SessionObservations) PutClxOverride(ctx context.Context, sessionID int32, commandID string, override *pb.ClxOverride, expected uint64) (*pb.CommandReply, error) {
	if override == nil {
		return nil, fmt.Errorf("missing CLX override")
	}
	copy := proto.Clone(override).(*pb.ClxOverride)
	return s.put(ctx, sessionID, commandID, "clx", copy.Callsign+"."+copy.Key, &pb.EntityRecord{Value: &pb.EntityRecord_ClxOverride{ClxOverride: copy}}, expected)
}

func (s SessionObservations) RemoveClxOverride(ctx context.Context, sessionID int32, commandID, callsign, key string, expected uint64) (*pb.CommandReply, error) {
	return s.remove(ctx, sessionID, commandID, "clx", callsign+"."+key, pb.EntityKind_CLX_OVERRIDE, expected)
}

func (s SessionObservations) PutDeadline(ctx context.Context, sessionID int32, commandID string, deadline *pb.SessionDeadline, expected uint64) (*pb.CommandReply, error) {
	if deadline == nil {
		return nil, fmt.Errorf("missing deadline")
	}
	copy := proto.Clone(deadline).(*pb.SessionDeadline)
	return s.put(ctx, sessionID, commandID, "session-deadline", copy.Id, &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: copy}}, expected)
}

func (s SessionObservations) RemoveDeadline(ctx context.Context, sessionID int32, commandID, id string, expected uint64) (*pb.CommandReply, error) {
	return s.remove(ctx, sessionID, commandID, "session-deadline", id, pb.EntityKind_SESSION_DEADLINE, expected)
}

func (s SessionObservations) AircraftDisconnectPending(ctx context.Context, sessionID int32, aircraft string) (*pb.SessionDeadline, uint64, error) {
	state, err := s.List(ctx, sessionID)
	if err != nil {
		return nil, 0, err
	}
	for _, item := range state.EntitiesByKind(pb.EntityKind_SESSION_DEADLINE) {
		deadline := item.GetValue().GetSessionDeadline()
		if deadline != nil && deadline.Kind == "aircraft-disconnect" && deadline.Callsign == aircraft {
			return proto.Clone(deadline).(*pb.SessionDeadline), item.Revision, nil
		}
	}
	return nil, 0, nil
}

func (s SessionObservations) RecordSync(ctx context.Context, sessionID int32, commandID string, sync *pb.SessionSync) (*pb.CommandReply, error) {
	if sync == nil {
		return nil, fmt.Errorf("missing session sync")
	}
	request := &pb.CommandRequest{ProtocolRevision: 1, Aggregate: sessionRef(sessionID), CommandId: commandID,
		Actor:   &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "euroscope-sync"},
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_RecordSync{RecordSync: &pb.RecordSessionSync{Sync: proto.Clone(sync).(*pb.SessionSync)}}}}}
	return s.Execute(ctx, request)
}

// PlanSessionObservations is part of the candidate owner writer's planner
// chain. Other session actions continue through the controller/strip planners.
func PlanSessionObservations(ctx context.Context, request *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if action := request.GetClient().GetMessage(); action != nil {
		return planFrontendMessage(request, state, action)
	}
	if action := request.GetSystem().GetRecordSync(); action != nil {
		return planSessionSync(request, state, action)
	}
	if update := request.GetSystem().GetUpdateEntity(); update != nil {
		kind, _ := recordKind(update.Value)
		if kind == pb.EntityKind_ATIS || kind == pb.EntityKind_CLX_OVERRIDE || kind == pb.EntityKind_SESSION_DEADLINE {
			return planObservationEntity(ctx, request, state, kind)
		}
	}
	if deletion := request.GetSystem().GetRemoveEntity(); deletion != nil && (deletion.Kind == pb.EntityKind_CLX_OVERRIDE || deletion.Kind == pb.EntityKind_SESSION_DEADLINE) {
		return planObservationEntity(ctx, request, state, deletion.Kind)
	}
	return PlanControllerSector(ctx, request, state)
}

func observationSession(request *pb.CommandRequest, state *Aggregate) (*pb.EntitySnapshot, error) {
	if request.GetAggregate().GetSession() == nil || state == nil {
		return nil, fmt.Errorf("session observation requires a session")
	}
	entry := state.Entities[strconv.Itoa(int(request.Aggregate.GetSession().Id))]
	if entry == nil || entry.GetValue().GetSession() == nil || entry.GetValue().GetSession().Tombstoned {
		return nil, fmt.Errorf("session is not active")
	}
	return entry, nil
}

func planFrontendMessage(request *pb.CommandRequest, state *Aggregate, action *pb.MessageAction) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	sessionEntry, err := observationSession(request, state)
	if err != nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, err
	}
	message := action.GetBroadcast()
	if message == nil || request.Actor.GetKind() != pb.Actor_CONTROLLER || request.Actor.GetId() == "" || request.Actor.GetSessionId() != request.Aggregate.GetSession().Id || request.ExpectedEntityRevision != nil ||
		strings.TrimSpace(message.Text) == "" || len(message.Text) > 4096 {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("invalid frontend message")
	}
	session := proto.Clone(sessionEntry.GetValue().GetSession()).(*pb.Session)
	if session.NextMessageId == 0 || session.NextMessageId == math.MaxUint64 {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("message ID exhausted")
	}
	recipients := append([]string(nil), message.Recipients...)
	sort.Strings(recipients)
	for i, recipient := range recipients {
		if recipient == "" || strings.TrimSpace(recipient) != recipient || (i > 0 && recipient == recipients[i-1]) {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("invalid message recipients")
		}
	}
	id := session.NextMessageId
	session.NextMessageId++
	value := &pb.FrontendMessage{Id: id, Sender: request.Actor.Id, Text: message.Text, Broadcast: len(recipients) == 0,
		Recipients: recipients, CreatedAt: timestamppb.New(time.Now().UTC())}
	changes := []*pb.EntityChange{
		{Key: sessionEntry.Key, Revision: sessionEntry.Revision + 1, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: session}}}},
		{Key: strconv.FormatUint(id, 10), Revision: 1, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_FrontendMessage{FrontendMessage: value}}}},
	}
	// Preserve the existing frontend's 100-message history bound.
	old := state.EntitiesByKind(pb.EntityKind_FRONTEND_MESSAGE)
	if len(old) >= 100 {
		sort.Slice(old, func(i, j int) bool {
			return old[i].GetValue().GetFrontendMessage().Id < old[j].GetValue().GetFrontendMessage().Id
		})
		for _, item := range old[:len(old)-99] {
			changes = append(changes, &pb.EntityChange{Key: item.Key, Revision: item.Revision + 1,
				Operation: &pb.EntityChange_Delete{Delete: &pb.DeleteEntity{Kind: pb.EntityKind_FRONTEND_MESSAGE}}})
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		ki, _ := changeKind(changes[i])
		kj, _ := changeKind(changes[j])
		if ki != kj {
			return ki < kj
		}
		return changes[i].Key < changes[j].Key
	})
	return &pb.DomainChange{Changes: changes}, pb.CommandReply_COMMITTED, sessionEntry.Revision, nil
}

func planSessionSync(request *pb.CommandRequest, state *Aggregate, action *pb.RecordSessionSync) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	entry, err := observationSession(request, state)
	if err != nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, err
	}
	sync := action.GetSync()
	master := entry.GetValue().GetSession().GetMaster()
	if request.Actor.GetKind() != pb.Actor_SYSTEM || request.ExpectedEntityRevision != nil || sync == nil || master == nil || state.Owner == nil || master.OwnerEpoch != state.Owner.Epoch ||
		sync.ConnectionId == "" || sync.ConnectionId != master.ConnectionId || sync.MasterEpoch != master.Epoch ||
		sync.CompletedAt == nil || sync.CompletedAt.CheckValid() != nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, entry.Revision, fmt.Errorf("sync does not match the current master")
	}
	session := proto.Clone(entry.GetValue().GetSession()).(*pb.Session)
	if proto.Equal(session.Sync, sync) {
		return &pb.DomainChange{}, pb.CommandReply_COMMITTED, entry.Revision, nil
	}
	session.Sync = proto.Clone(sync).(*pb.SessionSync)
	return &pb.DomainChange{Changes: []*pb.EntityChange{{Key: entry.Key, Revision: entry.Revision + 1,
		Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: session}}}}}}, pb.CommandReply_COMMITTED, entry.Revision, nil
}

func planObservationEntity(ctx context.Context, request *pb.CommandRequest, state *Aggregate, kind pb.EntityKind) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	entry, err := observationSession(request, state)
	if err != nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, err
	}
	if request.Actor.GetKind() != pb.Actor_SYSTEM {
		return nil, pb.CommandReply_UNAUTHORIZED, 0, fmt.Errorf("system observation required")
	}
	change, status, current, err := PlanSystemEntity(ctx, request, state)
	if err != nil {
		return change, status, current, err
	}
	value := change.Changes[0].GetUpsert()
	switch kind {
	case pb.EntityKind_ATIS:
		atis := value.GetAtis()
		if atis == nil || atis.Airport != entry.GetValue().GetSession().Airport || atis.ObservedAt == nil || atis.ObservedAt.CheckValid() != nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid session ATIS")
		}
	case pb.EntityKind_CLX_OVERRIDE:
		if override := value.GetClxOverride(); override != nil && (override.Callsign == "" || override.Key == "" || override.Actor == "" || override.CreatedAt == nil || override.CreatedAt.CheckValid() != nil) {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid CLX override")
		}
	case pb.EntityKind_SESSION_DEADLINE:
		if deadline := value.GetSessionDeadline(); deadline != nil && (deadline.Id == "" || deadline.SourceRevision == 0 || deadline.DueAt == nil || deadline.DueAt.CheckValid() != nil || !validSessionDeadlineKind(deadline.Kind)) {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid session deadline")
		}
	}
	return change, status, current, nil
}

func validSessionDeadlineKind(kind string) bool {
	switch kind {
	case "aircraft-disconnect", "controller-offline", "session-disconnect", "session-cleanup", "session-update", "strip-auto-hide", "pdc-response":
		return true
	}
	return false
}
