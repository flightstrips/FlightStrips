package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/shared"
	"github.com/stretchr/testify/require"
)

type contentionBatchScope struct{}

func (contentionBatchScope) Context(ctx context.Context, _ int) context.Context { return ctx }
func (contentionBatchScope) Done(int)                                           {}

// A transport can carry several airport sessions. Waiting for one session's
// operational transition must leave capacity for another session to persist.
func TestPositionTransitionWaitReleasesExecutionSlot(t *testing.T) {
	service := &StripService{}
	unlock := service.positionTransition(context.Background(), 1)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(unlock) }
	d := shared.NewBatchPositionDispatcher(1, 16, make(chan struct{}, 1), func(int) shared.PositionBatchScope { return contentionBatchScope{} })
	t.Cleanup(func() { release(); require.NoError(t, d.Close(context.Background())) })
	entering, resumed, unrelated := make(chan struct{}), make(chan struct{}), make(chan struct{})
	require.NoError(t, d.Submit(context.Background(), "1/A", func(ctx context.Context) {
		close(entering)
		done := service.positionTransition(ctx, 1)
		done()
		close(resumed)
	}))
	<-entering
	start := time.Now()
	require.NoError(t, d.Submit(context.Background(), "2/B", func(context.Context) { close(unrelated) }))
	select {
	case <-unrelated:
		t.Logf("independent report completed in %s while transition stayed locked", time.Since(start))
	case <-time.After(200 * time.Millisecond):
		t.Fatal("transition waiter monopolized the execution slot")
	}
	select {
	case <-resumed:
		t.Fatal("transition escaped the session lock")
	default:
	}
	release()
	select {
	case <-resumed:
	case <-time.After(time.Second):
		t.Fatal("transition did not reacquire execution after unlocking")
	}
}
