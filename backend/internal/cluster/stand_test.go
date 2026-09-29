package cluster

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/sat"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func standTestRegistry(t *testing.T) *sat.StandCapabilityRegistry {
	t.Helper()
	r, err := sat.LoadStandCapabilities(strings.NewReader("STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30\nBLOCKS:A2\nSTAND:EKCH:A2:N055.37.42.710:E012.38.33.451:30\nBLOCKS:A1\nSTAND:EKCH:B1:N055.37.42.710:E012.38.33.452:30\n"))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func standTestPolicy(t *testing.T, registry *sat.StandCapabilityRegistry) *sat.AirlineAssignmentConfig {
	t.Helper()
	names := []string{sat.FallbackAirlinerDefault, sat.FallbackBusinessVIP, sat.FallbackCargo, sat.FallbackMilitary, sat.FallbackMilitaryHelicopter, sat.FallbackHelicopter, sat.FallbackGAPrivate, sat.FallbackUnknown}
	fallbacks := make([]string, 0, len(names))
	for _, name := range names {
		fallbacks = append(fallbacks, fmt.Sprintf(`%q:{"stands":{"tier1":{"B1":100}}}`, name))
	}
	document := `{"rules":[{"id":"sas","callsigns":["SAS"],"stands":{"tier1":{"A1":100},"tier2":{"B1":100}}}],"stand_groups":{},"fallback_rules":{` + strings.Join(fallbacks, ",") + `}}`
	policy, err := sat.LoadAirlineAssignment(strings.NewReader(document), registry)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func standRequest(callsign, stand string, change isTestStandChange, actor *pb.Actor, expected uint64) *pb.CommandRequest {
	action := &pb.StandAction{Callsign: callsign, Stand: stand}
	switch change {
	case testManual:
		action.Change = &pb.StandAction_Manual{Manual: &pb.ManualStand{}}
	case testBlock:
		action.Callsign = ""
		action.Change = &pb.StandAction_CreateBlock{CreateBlock: &pb.CreateStandBlock{Reason: "closed"}}
	case testRemoveBlock:
		action.Callsign = ""
		action.Change = &pb.StandAction_RemoveBlock{RemoveBlock: &pb.RemoveStandBlock{}}
	case testAck:
		action.Change = &pb.StandAction_Acknowledge{Acknowledge: &pb.AcknowledgeStand{}}
	}
	return &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(1), Actor: actor, ExpectedEntityRevision: &expected, Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Stand{Stand: action}}}}
}

type isTestStandChange int

const (
	testManual isTestStandChange = iota
	testBlock
	testRemoveBlock
	testAck
)

