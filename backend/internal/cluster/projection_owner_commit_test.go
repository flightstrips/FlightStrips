package cluster

import (
	"context"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func ownerCommitProjectionFixture(t *testing.T) (*memoryStore, Writer, *pb.AggregateRef, *Projection) {
	t.Helper()
	store, writer, ref := fixture(t)
	p := &Projection{states: map[string]*Aggregate{}, listeners: map[uint64]*projectionListener{},
		lastSnapshot: map[string]time.Time{}, sinceSnapshot: map[string]uint64{}, started: true,
		checked: time.Now(), positionReady: true, presenceReady: true, highWater: 1}
	require.NoError(t, p.apply(store.entries[0]))
	return store, writer, ref, p
}

func TestOwnerLocalCommitPreservesOrderedReplayAndSingleDelta(t *testing.T) {
	store, writer, ref, p := ownerCommitProjectionFixture(t)
	_, updates, closeFn, err := p.SubscribeInitial(ref)
	require.NoError(t, err)
	defer closeFn()
	other := &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: "EKCH"}}}
	claim := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: other,
		Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "node-b"},
		Fact:  &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-b", Epoch: 1}}}
	data, err := proto.Marshal(claim)
	require.NoError(t, err)
	_, err = store.Publish(context.Background(), "fs.v1.state.airport.EKCH", 0, data)
	require.NoError(t, err)
	reply := writer.Execute(context.Background(), command(ref, "local", 0))
	require.Equal(t, pb.CommandReply_COMMITTED, reply.Status)
	entry := store.entries[2]
	p.highWater = entry.StreamSequence
	rawTime := p.lastAppliedServerTime
	require.NoError(t, p.applyCommitted(entry, 1))
	require.Equal(t, uint64(1), p.applied, "local subject commit must not move ordered global cursor")
	require.Equal(t, rawTime, p.lastAppliedServerTime)
	require.Equal(t, uint64(1), p.states[entry.Subject].Revision)
	require.Equal(t, entry.StreamSequence, (<-updates).StreamSequence)
	// Repeating the receipt or encountering it in raw replay must not duplicate
	// frontend publication or reduce the retained command twice.
	require.NoError(t, p.applyCommitted(entry, 1))
	require.NoError(t, p.apply(store.entries[1]))
	require.Equal(t, "node-b", p.states["fs.v1.state.airport.EKCH"].Owner.NodeId)
	require.Equal(t, uint64(2), p.applied)
	p.mu.Lock()
	p.stateChanged = make(chan struct{})
	changed := p.stateChanged
	p.mu.Unlock()
	require.NoError(t, p.apply(entry))
	select {
	case <-changed:
	default:
		t.Fatal("raw replay of a locally installed receipt did not wake global cursor waiters")
	}
	require.Equal(t, entry.StreamSequence, p.applied)
	require.Equal(t, entry.ServerTime, p.lastAppliedServerTime)
	require.NoError(t, p.WaitApplied(context.Background(), entry.StreamSequence))
	select {
	case delta := <-updates:
		t.Fatalf("duplicate committed delta: %v", delta)
	default:
	}
	require.Equal(t, uint64(1), p.states[entry.Subject].Revision)
}

func TestOwnerLocalCommitRejectsChangedPredecessorAndExpiredReceipt(t *testing.T) {
	t.Run("changed predecessor", func(t *testing.T) {
		store, writer, ref, p := ownerCommitProjectionFixture(t)
		require.Equal(t, pb.CommandReply_COMMITTED, writer.Execute(context.Background(), command(ref, "local", 0)).Status)
		entry := store.entries[1]
		require.ErrorContains(t, p.applyCommitted(entry, 0), "predecessor changed")
		require.Zero(t, p.states[entry.Subject].Revision)
		require.Equal(t, uint64(1), p.applied)
		require.NoError(t, p.apply(entry), "raw replay remains available after failed fast path")
		require.Equal(t, uint64(1), p.states[entry.Subject].Revision)
	})
	t.Run("expired receipt", func(t *testing.T) {
		store, writer, ref, p := ownerCommitProjectionFixture(t)
		request := command(ref, "local", 0)
		require.Equal(t, pb.CommandReply_COMMITTED, writer.Execute(context.Background(), request).Status)
		entry := store.entries[1]
		entry.ServerTime = p.states[entry.Subject].Owner.LeaseUntil.AsTime().Add(time.Nanosecond)
		require.NoError(t, p.applyCommitted(entry, 1), "expired durable event is an accepted checkpoint, not a domain commit")
		require.Nil(t, p.states[entry.Subject].Ledger[request.CommandId])
		require.Zero(t, p.states[entry.Subject].Revision)
		require.Equal(t, uint64(1), p.applied)
		require.NoError(t, p.apply(entry))
		require.Equal(t, uint64(2), p.applied)
		require.Nil(t, p.states[entry.Subject].Ledger[request.CommandId])
	})
}

func TestOwnerPlanningReadAllowsGlobalLagButRetainsHealthAndIdentity(t *testing.T) {
	_, _, ref, p := ownerCommitProjectionFixture(t)
	p.highWater = 100
	state, err := p.ReadOwned(ref, "node-a")
	require.NoError(t, err)
	state.Owner.NodeId = "mutated"
	require.Equal(t, "node-a", p.states["fs.v1.state.global"].Owner.NodeId, "planning values must be detached")
	_, err = p.ReadOwned(ref, "node-b")
	require.ErrorContains(t, err, "owner changed")
	p.checked = time.Now().Add(-3 * time.Second)
	_, err = p.ReadOwned(ref, "node-a")
	require.Error(t, err, "global lag permission must not waive broker metadata health")
	p.checked = time.Now()
	p.positionReady = false
	_, err = p.ReadOwned(ref, "node-a")
	require.Error(t, err, "position replay initialization remains required")
}
