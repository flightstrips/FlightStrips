package cluster

import (
	"context"
	"sync"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func amanFixture(t *testing.T) (*memoryStore, AmanAdapter) {
	t.Helper()
	store := &memoryStore{}
	ref := airportRef("EKCH")
	owner := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "node-a"}, Fact: &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
	data, err := proto.Marshal(owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Publish(context.Background(), "fs.v1.state.airport.EKCH", 0, data); err != nil {
		t.Fatal(err)
	}
	return store, AmanAdapter{Writer: Writer{Store: store, NodeID: "node-a"}}
}

func amanTransition(id string, old, next uint64, flight string) AmanTransition {
	ref := airportRef("EKCH")
	now := timestamppb.New(time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC))
	airport := &pb.AmanAirport{Airport: "EKCH", Revision: next, PolicyVersion: "aman-cph-v3", EffectiveMode: "shadow", GeneratedAt: now}
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "aman-test"}, ExpectedEntityRevision: &old, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "EKCH", Value: &pb.EntityRecord{Value: &pb.EntityRecord_AmanAirport{AmanAirport: airport}}}}}}}
	result := AmanTransition{Request: request, Airport: airport, Flights: []*pb.AmanFlight{{Callsign: flight, State: "planned", SequenceDisposition: "active", DataStatus: "fresh", FreezeReason: "none", UpdatedAt: now}}, Coordinations: []*pb.AmanCoordination{{Id: uuid.NewString(), Callsign: flight, RecipientController: "123456", RecipientStatus: "online", State: "pending", Request: &pb.AmanCoordination_RouteDirect{RouteDirect: &pb.AmanRouteDirect{Route: proto.String("TESPI TNO")}}, CreatedAt: now, UpdatedAt: now}}, Audits: []*pb.AmanAudit{{Id: uuid.NewString(), AirportRevision: next, CreatedAt: now, Actor: request.Actor, Fact: &pb.AmanAudit_Command{Command: &pb.AmanCommandAudit{CommandId: id, CommandKind: "reconcile", Outcome: "accepted"}}}}, Validations: []*pb.AmanValidation{{Id: uuid.NewString(), Callsign: flight, Rule: "route", Result: "valid", ObservedAt: now}}, Observations: []*pb.VatsimObservation{{ProviderId: id, Callsign: flight, Digest: "sha256:fixture", ObservedAt: now}}}
	return result
}

func TestAmanTransitionReplayAndDuplicateCommand(t *testing.T) {
	store, adapter := amanFixture(t)
	transition := amanTransition(uuid.NewString(), 0, 1, "SAS123")
	first := adapter.Commit(context.Background(), transition)
	if first.Status != pb.CommandReply_COMMITTED || first.Outcome.Status != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("commit: %v", first)
	}
	again := adapter.Commit(context.Background(), transition)
	if again.Status != pb.CommandReply_COMMITTED || again.StreamSequence == nil || *again.StreamSequence != *first.StreamSequence || store.commits != 2 {
		t.Fatalf("duplicate command added revision: %v", again)
	}
	one, err := adapter.Read(context.Background(), "EKCH")
	if err != nil {
		t.Fatal(err)
	}
	if one.Airport.Revision != 1 || len(one.Flights) != 1 || len(one.Coordinations) != 1 || len(one.Audits) != 1 || len(one.Validations) != 1 || len(one.Observations) != 1 {
		t.Fatalf("incomplete board: %+v", one)
	}
	// Independent backends reconstruct the same typed board without any prior
	// process memory or PostgreSQL state.
	for i := 0; i < 2; i++ {
		restarted := AmanAdapter{Writer: Writer{Store: store, NodeID: "node-a"}}
		board, err := restarted.Read(context.Background(), "EKCH")
		if err != nil || !proto.Equal(board.Airport, one.Airport) || !proto.Equal(board.Flights[0], one.Flights[0]) || !proto.Equal(board.Coordinations[0], one.Coordinations[0]) || !proto.Equal(board.Audits[0], one.Audits[0]) {
			t.Fatalf("replay backend %d: %+v %v", i, board, err)
		}
	}
}

