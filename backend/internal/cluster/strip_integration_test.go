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

func TestNATSStripAllocationAndProjectionRestart(t *testing.T) {
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
	nc, err := natsresources.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := nc.JetStream(nats.MaxWait(3 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	store := NATSStore{JS: js}
	random := uuid.New()
	id := int32(100000 + binary.BigEndian.Uint32(random[:4])%1000000000)
	ref := sessionRef(id)
	subject, _ := Subject(ref)
	claim := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, Fact: &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
	data, err := proto.Marshal(claim)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Publish(ctx, subject, 0, data); err != nil {
		t.Fatal(err)
	}
	firstCtx, stopFirst := context.WithCancel(ctx)
	defer stopFirst()
	secondCtx, stopSecond := context.WithCancel(ctx)
	defer stopSecond()
	first, second := startProjection(t, firstCtx, nc, cfg), startProjection(t, secondCtx, nc, cfg)
	writers := []Writer{{Store: store, NodeID: "node-a", Projection: first, Plan: PlanStrip}, {Store: store, NodeID: "node-a", Projection: second, Plan: PlanStrip}}
	zero := uint64(0)
	seed := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: fmt.Sprint(id), Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: id, Airport: "EKCH", Name: "LIVE", NextStripId: 1}}}}}}}}
	if reply := writers[0].Execute(ctx, seed); reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatal(reply)
	}
	var wg sync.WaitGroup
	replies := make(chan *pb.CommandReply, 2)
	for i, callsign := range []string{"SAS101", "SAS102"} {
		wg.Add(1)
		go func(i int, callsign string) {
			defer wg.Done()
			adapter := StripState{Store: LocalLifecycleStore{Writer: writers[i]}}
			reply, _ := adapter.Put(ctx, id, &pb.Strip{Callsign: callsign, Bay: "CLEARED", Departure: "EKCH"}, 0)
			replies <- reply
		}(i, callsign)
	}
	wg.Wait()
	close(replies)
	for reply := range replies {
		if reply == nil || reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
			t.Fatalf("strip CAS result: %v", reply)
		}
	}
	adapter := StripState{Store: LocalLifecycleStore{Writer: writers[0]}}
	before, _, err := adapter.List(ctx, id)
	if err != nil || len(before) != 2 || before[0].Id == before[1].Id {
		t.Fatalf("before restart: %v %v", before, err)
	}
	stopFirst()
	stopSecond()
	restarted := startProjection(t, ctx, nc, cfg)
	replayed := StripState{Store: LocalLifecycleStore{Writer: Writer{Store: store, Projection: restarted}}}
	after, _, err := replayed.List(ctx, id)
	if err != nil || len(after) != 2 || !proto.Equal(before[0], after[0]) || !proto.Equal(before[1], after[1]) {
		t.Fatalf("projection restart: %v %v", after, err)
	}
}
