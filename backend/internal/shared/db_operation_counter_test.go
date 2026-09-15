package shared

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDBOperationCounterIncludesConcurrentQueriesAndStopsAtHandlerCompletion(t *testing.T) {
	ctx, counter := WithDBOperationCounter(context.Background())
	state := &WebsocketMessageState{AutoCountDBOperations: true}
	ctx = WithWebsocketMessageState(ctx, state)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				TraceDBOperation(ctx)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, 1000, counter.Finish())
	TraceDBOperation(context.WithoutCancel(ctx))
	require.Equal(t, 1000, counter.Finish(), "detached work cannot change a completed metric sample")
	require.Zero(t, state.DBOperations, "the tracer must not also mutate the non-concurrent message cache")
}