func TestAmanRepeatedVatsimObservationAddsNoRevision(t *testing.T) {
	store, adapter := amanFixture(t)
	transition := amanTransition(uuid.NewString(), 0, 1, "SAS123")
	transition.Observations[0].ProviderId = "vatsim-snapshot-123"
	request, err := AmanVatsimObservationRequest("EKCH", transition.Observations[0], 0)
	if err != nil {
		t.Fatal(err)
	}
	transition.Request = request
	transition.Audits[0].Actor = request.Actor
	transition.Audits[0].GetCommand().CommandId = request.CommandId
	first := adapter.Commit(context.Background(), transition)
	if first.Outcome == nil || first.Outcome.Status != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("observation: %v", first)
	}
	second := adapter.Commit(context.Background(), transition)
	if second.GetStreamSequence() != first.GetStreamSequence() || store.commits != 2 {
		t.Fatalf("repeated observation advanced revision: %v", second)
	}
	changed := proto.Clone(transition.Request).(*pb.CommandRequest)
	changed.GetSystem().GetUpdateEntity().Value.GetVatsimObservation().Digest = "different"
	if reply := adapter.Commit(context.Background(), AmanTransition{Request: changed, Airport: transition.Airport}); reply.Status != pb.CommandReply_INVALID_ARGUMENT || store.commits != 2 {
		t.Fatalf("changed observation identity accepted: %v", reply)
	}
}

func TestAmanCompetingRevisionHasOneWinningBoard(t *testing.T) {
	_, adapter := amanFixture(t)
	first, second := amanTransition(uuid.NewString(), 0, 1, "SAS123"), amanTransition(uuid.NewString(), 0, 1, "SAS456")
	var wg sync.WaitGroup
	wg.Add(2)
	replies := make([]*pb.CommandReply, 2)
	go func() { defer wg.Done(); replies[0] = adapter.Commit(context.Background(), first) }()
	go func() { defer wg.Done(); replies[1] = adapter.Commit(context.Background(), second) }()
	wg.Wait()
	var wins, conflicts int
	for _, reply := range replies {
		if reply.Status != pb.CommandReply_COMMITTED {
			t.Fatalf("transport result: %v", reply)
		}
		if reply.Outcome.Status == pb.CommandOutcome_SUCCEEDED {
			wins++
		} else if reply.Outcome.ReasonCode == pb.CommandReply_REVISION_CONFLICT.String() {
			conflicts++
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("revision race: %v", replies)
	}
	board, err := adapter.Read(context.Background(), "EKCH")
	if err != nil || len(board.Flights) != 1 || board.Airport.Revision != 1 {
		t.Fatalf("board after race: %+v %v", board, err)
	}
}

type amanTestDestination struct{ writer Writer }

func (d amanTestDestination) Execute(ctx context.Context, request *pb.CommandRequest) *pb.CommandReply {
	return d.writer.Execute(ctx, request)
}
func (d amanTestDestination) Read(ctx context.Context, ref *pb.AggregateRef) (*Aggregate, error) {
	subject, err := Subject(ref)
	if err != nil {
		return nil, err
	}
	return d.writer.load(ctx, subject, ref)
}

func TestAmanIntentResumesAfterDestinationCommit(t *testing.T) {
	store, adapter := amanFixture(t)
	session := sessionRef(1)
	owner := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: session, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "node-a"}, Fact: &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
	data, _ := proto.Marshal(owner)
	if _, err := store.Publish(context.Background(), "fs.v1.state.session.1", 0, data); err != nil {
		t.Fatal(err)
	}
	workflowID := uuid.NewString()
	stepID, _ := AmanIntentID(workflowID, "session-update")
	transition := amanTransition(uuid.NewString(), 0, 1, "SAS123")
	sourceRevision := uint64(1)
	transition.Workflows = []*pb.WorkflowRecord{{WorkflowId: workflowID, Source: airportRef("EKCH"), Destination: session, Step: "session-update", DerivedCommandId: stepID, Status: pb.WorkflowRecord_PENDING, SourceRevision: &sourceRevision}}
	if reply := adapter.Commit(context.Background(), transition); reply.Outcome == nil || reply.Outcome.Status != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("intent: %v", reply)
	}
	zero := uint64(0)
	seed := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: session, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "1", Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: 1, Airport: "EKCH", Name: "LIVE", NextStripId: 1}}}}}}}}
	if reply := (Writer{Store: store, NodeID: "node-a", Plan: PlanSystemEntity}).Execute(context.Background(), seed); reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("session seed: %v", reply)
	}
	destination := amanTestDestination{Writer{Store: store, NodeID: "node-a", Plan: PlanStrip}}
	step := func(workflow *pb.WorkflowRecord) (*pb.CommandRequest, error) {
		return &pb.CommandRequest{ProtocolRevision: 1, CommandId: workflow.DerivedCommandId, Aggregate: session, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "aman-intent"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "SAS123", Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Callsign: "SAS123", Bay: "CLEARED", Route: "TESPI TNO"}}}}}}}}, nil
	}
	// The destination commits, then the initiator dies before recording completion.
	stepRequest, _ := step(transition.Workflows[0])
	if reply := destination.Execute(context.Background(), stepRequest); reply.Outcome == nil || reply.Outcome.Status != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("destination: %v", reply)
	}
	before := store.commits
	runner := AmanIntentRunner{AirportWriter: Writer{Store: store, NodeID: "node-a"}, Destination: destination, Step: step}
	if err := runner.Resume(context.Background(), "EKCH"); err != nil {
		t.Fatal(err)
	}
	if store.commits != before+1 {
		t.Fatalf("resumed step duplicated destination write: %d -> %d", before, store.commits)
	}
	sessionState, err := destination.Read(context.Background(), session)
	if err != nil || sessionState.Indexes[pb.EntityKind_STRIP]["SAS123"].Revision != 1 {
		t.Fatalf("resumed strip mutation: %v %v", sessionState, err)
	}
	if err := runner.Resume(context.Background(), "EKCH"); err != nil {
		t.Fatal(err)
	}
	if store.commits != before+1 {
		t.Fatalf("completed intent replayed")
	}
	state, err := runner.AirportWriter.load(context.Background(), "fs.v1.state.airport.EKCH", airportRef("EKCH"))
	if err != nil || state.Workflows[workflowID].Status != pb.WorkflowRecord_COMPLETED {
		t.Fatalf("workflow: %v %v", state.Workflows[workflowID], err)
	}
}

