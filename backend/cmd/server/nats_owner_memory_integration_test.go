package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/natsresources"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type memoryOwnerPublishGate struct {
	cluster.NATSStore
	release chan struct{}
	reached chan struct{}
	once    sync.Once
}

func (s *memoryOwnerPublishGate) Publish(ctx context.Context, subject string, expected uint64, data []byte) (uint64, error) {
	s.once.Do(func() { close(s.reached) })
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-s.release:
	}
	return s.NATSStore.Publish(ctx, subject, expected, data)
}

// These assertions deliberately distinguish acknowledged execution in RAM from
// durability. An abrupt process death may lose the acknowledged pending tail.
// All brokers and compiled application processes belong to this fixture.
func TestServerNATSOwnerMemoryAcceptanceDrainAndCrashTail(t *testing.T) {
	if os.Getenv("NATS_TASK22") != "1" {
		t.Skip("requires explicit disposable Task22 fixture")
	}
	f := newEntrypointFixture(t, true)
	_, ref, _ := f.seededSession()
	actor := &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "111111", SessionId: proto.Int32(ref.GetSession().Id)}
	marked := func(id string, value bool) *pb.CommandRequest {
		strip := f.state(ref).Indexes[pb.EntityKind_STRIP]["SAS123"]
		frame := markedCommand(id, "SAS123", strip.Revision, value)
		return &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: ref, Actor: actor, ExpectedEntityRevision: frame.GetCommand().ExpectedEntityRevision, Command: &pb.CommandRequest_Client{Client: frame.GetCommand().Action}}
	}
	assertBlockedReply := func(request *pb.CommandRequest) *pb.CommandReply {
		f.arm("before-publish-domain", request.CommandId)
		started := time.Now()
		reply := f.route(request)
		require.Equal(t, pb.CommandReply_COMMITTED, reply.Status)
		require.Equal(t, pb.CommandOutcome_SUCCEEDED, reply.GetOutcome().GetStatus())
		require.True(t, reply.MemoryAccepted, "blocked persistence must return explicit RAM acceptance")
		require.Nil(t, reply.StreamSequence)
		require.Zero(t, reply.GetOutcome().CommittedStreamSequence)
		f.await("background persistence reached its existing fault gate", func() bool {
			data, err := os.ReadFile(filepath.Join(f.dir, "gate", "reached.json"))
			var checkpoint struct {
				CommandID string `json:"command_id"`
				Point     string `json:"point"`
			}
			return err == nil && json.Unmarshal(data, &checkpoint) == nil && checkpoint.CommandID == request.CommandId && checkpoint.Point == "before-publish-domain"
		})
		require.Nil(t, f.state(ref).Ledger[request.CommandId], "observer replay must not invent durability for RAM acceptance")
		t.Logf("MEMORY_ACCEPTED command_id=%s elapsed=%s stream_sequence_absent=true", request.CommandId, time.Since(started))
		return reply
	}

	persistedID := uuid.NewString()
	assertBlockedReply(marked(persistedID, true))
	require.NoError(t, os.Remove(filepath.Join(f.dir, "gate", "control.json")))
	f.await("released RAM command durable on independent observer", func() bool {
		s, err := f.projection.ReadDurable(ref)
		return err == nil && s.Ledger[persistedID] != nil && s.Indexes[pb.EntityKind_STRIP]["SAS123"].Value.GetStrip().Marked
	})
	persisted := f.state(ref).Ledger[persistedID].CommittedStreamSequence
	require.Positive(t, persisted)

	lostID := uuid.NewString()
	before := f.state(ref).Owner
	assertBlockedReply(marked(lostID, false))
	dead := f.killAt("before-publish-domain", lostID)
	f.await("survivor acquires a fresh owner generation", func() bool {
		s, err := f.projection.ReadDurable(ref)
		return err == nil && s.Owner != nil && s.Owner.Epoch > before.Epoch && s.Owner.NodeId != before.NodeId
	})
	state := f.state(ref)
	require.Nil(t, state.Ledger[lostID], "crash must not manufacture the unpersisted accepted tail")
	require.Equal(t, persisted, state.Ledger[persistedID].CommittedStreamSequence)
	require.True(t, state.Indexes[pb.EntityKind_STRIP]["SAS123"].Value.GetStrip().Marked)
	f.restart(dead)
	state = f.state(ref)
	require.Nil(t, state.Ledger[lostID], "new process must rebuild only broker-confirmed state")
	require.True(t, state.Indexes[pb.EntityKind_STRIP]["SAS123"].Value.GetStrip().Marked)

	// Exercise the same queue's orderly Drain against real broker CAS and server
	// timestamps. Owner renewal remains running while persistence is blocked.
	for _, app := range f.apps {
		app.stop()
	}
	nc, err := natsresources.Connect(f.resources)
	require.NoError(t, err)
	defer nc.Close()
	projection, err := cluster.NewProjection(nc, f.resources)
	require.NoError(t, err)
	durableStore := cluster.NATSStore{JS: projection.JS}
	owner, err := cluster.NewOwnerRuntime(nc, projection, durableStore)
	require.NoError(t, err)
	require.NoError(t, owner.Track(ref))
	gate := &memoryOwnerPublishGate{NATSStore: durableStore, release: make(chan struct{}), reached: make(chan struct{})}
	owners := cluster.NewAsyncSessionOwners(projection, owner, gate)
	projection.Async = owners
	projectionCtx, stopProjection := context.WithCancel(f.ctx)
	projectionDone := make(chan error, 1)
	go func() { projectionDone <- projection.Run(projectionCtx) }()
	defer func() { stopProjection(); <-projectionDone }()
	f.await("queue projection complete durable baseline", func() bool { return projection.Ready() == nil })
	ownerCtx, stopOwner := context.WithCancel(f.ctx)
	ownerDone := make(chan error, 1)
	go func() { ownerDone <- owner.Run(ownerCtx) }()
	defer func() { stopOwner(); <-ownerDone }()
	f.await("queue owns the next durable owner epoch", func() bool { return owners.Active(ref) })
	writer := cluster.Writer{Store: gate, Projection: projection, Lease: owner, NodeID: owner.NodeID, Plan: func(context.Context, *pb.CommandRequest, *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		return &pb.DomainChange{}, pb.CommandReply_COMMITTED, 0, nil
	}}
	id := uuid.NewString()
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "task22-memory-drain"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_CreateSession{CreateSession: &pb.CreateSession{Id: ref.GetSession().Id, Airport: "EKCH", Name: "native-drain"}}}}}
	reply := writer.Execute(f.ctx, request)
	require.True(t, reply.MemoryAccepted, fmt.Sprintf("owner execution rejected: %v", reply))
	require.Nil(t, reply.StreamSequence)
	select {
	case <-gate.reached:
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
	}
	short, cancel := context.WithTimeout(f.ctx, 50*time.Millisecond)
	require.ErrorIs(t, owners.Drain(short), context.DeadlineExceeded)
	cancel()
	raw, err := projection.ReadDurable(ref)
	require.NoError(t, err)
	require.Nil(t, raw.Ledger[id])
	close(gate.release)
	flush, cancelFlush := context.WithTimeout(f.ctx, 10*time.Second)
	defer cancelFlush()
	require.NoError(t, owners.Drain(flush))
	f.await("gracefully drained command durably replayed by observer", func() bool {
		s, err := f.projection.ReadDurable(ref)
		return err == nil && s.Ledger[id] != nil && s.Ledger[id].CommittedStreamSequence > 0
	})
	require.Error(t, owners.Execute(f.ctx, ref, func(context.Context) error { return nil }), "sealed runtime must reject new turns")
	t.Logf("MEMORY_DRAIN command_id=%s durable_sequence=%d pending=%t", id, f.state(ref).Ledger[id].CommittedStreamSequence, owners.Pending(ref))
}
