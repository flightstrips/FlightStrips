package positionload

import (
	"context"
	"fmt"
	"os"
	"testing"

	"FlightStrips/internal/pdc/testdata"
	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if err := testdata.ShutdownTestDB(); err != nil && code == 0 {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}

func TestCaptureCountsProductionPGXQuerySpans(t *testing.T) {
	base, _ := testdata.SetupTestDB(t)
	collector := &capture{queries: map[trace.TraceID]int{}, pools: map[trace.TraceID]float64{}}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(collector))
	defer provider.Shutdown(context.Background())
	cfg := base.Config()
	cfg.ConnConfig.Tracer = otelpgx.NewTracer(otelpgx.WithTracerProvider(provider), otelpgx.WithQuerySpanNamePrefix())
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	require.NoError(t, err)
	defer pool.Close()
	ctx, span := provider.Tracer("load-test").Start(t.Context(), "aircraft_position_update")
	var result int
	require.NoError(t, pool.QueryRow(ctx, "SELECT 1").Scan(&result))
	span.End()
	require.Equal(t, 1, result)
	require.Equal(t, 1, collector.completed)
	require.Len(t, collector.samples, 1)
	require.Equal(t, 1, collector.samples[0].Queries, "count the actual query once, excluding its prepared-statement span")
}
