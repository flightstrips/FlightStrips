package cluster

import (
	"context"
	"encoding/binary"
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

func TestNATSControllerSectorCASAndRestart(t *testing.T) {
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
	randomID := uuid.New()
	ref := sessionRef(int32(100000 + binary.BigEndian.Uint32(randomID[:4])%1000000000))
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
	secondCtx, stopSecond := context.WithCancel(ctx)
	first, second := startProjection(t, firstCtx, nc, cfg), startProjection(t, secondCtx, nc, cfg)
	writers := []Writer{{Store: store, NodeID: "node-a", Projection: first, Plan: PlanControllerSector}, {Store: store, NodeID: "node-a", Projection: second, Plan: PlanControllerSector}}
	var wg sync.WaitGroup
	replies := make(chan *pb.CommandReply, 2)
	for i, cid := range []string{"cid-a", "cid-b"} {
		wg.Add(1)
		go func(i int, cid string) {
			defer wg.Done()
			adapter := ControllerSector{Store: LocalLifecycleStore{Writer: writers[i]}}
			reply, _ := adapter.PutController(ctx, ref.GetSession().Id, &pb.Controller{Cid: cid, Callsign: "EKCH_A_TWR"}, 0)
			replies <- reply
		}(i, cid)
	}
	wg.Wait()
	close(replies)
	committed, rejected := 0, 0
	for reply := range replies {
		if reply == nil {
			t.Fatal("missing NATS reply")
		}
		if reply.GetOutcome().GetStatus() == pb.CommandOutcome_SUCCEEDED {
			committed++
		} else if reply.GetOutcome().GetReasonCode() == "INVALID_ARGUMENT" {
			rejected++
		}
	}
	if committed != 1 || rejected != 1 {
		t.Fatalf("NATS CAS outcomes: committed=%d rejected=%d", committed, rejected)
	}
	before := ControllerSector{Store: LocalLifecycleStore{Writer: writers[0]}}
	controllers, err := before.Controllers(ctx, ref.GetSession().Id)
	if err != nil || len(controllers) != 1 {
		t.Fatalf("controller projection=%v err=%v", controllers, err)
	}
	zero := uint64(0)
	seedRecord := &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: ref.GetSession().Id, Airport: "EKCH", Name: "LIVE"}}}
	seedSystem := &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: subject[len("fs.v1.state.session."):], Value: seedRecord}}}
	seed := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_System{System: seedSystem}}
	if reply := writers[0].Execute(ctx, seed); reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatal(reply)
	}
	if _, err := before.ReplaceSectorOwners(ctx, ref.GetSession().Id, []*pb.SectorOwner{{Sector: "GND", Position: "121.900", Identifier: "G"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := before.ChangeRunways(ctx, ref.GetSession().Id, controllers[0].Cid, 1, []*pb.Runway{{Name: "22L", Departure: true}}); err != nil {
		t.Fatal(err)
	}
	stopFirst()
	stopSecond()
	restarted := startProjection(t, ctx, nc, cfg)
	adapter := ControllerSector{Store: LocalLifecycleStore{Writer: Writer{Store: store, Projection: restarted}}}
	controllers, err = adapter.Controllers(ctx, ref.GetSession().Id)
	if err != nil || len(controllers) != 1 {
		t.Fatalf("restart controllers=%v err=%v", controllers, err)
	}
	owners, err := adapter.SectorOwners(ctx, ref.GetSession().Id)
	if err != nil || len(owners) != 1 || owners[0].Sector != "GND" {
		t.Fatalf("restart sectors=%v err=%v", owners, err)
	}
	session, _, err := adapter.Session(ctx, ref.GetSession().Id)
	if err != nil || len(session.Runways) != 1 || session.Runways[0].Name != "22L" {
		t.Fatalf("restart runways=%v err=%v", session, err)
	}
	online, err := adapter.OperationalControllers(ctx, ref.GetSession().Id, nil, time.Now())
	if err != nil || len(online) != 0 {
		t.Fatalf("restart fabricated presence: %v %v", online, err)
	}
}
