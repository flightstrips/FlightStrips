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

func TestAsyncAcceptanceCopyPreservesPublishedEntitiesIndexesAndRenewedOwner(t *testing.T) {
	m, p, ref, gate, store := asyncOwnersFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	released := false
	t.Cleanup(func() {
		if !released {
			close(gate)
		}
		flush, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = m.Drain(flush)
	})
	subject := mustAsyncSubject(ref)
	p.mu.Lock()
	state := p.states[subject]
	state.Entities["42"] = &pb.EntitySnapshot{Key: "42", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: 42, Airport: "EKCH", Name: "LIVE", NextStripId: 2}}}}
	state.Entities["SAS199"] = &pb.EntitySnapshot{Key: "SAS199", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Id: 1, Revision: 1, Callsign: "SAS199", Bay: "CLEARED", Sequence: 1, Remarks: "before"}}}}
	state.rebuildIndexes()
	p.mu.Unlock()
	require.NoError(t, m.Execute(ctx, ref, func(context.Context) error { return nil }))
	before := m.Control(ref)
	beforeSnapshot := copyTestSnapshot(t, before)
	upsert := asyncDomainEvent(ref, 1)
	strip := proto.Clone(before.Entities["SAS199"].Value).(*pb.EntityRecord)
	strip.GetStrip().Remarks = "after"
	strip.GetStrip().Revision = 2
	upsert.GetDomainChanged().Changes = []*pb.EntityChange{{Key: "SAS199", Revision: 2, Operation: &pb.EntityChange_Upsert{Upsert: strip}}}
	accept := func(event *pb.StateEvent) error {
		return m.Execute(ctx, ref, func(turn context.Context) error {
			base, err := m.Read(ref)
			if err != nil {
				return err
			}
			_, err = m.AcceptState(turn, base, event)
			return err
		})
	}
	require.NoError(t, accept(upsert))
	select {
	case <-store.started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	changed := m.Control(ref)
	changedSnapshot := copyTestSnapshot(t, changed)
	require.Equal(t, "after", changed.Indexes[pb.EntityKind_STRIP]["SAS199"].Value.GetStrip().Remarks)
	require.Equal(t, "before", before.Entities["SAS199"].Value.GetStrip().Remarks)
	require.Zero(t, changed.Ledger[upsert.GetCommandId()].CommittedStreamSequence)
	// Mutating the caller's original event must not change either the accepted
	// graph or the immutable event subsequently published by the FIFO worker.
	upsert.GetDomainChanged().Changes[0].GetUpsert().GetStrip().Remarks = "caller mutation"
	upsert.Actor.Id = "caller mutation"
	renew := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "node-a"}, Fact: &pb.StateEvent_OwnerRenewed{OwnerRenewed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
	data, err := proto.Marshal(renew)
	require.NoError(t, err)
	sequence, err := store.memoryStore.Publish(ctx, subject, 1, data)
	require.NoError(t, err)
	entry, err := store.Committed(ctx, sequence)
	require.NoError(t, err)
	require.NoError(t, p.apply(entry))
	remove := asyncDomainEvent(ref, 2)
	remove.GetDomainChanged().Changes = []*pb.EntityChange{{Key: "SAS199", Revision: 3, Operation: &pb.EntityChange_Delete{Delete: &pb.DeleteEntity{Kind: pb.EntityKind_STRIP}}}}
	require.NoError(t, accept(remove))
	latest := m.Control(ref)
	require.Nil(t, latest.Indexes[pb.EntityKind_STRIP]["SAS199"])
	require.NotEqual(t, changed.Owner.LeaseUntil, latest.Owner.LeaseUntil)
	require.True(t, proto.Equal(beforeSnapshot, copyTestSnapshot(t, before)))
	require.True(t, proto.Equal(changedSnapshot, copyTestSnapshot(t, changed)))
	close(gate)
	released = true
	require.NoError(t, m.FlushSession(ctx, ref))
	durable, err := m.Read(ref)
	require.NoError(t, err)
	require.Nil(t, durable.Entities["SAS199"])
	require.Equal(t, "test", durable.Ledger[upsert.GetCommandId()].Actor.Id)
	require.NotZero(t, durable.Ledger[upsert.GetCommandId()].CommittedStreamSequence)
	require.True(t, proto.Equal(changedSnapshot, copyTestSnapshot(t, changed)), "durable receipts must not mutate the prior pending outcome")
}

func TestAsyncAcceptanceCopyEffectUpdateDetachesPriorOutcome(t *testing.T) {
	m, p, ref, gate, _ := asyncOwnersFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	released := false
	t.Cleanup(func() {
		if !released {
			close(gate)
		}
		flush, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = m.Drain(flush)
	})
	effect := seedAsyncEffect(t, p, ref, true)
	subject := mustAsyncSubject(ref)
	p.mu.Lock()
	p.states[subject].Ledger[effect.CommandId].CommittedStreamSequence = 1
	p.mu.Unlock()
	require.NoError(t, m.Execute(ctx, ref, func(context.Context) error { return nil }))
	prior := m.Control(ref)
	priorSnapshot := copyTestSnapshot(t, prior)
	updated := proto.Clone(effect).(*pb.EffectRecord)
	updated.Status = pb.EffectRecord_EXECUTED
	id := effect.CommandId
	event := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, AggregateRevision: 1, OwnerEpoch: 1, CommandId: &id, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "node-a"}, Fact: &pb.StateEvent_EffectChanged{EffectChanged: updated}}
	require.NoError(t, m.Execute(ctx, ref, func(turn context.Context) error {
		base, err := m.Read(ref)
		if err != nil {
			return err
		}
		_, err = m.AcceptState(turn, base, event)
		return err
	}))
	next := m.Control(ref)
	require.Equal(t, pb.EffectRecord_EXECUTED, next.Effects[id].Status)
	require.Equal(t, pb.CommandOutcome_SUCCEEDED, next.Ledger[id].Status)
	require.Zero(t, next.Ledger[id].CommittedStreamSequence)
	require.Equal(t, pb.EffectRecord_DISPATCH_CLAIMED, prior.Effects[id].Status)
	require.Equal(t, pb.CommandOutcome_ACCEPTED, prior.Ledger[id].Status)
	require.Equal(t, uint64(1), prior.Ledger[id].CommittedStreamSequence)
	require.True(t, proto.Equal(priorSnapshot, copyTestSnapshot(t, prior)))
	close(gate)
	released = true
	require.NoError(t, m.FlushSession(ctx, ref))
	require.True(t, proto.Equal(priorSnapshot, copyTestSnapshot(t, prior)))
}
