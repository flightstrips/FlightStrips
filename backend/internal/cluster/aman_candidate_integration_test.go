package cluster

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestAmanCandidateTwoReplicaObservationAndSupersessionNATS(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	defer cancel()
	port := 4222
	if raw := os.Getenv("NATS_TEST_PORT_BASE"); raw != "" {
		var err error
		port, err = strconv.Atoi(raw)
		if err != nil || port < 1024 || port > 65533 {
			t.Fatalf("invalid NATS_TEST_PORT_BASE %q", raw)
		}
	}
	url := func(user string, offset int) string {
		return fmt.Sprintf("nats://%s:%s-local-only@127.0.0.1:%d", user, user, port+offset)
	}
	cfg := natsresources.Config{URLs: []string{url("bootstrap", 0), url("bootstrap", 1), url("bootstrap", 2)}, ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
	admin, err := natsresources.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if err := natscluster.WaitForQuorum(ctx, admin); err != nil {
		t.Fatal(err)
	}
	if err := natsresources.Bootstrap(ctx, admin, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.URLs = []string{url("backend", 0), url("backend", 1), url("backend", 2)}
	seed := uuid.New()
	icao := string([]byte{'A' + seed[0]%26, 'A' + seed[1]%26, 'A' + seed[2]%26, 'A' + seed[3]%26})
	sessionID := int32(seed[4])<<16 | int32(seed[5])<<8 | int32(seed[6])
	sessionID += 1000
	global, airport, session := globalRef(), airportRef(icao), sessionRef(sessionID)
	type replica struct {
		nc     *nats.Conn
		owner  *OwnerRuntime
		source NavigationWeather
		state  AmanAdapter
		stop   context.CancelFunc
	}
	var nodes [2]replica
	for i := range nodes {
		nc, err := natsresources.Connect(cfg)
		if err != nil {
			t.Fatal(err)
		}
		projection := startProjection(t, ctx, nc, cfg)
		store := NATSStore{JS: projection.JS}
		owner, err := NewOwnerRuntime(nc, projection, store)
		if err != nil {
			t.Fatal(err)
		}
		for _, ref := range []*pb.AggregateRef{global, airport, session} {
			if err := owner.Track(ref); err != nil {
				t.Fatal(err)
			}
		}
		objects, err := projection.JS.ObjectStore("FS_OBJECTS")
		if err != nil {
			t.Fatal(err)
		}
		runCtx, stop := context.WithCancel(ctx)
		writer := Writer{Store: store, NodeID: owner.NodeID, Projection: projection, Lease: owner}
		nodes[i] = replica{nc, owner, NavigationWeather{Writer: writer, Objects: NATSObjects{Store: objects}}, AmanAdapter{Writer: writer}, stop}
		go func() { _ = owner.Run(runCtx) }()
	}
	defer func() {
		for _, node := range nodes {
			node.stop()
			node.nc.Close()
		}
	}()
	waitOwner := func(ref *pb.AggregateRef, exclude string) int {
		t.Helper()
		for ctx.Err() == nil {
			for i := range nodes {
				if nodes[i].owner.NodeID != exclude && nodes[i].owner.CanWrite(ref) {
					return i
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("AMAN owner handoff timed out")
		return -1
	}
	globalOwner := waitOwner(global, "")
	first := waitOwner(airport, "")
	second := 1 - first
	at := timestamppb.Now()
	page := &pb.ProviderPage{Provider: "vatsim", Resource: "network-data/v3", Parsed: &pb.ProviderPage_Vatsim{Vatsim: &pb.VatsimPage{SnapshotAt: at,
		Flights: []*pb.VatsimFlight{{Cid: "12345", Callsign: "SAS123", State: "online", FlightPlan: &pb.VatsimFlightPlan{Origin: "EDDF", Destination: icao, Revision: 3}}}}}}
	name, sha, err := nodes[globalOwner].source.PublishProvider(page)
	if err != nil {
		t.Fatal(err)
	}
	if reply, err := nodes[globalOwner].source.PutCheckpointFor(ctx, global, uuid.NewString(), &pb.ProviderCheckpoint{Provider: "vatsim", Resource: "network-data/v3", ObjectName: name, Sha256: sha}); err != nil || reply.Status != pb.CommandReply_COMMITTED {
		t.Fatalf("source checkpoint: %v %v", reply, err)
	}
	workflowID := uuid.NewString()
	stepID, _ := AmanIntentID(workflowID, "session-update")
	evaluate := func(_ context.Context, board AmanBoard, flight *pb.VatsimFlight, _ *pb.VatsimObservation, _ *pb.CommandRequest) (AmanTransition, error) {
		sourceRevision := uint64(1)
		return AmanTransition{Airport: &pb.AmanAirport{Airport: icao, Revision: 1, PolicyVersion: "v1", GeneratedAt: at},
			Flights:   []*pb.AmanFlight{{Callsign: flight.Callsign, State: "planned", FreezeReason: "none", UpdatedAt: at, LatestObservation: &pb.AmanFlightObservation{Callsign: flight.Callsign, Provider: "vatsim", SourceStatus: "fresh", ReconciledAt: at}}},
			Workflows: []*pb.WorkflowRecord{{WorkflowId: workflowID, Source: airport, Destination: session, Step: "session-update", DerivedCommandId: stepID, Status: pb.WorkflowRecord_PENDING, SourceRevision: &sourceRevision}}}, nil
	}
	if reply := (AmanCandidateWorker{Source: nodes[second].source, State: nodes[second].state}).ObserveVatsim(ctx, icao, "SAS123", evaluate); reply.GetOutcome().GetStatus() == pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("nonowner committed observation: %v", reply)
	}
	firstReply := (AmanCandidateWorker{Source: nodes[first].source, State: nodes[first].state}).ObserveVatsim(ctx, icao, "SAS123", evaluate)
	if firstReply.Status != pb.CommandReply_COMMITTED || firstReply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("owner observation: %v", firstReply)
	}
	sessionOwner := waitOwner(session, "")
	zero := uint64(0)
	sessionSeed := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: session, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: fmt.Sprint(sessionID), Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: sessionID, Airport: icao, Name: "LIVE", NextStripId: 1}}}}}}}}
	sessionWriter := nodes[sessionOwner].state.Writer
	sessionWriter.Plan = PlanSystemEntity
	if reply := sessionWriter.Execute(ctx, sessionSeed); reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("session seed: %v", reply)
	}
	step := func(workflow *pb.WorkflowRecord) (*pb.CommandRequest, error) {
		return &pb.CommandRequest{ProtocolRevision: 1, CommandId: workflow.DerivedCommandId, Aggregate: session, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "aman-intent"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "SAS123", Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Callsign: "SAS123", Bay: "CLEARED", Route: "TESPI TNO"}}}}}}}}, nil
	}
	destinationWriter := nodes[sessionOwner].state.Writer
	destinationWriter.Plan = PlanStrip
	stepRequest, _ := step(&pb.WorkflowRecord{DerivedCommandId: stepID})
	if reply := destinationWriter.Execute(ctx, stepRequest); reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("destination before owner death: %v", reply)
	}
	nodes[first].stop()
	nodes[first].nc.Close()
	if survivor := waitOwner(airport, nodes[first].owner.NodeID); survivor != second {
		t.Fatal("wrong takeover owner")
	}
	replay := (AmanCandidateWorker{Source: nodes[second].source, State: nodes[second].state}).ObserveVatsim(ctx, icao, "SAS123", func(context.Context, AmanBoard, *pb.VatsimFlight, *pb.VatsimObservation, *pb.CommandRequest) (AmanTransition, error) {
		t.Fatal("replayed observation evaluated")
		return AmanTransition{}, nil
	})
	if replay.GetStreamSequence() != firstReply.GetStreamSequence() {
		t.Fatalf("observation replay duplicated: %v", replay)
	}
	takeoverDestination := nodes[second].state.Writer
	takeoverDestination.Plan = PlanStrip
	runner := AmanIntentRunner{AirportWriter: nodes[second].state.Writer, Destination: amanTestDestination{takeoverDestination}, Step: step}
	if err := runner.Resume(ctx, icao); err != nil {
		t.Fatal(err)
	}
	stateBeforeTick, err := nodes[second].state.Writer.load(ctx, mustSubject(t, airport), airport)
	if err != nil || stateBeforeTick.Workflows[workflowID].Status != pb.WorkflowRecord_COMPLETED {
		t.Fatalf("destination completion after takeover: %v %v", stateBeforeTick.Workflows[workflowID], err)
	}
	sessionState, err := takeoverDestination.load(ctx, mustSubject(t, session), session)
	if err != nil || sessionState.Indexes[pb.EntityKind_STRIP]["SAS123"].Revision != 1 {
		t.Fatalf("destination replay duplicated strip: %v %v", sessionState, err)
	}
	supersedeID := uuid.NewString()
	supersedeStep, _ := AmanIntentID(supersedeID, "session-update")
	tick := (AmanCandidateWorker{State: nodes[second].state}).Reconcile(ctx, icao, at.AsTime().Add(time.Minute), func(_ context.Context, board AmanBoard, _ time.Time) (AmanTransition, error) {
		sourceRevision := board.Airport.Revision + 1
		return AmanTransition{Airport: &pb.AmanAirport{Airport: icao, Revision: sourceRevision, PolicyVersion: "v1", GeneratedAt: at}, Flights: board.Flights,
			Workflows: []*pb.WorkflowRecord{{WorkflowId: supersedeID, Source: airport, Destination: session, Step: "session-update", DerivedCommandId: supersedeStep, Status: pb.WorkflowRecord_PENDING, SourceRevision: &sourceRevision}}}, nil
	})
	if tick.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("newer airport revision: %v", tick)
	}
	newer := (AmanCandidateWorker{State: nodes[second].state}).Reconcile(ctx, icao, at.AsTime().Add(2*time.Minute), func(_ context.Context, board AmanBoard, _ time.Time) (AmanTransition, error) {
		return AmanTransition{Airport: &pb.AmanAirport{Airport: icao, Revision: board.Airport.Revision + 1, PolicyVersion: "v1", GeneratedAt: at}, Flights: board.Flights}, nil
	})
	if newer.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("newer airport revision: %v", newer)
	}
	called := false
	runner = AmanIntentRunner{AirportWriter: nodes[second].state.Writer, Step: func(*pb.WorkflowRecord) (*pb.CommandRequest, error) { called = true; return nil, nil }}
	if err := runner.Resume(ctx, icao); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("superseded airport revision dispatched session step")
	}
	board, err := nodes[second].state.Read(ctx, icao)
	if err != nil || board.Airport.Revision != 3 || len(board.Observations) != 1 {
		t.Fatalf("takeover board: %+v %v", board, err)
	}
	subject, _ := Subject(airport)
	state, err := nodes[second].state.Writer.load(ctx, subject, airport)
	if err != nil || state.Workflows[supersedeID].Status != pb.WorkflowRecord_SUPERSEDED {
		t.Fatalf("workflow supersession: %v %v", state.Workflows[supersedeID], err)
	}
	globalTakeover := waitOwner(global, "")
	empty := &pb.ProviderPage{Provider: "vatsim", Resource: "network-data/v3", Parsed: &pb.ProviderPage_Vatsim{Vatsim: &pb.VatsimPage{SnapshotAt: timestamppb.New(at.AsTime().Add(3 * time.Minute))}}}
	name, sha, err = nodes[globalTakeover].source.PublishProvider(empty)
	if err != nil {
		t.Fatal(err)
	}
	if reply, err := nodes[globalTakeover].source.PutCheckpointFor(ctx, global, uuid.NewString(), &pb.ProviderCheckpoint{Provider: "vatsim", Resource: "network-data/v3", ObjectName: name, Sha256: sha}); err != nil || reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("empty generation: %v %v", reply, err)
	}
	missingCalls := 0
	reconciler := VatsimAirportReconciler{Worker: AmanCandidateWorker{Source: nodes[second].source, State: nodes[second].state},
		EvaluatePresent: evaluate,
		EvaluateMissing: func(_ context.Context, board AmanBoard, flight *pb.VatsimFlight, _ *pb.VatsimObservation, _ *pb.CommandRequest) (AmanTransition, error) {
			missingCalls++
			if flight != nil {
				t.Fatal("missing evaluator received flight")
			}
			changed := make([]*pb.AmanFlight, 0, len(board.Flights))
			for _, old := range board.Flights {
				copy := proto.Clone(old).(*pb.AmanFlight)
				copy.LatestObservation.Missing = true
				changed = append(changed, copy)
			}
			return AmanTransition{Airport: &pb.AmanAirport{Airport: icao, Revision: board.Airport.Revision + 1, PolicyVersion: "v1", GeneratedAt: empty.GetVatsim().SnapshotAt}, Flights: changed}, nil
		},
	}
	if err := reconciler.Reconcile(ctx, icao); err != nil {
		t.Fatal(err)
	}
	if err := reconciler.Reconcile(ctx, icao); err != nil {
		t.Fatal(err)
	}
	if missingCalls != 1 {
		t.Fatalf("missing observation evaluated %d times", missingCalls)
	}
	board, err = nodes[second].state.Read(ctx, icao)
	if err != nil || board.Airport.Revision != 4 || !board.Flights[0].LatestObservation.Missing {
		t.Fatalf("takeover missing board: %+v %v", board, err)
	}
}

func mustSubject(t *testing.T, ref *pb.AggregateRef) string {
	t.Helper()
	subject, err := Subject(ref)
	if err != nil {
		t.Fatal(err)
	}
	return subject
}