func TestAmanIntentSupersedesStaleSourceWithoutDestinationWrite(t *testing.T) {
	store, adapter := amanFixture(t)
	workflowID := uuid.NewString()
	stepID, _ := AmanIntentID(workflowID, "session-update")
	transition := amanTransition(uuid.NewString(), 0, 1, "SAS123")
	sourceRevision := uint64(1)
	transition.Workflows = []*pb.WorkflowRecord{{WorkflowId: workflowID, Source: airportRef("EKCH"), Destination: sessionRef(1), Step: "session-update", DerivedCommandId: stepID, Status: pb.WorkflowRecord_PENDING, SourceRevision: &sourceRevision}}
	if got := adapter.Commit(context.Background(), transition); got.Outcome == nil || got.Outcome.Status != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("first: %v", got)
	}
	newer := amanTransition(uuid.NewString(), 1, 2, "SAS123")
	if got := adapter.Commit(context.Background(), newer); got.Outcome == nil || got.Outcome.Status != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("newer: %v", got)
	}
	called := false
	runner := AmanIntentRunner{AirportWriter: Writer{Store: store, NodeID: "node-a"}, Step: func(*pb.WorkflowRecord) (*pb.CommandRequest, error) { called = true; return nil, nil }}
	if err := runner.Resume(context.Background(), "EKCH"); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("stale source dispatched to session")
	}
	state, err := runner.AirportWriter.load(context.Background(), "fs.v1.state.airport.EKCH", airportRef("EKCH"))
	if err != nil || state.Workflows[workflowID].Status != pb.WorkflowRecord_SUPERSEDED {
		t.Fatalf("stale workflow: %v %v", state.Workflows[workflowID], err)
	}
}
