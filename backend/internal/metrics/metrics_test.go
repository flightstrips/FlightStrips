package metrics

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func resetInstrumentsForTest() {
	once = sync.Once{}
	inst = nil
}

func collectMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	return rm
}

func attributesMatch(set attribute.Set, want map[string]string) bool {
	actual := map[string]string{}
	for _, kv := range set.ToSlice() {
		actual[string(kv.Key)] = kv.Value.AsString()
	}

	for key, wantValue := range want {
		if actual[key] != wantValue {
			return false
		}
	}
	return true
}

func findInt64MetricValue(t *testing.T, rm metricdata.ResourceMetrics, metricName string, want map[string]string) int64 {
	t.Helper()

	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != metricName {
				continue
			}

			switch data := metric.Data.(type) {
			case metricdata.Sum[int64]:
				for _, point := range data.DataPoints {
					if attributesMatch(point.Attributes, want) {
						return point.Value
					}
				}
			case metricdata.Gauge[int64]:
				for _, point := range data.DataPoints {
					if attributesMatch(point.Attributes, want) {
						return point.Value
					}
				}
			}
		}
	}

	t.Fatalf("metric %q with attributes %v not found", metricName, want)
	return 0
}

func findFloat64HistogramSum(t *testing.T, rm metricdata.ResourceMetrics, metricName string, want map[string]string) float64 {
	t.Helper()

	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != metricName {
				continue
			}

			if data, ok := metric.Data.(metricdata.Histogram[float64]); ok {
				for _, point := range data.DataPoints {
					if attributesMatch(point.Attributes, want) {
						return point.Sum
					}
				}
			}
		}
	}

	t.Fatalf("metric %q with attributes %v not found", metricName, want)
	return 0
}

func TestConnectionAndClientMetricsUseReadableLabels(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	resetInstrumentsForTest()
	t.Cleanup(func() {
		otel.SetMeterProvider(previousProvider)
		resetInstrumentsForTest()
	})

	ConnectionOpened(context.Background(), "live", "ekch", "frontend", "ekch_del")
	ConnectionClosed(context.Background(), "live", "ekch", "frontend", "ekch_del")

	rm := collectMetrics(t, reader)

	totalValue := findInt64MetricValue(t, rm, "websocket.connections.active", map[string]string{
		"session_name": "LIVE",
		"airport":      "EKCH",
		"source":       "frontend",
	})
	if totalValue != 0 {
		t.Fatalf("expected connection up/down counter to net to 0, got %d", totalValue)
	}

	clientValue := findInt64MetricValue(t, rm, "websocket.clients.active", map[string]string{
		"session_name": "LIVE",
		"airport":      "EKCH",
		"source":       "frontend",
		"callsign":     "EKCH_DEL",
	})
	if clientValue != 0 {
		t.Fatalf("expected client up/down counter to net to 0, got %d", clientValue)
	}
}

func TestPDCAndTrafficMetricsUseSessionNameAndAirport(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	resetInstrumentsForTest()
	t.Cleanup(func() {
		otel.SetMeterProvider(previousProvider)
		resetInstrumentsForTest()
	})

	PDCRequestReceived(context.Background(), "playback", "ekch", "web")
	PDCRequestOutcome(context.Background(), "playback", "ekch", "web", "requested_manual_review")
	PDCStateChange(context.Background(), "playback", "ekch", "REQUESTED")
	RecordTrafficSnapshot(context.Background(), "playback", "ekch", 3, 2, 1, 4)

	rm := collectMetrics(t, reader)

	if got := findInt64MetricValue(t, rm, "pdc.requests.received", map[string]string{
		"session_name": "PLAYBACK",
		"airport":      "EKCH",
		"channel":      "WEB",
	}); got != 1 {
		t.Fatalf("expected received counter 1, got %d", got)
	}

	if got := findInt64MetricValue(t, rm, "pdc.requests.outcomes", map[string]string{
		"session_name": "PLAYBACK",
		"airport":      "EKCH",
		"channel":      "WEB",
		"outcome":      "requested_manual_review",
	}); got != 1 {
		t.Fatalf("expected outcome counter 1, got %d", got)
	}

	if got := findInt64MetricValue(t, rm, "pdc.state_changes", map[string]string{
		"session_name": "PLAYBACK",
		"airport":      "EKCH",
		"state":        "REQUESTED",
	}); got != 1 {
		t.Fatalf("expected state change counter 1, got %d", got)
	}

	if got := findInt64MetricValue(t, rm, "traffic.departures.rate_15m", map[string]string{
		"session_name": "PLAYBACK",
		"airport":      "EKCH",
	}); got != 4 {
		t.Fatalf("expected departure traffic gauge 4, got %d", got)
	}
}

func TestEuroscopeSyncMetricsUseSessionNameAndAirport(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	resetInstrumentsForTest()
	t.Cleanup(func() {
		otel.SetMeterProvider(previousProvider)
		resetInstrumentsForTest()
	})

	RecordEuroscopeSync(context.Background(), "live", "ekch", 12, 4, 3, 1, 9, 150*time.Millisecond)

	rm := collectMetrics(t, reader)

	attrs := map[string]string{
		"session_name": "LIVE",
		"airport":      "EKCH",
	}

	if got := findInt64MetricValue(t, rm, "euroscope.sync.input_strips", attrs); got != 12 {
		t.Fatalf("expected strip input counter 12, got %d", got)
	}
	if got := findInt64MetricValue(t, rm, "euroscope.sync.input_controllers", attrs); got != 4 {
		t.Fatalf("expected controller input counter 4, got %d", got)
	}
	if got := findInt64MetricValue(t, rm, "euroscope.sync.changed_strips", attrs); got != 3 {
		t.Fatalf("expected changed strip counter 3, got %d", got)
	}
	if got := findInt64MetricValue(t, rm, "euroscope.sync.changed_controllers", attrs); got != 1 {
		t.Fatalf("expected changed controller counter 1, got %d", got)
	}
	if got := findInt64MetricValue(t, rm, "euroscope.sync.db_operations", attrs); got != 9 {
		t.Fatalf("expected db operations counter 9, got %d", got)
	}
	if got := findFloat64HistogramSum(t, rm, "euroscope.sync.duration", attrs); got != 0.15 {
		t.Fatalf("expected sync duration histogram sum 0.15, got %f", got)
	}
}
