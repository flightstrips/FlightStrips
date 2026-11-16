package shared

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestPositionExecutionWaitTracing(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous); require.NoError(t, provider.Shutdown(context.Background())) })
	ctx, root := provider.Tracer("test").Start(context.Background(), "aircraft_position_update")
	defer root.End()
	local, global := make(chan struct{}, 1), make(chan struct{}, 1)
	p := &positionExecution{local: local, global: global, root: context.Background()}
	ctx = context.WithValue(ctx, positionExecutionKey{}, p)
	p.acquire(ctx)
	resume := SuspendPositionExecutionFor(ctx, "batch.snapshot")
	local <- struct{}{} // An independent report occupies the released slot.
	cancelCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	p.acquireFor(cancelCtx, "cancelled")
	require.False(t, p.held)
	require.Len(t, local, 1, "cancellation must not release another report's slot")
	<-local
	resume()
	require.True(t, p.held)
	p.release()

	// A physical batch retains the initiating trace, but uses dispatcher-root
	// cancellation rather than a single participant's expired deadline.
	ran := false
	require.NoError(t, RunPositionBatch(context.WithValue(cancelCtx, positionExecutionKey{}, p), func() { ran = true }))
	require.True(t, ran)
	var reasons []string
	for _, span := range recorder.Ended() {
		require.Equal(t, root.SpanContext().SpanID(), span.Parent().SpanID())
		require.Equal(t, "position.wait.execution", span.Name())
		attrs := map[attribute.Key]attribute.Value{}
		for _, attr := range span.Attributes() {
			attrs[attr.Key] = attr.Value
		}
		reasons = append(reasons, attrs["position.wait.reason"].AsString())
		require.GreaterOrEqual(t, attrs["position.wait.local_ms"].AsFloat64(), 0.)
		require.GreaterOrEqual(t, attrs["position.wait.global_ms"].AsFloat64(), 0.)
		if attrs["position.wait.reason"].AsString() == "cancelled" {
			require.False(t, attrs["position.execution.acquired"].AsBool())
			require.GreaterOrEqual(t, span.EndTime().Sub(span.StartTime()), 10*time.Millisecond)
		}
	}
	require.Equal(t, []string{"dispatch", "cancelled", "batch.snapshot", "batch.execute"}, reasons)
}
