package cluster

import (
	"context"
	"fmt"
	"sync"
	"testing"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

func candidateFixture(t *testing.T) (*memoryStore, PdcTactical) {
	t.Helper()
	store, strips := stripFixture(t)
	ctx := context.Background()
	if _, err := (ControllerSector{Store: strips.Store}).PutController(ctx, 1, &pb.Controller{Cid: "cid-1", Callsign: "EKCH_A_TWR", Position: "118.100"}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := (ControllerSector{Store: strips.Store}).PutController(ctx, 1, &pb.Controller{Cid: "cid-2", Callsign: "EKCH_A_GND", Position: "121.900"}, 0); err != nil {
		t.Fatal(err)
	}
	return store, PdcTactical{Store: strips.Store}
}

func candidateRequest(actor *pb.Actor, expected uint64, action *pb.ClientCommand) *pb.CommandRequest {
	return &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(1), Actor: actor, ExpectedEntityRevision: &expected, Command: &pb.CommandRequest_Client{Client: action}}
}

func tacticalActor(cid string) *pb.Actor {
	session := int32(1)
	return &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: cid, SessionId: &session}
}

func tacticalCreate(bay, kind, label string) *pb.ClientCommand {
	return &pb.ClientCommand{Action: &pb.ClientCommand_Tactical{Tactical: &pb.TacticalAction{Change: &pb.TacticalAction_Create{Create: &pb.CreateTactical{Bay: bay, Kind: kind, Label: label}}}}}
}

func TestCandidateTacticalReplayAndConcurrentCommandID(t *testing.T) {
	store, adapter := candidateFixture(t)
	ctx := context.Background()
	req := candidateRequest(tacticalActor("cid-1"), 0, tacticalCreate("TAXI", "CROSSING", ""))
	var replies [2]*pb.CommandReply
	var errs [2]error
	var wg sync.WaitGroup
	for i := range replies {
		wg.Add(1)
		go func(i int) { defer wg.Done(); replies[i], errs[i] = adapter.Execute(ctx, req) }(i)
	}
	wg.Wait()
	for i := range replies {
		if errs[i] != nil || replies[i].Status != pb.CommandReply_COMMITTED {
			t.Fatalf("concurrent create %d: %v %v", i, replies[i], errs[i])
		}
	}
	if replies[0].GetStreamSequence() != replies[1].GetStreamSequence() {
		t.Fatal("one command ID committed twice")
	}
	created, revision, err := adapter.Tactical(ctx, 1, 1)
	if err != nil || created.Id != 1 || created.Sequence != 1000 || revision != 1 {
		t.Fatalf("created tactical strip: %v %d %v", created, revision, err)
	}
	move := candidateRequest(tacticalActor("cid-1"), revision, &pb.ClientCommand{Action: &pb.ClientCommand_Tactical{Tactical: &pb.TacticalAction{StripId: 1, Change: &pb.TacticalAction_Move{Move: &pb.MoveTactical{Bay: proto.String("PUSH")}}}}})
	if reply, err := adapter.Execute(ctx, move); err != nil || reply.Status != pb.CommandReply_COMMITTED {
		t.Fatalf("move: %v %v", reply, err)
	}
	restarted := PdcTactical{Store: LocalLifecycleStore{Writer: Writer{Store: store}}}
	after, _, err := restarted.Tactical(ctx, 1, 1)
	if err != nil || after.Bay != "PUSH" || after.Sequence != 1000 {
		t.Fatalf("replay lost tactical movement: %v %v", after, err)
	}
	if reply, err := adapter.Execute(ctx, move); err != nil || reply.GetStreamSequence() == 0 {
		t.Fatalf("retry after restart: %v %v", reply, err)
	}
}

func TestCandidateTacticalAuthorizationAndTimer(t *testing.T) {
	store, adapter := candidateFixture(t)
	ctx := context.Background()
	req := candidateRequest(tacticalActor("cid-1"), 0, tacticalCreate("DEPART", "START", ""))
	if reply, err := adapter.Execute(ctx, req); err != nil {
		t.Fatalf("create: %v %v", reply, err)
	}
	mark := candidateRequest(tacticalActor("cid-2"), 1, &pb.ClientCommand{Action: &pb.ClientCommand_Tactical{Tactical: &pb.TacticalAction{StripId: 1, Change: &pb.TacticalAction_Mark{Mark: &pb.MarkTactical{Marked: true}}}}})
	if reply, err := adapter.Execute(ctx, mark); err == nil || reply.Status != pb.CommandReply_UNAUTHORIZED {
		t.Fatalf("non-owner mark: %v %v", reply, err)
	}
	timer := candidateRequest(tacticalActor("cid-1"), 1, &pb.ClientCommand{Action: &pb.ClientCommand_Tactical{Tactical: &pb.TacticalAction{StripId: 1, Change: &pb.TacticalAction_StartTimer{StartTimer: &pb.StartTacticalTimer{}}}}})
	if reply, err := adapter.Execute(ctx, timer); err != nil {
		t.Fatalf("start timer: %v %v", reply, err)
	}
	restarted := PdcTactical{Store: LocalLifecycleStore{Writer: Writer{Store: store}}}
	tactical, _, err := restarted.Tactical(ctx, 1, 1)
	if err != nil || tactical.TimerStartedAt == nil {
		t.Fatalf("timer was not durable: %v %v", tactical, err)
	}
}

