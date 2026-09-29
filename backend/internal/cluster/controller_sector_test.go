package cluster

import (
	"context"
	"sync"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

func controllerFixture(t *testing.T) (*memoryStore, ControllerSector, Writer) {
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
	return store, ControllerSector{Store: LocalLifecycleStore{Writer: writer}}, writer
}

func TestControllerSectorConcurrentIdentityAndReplay(t *testing.T) {
	store, adapter, writer := controllerFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	replies := make(chan *pb.CommandReply, 2)
	for _, cid := range []string{"cid-a", "cid-b"} {
		wg.Add(1)
		go func(cid string) {
			defer wg.Done()
			reply, _ := adapter.PutController(ctx, 1, &pb.Controller{Cid: cid, Callsign: "EKCH_A_TWR", Position: "118.100"}, 0)
			replies <- reply
		}(cid)
	}
	wg.Wait()
	close(replies)
	committed, rejected := 0, 0
	for reply := range replies {
		if reply == nil {
			t.Fatal("missing reply")
		}
		if reply.GetOutcome().GetStatus() == pb.CommandOutcome_SUCCEEDED {
			committed++
		} else if reply.GetOutcome().GetReasonCode() == "INVALID_ARGUMENT" {
			rejected++
		} else {
			t.Fatalf("unexpected concurrent outcome: %v", reply)
		}
	}
	if committed != 1 || rejected != 1 {
		t.Fatalf("committed=%d rejected=%d", committed, rejected)
	}
	controllers, err := adapter.Controllers(ctx, 1)
	if err != nil || len(controllers) != 1 {
		t.Fatalf("controllers=%v err=%v", controllers, err)
	}
	if _, err := adapter.PutSectorOwner(ctx, 1, &pb.SectorOwner{Sector: "GND", ControllerCid: controllers[0].Cid, Position: "121.900", Identifier: "G"}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ChangeRunways(ctx, 1, controllers[0].Cid, 1, []*pb.Runway{{Name: "22L", Departure: true}, {Name: "22R", Arrival: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.UpdateRunwayStatus(ctx, 1, controllers[0].Cid, 2, "22L/22R", "LOW_VIS"); err != nil {
		t.Fatal(err)
	}
	if reply, err := adapter.ChangeLayout(ctx, 1, controllers[0].Cid, 2, "TWR"); err == nil || reply.GetOutcome().GetReasonCode() != "REVISION_CONFLICT" {
		t.Fatalf("stale layout: %v %v", reply, err)
	}

	// A new backend reconstructs records and indexes solely from the log.
	restarted := ControllerSector{Store: LocalLifecycleStore{Writer: Writer{Store: store}}}
	restored, err := restarted.Controllers(ctx, 1)
	if err != nil || !proto.Equal(restored[0], controllers[0]) {
		t.Fatalf("restored controllers=%v err=%v", restored, err)
	}
	owners, err := restarted.SectorOwners(ctx, 1)
	if err != nil || len(owners) != 1 || owners[0].Sector != "GND" {
		t.Fatalf("restored owners=%v err=%v", owners, err)
	}
	session, revision, err := restarted.Session(ctx, 1)
	if err != nil || revision != 3 || len(session.Runways) != 2 || session.RunwayStatuses[0].Status != "LOW_VIS" {
		t.Fatalf("restored runway state=%v revision=%d err=%v", session, revision, err)
	}
	if online, err := restarted.OperationalControllers(ctx, 1, nil, time.Now()); err != nil || len(online) != 0 {
		t.Fatalf("restart fabricated online controllers: %v %v", online, err)
	}
	if _, err := writer.load(ctx, "fs.v1.state.session.1", sessionRef(1)); err != nil {
		t.Fatal(err)
	}
}

func TestControllerPresenceIsIndependentOfDurableIdentity(t *testing.T) {
	_, adapter, _ := controllerFixture(t)
	ctx := context.Background()
	if _, err := adapter.PutController(ctx, 1, &pb.Controller{Cid: "cid-a", Callsign: "EKCH_A_TWR", Position: "118.100"}, 0); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	node := KVPresence{Value: &pb.PresenceValue{Present: &pb.PresenceValue_Node{Node: &pb.NodePresence{NodeId: "node-a", Ready: true}}}, Observed: now}
	client := KVPresence{Value: &pb.PresenceValue{Present: &pb.PresenceValue_Client{Client: &pb.ClientPresence{ConnectionId: uuid.NewString(), NodeId: "node-a", SessionId: 1, Cid: "cid-a", Callsign: "EKCH_A_TWR", Position: "118.100", Kind: pb.ClientPresence_EUROSCOPE}}}, Observed: now}
	online, err := adapter.OperationalControllers(ctx, 1, []KVPresence{node, client}, now)
	if err != nil || len(online) != 1 {
		t.Fatalf("live presence: %v %v", online, err)
	}
	stale := now.Add(11 * time.Second)
	online, err = adapter.OperationalControllers(ctx, 1, []KVPresence{node, client}, stale)
	if err != nil || len(online) != 0 {
		t.Fatalf("expired presence: %v %v", online, err)
	}
}

func TestControllerRevisionAndSectorValidation(t *testing.T) {
	_, adapter, _ := controllerFixture(t)
	ctx := context.Background()
	if _, err := adapter.PutController(ctx, 1, &pb.Controller{Cid: "cid-a", Callsign: "EKCH_A_TWR"}, 0); err != nil {
		t.Fatal(err)
	}
	if reply, err := adapter.PutController(ctx, 1, &pb.Controller{Cid: "cid-a", Callsign: "EKCH_A_TWR", Position: "118.100"}, 0); err == nil || reply.GetOutcome().GetReasonCode() != "REVISION_CONFLICT" {
		t.Fatalf("stale controller write: %v %v", reply, err)
	}
	if _, err := adapter.PutController(ctx, 1, &pb.Controller{Cid: "cid-a", Callsign: "EKCH_A_TWR", Position: "118.100"}, 1); err != nil {
		t.Fatal(err)
	}
	if reply, err := adapter.PutSectorOwner(ctx, 1, &pb.SectorOwner{Sector: "GND", Position: "121.900"}, 0); err == nil || reply.GetOutcome().GetReasonCode() != "INVALID_ARGUMENT" {
		t.Fatalf("invalid sector: %v %v", reply, err)
	}
	if _, err := adapter.PutSectorOwner(ctx, 1, &pb.SectorOwner{Sector: "GND", Position: "121.900", Identifier: "G"}, 0); err != nil {
		t.Fatal(err)
	}
	if reply, err := adapter.PutSectorOwner(ctx, 1, &pb.SectorOwner{Sector: "GND", Position: "121.900", Identifier: "T"}, 0); err == nil || reply.GetOutcome().GetReasonCode() != "REVISION_CONFLICT" {
		t.Fatalf("stale sector write: %v %v", reply, err)
	}
}

func TestControllerLayoutAndSectorReplacementAreAtomic(t *testing.T) {
	_, adapter, _ := controllerFixture(t)
	ctx := context.Background()
	for _, c := range []*pb.Controller{{Cid: "cid-a", Callsign: "EKCH_A_TWR", Position: "118.100"}, {Cid: "cid-b", Callsign: "EKCH_D_TWR", Position: "118.100"}} {
		if _, err := adapter.PutController(ctx, 1, c, 0); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := adapter.SetPositionLayout(ctx, 1, "118.100", "TWR"); err != nil {
		t.Fatal(err)
	}
	controllers, err := adapter.Controllers(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range controllers {
		if c.LayoutId != "TWR" || c.Revision != 2 {
			t.Fatalf("layout not applied atomically: %v", controllers)
		}
	}
	initial := []*pb.SectorOwner{{Sector: "GND", Position: "121.900", Identifier: "G"}, {Sector: "TWR", Position: "118.100", Identifier: "T"}}
	if _, err := adapter.ReplaceSectorOwners(ctx, 1, initial); err != nil {
		t.Fatal(err)
	}
	if reply, err := adapter.ReplaceSectorOwners(ctx, 1, []*pb.SectorOwner{{Sector: "APP", Position: "119.800", Identifier: "A"}, {Sector: "APP", Position: "119.800", Identifier: "B"}}); err == nil || reply.GetOutcome().GetReasonCode() != "INVALID_ARGUMENT" {
		t.Fatalf("duplicate replacement: %v %v", reply, err)
	}
	owners, err := adapter.SectorOwners(ctx, 1)
	if err != nil || len(owners) != 2 {
		t.Fatalf("failed replacement partially applied: %v %v", owners, err)
	}
	if _, err := adapter.ReplaceSectorOwners(ctx, 1, []*pb.SectorOwner{{Sector: "APP", Position: "119.800", Identifier: "A"}}); err != nil {
		t.Fatal(err)
	}
	owners, err = adapter.SectorOwners(ctx, 1)
	if err != nil || len(owners) != 1 || owners[0].Sector != "APP" {
		t.Fatalf("replacement result: %v %v", owners, err)
	}
}

func TestMasterPositionOrderReadsAirportProjection(t *testing.T) {
	store, adapter, _ := controllerFixture(t)
	ctx := context.Background()
	ref := &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: "EKCH"}}}
	subject, _ := Subject(ref)
	claim := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, Fact: &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
	data, err := proto.Marshal(claim)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Publish(ctx, subject, 0, data); err != nil {
		t.Fatal(err)
	}
	zero := uint64(0)
	req := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "airport-policy"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "EKCH", Value: &pb.EntityRecord{Value: &pb.EntityRecord_AirportPolicy{AirportPolicy: &pb.AirportPolicy{Airport: "EKCH", MasterPositionOrder: []string{"TWR", "GND"}}}}}}}}}
	if reply := (Writer{Store: store, NodeID: "node-a"}).Execute(ctx, req); reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatal(reply)
	}
	order, err := adapter.MasterPositionOrder(ctx, "ekch")
	if err != nil || len(order) != 2 || order[0] != "TWR" {
		t.Fatalf("master order=%v err=%v", order, err)
	}
}