func TestStandRaceReevaluatesAfterCASAndRetainsHistory(t *testing.T) {
	store, strips := stripFixture(t)
	ctx := context.Background()
	for _, key := range []string{"SAS101", "SAS102"} {
		if reply, err := strips.Put(ctx, 1, &pb.Strip{Callsign: key, Bay: "CLEARED", Departure: "EKCH", Destination: "ESSA"}, 0); err != nil {
			t.Fatalf("put strip: %v %v", reply, err)
		}
	}
	adapter := StandState{Stands: standTestRegistry(t)}
	base := adapter.PlanStand
	gate := make(chan struct{})
	var mu sync.Mutex
	planned := 0
	plan := func(ctx context.Context, r *pb.CommandRequest, a *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		mu.Lock()
		planned++
		if planned == 2 {
			close(gate)
		}
		mu.Unlock()
		<-gate
		return base(ctx, r, a)
	}
	writers := []Writer{{Store: store, NodeID: "node-a", Plan: plan}, {Store: store, NodeID: "node-a", Plan: plan}}
	replies := make([]*pb.CommandReply, 2)
	var wg sync.WaitGroup
	for i, key := range []string{"SAS101", "SAS102"} {
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			replies[i] = writers[i].Execute(ctx, standRequest(key, "A1", testManual, &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "cid-1"}, 0))
		}(i, key)
	}
	wg.Wait()
	success, failed := 0, 0
	for _, reply := range replies {
		if reply.Status != pb.CommandReply_COMMITTED {
			t.Fatalf("race transport: %v", reply)
		}
		if reply.Outcome.Status == pb.CommandOutcome_SUCCEEDED {
			success++
		} else {
			failed++
		}
	}
	if success != 1 || failed != 1 {
		t.Fatalf("incompatible race: %v", replies)
	}
	reader := Writer{Store: store}
	state, err := reader.load(ctx, "fs.v1.state.session.1", sessionRef(1))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Indexes[pb.EntityKind_STAND_ASSIGNMENT]) != 1 || len(state.Ledger) < 2 {
		t.Fatalf("lost stand history: %+v", state)
	}
	winner := state.EntitiesByKind(pb.EntityKind_STAND_ASSIGNMENT)[0].GetValue().GetStandAssignment()
	if winner.Stand != "A1" || winner.Direction != "DEPARTURE" || !winner.Manual || winner.Revision != 1 || winner.ExpiresAt == nil || !winner.ExpiresAt.AsTime().After(winner.AssignedAt.AsTime()) {
		t.Fatalf("assignment metadata: %v", winner)
	}
	blocked := standRequest("", "A2", testBlock, &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "cid-1"}, 0)
	if reply := writers[0].Execute(ctx, blocked); reply.Outcome.GetStatus() != pb.CommandOutcome_FAILED {
		t.Fatalf("adjacency block accepted: %v", reply)
	}
	open := standRequest("", "B1", testBlock, &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "cid-1"}, 0)
	if reply := writers[0].Execute(ctx, open); reply.Outcome.GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("block rejected: %v", reply)
	}
	restarted, err := reader.load(ctx, "fs.v1.state.session.1", sessionRef(1))
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(state.Indexes[pb.EntityKind_STAND_ASSIGNMENT][winner.Callsign].Value, restarted.Indexes[pb.EntityKind_STAND_ASSIGNMENT][winner.Callsign].Value) || len(restarted.Indexes[pb.EntityKind_STAND_BLOCK]) != 1 {
		t.Fatal("replay lost assignment or block")
	}
}

func TestStandAcknowledgmentAndExpiryRequireCurrentRevision(t *testing.T) {
	store, strips := stripFixture(t)
	ctx := context.Background()
	if reply, err := strips.Put(ctx, 1, &pb.Strip{Callsign: "SAS101", Bay: "CLEARED", Departure: "EKCH", Destination: "ESSA"}, 0); err != nil {
		t.Fatalf("strip: %v %v", reply, err)
	}
	adapter := StandState{Stands: standTestRegistry(t)}
	writer := Writer{Store: store, NodeID: "node-a", Plan: adapter.PlanStand}
	assigned := writer.Execute(ctx, standRequest("SAS101", "A1", testManual, &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "cid-1"}, 0))
	if assigned.Outcome.GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatal(assigned)
	}
	ack := writer.Execute(ctx, standRequest("SAS101", "", testAck, &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "cid-1"}, 1))
	if ack.Outcome.GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatal(ack)
	}
	stale := writer.Execute(ctx, standRequest("SAS101", "", testAck, &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "cid-1"}, 1))
	if stale.Outcome.GetReasonCode() != "REVISION_CONFLICT" {
		t.Fatalf("stale acknowledgment: %v", stale)
	}
	// A deadline is committed by a system lifecycle action. A later system
	// deletion must still match that exact revision and elapsed deadline.
	due := time.Now().UTC().Add(time.Hour)
	stage := "RESERVED"
	observed := standRequest("SAS101", "A1", testManual, &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "lifecycle"}, 2)
	observed.GetClient().GetStand().Stage = &stage
	observed.GetClient().GetStand().ExpiresAt = timestamppb.New(due)
	cid, revision := int64(123), int64(5)
	observed.GetClient().GetStand().VatsimCid = &cid
	observed.GetClient().GetStand().VatsimRevision = &revision
	if reply := writer.Execute(ctx, observed); reply.Outcome.GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatal(reply)
	}
	expected := uint64(3)
	changed := standRequest("SAS101", "A1", testManual, &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "lifecycle"}, expected)
	otherCID := int64(456)
	changed.GetClient().GetStand().VatsimCid = &otherCID
	changed.GetClient().GetStand().VatsimRevision = &revision
	if reply := writer.Execute(ctx, changed); reply.Outcome.GetReasonCode() != "REVISION_CONFLICT" {
		t.Fatalf("changed VATSIM identity accepted: %v", reply)
	}
	expiry := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(1), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "expiry"}, ExpectedEntityRevision: &expected, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_RemoveEntity{RemoveEntity: &pb.RemoveEntity{Key: "SAS101", Kind: pb.EntityKind_STAND_ASSIGNMENT}}}}}
	if reply := writer.Execute(ctx, expiry); reply.Outcome.GetStatus() != pb.CommandOutcome_FAILED {
		t.Fatalf("early expiry: %v", reply)
	}
	adapter.Now = func() time.Time { return due.Add(time.Second) }
	writer.Plan = adapter.PlanStand
	expiry.CommandId = uuid.NewString()
	if reply := writer.Execute(ctx, expiry); reply.Outcome.GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("due expiry: %v", reply)
	}
}

