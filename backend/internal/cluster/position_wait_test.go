package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func readyPositionWaitFixture() *Projection {
	return &Projection{started: true, positionReady: true, presenceReady: true, checked: time.Now(), positions: map[string]KVPosition{}, states: map[string]*Aggregate{}}
}
func TestPositionWaitRequiresExactEpochAndRevision(t *testing.T) {
	p := readyPositionWaitFixture()
	p.positions["1.SAS123.1"] = KVPosition{Revision: 100}
	p.positions["1.SAS123.2"] = KVPosition{Revision: 4}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- p.WaitPositionApplied(ctx, 1, "SAS123", 2, 5) }()
	select {
	case <-done:
		t.Fatal("an old epoch or revision satisfied the barrier")
	case <-time.After(5 * time.Millisecond):
	}
	p.mu.Lock()
	p.positions["1.SAS123.2"] = KVPosition{Revision: 5}
	p.wakePositionWaitersLocked("1.SAS123.2")
	p.mu.Unlock()
	require.NoError(t, <-done)
}
func TestReplayReadWaitKeepsImmediateReadinessFence(t *testing.T) {
	p := readyPositionWaitFixture()
	p.applied, p.highWater = 1, 2
	require.ErrorContains(t, p.Ready(), "replay behind stream")
	result := make(chan error, 1)
	go func() { result <- p.readyForRead() }()
	p.mu.Lock()
	p.applied = 2
	p.mu.Unlock()
	require.NoError(t, <-result)
	p.mu.Lock()
	p.observationErr = context.Canceled
	p.mu.Unlock()
	require.ErrorIs(t, p.readyForRead(), context.Canceled)
}
func TestSinglePositionSelectionMatchesSnapshotAcrossEpochs(t *testing.T) {
	p := readyPositionWaitFixture()
	for _, epoch := range []uint64{1, 2, 3} {
		p.positions[positionKey(1, "SAS123", epoch)] = KVPosition{Value: &pb.PositionValue{SessionId: 1, AircraftKey: "SAS123", OwnerEpoch: epoch}, Revision: epoch}
	}
	all := p.positionSnapshotLocked(1)
	selected, ok := p.selectedPositionLocked(1, "SAS123")
	require.True(t, ok)
	require.Len(t, all, 1)
	require.Equal(t, all[0].Revision, selected.Revision)
	require.Equal(t, all[0].Stale, selected.Stale)
}

func TestProjectionFailureWakesCommitWaiters(t *testing.T) {
	for _, position := range []bool{false, true} {
		p := readyPositionWaitFixture()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		done := make(chan error, 1)
		go func() {
			if position {
				done <- p.WaitPositionApplied(ctx, 1, "SAS123", 1, 1)
			} else {
				done <- p.WaitApplied(ctx, 1)
			}
		}()
		require.Eventually(t, func() bool {
			p.mu.RLock()
			defer p.mu.RUnlock()
			return p.stateChanged != nil || len(p.positionWaiters) != 0
		}, time.Second, time.Millisecond)
		p.failObservation(context.Canceled)
		require.ErrorIs(t, <-done, context.Canceled)
		cancel()
	}
}

func TestAlreadyAppliedPositionWaitDoesNotRequireExclusiveLock(t *testing.T) {
	p := readyPositionWaitFixture()
	p.positions["1.SAS123.2"] = KVPosition{Revision: 5}
	p.mu.RLock()
	done := make(chan error, 1)
	go func() { done <- p.WaitPositionApplied(context.Background(), 1, "SAS123", 2, 5) }()
	select {
	case err := <-done:
		p.mu.RUnlock()
		require.NoError(t, err)
	case <-time.After(time.Second):
		p.mu.RUnlock()
		<-done
		t.Fatal("already-applied exact revision waited for an exclusive projection lock")
	}
}
