package shared

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPositionDispatcherFIFOAndIndependentAircraft(t *testing.T) {
	d := NewPositionDispatcher(4, 256, make(chan struct{}, 4))
	t.Cleanup(func() { require.NoError(t, d.Close(context.Background())) })
	entered, release, other := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	var order []int
	require.NoError(t, d.Submit(context.Background(), "1/A", func(context.Context) { close(entered); <-release; mu.Lock(); order = append(order, 0); mu.Unlock() }))
	<-entered
	for i := 1; i <= 100; i++ {
		n := i
		require.NoError(t, d.Submit(context.Background(), "1/A", func(context.Context) { mu.Lock(); order = append(order, n); mu.Unlock() }))
	}
	require.NoError(t, d.Submit(context.Background(), "1/B", func(context.Context) { close(other) }))
	select {
	case <-other:
	case <-time.After(time.Second):
		t.Fatal("independent aircraft blocked behind A")
	}
	close(release)
	require.NoError(t, d.Barrier(context.Background()))
	require.Len(t, order, 101)
	for i, n := range order {
		require.Equal(t, i, n)
	}
}

func TestPositionDispatcherBoundsQueueAndCancelsDrain(t *testing.T) {
	d := NewPositionDispatcher(1, 2, nil)
	entered := make(chan struct{})
	var completed, cancelled atomic.Int32
	run := func(ctx context.Context) { <-ctx.Done(); completed.Add(1); cancelled.Add(1) }
	require.NoError(t, d.Submit(context.Background(), "A", func(ctx context.Context) { close(entered); run(ctx) }))
	<-entered
	require.NoError(t, d.Submit(context.Background(), "A", run))
	require.NoError(t, d.Submit(context.Background(), "B", run))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, d.Submit(ctx, "C", run), context.DeadlineExceeded)
	require.Equal(t, 2, d.Depth())
	require.ErrorIs(t, d.Close(ctx), context.DeadlineExceeded)
	require.Equal(t, int32(3), completed.Load())
	require.Equal(t, int32(3), cancelled.Load())
	require.ErrorIs(t, d.Submit(context.Background(), "D", run), context.Canceled)
}

func TestPositionDispatcherGlobalBudget(t *testing.T) {
	budget := make(chan struct{}, 2)
	a, b := NewPositionDispatcher(4, 16, budget), NewPositionDispatcher(4, 16, budget)
	var active, peak atomic.Int32
	run := func(context.Context) {
		n := active.Add(1)
		for {
			old := peak.Load()
			if old >= n || peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(time.Millisecond)
		active.Add(-1)
	}
	for i := 0; i < 16; i++ {
		require.NoError(t, a.Submit(context.Background(), string(rune('A'+i)), run))
		require.NoError(t, b.Submit(context.Background(), string(rune('A'+i)), run))
	}
	require.NoError(t, a.Close(context.Background()))
	require.NoError(t, b.Close(context.Background()))
	require.Equal(t, int32(2), peak.Load())
}

func TestPositionDispatcherPausesDelayedCallbacksDuringBarrier(t *testing.T) {
	d := NewPositionDispatcher(4, 256, nil)
	defer d.Close(context.Background())
	ran := make(chan struct{})
	require.NoError(t, d.RunBarrier(context.Background(), func() {
		require.NoError(t, d.Submit(context.Background(), "A", func(context.Context) { close(ran) }))
		select {
		case <-ran:
			t.Fatal("delayed callback crossed operational barrier")
		case <-time.After(10 * time.Millisecond):
		}
	}))
	require.NoError(t, d.Barrier(context.Background()))
	select {
	case <-ran:
	default:
		t.Fatal("delayed callback did not resume")
	}
}
