package telemetry

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestSetupDualLoggerPreservesConsoleLogLevel(t *testing.T) {
	originalLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	var output bytes.Buffer
	consoleHandler := slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelWarn})
	SetupDualLogger(consoleHandler)

	slog.Info("filtered info message")
	slog.Warn("visible warning message")

	logged := output.String()
	if strings.Contains(logged, "filtered info message") {
		t.Fatal("console handler logged a message below its configured level")
	}
	if !strings.Contains(logged, "visible warning message") {
		t.Fatal("console handler did not log a message at its configured level")
	}
}

func TestMultiHandlerFiltersEachDestinationIndependently(t *testing.T) {
	var consoleOutput bytes.Buffer
	consoleHandler := slog.NewTextHandler(&consoleOutput, &slog.HandlerOptions{Level: slog.LevelWarn})
	alwaysEnabledHandler := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(&multiHandler{handlers: []slog.Handler{consoleHandler, alwaysEnabledHandler}})

	logger.Info("telemetry-only info message")
	logger.Warn("console warning message")

	logged := consoleOutput.String()
	if strings.Contains(logged, "telemetry-only info message") {
		t.Fatal("console handler logged a message below its configured level when another handler was enabled")
	}
	if !strings.Contains(logged, "console warning message") {
		t.Fatal("console handler did not log a message at its configured level")
	}
}

func collectMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	return rm
}

func findInt64MetricValue(t *testing.T, rm metricdata.ResourceMetrics, metricName string) int64 {
	t.Helper()

	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != metricName {
				continue
			}

			switch data := metric.Data.(type) {
			case metricdata.Sum[int64]:
				if len(data.DataPoints) > 0 {
					return data.DataPoints[0].Value
				}
			case metricdata.Gauge[int64]:
				if len(data.DataPoints) > 0 {
					return data.DataPoints[0].Value
				}
			}
		}
	}

	t.Fatalf("metric %q not found", metricName)
	return 0
}

func TestStartRuntimeMetricsRegistersGoRuntimeMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	if err := startRuntimeMetrics(provider); err != nil {
		t.Fatalf("start runtime metrics: %v", err)
	}

	rm := collectMetrics(t, reader)

	if got := findInt64MetricValue(t, rm, "process.runtime.go.goroutines"); got < 1 {
		t.Fatalf("expected goroutine metric to be positive, got %d", got)
	}

	if got := findInt64MetricValue(t, rm, "process.runtime.go.mem.heap_alloc"); got <= 0 {
		t.Fatalf("expected heap allocation metric to be positive, got %d", got)
	}

	if got := findInt64MetricValue(t, rm, "process.runtime.go.gc.count"); got < 0 {
		t.Fatalf("expected gc count metric to be non-negative, got %d", got)
	}
}
