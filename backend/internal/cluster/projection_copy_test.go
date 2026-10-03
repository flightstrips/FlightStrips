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

func TestProjectionApplyPreservesPublishedState(t *testing.T) {
	store, writer, ref := fixture(t)
	require.Equal(t, pb.CommandReply_COMMITTED, writer.Execute(context.Background(), command(ref, "First", 0)).Status)
	require.Equal(t, pb.CommandReply_COMMITTED, writer.Execute(context.Background(), command(ref, "Second", 1)).Status)
	entries, err := store.Replay(context.Background(), "fs.v1.state.global")
	require.NoError(t, err)
	state := NewAggregate(ref)
	for _, entry := range entries {
		previous := copyTestSnapshot(t, state)
		next := copyAggregateForApply(state)
		_, err := next.Apply(entry)
		require.NoError(t, err)
		require.True(t, proto.Equal(previous, copyTestSnapshot(t, state)), "applying a new event changed a published snapshot")
		state = next
	}
	previous := copyTestSnapshot(t, state)
	event := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, AggregateRevision: state.Revision, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "node-a"}, Fact: &pb.StateEvent_OwnerRenewed{OwnerRenewed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
	data, err := proto.Marshal(event)
	require.NoError(t, err)
	next := copyAggregateForApply(state)
	accepted, err := next.Apply(AppliedEvent{Subject: "fs.v1.state.global", StreamSequence: state.StreamSequence + 1, SubjectSequence: state.SubjectSequence + 1, ServerTime: entries[len(entries)-1].ServerTime.Add(time.Second), Data: data})
	require.NoError(t, err)
	require.True(t, accepted)
	require.False(t, proto.Equal(next.Owner, state.Owner), "renewal must actually advance the lease")
	require.True(t, proto.Equal(previous, copyTestSnapshot(t, state)), "lease renewal changed the previous published owner")
}

func TestWriterProjectionOutcomeIsDetachedAndActorBound(t *testing.T) {
	store, writer, ref := fixture(t)
	ctx := context.Background()
	request := command(ref, "Copenhagen", 0)
	committed := writer.Execute(ctx, request)
	require.Equal(t, pb.CommandReply_COMMITTED, committed.Status)
	subject, _ := Subject(ref)
	entries, err := store.Replay(ctx, subject)
	require.NoError(t, err)
	state := NewAggregate(ref)
	for _, entry := range entries {
		_, err := state.Apply(entry)
		require.NoError(t, err)
	}
	p := &Projection{states: map[string]*Aggregate{subject: state}, started: true, checked: time.Now(), positionReady: true, presenceReady: true, highWater: state.StreamSequence, applied: state.StreamSequence}
	writer.Projection = p
	got := writer.Outcome(ctx, ref, request.CommandId, request.Actor)
	require.Equal(t, pb.CommandReply_COMMITTED, got.Status)
	require.True(t, proto.Equal(committed.Outcome, got.Outcome))
	got.Outcome.Actor.Id = "mutated"
	require.Equal(t, pb.CommandReply_COMMITTED, writer.Outcome(ctx, ref, request.CommandId, request.Actor).Status)
	other := proto.Clone(request.Actor).(*pb.Actor)
	other.Id = "other"
	require.Equal(t, pb.CommandReply_UNAUTHORIZED, writer.Outcome(ctx, ref, request.CommandId, other).Status)
	p.positionReady = false
	require.Equal(t, pb.CommandReply_UNAVAILABLE, writer.Outcome(ctx, ref, request.CommandId, request.Actor).Status)
}

