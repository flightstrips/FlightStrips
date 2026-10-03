package cluster

import (
	"context"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestKeyedPositionWaitIgnoresOtherAircraftSessionsAndEpochs(t *testing.T) {
	p := readyPositionWaitFixture()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- p.WaitPositionApplied(ctx, 1, "SAS123", 2, 5) }()
	var target *positionWaitNotification
	require.Eventually(t, func() bool {
		p.positionWaitersMu.Lock()
		defer p.positionWaitersMu.Unlock()
		target = p.positionWaiters["1.SAS123.2"]
		return target != nil
	}, time.Second, time.Millisecond)
	p.mu.Lock()
	for _, key := range []string{"1.SAS999.2", "2.SAS123.2", "1.SAS123.1"} {
		p.positions[key] = KVPosition{Revision: 100}
		p.wakePositionWaitersLocked(key)
	}
	p.mu.Unlock()
	select {
	case <-target.changed:
		t.Fatal("unrelated aircraft, session, or epoch woke exact-key waiter")
	default:
	}
	p.mu.Lock()
	p.positions["1.SAS123.2"] = KVPosition{Revision: 5}
	p.wakePositionWaitersLocked("1.SAS123.2")
	p.mu.Unlock()
	require.NoError(t, <-done)
	p.positionWaitersMu.Lock()
	defer p.positionWaitersMu.Unlock()
	require.Empty(t, p.positionWaiters)
}

func TestKeyedPositionWaitFailuresWakeEveryKey(t *testing.T) {
	for _, observationFailure := range []bool{false, true} {
		p := readyPositionWaitFixture()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		done := make(chan error, 2)
		for _, aircraft := range []string{"SAS123", "SAS999"} {
			go func(key string) { done <- p.WaitPositionApplied(ctx, 1, key, 2, 5) }(aircraft)
		}
		require.Eventually(t, func() bool {
			p.positionWaitersMu.Lock()
			defer p.positionWaitersMu.Unlock()
			return len(p.positionWaiters) == 2
		}, time.Second, time.Millisecond)
		failure := errors.New("watcher stopped")
		if observationFailure {
			p.failObservation(failure)
		} else {
			p.fail(failure)
		}
		for i := 0; i < 2; i++ {
			require.ErrorIs(t, <-done, failure)
		}
		cancel()
		p.positionWaitersMu.Lock()
		require.Empty(t, p.positionWaiters)
		p.positionWaitersMu.Unlock()
	}
}

func TestCancelledKeyedPositionWaitCleansRegistrationsAndRetainsPeers(t *testing.T) {
	p := readyPositionWaitFixture()
	parent, cancelParent := context.WithTimeout(context.Background(), time.Second)
	defer cancelParent()
	first, cancelFirst := context.WithCancel(parent)
	firstDone, peerDone := make(chan error, 1), make(chan error, 1)
	go func() { firstDone <- p.WaitPositionApplied(first, 1, "SAS123", 2, 5) }()
	go func() { peerDone <- p.WaitPositionApplied(parent, 1, "SAS123", 2, 5) }()
	require.Eventually(t, func() bool {
		p.positionWaitersMu.Lock()
		defer p.positionWaitersMu.Unlock()
		waiter := p.positionWaiters["1.SAS123.2"]
		return waiter != nil && waiter.users == 2
	}, time.Second, time.Millisecond)
	cancelFirst()
	require.ErrorIs(t, <-firstDone, context.Canceled)
	p.positionWaitersMu.Lock()
	require.Equal(t, 1, p.positionWaiters["1.SAS123.2"].users)
	p.positionWaitersMu.Unlock()
	p.mu.Lock()
	p.positions["1.SAS123.2"] = KVPosition{Revision: 5}
	p.wakePositionWaitersLocked("1.SAS123.2")
	p.mu.Unlock()
	require.NoError(t, <-peerDone)
	batch, cancelBatch := context.WithCancel(parent)
	done := make(chan error, 200)
	for i := 0; i < 200; i++ {
		go func(key string) { done <- p.WaitPositionApplied(batch, 1, key, 2, 5) }(fmt.Sprintf("BAW%03d", i))
	}
	require.Eventually(t, func() bool {
		p.positionWaitersMu.Lock()
		defer p.positionWaitersMu.Unlock()
		return len(p.positionWaiters) == 200
	}, time.Second, time.Millisecond)
	cancelBatch()
	for i := 0; i < 200; i++ {
		require.ErrorIs(t, <-done, context.Canceled)
	}
	p.positionWaitersMu.Lock()
	defer p.positionWaitersMu.Unlock()
	require.Empty(t, p.positionWaiters, "cancelled never-updated keys must not accumulate")
}