func TestCandidatePdcPendingRequestReplaysOnce(t *testing.T) {
	store, adapter := candidateFixture(t)
	ctx := context.Background()
	strips := StripState{Store: adapter.Store}
	if _, err := strips.Put(ctx, 1, &pb.Strip{Callsign: "SAS101", Bay: "NOT_CLEARED", Departure: "EKCH"}, 0); err != nil {
		t.Fatal(err)
	}
	session := int32(1)
	pilot := &pb.Actor{Kind: pb.Actor_PILOT, Id: "SAS101", SessionId: &session}
	req := candidateRequest(pilot, 0, &pb.ClientCommand{Action: &pb.ClientCommand_Pdc{Pdc: &pb.PdcAction{Callsign: "SAS101", Change: &pb.PdcAction_Issue{Issue: &pb.IssuePdc{RequestRemarks: "request via web", RequestChannel: "WEB"}}}}})
	first, err := adapter.Execute(ctx, req)
	if err != nil || first.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("request: %v %v", first, err)
	}
	restarted := PdcTactical{Store: LocalLifecycleStore{Writer: Writer{Store: store}}}
	pdc, revision, err := restarted.Pdc(ctx, 1, "SAS101")
	if err != nil || revision != 1 || pdc.State != "REQUESTED" || pdc.RequestedAt == nil || pdc.RequestRemarks != "request via web" || pdc.RequestChannel != "WEB" {
		t.Fatalf("pending PDC replay: %v %d %v", pdc, revision, err)
	}
	before := store.commits
	second, err := adapter.Execute(ctx, req)
	if err != nil || store.commits != before || second.GetStreamSequence() != first.GetStreamSequence() {
		t.Fatalf("PDC retry duplicated result: %v %v", second, err)
	}
	conflict := candidateRequest(pilot, 0, req.GetClient())
	if reply, err := adapter.Execute(ctx, conflict); err == nil || reply.GetOutcome().GetReasonCode() != "REVISION_CONFLICT" {
		t.Fatalf("concurrent request should conflict: %v %v", reply, err)
	}
	if _, err := strips.Delete(ctx, 1, "SAS101", 1); err != nil {
		t.Fatalf("strip and PDC cascade: %v", err)
	}
	state, err := (Writer{Store: store}).load(ctx, fmt.Sprintf("fs.v1.state.session.%d", session), sessionRef(session))
	if err != nil || len(state.Indexes[pb.EntityKind_PDC_SEQUENCE]) != 0 {
		t.Fatalf("PDC orphaned after strip delete: %v %v", state, err)
	}
}

