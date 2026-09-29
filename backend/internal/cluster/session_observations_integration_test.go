package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strconv"
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

func TestNATSSessionObservationsAcrossNodesAndRestart(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	basePort := 4222
	if text := os.Getenv("NATS_TEST_PORT_BASE"); text != "" {
		var err error
		basePort, err = strconv.Atoi(text)
		if err != nil {
			t.Fatal(err)
		}
	}
	url := func(user string, offset int) string {
		return fmt.Sprintf("nats://%s:%s-local-only@127.0.0.1:%d", user, user, basePort+offset)
	}
	cfg := natsresources.Config{URLs: []string{url("bootstrap", 0), url("bootstrap", 1), url("bootstrap", 2)},
		ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
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
	cfg.URLs = []string{url("backend", 0)}
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
	claim := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"},
		Fact: &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
	data, _ := proto.Marshal(claim)
	if _, err := store.Publish(ctx, subject, 0, data); err != nil {
		t.Fatal(err)
	}
	first := startProjection(t, ctx, nc, cfg)
	second := startProjection(t, ctx, nc, cfg)
	writer := Writer{Store: store, NodeID: "node-a", Projection: first, Plan: PlanStrip}
	adapter := SessionObservations{Store: LocalLifecycleStore{Writer: writer}}
	zero := uint64(0)
	session := &pb.Session{Id: id, Airport: "EKCH", Name: "LIVE", NextStripId: 1, NextMessageId: 1,
		Master: &pb.MasterTerm{ConnectionId: "master-" + random.String(), Cid: "123", Epoch: 1, OwnerEpoch: 1}}
	seed := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, ExpectedEntityRevision: &zero,
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: fmt.Sprint(id), Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: session}}}}}}}
	if reply := writer.Execute(ctx, seed); reply.Status != pb.CommandReply_COMMITTED {
		t.Fatal(reply)
	}
	node := &pb.PresenceValue{SchemaVersion: 1, Present: &pb.PresenceValue_Node{Node: &pb.NodePresence{NodeId: "node-a", Ready: true, StartedAt: timestamppb.Now()}}}
	client := &pb.PresenceValue{SchemaVersion: 1, Present: &pb.PresenceValue_Client{Client: &pb.ClientPresence{
		ConnectionId: session.Master.ConnectionId, NodeId: "node-a", SessionId: id, Cid: "123", Kind: pb.ClientPresence_EUROSCOPE, ConnectedAt: timestamppb.Now()}}}
	for key, value := range map[string]*pb.PresenceValue{"node.node-a": node, "client." + session.Master.ConnectionId: client} {
		encoded, _ := proto.Marshal(value)
		if _, err := first.Presence.Put(key, encoded); err != nil {
			t.Fatal(err)
		}
	}
	syncReply, err := adapter.RecordSync(ctx, id, uuid.NewString(), &pb.SessionSync{ConnectionId: session.Master.ConnectionId, MasterEpoch: 1, CompletedAt: timestamppb.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := second.WaitApplied(ctx, syncReply.GetStreamSequence()); err != nil {
		t.Fatal(err)
	}
	_, firstDeltas, firstObservations, stopFirstDeltas, err := first.SubscribeObservedInitial(id)
	if err != nil {
		t.Fatal(err)
	}
	defer stopFirstDeltas()
	_, secondDeltas, secondObservations, stopSecondDeltas, err := second.SubscribeObservedInitial(id)
	if err != nil {
		t.Fatal(err)
	}
	defer stopSecondDeltas()
	if _, err := adapter.SendMessage(ctx, id, uuid.NewString(), "123", "hello", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.PutAtis(ctx, id, uuid.NewString(), &pb.Atis{Airport: "EKCH", Metar: "EKCH 291900Z", ArrivalCode: "A", DepartureCode: "B", ObservedAt: timestamppb.Now()}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.PutClxOverride(ctx, id, uuid.NewString(), &pb.ClxOverride{Callsign: "SAS101", Key: "SID", Actor: "123", CreatedAt: timestamppb.Now()}, 0); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		var a, b *pb.FrontendDelta
		select {
		case a = <-firstDeltas:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		select {
		case b = <-secondDeltas:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		if !proto.Equal(a, b) {
			t.Fatalf("backend live deltas diverged: %v %v", a, b)
		}
	}
	var synced bool
	for ctx.Err() == nil {
		a, _ := first.OperationalSync(ref)
		b, _ := second.OperationalSync(ref)
		if a != nil && b != nil {
			synced = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !synced {
		t.Fatal("master sync not visible on both backends")
	}
	positions, err := NewPositionWriter(first.Positions, id, 1, session.Master.ConnectionId, first.PositionAuthority("node-a"), 2, 32)
	if err != nil {
		t.Fatal(err)
	}
	defer positions.Close(context.Background())
	receipt, err := positions.QueuePosition(ctx, "SAS101", &pb.AircraftPosition{Latitude: 55.6, Longitude: 12.6}, time.Now())
	if err != nil || (<-receipt).Err != nil {
		t.Fatalf("position write: %v", err)
	}
	positionDelta := func(channel <-chan *pb.FrontendObservation) *pb.FrontendObservation {
		for {
			select {
			case observation := <-channel:
				if observation.GetPosition() != nil && observation.GetPosition().AircraftKey == "SAS101" {
					return observation
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
	}
	firstPosition, secondPosition := positionDelta(firstObservations), positionDelta(secondObservations)
	if !proto.Equal(firstPosition, secondPosition) || firstPosition.Stale || firstPosition.SourceRevision == 0 {
		t.Fatalf("backend position observations diverged: %v %v", firstPosition, secondPosition)
	}
	var left, right *pb.FrontendInitial
	for ctx.Err() == nil {
		var closeLeft, closeRight func()
		left, _, _, closeLeft, err = first.SubscribeObservedInitial(id)
		if err != nil {
			t.Fatal(err)
		}
		right, _, _, closeRight, err = second.SubscribeObservedInitial(id)
		if err != nil {
			t.Fatal(err)
		}
		closeLeft()
		closeRight()
		if proto.Equal(left, right) && len(left.Positions) == 1 && len(left.Entities) >= 4 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if ctx.Err() != nil || !proto.Equal(left, right) || !left.Writable || len(left.TaggedObservations) < 3 {
		t.Fatalf("backends diverged: left=%v right=%v error=%v", left, right, ctx.Err())
	}
	// A new process replays retained state and KV, but its old sync marker is
	// stale until this incarnation observes a new full master sync.
	restarted := startProjection(t, ctx, nc, cfg)
	initial, _, _, closeInitial, err := restarted.SubscribeObservedInitial(id)
	if err != nil {
		t.Fatal(err)
	}
	closeInitial()
	if initial.Writable || len(initial.Positions) != 1 || !initial.TaggedObservations[0].Stale {
		t.Fatalf("restart reused prior sync: %v", initial)
	}
	encodedInitial, err := (proto.MarshalOptions{Deterministic: true}).Marshal(initial)
	if err != nil {
		t.Fatal(err)
	}
	checksum := sha256.Sum256(encodedInitial)
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSessionObservationChild$")
	child.Env = append(os.Environ(), "NATS_OBSERVATION_CHILD_SESSION="+strconv.Itoa(int(id)), "NATS_TEST_PORT_BASE="+strconv.Itoa(basePort))
	output, err := child.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "SESSION_OBSERVATION:"+hex.EncodeToString(checksum[:])) {
		t.Fatalf("separate backend process observed different restart state: %v: %s", err, output)
	}
	for key, value := range map[string]*pb.PresenceValue{"node.node-a": node, "client." + session.Master.ConnectionId: client} {
		encoded, _ := proto.Marshal(value)
		if _, err := first.Presence.Put(key, encoded); err != nil {
			t.Fatal(err)
		}
	}
	newSync, err := adapter.RecordSync(ctx, id, uuid.NewString(), &pb.SessionSync{ConnectionId: session.Master.ConnectionId, MasterEpoch: 1, CompletedAt: timestamppb.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.WaitApplied(ctx, newSync.GetStreamSequence()); err != nil {
		t.Fatal(err)
	}
	for ctx.Err() == nil {
		if sync, _ := restarted.OperationalSync(ref); sync != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if ctx.Err() != nil {
		t.Fatal("fresh master sync did not restore operational state")
	}
	initial, _, _, closeInitial, err = restarted.SubscribeObservedInitial(id)
	if err != nil {
		t.Fatal(err)
	}
	closeInitial()
	if !initial.Writable || len(initial.Positions) != 1 || initial.TaggedObservations[0].Stale {
		t.Fatalf("fresh sync did not clear stale observation: %v", initial)
	}
}

func TestSessionObservationChild(t *testing.T) {
	text := os.Getenv("NATS_OBSERVATION_CHILD_SESSION")
	if text == "" {
		t.Skip("child process only")
	}
	id, err := strconv.Atoi(text)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(os.Getenv("NATS_TEST_PORT_BASE"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg := natsresources.Config{URLs: []string{fmt.Sprintf("nats://backend:backend-local-only@127.0.0.1:%d", port)},
		ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
	nc, err := natsresources.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	p := startProjection(t, ctx, nc, cfg)
	initial, _, _, stop, err := p.SubscribeObservedInitial(int32(id))
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	encoded, err := (proto.MarshalOptions{Deterministic: true}).Marshal(initial)
	if err != nil {
		t.Fatal(err)
	}
	checksum := sha256.Sum256(encoded)
	fmt.Println("SESSION_OBSERVATION:" + hex.EncodeToString(checksum[:]))
}
