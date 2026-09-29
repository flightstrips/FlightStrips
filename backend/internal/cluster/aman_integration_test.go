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
)

func TestAmanNATSIndependentProjectionReplay(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	port := 4222
	if configured := os.Getenv("NATS_AMAN_PORT_BASE"); configured != "" {
		var err error
		port, err = strconv.Atoi(configured)
		if err != nil || port < 1024 || port > 65533 { t.Fatalf("invalid NATS_AMAN_PORT_BASE %q", configured) }
	}
	cfg := natsresources.Config{URLs: []string{fmt.Sprintf("nats://bootstrap:bootstrap-local-only@127.0.0.1:%d", port), fmt.Sprintf("nats://bootstrap:bootstrap-local-only@127.0.0.1:%d", port+1), fmt.Sprintf("nats://bootstrap:bootstrap-local-only@127.0.0.1:%d", port+2)}, ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
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
	cfg.URLs = []string{fmt.Sprintf("nats://backend:backend-local-only@127.0.0.1:%d", port)}
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
	id := uuid.New()
	icao := string([]byte{'A' + id[0]%26, 'A' + id[1]%26, 'A' + id[2]%26, 'A' + id[3]%26})
	ref := airportRef(icao)
	subject, _ := Subject(ref)
	owner := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "node-a"}, Fact: &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
	data, _ := proto.Marshal(owner)
	if _, err := store.Publish(ctx, subject, 0, data); err != nil {
		t.Fatal(err)
	}
	firstCtx, stopFirst := context.WithCancel(ctx)
	first := startProjection(t, firstCtx, nc, cfg)
	transition := amanTransition(uuid.NewString(), 0, 1, "SAS123")
	transition.Airport.Airport = icao
	transition.Request.Aggregate = ref
	transition.Request.GetSystem().GetUpdateEntity().Key = icao
	transition.Request.GetSystem().GetUpdateEntity().Value.GetAmanAirport().Airport = icao
	adapter := AmanAdapter{Writer: Writer{Store: store, NodeID: "node-a", Projection: first}}
	reply := adapter.Commit(ctx, transition)
	if reply.Status != pb.CommandReply_COMMITTED || reply.Outcome.Status != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("NATS AMAN commit: %v", reply)
	}
	one, err := adapter.Read(ctx, icao)
	if err != nil {
		t.Fatal(err)
	}
	stopFirst()
	second := startProjection(t, ctx, nc, cfg)
	third := startProjection(t, ctx, nc, cfg)
	if err := second.WaitApplied(ctx, reply.GetStreamSequence()); err != nil {
		t.Fatal(err)
	}
	for _, projection := range []*Projection{second, third} {
		if err := projection.WaitApplied(ctx, reply.GetStreamSequence()); err != nil {
			t.Fatal(err)
		}
		replayed := AmanAdapter{Writer: Writer{Store: store, NodeID: "node-a", Projection: projection}}
		board, err := replayed.Read(ctx, icao)
		if err != nil || !proto.Equal(one.Airport, board.Airport) || !proto.Equal(one.Flights[0], board.Flights[0]) || !proto.Equal(one.Audits[0], board.Audits[0]) {
			t.Fatalf("independent AMAN projection: %+v %+v %v", one, board, err)
		}
	}
}
