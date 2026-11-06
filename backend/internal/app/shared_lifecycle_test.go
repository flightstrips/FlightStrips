package app

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConcurrentSessionDemandDoesNotStartDuplicateApplicationWorkers(t *testing.T) {
	var starts atomic.Int32
	started, stopped := make(chan struct{}), make(chan struct{})
	application := &App{workers: []func(context.Context){func(ctx context.Context) {
		if starts.Add(1) == 1 {
			close(started)
		}
		<-ctx.Done()
		close(stopped)
	}}}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var readers sync.WaitGroup
	for i := 0; i < 40; i++ {
		readers.Go(func() { application.StartWorkers(ctx) })
	}
	readers.Wait()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	require.EqualValues(t, 1, starts.Load())
	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("application cancellation did not stop shared worker")
	}
}