func TestCandidatePdcIssuePersistsDeadlineAndTypedEffect(t *testing.T) {
	store, adapter := candidateFixture(t)
	ctx := context.Background()
	strips := StripState{Store: adapter.Store}
	if _, err := strips.Put(ctx, 1, &pb.Strip{Callsign: "SAS101", Bay: "NOT_CLEARED", Departure: "EKCH"}, 0); err != nil {
		t.Fatal(err)
	}
	sessionID := int32(1)
	pilot := &pb.Actor{Kind: pb.Actor_PILOT, Id: "SAS101", SessionId: &sessionID}
	requested := candidateRequest(pilot, 0, &pb.ClientCommand{Action: &pb.ClientCommand_Pdc{Pdc: &pb.PdcAction{Callsign: "SAS101", Change: &pb.PdcAction_Issue{Issue: &pb.IssuePdc{}}}}})
	if reply, err := adapter.Execute(ctx, requested); err != nil {
		t.Fatalf("request: %v %v", reply, err)
	}
	reader := Writer{Store: store}
	state, err := reader.load(ctx, "fs.v1.state.session.1", sessionRef(1))
	if err != nil {
		t.Fatal(err)
	}
	session := proto.Clone(state.Indexes[pb.EntityKind_SESSION]["1"].GetValue().GetSession()).(*pb.Session)
	session.Master = &pb.MasterTerm{Cid: "cid-1", ConnectionId: "connection-1", Epoch: 1, OwnerEpoch: 1}
	expected := state.Indexes[pb.EntityKind_SESSION]["1"].Revision
	masterUpdate := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(1), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test-master"}, ExpectedEntityRevision: &expected, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "1", Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: session}}}}}}}
	if reply := (Writer{Store: store, NodeID: "node-a", Plan: PlanSystemEntity}).Execute(ctx, masterUpdate); reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("master seed: %v", reply)
	}
	issue := candidateRequest(tacticalActor("cid-1"), 1, &pb.ClientCommand{Action: &pb.ClientCommand_Pdc{Pdc: &pb.PdcAction{Callsign: "SAS101", Change: &pb.PdcAction_Issue{Issue: &pb.IssuePdc{Clearance: "CLEARED TO EKCH"}}}}})
	first, err := adapter.Execute(ctx, issue)
	if err != nil || first.Status != pb.CommandReply_PENDING || first.GetOutcome().GetStatus() != pb.CommandOutcome_ACCEPTED {
		t.Fatalf("issue: %v %v", first, err)
	}
	restarted := PdcTactical{Store: LocalLifecycleStore{Writer: reader}}
	pdc, _, err := restarted.Pdc(ctx, 1, "SAS101")
	if err != nil || pdc.State != "CLEARED" || pdc.Sequence != 1 || pdc.Deadline == nil || pdc.IssuedAt == nil || pdc.Sent {
		t.Fatalf("issued PDC replay: %v %v", pdc, err)
	}
	state, err = reader.load(ctx, "fs.v1.state.session.1", sessionRef(1))
	if err != nil {
		t.Fatal(err)
	}
	deadline := state.Indexes[pb.EntityKind_SESSION_DEADLINE]["pdc.SAS101"].GetValue().GetSessionDeadline()
	effect := state.Effects[issue.CommandId]
	if deadline == nil || !proto.Equal(deadline.DueAt, pdc.Deadline) || effect == nil || effect.GetPdc().GetAction() != "ISSUE" || effect.TargetCid != "cid-1" || effect.Status != pb.EffectRecord_WAITING {
		t.Fatalf("missing durable deadline or effect: %v %v", deadline, effect)
	}
	before := store.commits
	second, err := adapter.Execute(ctx, issue)
	if err != nil || store.commits != before || second.GetStreamSequence() != first.GetStreamSequence() {
		t.Fatalf("issue retried external request: %v %v", second, err)
	}
}

func TestCandidateGenericWriteCannotBypassTacticalAllocator(t *testing.T) {
	store, _ := candidateFixture(t)
	zero := uint64(0)
	req := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(1), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "1", Value: &pb.EntityRecord{Value: &pb.EntityRecord_TacticalStrip{TacticalStrip: &pb.TacticalStrip{Id: 1, Revision: 1, Bay: "TAXI", Kind: "CROSSING", OwnerCid: "cid-1", Sequence: 1000}}}}}}}}
	reply := (Writer{Store: store, NodeID: "node-a", Plan: PlanSystemEntity}).Execute(context.Background(), req)
	if reply.Status != pb.CommandReply_INVALID_ARGUMENT {
		t.Fatalf("generic tactical write bypassed ID allocator: %v", reply)
	}
}

func TestCandidateTacticalMoveSharesFlightStripBayOrder(t *testing.T) {
	_, adapter := candidateFixture(t)
	ctx := context.Background()
	strips := StripState{Store: adapter.Store}
	if _, err := strips.Put(ctx, 1, &pb.Strip{Callsign: "SAS101", Bay: "TAXI", Departure: "EKCH"}, 0); err != nil {
		t.Fatal(err)
	}
	create := candidateRequest(tacticalActor("cid-1"), 0, tacticalCreate("TAXI", "CROSSING", ""))
	if reply, err := adapter.Execute(ctx, create); err != nil {
		t.Fatalf("create: %v %v", reply, err)
	}
	before, _, err := adapter.Tactical(ctx, 1, 1)
	if err != nil || before.Sequence != 2000 {
		t.Fatalf("tactical append ignored flight strip: %v %v", before, err)
	}
	move := candidateRequest(tacticalActor("cid-1"), 1, &pb.ClientCommand{Action: &pb.ClientCommand_Tactical{Tactical: &pb.TacticalAction{StripId: 1, Change: &pb.TacticalAction_Move{Move: &pb.MoveTactical{}}}}})
	if reply, err := adapter.Execute(ctx, move); err != nil {
		t.Fatalf("move: %v %v", reply, err)
	}
	after, _, err := adapter.Tactical(ctx, 1, 1)
	flight, _, flightErr := strips.ByCallsign(ctx, 1, "SAS101")
	if err != nil || flightErr != nil || after.Sequence != 1000 || flight.Sequence != 2000 {
		t.Fatalf("unified bay order: tactical=%v flight=%v errors=%v %v", after, flight, err, flightErr)
	}
}
