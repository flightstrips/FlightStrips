package shared

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func testPositionDispatcherFIFOAndIndependentAircraft(t *testing.T, newDispatcher dispatcherFactory) {
	d := newDispatcher(4, 256, make(chan struct{}, 4))
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

func testPositionDispatcherBoundsQueueAndCancelsDrain(t *testing.T, newDispatcher dispatcherFactory) {
	d := newDispatcher(1, 2, nil)
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

func testPositionDispatcherGlobalBudget(t *testing.T, newDispatcher dispatcherFactory) {
	budget := make(chan struct{}, 2)
	a, b := newDispatcher(4, 16, budget), newDispatcher(4, 16, budget)
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

func testPositionDispatcherPausesDelayedCallbacksDuringBarrier(t *testing.T, newDispatcher dispatcherFactory) {
	d := newDispatcher(4, 256, nil)
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

type dispatcherFactory func(int, int, chan struct{}) *PositionDispatcher

type noopBatchScope struct{}

func (noopBatchScope) Context(ctx context.Context, _ int) context.Context { return ctx }
func (noopBatchScope) Done(int)                                           {}

func TestPositionDispatcher(t *testing.T) {
	for name, factory := range map[string]dispatcherFactory{
		"individual": NewPositionDispatcher,
		"batched": func(workers, pending int, budget chan struct{}) *PositionDispatcher {
			return NewBatchPositionDispatcher(workers, pending, budget, func(int) PositionBatchScope { return noopBatchScope{} })
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Run("FIFOAndIndependentAircraft", func(t *testing.T) { testPositionDispatcherFIFOAndIndependentAircraft(t, factory) })
			t.Run("BoundsQueueAndCancelsDrain", func(t *testing.T) { testPositionDispatcherBoundsQueueAndCancelsDrain(t, factory) })
			t.Run("GlobalBudget", func(t *testing.T) { testPositionDispatcherGlobalBudget(t, factory) })
			t.Run("PausesDelayedCallbacksDuringBarrier", func(t *testing.T) { testPositionDispatcherPausesDelayedCallbacksDuringBarrier(t, factory) })
		})
	}
}

// A master-change writer can block a later report while an earlier report waits
// for a batch. The later report must not monopolize the earlier report's slot.
func TestPositionBatchExecutionAllowsMasterReplacement(t *testing.T) {
	d := NewBatchPositionDispatcher(1, 256, make(chan struct{}, 1), func(int) PositionBatchScope { return noopBatchScope{} })
	var authority sync.RWMutex
	firstWaiting, resumeFirst, firstDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	writerDone, secondWaiting, secondDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	require.NoError(t, d.Submit(context.Background(), "A", func(ctx context.Context) {
		authority.RLock()
		resume := SuspendPositionExecution(ctx)
		close(firstWaiting)
		<-resumeFirst
		resume()
		authority.RUnlock()
		close(firstDone)
	}))
	<-firstWaiting
	go func() { authority.Lock(); authority.Unlock(); close(writerDone) }()
	require.Eventually(t, func() bool {
		if authority.TryRLock() {
			authority.RUnlock()
			return false
		}
		return true
	}, time.Second, time.Millisecond)
	require.NoError(t, d.Submit(context.Background(), "B", func(ctx context.Context) {
		resume := SuspendPositionExecution(ctx)
		close(secondWaiting)
		authority.RLock()
		resume()
		authority.RUnlock()
		close(secondDone)
	}))
	<-secondWaiting
	close(resumeFirst)
	for _, done := range []chan struct{}{firstDone, writerDone, secondDone} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("master replacement deadlocked an execution slot")
		}
	}
	require.NoError(t, d.Close(context.Background()))
}

func TestPositionBatchCancelWhileGlobalBudgetIsOccupied(t *testing.T) {
	budget := make(chan struct{}, 1)
	budget <- struct{}{}
	d := NewBatchPositionDispatcher(1, 256, budget, func(int) PositionBatchScope { return noopBatchScope{} })
	var cancelled atomic.Int32
	for _, key := range []string{"A", "B", "C"} {
		require.NoError(t, d.Submit(context.Background(), key, func(ctx context.Context) {
			if ctx.Err() != nil {
				cancelled.Add(1)
			}
		}))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Close(ctx) }()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("cancelled jobs waited for another client's pool slot")
	}
	require.Equal(t, int32(3), cancelled.Load())
	require.Len(t, budget, 1, "must not release a slot owned by another client")
}

func TestPositionBatchCancelWhileSQLWaitsForBudget(t *testing.T) {
	budget := make(chan struct{}, 1)
	d := NewBatchPositionDispatcher(1, 256, budget, func(int) PositionBatchScope { return noopBatchScope{} })
	waiting := make(chan struct{})
	result := make(chan error, 1)
	var executed atomic.Bool
	require.NoError(t, d.Submit(context.Background(), "A", func(ctx context.Context) {
		resume := SuspendPositionExecution(ctx)
		budget <- struct{}{} // another client's work takes the released slot
		close(waiting)
		result <- RunPositionBatch(ctx, func() { executed.Store(true) })
		resume()
	}))
	<-waiting
	d.Cancel()
	done := make(chan error, 1)
	go func() { done <- d.Close(context.Background()) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("cancelled SQL batch waited for another client's slot")
	}
	require.ErrorIs(t, <-result, context.Canceled)
	require.False(t, executed.Load())
	require.Len(t, budget, 1)
}
