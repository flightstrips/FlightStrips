package cluster

import (
	"context"
	"errors"
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

func TestNATSSubjectCAS(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cfg := natsresources.Config{URLs: []string{"nats://bootstrap:bootstrap-local-only@127.0.0.1:4222", "nats://bootstrap:bootstrap-local-only@127.0.0.1:4223", "nats://bootstrap:bootstrap-local-only@127.0.0.1:4224"}, ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
	nc, err := natsresources.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	if err := natscluster.WaitForQuorum(ctx, nc); err != nil {
		t.Fatal(err)
	}
	if err := natsresources.Bootstrap(ctx, nc, cfg); err != nil {
		t.Fatal(err)
	}
	appCfg := cfg
	appCfg.URLs = []string{"nats://backend:backend-local-only@127.0.0.1:4222"}
	appNC, err := natsresources.Connect(appCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer appNC.Close()
	js, err := appNC.JetStream(nats.MaxWait(3 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	store := NATSStore{JS: js}
	random := uuid.New()
	icao := []byte{'A' + random[0]%26, 'A' + random[1]%26, 'A' + random[2]%26, 'A' + random[3]%26}
	ref := &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: string(icao)}}}
	subject, _ := Subject(ref)
	e := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, Fact: &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
	data, err := proto.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	results := make([]error, 2)
	for i := 0; i < 2; i++ {
		go func(i int) { defer wg.Done(); _, results[i] = store.Publish(ctx, subject, 0, data) }(i)
	}
	wg.Wait()
	if !((results[0] == nil && errors.Is(results[1], ErrCAS)) || (results[1] == nil && errors.Is(results[0], ErrCAS))) {
		t.Fatalf("expected exactly one PubAck and one CAS conflict: %#v; %#v", results[0], results[1])
	}
	entries, err := store.Replay(ctx, subject)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].StreamSequence == 0 || entries[0].ServerTime.IsZero() {
		t.Fatalf("invalid replay checkpoint: %+v", entries)
	}
	zero := uint64(0)
	r := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: string(icao), Value: &pb.EntityRecord{Value: &pb.EntityRecord_AirportPolicy{AirportPolicy: &pb.AirportPolicy{Airport: string(icao)}}}}}}}}
	w := Writer{Store: store, NodeID: "node-a"}
	first := w.Execute(ctx, r)
	if first.Status != pb.CommandReply_COMMITTED || first.GetStreamSequence() <= entries[0].StreamSequence {
		t.Fatalf("writer did not cross replay barrier: %v", first)
	}
	again := w.Execute(ctx, r)
	if again.Status != pb.CommandReply_COMMITTED || again.GetStreamSequence() != first.GetStreamSequence() {
		t.Fatalf("real NATS retry appended another event: %v", again)
	}
}