func TestKeyedPositionRegistrationCannotMissPublication(t *testing.T) {
	p := readyPositionWaitFixture()
	key := "1.SAS123.2"
	// Match the production read-check/register sequence while a publication is
	// ready to acquire the write lock. Registration must precede its notification.
	p.mu.RLock()
	started, finished := make(chan struct{}), make(chan struct{})
	go func() {
		close(started)
		p.mu.Lock()
		p.positions[key] = KVPosition{Revision: 5}
		p.wakePositionWaitersLocked(key)
		p.mu.Unlock()
		close(finished)
	}()
	<-started
	waiter := p.registerPositionWaiterLocked(key)
	p.mu.RUnlock()
	select {
	case <-waiter.changed:
	case <-time.After(time.Second):
		t.Fatal("publication was missed between read and registration")
	}
	<-finished
	p.releasePositionWaiter(key, waiter)
	p.positionWaitersMu.Lock()
	defer p.positionWaitersMu.Unlock()
	require.Empty(t, p.positionWaiters)
}
func TestCancelledPositionWaitReleasesWithoutPublicationLock(t *testing.T) {
	p := readyPositionWaitFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- p.WaitPositionApplied(ctx, 1, "SAS123", 2, 5) }()
	require.Eventually(t, func() bool {
		p.positionWaitersMu.Lock()
		defer p.positionWaitersMu.Unlock()
		return p.positionWaiters["1.SAS123.2"] != nil
	}, time.Second, time.Millisecond)
	p.mu.Lock()
	defer p.mu.Unlock()
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("cancellation cleanup waited for the publication write lock")
	}
	p.positionWaitersMu.Lock()
	defer p.positionWaitersMu.Unlock()
	require.Empty(t, p.positionWaiters)
}
func TestKeyedPositionCancellationRacesPublicationWithoutLeaking(t *testing.T) {
	p := readyPositionWaitFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 200)
	for i := 0; i < 200; i++ {
		go func(key string) { done <- p.WaitPositionApplied(ctx, 1, key, 2, 5) }(fmt.Sprintf("BAW%03d", i))
	}
	require.Eventually(t, func() bool {
		p.positionWaitersMu.Lock()
		defer p.positionWaitersMu.Unlock()
		return len(p.positionWaiters) == 200
	}, time.Second, time.Millisecond)
	published := make(chan struct{})
	go func() {
		p.mu.Lock()
		for i := 0; i < 200; i++ {
			key := positionKey(1, fmt.Sprintf("BAW%03d", i), 2)
			p.positions[key] = KVPosition{Revision: 5}
			p.wakePositionWaitersLocked(key)
		}
		p.mu.Unlock()
		close(published)
	}()
	cancel()
	for i := 0; i < 200; i++ {
		select {
		case err := <-done:
			if err != nil {
				require.ErrorIs(t, err, context.Canceled)
			}
		case <-time.After(time.Second):
			t.Fatal("waiter stalled during cancellation/publication race")
		}
	}
	<-published
	p.positionWaitersMu.Lock()
	defer p.positionWaitersMu.Unlock()
	require.Empty(t, p.positionWaiters)
}

func TestAppliedPositionWaitStillRequiresStateAndHistoryReadiness(t *testing.T) {
	t.Run("state replay", func(t *testing.T) {
		p := readyPositionWaitFixture()
		p.positions["1.SAS123.2"] = KVPosition{Revision: 5}
		p.applied, p.highWater = 1, 2
		require.ErrorContains(t, p.WaitPositionApplied(context.Background(), 1, "SAS123", 2, 5), "replay behind stream")
		p.positionWaitersMu.Lock()
		defer p.positionWaitersMu.Unlock()
		require.Empty(t, p.positionWaiters)
	})
}