func TestAutomaticStandCASReevaluatesPolicySelection(t *testing.T) {
	store, strips := stripFixture(t)
	ctx := context.Background()
	for _, key := range []string{"SAS101", "SAS102"} {
		if reply, err := strips.Put(ctx, 1, &pb.Strip{Callsign: key, Bay: "CLEARED", Departure: "EKCH", Destination: "ESSA"}, 0); err != nil {
			t.Fatalf("strip: %v %v", reply, err)
		}
	}
	registry := standTestRegistry(t)
	adapter := StandState{Stands: registry, Policy: standTestPolicy(t, registry)}
	base := adapter.PlanStand
	gate := make(chan struct{})
	planned := 0
	var mu sync.Mutex
	plan := func(ctx context.Context, r *pb.CommandRequest, a *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		mu.Lock()
		planned++
		if planned == 2 {
			close(gate)
		}
		mu.Unlock()
		<-gate
		return base(ctx, r, a)
	}
	writers := []Writer{{Store: store, NodeID: "node-a", Plan: plan}, {Store: store, NodeID: "node-a", Plan: plan}}
	replies := make([]*pb.CommandReply, 2)
	var wg sync.WaitGroup
	for i, key := range []string{"SAS101", "SAS102"} {
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			request := standRequest(key, "", testManual, &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "cid-1"}, 0)
			request.GetClient().GetStand().Change = &pb.StandAction_Automatic{Automatic: &pb.AutomaticStand{}}
			replies[i] = writers[i].Execute(ctx, request)
		}(i, key)
	}
	wg.Wait()
	for _, reply := range replies {
		if reply.Status != pb.CommandReply_COMMITTED || reply.Outcome.GetStatus() != pb.CommandOutcome_SUCCEEDED {
			t.Fatalf("automatic race: %v", replies)
		}
	}
	state, err := (Writer{Store: store}).load(ctx, "fs.v1.state.session.1", sessionRef(1))
	if err != nil {
		t.Fatal(err)
	}
	assignments := state.EntitiesByKind(pb.EntityKind_STAND_ASSIGNMENT)
	if len(assignments) != 2 {
		t.Fatalf("assignments: %v", assignments)
	}
	used := map[string]bool{}
	for _, e := range assignments {
		v := e.GetValue().GetStandAssignment()
		if used[v.Stand] || v.RuleId == nil || *v.RuleId != "sas" {
			t.Fatalf("policy was not reevaluated: %v", assignments)
		}
		used[v.Stand] = true
	}
	if !used["A1"] || !used["B1"] {
		t.Fatalf("missing expected policy choices: %v", used)
	}
}

