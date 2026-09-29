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

func TestStripCommandIDRetryDoesNotDuplicateTransition(t *testing.T) {
	store, strips := stripFixture(t)
	ctx := context.Background()
	zero := uint64(0)
	req := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(1), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "strip"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "SAS101", Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Callsign: "SAS101", Bay: "CLEARED"}}}}}}}}
	first, err := strips.Execute(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	before := store.commits
	second, err := strips.Execute(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if store.commits != before || first.GetStreamSequence() != second.GetStreamSequence() {
		t.Fatalf("retry duplicated strip: first=%v second=%v", first, second)
	}
	changed := proto.Clone(req).(*pb.CommandRequest)
	changed.GetSystem().GetUpdateEntity().Value.GetStrip().Bay = "PUSH"
	if reply, err := strips.Execute(ctx, changed); err == nil || reply.Status != pb.CommandReply_INVALID_ARGUMENT || store.commits != before {
		t.Fatalf("changed command ID accepted: %v %v", reply, err)
	}
}

func TestStripGenericEntityWriteCannotBypassAllocator(t *testing.T) {
	store, _ := stripFixture(t)
	zero := uint64(0)
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(1), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "SAS101", Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Id: 1, Callsign: "SAS101", Revision: 1, Bay: "CLEARED", Sequence: 1000}}}}}}}}
	reply := (Writer{Store: store, NodeID: "node-a", Plan: PlanSystemEntity}).Execute(context.Background(), request)
	if reply.Status != pb.CommandReply_INVALID_ARGUMENT {
		t.Fatalf("generic strip write bypassed allocator: %v", reply)
	}
}

func stripFixture(t *testing.T) (*memoryStore, StripState) {
	t.Helper()
	store, registry := registryFixture(t, 1)
	if _, err := registry.GetOrCreateSession(context.Background(), "EKCH", "LIVE"); err != nil {
		t.Fatal(err)
	}
	reader := Writer{Store: store}
	writer := Writer{Store: store, NodeID: "node-a", Plan: SessionLifecyclePlanner(func(ctx context.Context, ref *pb.AggregateRef) (*Aggregate, error) {
		subject, err := Subject(ref)
		if err != nil {
			return nil, err
		}
		return reader.load(ctx, subject, ref)
	})}
	return store, StripState{Store: LocalLifecycleStore{Writer: writer}}
}