func TestProjectionReadCopyDetachesValuesAndIndexes(t *testing.T) {
	store, writer, ref := fixture(t)
	require.Equal(t, pb.CommandReply_COMMITTED, writer.Execute(context.Background(), command(ref, "Copenhagen", 0)).Status)
	entries, err := store.Replay(context.Background(), "fs.v1.state.global")
	require.NoError(t, err)
	state := NewAggregate(ref)
	for _, entry := range entries {
		_, err := state.Apply(entry)
		require.NoError(t, err)
	}
	state.Master = &pb.MasterTerm{ConnectionId: "connection"}
	state.Sync = &pb.SessionSync{ConnectionId: "connection"}
	state.Workflows["workflow"] = &pb.WorkflowRecord{DerivedCommandId: "derived"}
	state.Effects["effect"] = &pb.EffectRecord{CommandId: "effect"}
	previous := copyTestSnapshot(t, state)
	copied, err := cloneAggregate(state)
	require.NoError(t, err)
	require.True(t, proto.Equal(previous, copyTestSnapshot(t, copied)))
	copied.Ref.Target = nil
	copied.Owner.NodeId = "changed"
	copied.Master.ConnectionId = "changed"
	copied.Sync.ConnectionId = "changed"
	for _, value := range copied.Entities {
		value.Value.GetAirportRegistry().DisplayName = "changed"
	}
	for _, value := range copied.Ledger {
		value.Actor.Id = "changed"
	}
	copied.Workflows["workflow"].DerivedCommandId = "changed"
	copied.Effects["effect"].CommandId = "changed"
	delete(copied.Indexes[pb.EntityKind_AIRPORT_REGISTRY], "EKCH")
	require.NotNil(t, state.Indexes[pb.EntityKind_AIRPORT_REGISTRY]["EKCH"])
	require.True(t, proto.Equal(previous, copyTestSnapshot(t, state)), "mutating a read copy changed published state")
}

func TestProjectionApplyPreservesPriorEffectAndThrottle(t *testing.T) {
	state, at, id := squawkReducerState(t)
	before := copyTestSnapshot(t, state)
	next := copyAggregateForApply(state)
	applyEffectTestEvent(t, next, transitionEvent(next, id, pb.EffectRecord_DISPATCH_CLAIMED, "socket-1"), at.Add(time.Second))
	require.Equal(t, pb.EffectRecord_DISPATCH_CLAIMED, next.Effects[id].Status)
	require.NotEmpty(t, next.Indexes[pb.EntityKind_SESSION_SQUAWK_THROTTLE])
	require.True(t, proto.Equal(before, copyTestSnapshot(t, state)))
	before = copyTestSnapshot(t, next)
	terminal := copyAggregateForApply(next)
	applyEffectTestEvent(t, terminal, transitionEvent(terminal, id, pb.EffectRecord_UNKNOWN, ""), at.Add(32*time.Second))
	require.Equal(t, pb.CommandOutcome_UNKNOWN, terminal.Ledger[id].Status)
	require.True(t, proto.Equal(before, copyTestSnapshot(t, next)))
}

func copyTestSnapshot(t *testing.T, state *Aggregate) *pb.Snapshot {
	t.Helper()
	snapshot, err := state.Snapshot()
	require.NoError(t, err)
	return snapshot
}

func TestProjectionFocusedReadsDetachAndRejectLag(t *testing.T) {
	store, writer, ref := fixture(t)
	require.Equal(t, pb.CommandReply_COMMITTED, writer.Execute(context.Background(), command(ref, "Copenhagen", 0)).Status)
	entries, err := store.Replay(context.Background(), "fs.v1.state.global")
	require.NoError(t, err)
	state := NewAggregate(ref)
	for _, entry := range entries {
		_, err := state.Apply(entry)
		require.NoError(t, err)
	}
	p := &Projection{states: map[string]*Aggregate{"fs.v1.state.global": state}, started: true, checked: time.Now(), positionReady: true, presenceReady: true, applied: state.StreamSequence, highWater: state.StreamSequence}
	before := copyTestSnapshot(t, state)
	entities, err := p.ReadEntities(ref, pb.EntityKind_AIRPORT_REGISTRY)
	require.NoError(t, err)
	require.Len(t, entities, 1)
	entities[0].Value.GetAirportRegistry().DisplayName = "changed"
	entity, err := p.ReadEntity(ref, pb.EntityKind_AIRPORT_REGISTRY, "EKCH")
	require.NoError(t, err)
	require.Equal(t, "Copenhagen", entity.Value.GetAirportRegistry().DisplayName)
	entity.Value.GetAirportRegistry().DisplayName = "changed"
	owner, err := p.ReadOwner(ref)
	require.NoError(t, err)
	owner.LeaseUntil.Seconds++
	require.True(t, proto.Equal(before, copyTestSnapshot(t, state)))
	p.highWater++
	_, err = p.ReadEntity(ref, pb.EntityKind_AIRPORT_REGISTRY, "EKCH")
	require.Error(t, err)
	_, err = p.ReadEntities(ref, pb.EntityKind_AIRPORT_REGISTRY)
	require.Error(t, err)
	_, err = p.ReadOwner(ref)
	require.Error(t, err)
}
