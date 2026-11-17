package positionload

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestCaptureCountsCurrentSQLSpansThroughPositionPhases(t *testing.T) {
	collector := &capture{queries: make(map[trace.TraceID]int), pools: make(map[trace.TraceID]float64)}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(collector))
	defer provider.Shutdown(context.Background())
	tracer := provider.Tracer("test")
	ctx, root := tracer.Start(context.Background(), "aircraft_position_update")
	ctx, phase := tracer.Start(ctx, "position.update")
	_, query := tracer.Start(ctx, "SELECT", trace.WithAttributes(attribute.String("db.query.text", "SELECT 1")))
	query.End()
	phase.End()
	root.End()
	require.Equal(t, 1, collector.completed)
	require.Len(t, collector.samples, 1)
	require.Equal(t, 1, collector.samples[0].Queries)
	require.Len(t, collector.positionQueries, 1)
}
