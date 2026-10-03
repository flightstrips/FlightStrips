package cluster

import (
	"context"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/stretchr/testify/require"
)

func TestObservedInitialKeepsPendingMemoryOutcomeRevisionsContiguous(t *testing.T) {
	owners, projection, ref, gate, store := asyncOwnersFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	defer owners.cancel()
	subject, _ := Subject(ref)
	baseline := &pb.EntitySnapshot{Key: "42", Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{
		Session: &pb.Session{Id: 42, Airport: "EKCH", Name: "durable"},
	}}}
	projection.mu.Lock()
	projection.states[subject].Entities["42"] = baseline
	projection.states[subject].Indexes[pb.EntityKind_SESSION] = map[string]*pb.EntitySnapshot{"42": baseline}
	projection.mu.Unlock()
	accept := func(revision uint64, status pb.CommandOutcome_Status, name string) {
		t.Helper()
		require.NoError(t, owners.Execute(ctx, ref, func(turn context.Context) error {
			base, err := owners.Read(ref)
			if err != nil {
				return err
			}
			event := asyncDomainEvent(ref, revision)
			event.GetDomainChanged().Outcome.Status = status
			if status == pb.CommandOutcome_FAILED {
				event.GetDomainChanged().Outcome.ReasonCode = "REVISION_CONFLICT"
			} else {
				event.GetDomainChanged().Changes = []*pb.EntityChange{{Key: "42", Revision: base.Entities["42"].Revision + 1,
					Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_Session{
						Session: &pb.Session{Id: 42, Airport: "EKCH", Name: name},
					}}}}}
			}
			reply, err := owners.AcceptState(turn, base, event)
			if err == nil {
				require.True(t, reply.MemoryAccepted)
				require.Equal(t, status, reply.Outcome.Status)
			}
			return err
		}))
	}

	accept(1, pb.CommandOutcome_SUCCEEDED, "pending initial")
	select {
	case <-store.started:
	case <-ctx.Done():
		t.Fatal("background publisher did not reach the blocked store")
	}
	initial, updates, _, stop, err := projection.SubscribeObservedInitial(42)
	require.NoError(t, err)
	defer stop()
	require.Equal(t, uint64(1), initial.AggregateRevision)
	require.Equal(t, uint64(1), initial.StreamSequence, "initial must retain the durable broker checkpoint")
	require.Equal(t, "pending initial", initial.SessionName)
	require.Equal(t, "pending initial", initial.Entities[0].GetValue().GetSession().Name)
	initial.Entities[0].GetValue().GetSession().Name = "caller mutation"
	state, err := owners.Read(ref)
	require.NoError(t, err)
	require.Equal(t, "pending initial", state.Entities["42"].GetValue().GetSession().Name)

	accept(2, pb.CommandOutcome_FAILED, "")
	accept(3, pb.CommandOutcome_SUCCEEDED, "after conflict")
	last := initial.AggregateRevision
	for _, expected := range []uint64{2, 3} {
		select {
		case delta := <-updates:
			require.NotNil(t, delta)
			require.Equal(t, last+1, delta.AggregateRevision, "frontend exact revision-gap guard must remain satisfied")
			require.Equal(t, expected, delta.AggregateRevision)
			require.Zero(t, delta.StreamSequence)
			if expected == 2 {
				require.Empty(t, delta.Changes, "failed outcome still consumes and delivers its revision")
			} else {
				require.Equal(t, "after conflict", delta.Changes[0].GetUpsert().GetSession().Name)
			}
			last = delta.AggregateRevision
		case <-ctx.Done():
			t.Fatal("live delta waited for durable publication")
		}
	}
	projection.mu.RLock()
	require.Zero(t, projection.states[subject].Revision, "the publisher must still be blocked")
	projection.mu.RUnlock()
	close(gate)
	require.NoError(t, owners.Drain(ctx))
	select {
	case duplicate := <-updates:
		t.Fatalf("durable confirmation duplicated a RAM delta: %v", duplicate)
	default:
	}
}
