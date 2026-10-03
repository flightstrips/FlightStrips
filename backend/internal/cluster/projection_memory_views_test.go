package cluster

import (
	"context"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/stretchr/testify/require"
)

func TestOwnerMemoryViewsDetachPendingStateAndDeliverOnce(t *testing.T) {
	owners, projection, ref, gate, _ := asyncOwnersFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	accept := func(revision uint64, name string) {
		require.NoError(t, owners.Execute(ctx, ref, func(turn context.Context) error {
			base, err := owners.Read(ref)
			if err != nil {
				return err
			}
			event := asyncDomainEvent(ref, revision)
			event.GetDomainChanged().Changes = []*pb.EntityChange{{Key: "42", Revision: revision,
				Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: 42, Airport: "EKCH", Name: name}}}}}}
			_, err = owners.AcceptState(turn, base, event)
			return err
		}))
	}
	accept(1, "pending")
	durable, err := projection.readOwnedDurable(ref, owners.owner.NodeID)
	require.NoError(t, err)
	require.Empty(t, durable.Entities, "pending entities reached durable state")
	entity, err := projection.ReadEntity(ref, pb.EntityKind_SESSION, "42")
	require.NoError(t, err)
	require.Equal(t, "pending", entity.GetValue().GetSession().Name)
	entity.GetValue().GetSession().Name = "caller mutation"
	again, err := projection.ReadEntity(ref, pb.EntityKind_SESSION, "42")
	require.NoError(t, err)
	require.Equal(t, "pending", again.GetValue().GetSession().Name)
	planning, err := projection.ReadLifecyclePlanning(ref)
	require.NoError(t, err)
	require.Empty(t, planning.Ledger)
	require.Empty(t, planning.Effects)
	cached, err := projection.ReadLifecyclePlanningCached(ref, planning)
	require.NoError(t, err)
	require.Same(t, planning, cached)
	initial, updates, closeSub, err := projection.SubscribeInitial(ref)
	require.NoError(t, err)
	defer closeSub()
	require.Equal(t, "pending", initial.Indexes[pb.EntityKind_SESSION]["42"].GetValue().GetSession().Name)
	require.Equal(t, uint64(1), initial.StreamSequence, "a provisional checkpoint was exposed")
	accept(2, "next pending")
	select {
	case delta := <-updates:
		require.Equal(t, uint64(2), delta.AggregateRevision)
		require.Zero(t, delta.StreamSequence)
	case <-ctx.Done():
		t.Fatal("RAM update waited for background publication")
	}
	close(gate)
	require.NoError(t, owners.Drain(ctx))
	select {
	case delta := <-updates:
		t.Fatalf("durable replay duplicated RAM changes: %v", delta)
	default:
	}
}

func TestMemoryViewCannotOutliveRawOwnership(t *testing.T) {
	owners, projection, ref, gate, _ := asyncOwnersFixture(t)
	ctx := context.Background()
	require.NoError(t, owners.Execute(ctx, ref, func(context.Context) error { return nil }))
	subject, _ := Subject(ref)
	memory := owners.Control(ref)
	require.NotNil(t, memory)
	projection.mu.Lock()
	raw, err := cloneAggregate(projection.states[subject])
	require.NoError(t, err)
	raw.Owner.Epoch++
	projection.states[subject] = raw
	require.Same(t, raw, projection.acceptedStateLocked(subject))
	projection.mu.Unlock()
	require.Equal(t, uint64(1), memory.Owner.Epoch)
	close(gate)
	require.NoError(t, owners.Drain(ctx))
}
