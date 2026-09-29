package cluster

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
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

func TestIndependentProjectionReplayAndSnapshotFallback(t *testing.T) {
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
	id := uuid.New()
	icao := string([]byte{'A' + id[0]%26, 'A' + id[1]%26, 'A' + id[2]%26, 'A' + id[3]%26})
	ref := &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: icao}}}
	subject, _ := Subject(ref)
	owner := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "node-a"}, Fact: &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
	data, _ := proto.Marshal(owner)
	ownerSeq, err := store.Publish(ctx, subject, 0, data)
	if err != nil {
		t.Fatal(err)
	}
	first := startProjection(t, ctx, nc, cfg)
	second := startProjection(t, ctx, nc, cfg)
	position := &pb.PositionValue{SchemaVersion: 1, SessionId: 1000 + int32(id[4]), AircraftKey: icao, OwnerEpoch: 1, ObservedAt: timestamppb.Now(), Observation: &pb.PositionValue_Position{Position: &pb.AircraftPosition{Latitude: 55, Longitude: 12}}}
	positionData, _ := proto.Marshal(position)
	if _, err := first.Positions.Put(fmt.Sprintf("%d.%s.1", position.SessionId, icao), positionData); err != nil {
		t.Fatal(err)
	}
	nodeID := uuid.NewString()
	presence := &pb.PresenceValue{SchemaVersion: 1, Present: &pb.PresenceValue_Node{Node: &pb.NodePresence{NodeId: nodeID, StartedAt: timestamppb.Now(), Ready: true}}}
	presenceData, _ := proto.Marshal(presence)
	if _, err := first.Presence.Put("node."+nodeID, presenceData); err != nil {
		t.Fatal(err)
	}
	for ctx.Err() == nil {
		positions, nodes, e := second.ObservationSnapshot(position.SessionId)
		if e == nil && len(positions) > 0 && len(nodes) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if ctx.Err() != nil {
		t.Fatal("KV observers did not receive live values")
	}
	if err := first.WaitApplied(ctx, ownerSeq); err != nil {
		t.Fatal(err)
	}
	if err := second.WaitApplied(ctx, ownerSeq); err != nil {
		t.Fatal(err)
	}
	zero := uint64(0)
	command := &pb.CommandRequest{
		ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref,
		Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, ExpectedEntityRevision: &zero,
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{
			UpdateEntity: &pb.UpdateEntity{Key: icao, Value: &pb.EntityRecord{
				Value: &pb.EntityRecord_AirportPolicy{AirportPolicy: &pb.AirportPolicy{Airport: icao}},
			}},
		}}},
	}
	writer := Writer{Store: store, NodeID: "node-a", Projection: first}
	reply := writer.Execute(ctx, command)
	if reply.Status != pb.CommandReply_COMMITTED {
		t.Fatalf("local projection barrier: %v", reply)
	}
	if err := second.WaitApplied(ctx, reply.GetStreamSequence()); err != nil {
		t.Fatal(err)
	}
	a, err := first.Read(ref)
	if err != nil {
		t.Fatal(err)
	}
	b, err := second.Read(ref)
	if err != nil {
		t.Fatal(err)
	}
	if a.Revision != b.Revision || !proto.Equal(a.Entities[icao], b.Entities[icao]) {
		t.Fatal("replicas diverged")
	}
	if err := first.Snapshots.Save(a); err != nil {
		t.Fatal(err)
	}
	expected, err := b.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProjectionChild$")
	child.Env = append(os.Environ(), "PROJECTION_CHILD_REF="+icao)
	output, err := child.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "PROJECTION:"+expected.Sha256) {
		t.Fatalf("independent process did not replay the same state: %v: %s", err, output)
	}
	// A newer verified checkpoint becomes corrupt after publication. The
	// reader must use the older index revision and replay remaining events.
	command2 := proto.Clone(command).(*pb.CommandRequest)
	command2.CommandId = uuid.NewString()
	command2.ExpectedEntityRevision = proto.Uint64(1)
	command2.GetSystem().GetUpdateEntity().Value.GetAirportPolicy().CdmConfigurationVersion = "two"
	reply = writer.Execute(ctx, command2)
	if reply.Status != pb.CommandReply_COMMITTED {
		t.Fatal(reply)
	}
	latest, err := first.Read(ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Snapshots.Save(latest); err != nil {
		t.Fatal(err)
	}
	key, _ := snapshotKey(ref)
	entry, err := first.Snapshots.Index.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	index := &pb.SnapshotIndex{}
	if err := pb.UnmarshalStrict(entry.Value(), index); err != nil {
		t.Fatal(err)
	}
	adminJS, err := admin.JetStream(nats.MaxWait(3 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	adminObjects, err := adminJS.ObjectStore(cfg.Names.Objects)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adminObjects.PutBytes(index.ObjectName, []byte("corrupt")); err != nil {
		t.Fatal(err)
	}
	checkpoint, err := first.Snapshots.Load(ref)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Revision != a.Revision {
		t.Fatalf("did not fall back to previous verified snapshot: %d", checkpoint.Revision)
	}
	third := startProjection(t, ctx, nc, cfg)
	if err := third.WaitApplied(ctx, reply.GetStreamSequence()); err != nil {
		t.Fatal(err)
	}
	recovered, err := third.Read(ref)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Revision != latest.Revision || !proto.Equal(recovered.Entities[icao], latest.Entities[icao]) {
		t.Fatal("corrupt snapshot recovery diverged")
	}
}

func TestProjectionChild(t *testing.T) {
	icao := os.Getenv("PROJECTION_CHILD_REF")
	if icao == "" {
		t.Skip("subprocess helper")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg := natsresources.Config{URLs: []string{"nats://backend:backend-local-only@127.0.0.1:4222"}, ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
	nc, err := natsresources.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	p := startProjection(t, ctx, nc, cfg)
	ref := &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: icao}}}
	state, err := p.Read(ref)
	if err != nil {
		t.Fatal(err)
	}
	s, err := state.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("PROJECTION:" + s.Sha256)
}

func startProjection(t *testing.T, ctx context.Context, nc *nats.Conn, cfg natsresources.Config) *Projection {
	t.Helper()
	p, err := NewProjection(nc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = p.Run(ctx) }()
	for ctx.Err() == nil {
		if p.Ready() == nil {
			return p
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("projection did not become ready: %v (last readiness error: %v)", ctx.Err(), p.Ready())
	return nil
}
