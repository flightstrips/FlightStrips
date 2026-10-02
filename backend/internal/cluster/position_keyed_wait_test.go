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
		p.mu.RLock()
		defer p.mu.RUnlock()
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
	p.mu.RLock()
	defer p.mu.RUnlock()
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
		require.Eventually(t, func() bool { p.mu.RLock(); defer p.mu.RUnlock(); return len(p.positionWaiters) == 2 }, time.Second, time.Millisecond)
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
		p.mu.RLock()
		require.Empty(t, p.positionWaiters)
		p.mu.RUnlock()
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
		p.mu.RLock()
		defer p.mu.RUnlock()
		waiter := p.positionWaiters["1.SAS123.2"]
		return waiter != nil && waiter.users == 2
	}, time.Second, time.Millisecond)
	cancelFirst()
	require.ErrorIs(t, <-firstDone, context.Canceled)
	p.mu.RLock()
	require.Equal(t, 1, p.positionWaiters["1.SAS123.2"].users)
	p.mu.RUnlock()
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
	require.Eventually(t, func() bool { p.mu.RLock(); defer p.mu.RUnlock(); return len(p.positionWaiters) == 200 }, time.Second, time.Millisecond)
	cancelBatch()
	for i := 0; i < 200; i++ {
		require.ErrorIs(t, <-done, context.Canceled)
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	require.Empty(t, p.positionWaiters, "cancelled never-updated keys must not accumulate")
}
