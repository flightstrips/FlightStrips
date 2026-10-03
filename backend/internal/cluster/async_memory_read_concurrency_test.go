package cluster

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/stretchr/testify/require"
)

func TestAsyncOwnerDetachedReadsSurviveAcceptanceRebaseAndEpochReclaim(t *testing.T) {
	m, p, ref, gate, store := asyncOwnersFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	gateReleased := false
	t.Cleanup(func() {
		if !gateReleased {
			close(gate)
		}
		cancel()
		flush, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = m.Drain(flush)
	})
	p.mu.Lock()
	state := p.states[mustAsyncSubject(ref)]
	state.Entities["42"] = &pb.EntitySnapshot{Key: "42", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: 42, Airport: "EKCH", Name: "LIVE", NextStripId: 2}}}}
	state.Entities["SAS199"] = &pb.EntitySnapshot{Key: "SAS199", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Id: 1, Revision: 1, Callsign: "SAS199", Bay: "CLEARED", Sequence: 1}}}}
	state.rebuildIndexes()
	p.mu.Unlock()
	require.NoError(t, m.Execute(ctx, ref, func(context.Context) error { return nil }))
	before, err := m.Read(ref)
	require.NoError(t, err)
	initialControl := m.Control(ref)
	var ids []string
	accept := func(revision uint64) error {
		return m.Execute(ctx, ref, func(turn context.Context) error {
			base, err := m.Read(ref)
			if err != nil {
				return err
			}
			event := asyncDomainEvent(ref, revision)
			ids = append(ids, event.GetCommandId())
			_, err = m.AcceptState(turn, base, event)
			return err
		})
	}
	require.NoError(t, accept(1))
	select {
	case <-store.started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	pending, err := m.Read(ref)
	require.NoError(t, err)
	require.Equal(t, uint64(1), pending.Revision)
	require.Zero(t, pending.Ledger[ids[0]].CommittedStreamSequence)
	// Caller mutation must not alter the immutable graph used by later readers
	// or the accepted FIFO, even when a persistence rebase replaces that graph.
	pending.Owner.NodeId = "caller mutation"
	pending.Entities["SAS199"].GetValue().GetStrip().Bay = "caller mutation"
	pending.Ledger[ids[0]].Actor.Id = "caller mutation"
	var readers sync.WaitGroup
	defer readers.Wait()
	failures := make(chan error, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			<-start
			for j := 0; j < 32; j++ {
				view, err := m.Read(ref)
				if err != nil {
					failures <- err
					return
				}
				if view.Owner.NodeId != "node-a" || view.Owner.Epoch != 1 || view.Entities["SAS199"].GetValue().GetStrip().Bay != "CLEARED" || view.StreamSequence != 1 {
					failures <- fmt.Errorf("incoherent pending read: revision=%d owner=%v durable=%d", view.Revision, view.Owner, view.StreamSequence)
					return
				}
				for _, outcome := range view.Ledger {
					if outcome.Actor.Id != "test" || outcome.CommittedStreamSequence != 0 {
						failures <- fmt.Errorf("caller mutation or false durable receipt in revision %d", view.Revision)
						return
					}
				}
				view.Owner.Epoch = 999
				delete(view.Entities, "SAS199")
			}
		}()
	}
	close(start)
	for revision := uint64(2); revision <= 8; revision++ {
		require.NoError(t, accept(revision))
	}
	readers.Wait()
	close(failures)
	for failure := range failures {
		require.NoError(t, failure)
	}
	latest, err := m.Read(ref)
	require.NoError(t, err)
	require.Equal(t, uint64(8), latest.Revision)
	require.Zero(t, before.Revision, "a retained earlier read must not become the later RAM state")
	require.Equal(t, uint64(1), before.Owner.Epoch)
	require.Empty(t, before.Ledger)
	require.Zero(t, initialControl.Revision)
	close(gate)
	gateReleased = true
	require.NoError(t, m.FlushSession(ctx, ref))
	durable, err := m.Read(ref)
	require.NoError(t, err)
	require.Equal(t, uint64(8), durable.Revision)
	require.Equal(t, uint64(9), durable.StreamSequence)
	for i, id := range ids {
		require.Equal(t, uint64(i+2), durable.Ledger[id].CommittedStreamSequence)
		require.Zero(t, latest.Ledger[id].CommittedStreamSequence, "rebase must not mutate a captured pending view")
	}
	reclaimAsyncFixtureEpoch(t, p, ref)
	reclaimed, err := m.Read(ref)
	require.NoError(t, err)
	require.Equal(t, uint64(2), reclaimed.Owner.Epoch)
	require.Equal(t, uint64(10), reclaimed.StreamSequence)
	require.Equal(t, uint64(1), durable.Owner.Epoch, "epoch adoption must replace, not mutate, earlier views")
	require.Equal(t, uint64(1), initialControl.Owner.Epoch)
	require.NoError(t, m.Execute(ctx, ref, func(context.Context) error { return nil }))
	require.NoError(t, m.Drain(ctx))
}