func TestStripOrderRebalanceIsOneAtomicEvent(t *testing.T) {
	store, strips := stripFixture(t)
	ctx := context.Background()
	for i := 0; i < 12; i++ {
		if _, err := strips.Put(ctx, 1, &pb.Strip{Callsign: fmt.Sprintf("SAS%03d", i), Bay: "CLEARED", Departure: "EKCH"}, 0); err != nil {
			t.Fatal(err)
		}
	}
	controller := ControllerSector{Store: strips.Store}
	if _, err := controller.PutController(ctx, 1, &pb.Controller{Cid: "cid-1", Callsign: "EKCH_A_TWR", Position: "118.100"}, 0); err != nil {
		t.Fatal(err)
	}
	for i := 11; i >= 1; i-- {
		callsign := fmt.Sprintf("SAS%03d", i)
		strip, _, err := strips.ByCallsign(ctx, 1, callsign)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := strips.Edit(ctx, 1, "cid-1", strip.Revision, &pb.StripAction{Callsign: callsign, Change: &pb.StripAction_SetOrder{SetOrder: &pb.SetStripOrder{}}}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := store.Replay(ctx, "fs.v1.state.session.1")
	if err != nil {
		t.Fatal(err)
	}
	multiChange := false
	for _, entry := range entries {
		event := &pb.StateEvent{}
		if err := pb.UnmarshalStrict(entry.Data, event); err != nil {
			t.Fatal(err)
		}
		if len(event.GetDomainChanged().GetChanges()) > 1 {
			multiChange = true
		}
	}
	if !multiChange {
		t.Fatal("order rebalance did not commit as one multi-strip event")
	}
	list, _, err := strips.List(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[uint64]bool{}
	for _, strip := range list {
		if seen[strip.Sequence] {
			t.Fatalf("duplicate order %d", strip.Sequence)
		}
		seen[strip.Sequence] = true
	}
}

func TestStripConcurrentAllocationConflictAndReplay(t *testing.T) {
	store, strips := stripFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for _, callsign := range []string{"SAS101", "SAS102"} {
		wg.Add(1)
		go func(callsign string) {
			defer wg.Done()
			if reply, err := strips.Put(ctx, 1, &pb.Strip{Callsign: callsign, Bay: "NOT_CLEARED", Departure: "EKCH", Destination: "ESSA"}, 0); err != nil {
				t.Errorf("put %s: %v (%v)", callsign, err, reply)
			}
		}(callsign)
	}
	wg.Wait()
	list, revision, err := strips.List(ctx, 1)
	if err != nil || len(list) != 2 || list[0].Id == list[1].Id || list[0].Sequence == 0 || list[1].Sequence == 0 {
		t.Fatalf("allocation: strips=%v revision=%d err=%v", list, revision, err)
	}
	first, _, err := strips.ByCallsign(ctx, 1, "sas101")
	if err != nil {
		t.Fatal(err)
	}
	if first.Id != 1 && first.Id != 2 {
		t.Fatalf("unexpected strip ID: %d", first.Id)
	}
	byID, _, err := strips.ByID(ctx, 1, first.Id)
	if err != nil || !proto.Equal(byID, first) {
		t.Fatalf("numeric strip index: %v %v", byID, err)
	}
	entries, err := store.Replay(ctx, "fs.v1.state.session.1")
	if err != nil {
		t.Fatal(err)
	}
	allocations := 0
	for _, entry := range entries {
		event := &pb.StateEvent{}
		if err := pb.UnmarshalStrict(entry.Data, event); err != nil {
			t.Fatal(err)
		}
		if changes := event.GetDomainChanged().GetChanges(); len(changes) == 2 && changes[0].GetUpsert().GetSession() != nil && changes[1].GetUpsert().GetStrip() != nil {
			allocations++
		}
	}
	if allocations != 2 {
		t.Fatalf("expected one atomic session+strip event per allocation, got %d", allocations)
	}
	if reply, err := strips.Put(ctx, 1, &pb.Strip{Callsign: "SAS101", Bay: "NOT_CLEARED"}, 0); err == nil || reply.GetOutcome().GetReasonCode() != "REVISION_CONFLICT" {
		t.Fatalf("duplicate create: %v %v", reply, err)
	}
	if reply, err := strips.Delete(ctx, 1, "SAS101", first.Revision); err != nil {
		t.Fatalf("delete: %v %v", reply, err)
	}
	if _, err := strips.Put(ctx, 1, &pb.Strip{Callsign: "SAS101", Bay: "NOT_CLEARED"}, 0); err != nil {
		t.Fatal(err)
	}
	recreated, _, err := strips.ByCallsign(ctx, 1, "SAS101")
	if err != nil || recreated.Id <= 2 {
		t.Fatalf("reused strip ID: %v %v", recreated, err)
	}
	restarted := StripState{Store: LocalLifecycleStore{Writer: Writer{Store: store}}}
	replayed, replayRevision, err := restarted.List(ctx, 1)
	currentList, currentRevision, errCurrent := strips.List(ctx, 1)
	if err != nil || errCurrent != nil || replayRevision != currentRevision || len(replayed) != len(currentList) || !proto.Equal(replayed[0], currentList[0]) || !proto.Equal(replayed[1], currentList[1]) {
		t.Fatalf("replay: %v revision=%d err=%v", replayed, replayRevision, err)
	}
	state, err := (Writer{Store: store}).load(ctx, "fs.v1.state.session.1", sessionRef(1))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := state.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := aggregateFromSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(restored.Entities["SAS101"].GetValue().GetStrip(), recreated) {
		t.Fatal("snapshot strip differs from replay")
	}
	if restored.StripIDs[recreated.Id].Key != recreated.Callsign {
		t.Fatal("snapshot did not rebuild numeric strip index")
	}
}

func TestStripEditConflictOrderAndEuroScopeProtection(t *testing.T) {
	store, strips := stripFixture(t)
	ctx := context.Background()
	for _, callsign := range []string{"SAS101", "SAS102", "SAS103"} {
		if _, err := strips.Put(ctx, 1, &pb.Strip{Callsign: callsign, Bay: "CLEARED", Departure: "EKCH", Destination: "ESSA"}, 0); err != nil {
			t.Fatal(err)
		}
	}
	reader := Writer{Store: store}
	writer := Writer{Store: store, NodeID: "node-a", Plan: PlanStrip}
	controller := ControllerSector{Store: LocalLifecycleStore{Writer: writer}}
	if _, err := controller.PutController(ctx, 1, &pb.Controller{Cid: "cid-1", Callsign: "EKCH_A_TWR", Position: "118.100"}, 0); err != nil {
		t.Fatal(err)
	}
	third, _, _ := strips.ByCallsign(ctx, 1, "SAS103")
	order := &pb.StripAction{Callsign: "SAS103", Change: &pb.StripAction_SetOrder{SetOrder: &pb.SetStripOrder{}}}
	if reply, err := strips.Edit(ctx, 1, "cid-1", third.Revision, order); err != nil {
		t.Fatalf("order: %v %v", reply, err)
	}
	first, _, _ := strips.ByCallsign(ctx, 1, "SAS101")
	third, _, _ = strips.ByCallsign(ctx, 1, "SAS103")
	if third.Sequence >= first.Sequence {
		t.Fatalf("strip did not move to top: %d >= %d", third.Sequence, first.Sequence)
	}
	if reply, err := strips.Edit(ctx, 1, "cid-1", third.Revision-1, &pb.StripAction{Callsign: "SAS103", Change: &pb.StripAction_SetMarked{SetMarked: &pb.SetMarked{Marked: true}}}); err == nil || reply.GetOutcome().GetReasonCode() != "REVISION_CONFLICT" {
		t.Fatalf("stale edit: %v %v", reply, err)
	}
	// EuroScope cannot erase controller-owned order, mark, owner and clearance.
	if _, err := strips.Edit(ctx, 1, "cid-1", third.Revision, &pb.StripAction{Callsign: "SAS103", Change: &pb.StripAction_SetMarked{SetMarked: &pb.SetMarked{Marked: true}}}); err != nil {
		t.Fatal(err)
	}
	third, _, _ = strips.ByCallsign(ctx, 1, "SAS103")
	observed := &pb.Strip{Callsign: "SAS103", Bay: "UNKNOWN", Route: "N866", Departure: "EKCH", Destination: "ESSA", HasFlightPlan: true}
	if _, err := strips.Observe(ctx, 1, observed, third.Revision); err != nil {
		t.Fatal(err)
	}
	after, _, _ := strips.ByCallsign(ctx, 1, "SAS103")
	if !after.Marked || after.Bay != "CLEARED" || after.Sequence != third.Sequence || after.Route != "N866" || after.Id != third.Id {
		t.Fatalf("observation overwrote local state: %v", after)
	}
	if _, err := strips.Edit(ctx, 1, "cid-1", after.Revision, &pb.StripAction{Callsign: "SAS103", Change: &pb.StripAction_SetText{SetText: &pb.SetStripText{Field: pb.SetStripText_ROUTE, Value: "CONTROLLER"}}}); err != nil {
		t.Fatal(err)
	}
	after, _, _ = strips.ByCallsign(ctx, 1, "SAS103")
	if _, err := strips.Observe(ctx, 1, &pb.Strip{Callsign: "SAS103", Route: "EUROSCOPE"}, after.Revision); err != nil {
		t.Fatal(err)
	}
	after, _, _ = strips.ByCallsign(ctx, 1, "SAS103")
	if after.Route != "CONTROLLER" {
		t.Fatalf("EuroScope overwrote controller edit: %v", after)
	}
	// A second independently replayed backend sees the exact typed entity.
	state, err := reader.load(ctx, "fs.v1.state.session.1", sessionRef(1))
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(state.Entities["SAS103"].GetValue().GetStrip(), after) {
		t.Fatal("replay diverged")
	}
}