func TestAutomaticStandPromotionDisplacesSoftReservationAtomically(t *testing.T) {
	store, strips := stripFixture(t)
	ctx := context.Background()
	for _, key := range []string{"SAS101", "SAS102"} {
		if reply, err := strips.Put(ctx, 1, &pb.Strip{Callsign: key, Bay: "CLEARED", Departure: "ESSA", Destination: "EKCH"}, 0); err != nil {
			t.Fatalf("strip: %v %v", reply, err)
		}
	}
	registry := standTestRegistry(t)
	adapter := StandState{Stands: registry, Policy: standTestPolicy(t, registry)}
	writer := Writer{Store: store, NodeID: "node-a", Plan: adapter.PlanStand}
	auto := func(key, stage string) *pb.CommandRequest {
		r := standRequest(key, "", testManual, &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "lifecycle"}, 0)
		a := r.GetClient().GetStand()
		a.Change = &pb.StandAction_Automatic{Automatic: &pb.AutomaticStand{}}
		a.Stage = &stage
		return r
	}
	if reply := writer.Execute(ctx, auto("SAS101", "ESTIMATED")); reply.Outcome.GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatal(reply)
	}
	if reply := writer.Execute(ctx, auto("SAS102", "CONFIRMED")); reply.Outcome.GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatal(reply)
	}
	state, err := writer.load(ctx, "fs.v1.state.session.1", sessionRef(1))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Indexes[pb.EntityKind_STAND_ASSIGNMENT]) != 1 || state.Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SAS102"].GetValue().GetStandAssignment().Stand != "A1" || state.Indexes[pb.EntityKind_STRIP]["SAS101"].GetValue().GetStrip().Stand != "" {
		t.Fatalf("soft displacement was not atomic: %v", state.Indexes)
	}
}

func TestStandBlockOwnershipAndDeadline(t *testing.T) {
	store, _ := stripFixture(t)
	ctx := context.Background()
	adapter := StandState{Stands: standTestRegistry(t)}
	writer := Writer{Store: store, NodeID: "node-a", Plan: adapter.PlanStand}
	due := time.Now().UTC().Add(time.Hour)
	create := standRequest("", "B1", testBlock, &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "cid-1"}, 0)
	create.GetClient().GetStand().GetCreateBlock().ExpiresAt = timestamppb.New(due)
	if reply := writer.Execute(ctx, create); reply.Outcome.GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatal(reply)
	}
	if reply := writer.Execute(ctx, standRequest("", "B1", testRemoveBlock, &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "cid-2"}, 1)); reply.Status != pb.CommandReply_UNAUTHORIZED {
		t.Fatalf("foreign removal: %v", reply)
	}
	expected := uint64(1)
	expiry := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(1), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "expiry"}, ExpectedEntityRevision: &expected, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_RemoveEntity{RemoveEntity: &pb.RemoveEntity{Key: "B1", Kind: pb.EntityKind_STAND_BLOCK}}}}}
	if reply := writer.Execute(ctx, expiry); reply.Outcome.GetStatus() != pb.CommandOutcome_FAILED {
		t.Fatalf("early block expiry: %v", reply)
	}
	adapter.Now = func() time.Time { return due.Add(time.Second) }
	writer.Plan = adapter.PlanStand
	expiry.CommandId = uuid.NewString()
	if reply := writer.Execute(ctx, expiry); reply.Outcome.GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("due block expiry: %v", reply)
	}
	state, err := writer.load(ctx, "fs.v1.state.session.1", sessionRef(1))
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Indexes[pb.EntityKind_STAND_BLOCK]) != 0 {
		t.Fatal("expired block still present")
	}
}
