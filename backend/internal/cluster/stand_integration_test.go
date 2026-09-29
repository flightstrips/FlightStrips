package cluster

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

// Two independent connections and projections race on one session subject.
// The losing writer must read the winning event before deciding its outcome.
func TestNATSStandRaceBlocksAndProjectionRestart(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cfg := natsresources.Config{URLs: []string{"nats://bootstrap:bootstrap-local-only@127.0.0.1:4222", "nats://bootstrap:bootstrap-local-only@127.0.0.1:4223", "nats://bootstrap:bootstrap-local-only@127.0.0.1:4224"}, ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
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
	cfg.URLs = []string{"nats://backend:backend-local-only@127.0.0.1:4222"}
	connect := func() *nats.Conn {
		nc, err := natsresources.Connect(cfg)
		if err != nil {
			t.Fatal(err)
		}
		return nc
	}
	ncA, ncB := connect(), connect()
	defer ncA.Close()
	defer ncB.Close()
	jsA, err := ncA.JetStream(nats.MaxWait(3 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	jsB, err := ncB.JetStream(nats.MaxWait(3 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	stores := []NATSStore{{JS: jsA}, {JS: jsB}}
	random := uuid.New()
	id := int32(100000 + binary.BigEndian.Uint32(random[:4])%1000000000)
	ref := sessionRef(id)
	subject, _ := Subject(ref)
	claim := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, Fact: &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
	data, err := proto.Marshal(claim)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stores[0].Publish(ctx, subject, 0, data); err != nil {
		t.Fatal(err)
	}
	firstCtx, stopFirst := context.WithCancel(ctx)
	defer stopFirst()
	secondCtx, stopSecond := context.WithCancel(ctx)
	defer stopSecond()
	projections := []*Projection{startProjection(t, firstCtx, ncA, cfg), startProjection(t, secondCtx, ncB, cfg)}
	zero := uint64(0)
	seed := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "seed"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: fmt.Sprint(id), Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: id, Airport: "EKCH", Name: "LIVE", NextStripId: 1}}}}}}}}
	seedWriter := Writer{Store: stores[0], NodeID: "node-a", Projection: projections[0], Plan: PlanStrip}
	if reply := seedWriter.Execute(ctx, seed); reply.Outcome.GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatal(reply)
	}
	for _, key := range []string{"SAS101", "SAS102"} {
		strip := StripState{Store: LocalLifecycleStore{Writer: seedWriter}}
		if reply, err := strip.Put(ctx, id, &pb.Strip{Callsign: key, Bay: "CLEARED", Departure: "EKCH", Destination: "ESSA"}, 0); err != nil {
			t.Fatalf("put strip: %v %v", reply, err)
		}
	}
	adapter := StandState{Stands: standTestRegistry(t)}
	base := adapter.PlanStand
	gate := make(chan struct{})
	planned := 0
	var lock sync.Mutex
	plan := func(ctx context.Context, r *pb.CommandRequest, a *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		lock.Lock()
		planned++
		if planned == 2 {
			close(gate)
		}
		lock.Unlock()
		<-gate
		return base(ctx, r, a)
	}
	writers := []Writer{{Store: stores[0], NodeID: "node-a", Projection: projections[0], Plan: plan}, {Store: stores[1], NodeID: "node-a", Projection: projections[1], Plan: plan}}
	replies := make([]*pb.CommandReply, 2)
	var wg sync.WaitGroup
	for i, key := range []string{"SAS101", "SAS102"} {
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			request := standRequest(key, "A1", testManual, &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "cid-1"}, 0)
			request.Aggregate = ref
			replies[i] = writers[i].Execute(ctx, request)
		}(i, key)
	}
	wg.Wait()
	success, failed := 0, 0
	for _, reply := range replies {
		if reply.Status != pb.CommandReply_COMMITTED {
			t.Fatalf("race transport: %v", replies)
		}
		if reply.Outcome.GetStatus() == pb.CommandOutcome_SUCCEEDED {
			success++
		} else {
			failed++
		}
	}
	if success != 1 || failed != 1 {
		t.Fatalf("incompatible stand race: %v", replies)
	}
	block := standRequest("", "B1", testBlock, &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "cid-1"}, 0)
	block.Aggregate = ref
	if reply := writers[0].Execute(ctx, block); reply.Outcome.GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("stand block: %v", reply)
	}
	before, err := projections[0].Read(ref)
	if err != nil {
		t.Fatal(err)
	}
	stopFirst()
	stopSecond()
	restarted := startProjection(t, ctx, ncA, cfg)
	after, err := restarted.Read(ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Indexes[pb.EntityKind_STAND_ASSIGNMENT]) != 1 || len(after.StandAssignmentsByStand["A1"]) != 1 || len(after.Indexes[pb.EntityKind_STAND_BLOCK]) != 1 || !proto.Equal(before.Indexes[pb.EntityKind_STAND_BLOCK]["B1"].Value, after.Indexes[pb.EntityKind_STAND_BLOCK]["B1"].Value) {
		t.Fatal("stand projection did not survive restart")
	}
}
