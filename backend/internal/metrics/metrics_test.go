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

	ConnectionOpened(context.Background(), "live", "ekch", "frontend", "ekch_del", "0.16.0")
	ConnectionClosed(context.Background(), "live", "ekch", "frontend", "ekch_del", "0.16.0")

	rm := collectMetrics(t, reader)

	totalValue := findInt64MetricValue(t, rm, "websocket.connections.active", map[string]string{
		"session_name":   "LIVE",
		"airport":        "EKCH",
		"source":         "frontend",
		"client_version": "0.16.0",
	})
	if totalValue != 0 {
		t.Fatalf("expected connection up/down counter to net to 0, got %d", totalValue)
	}

	clientValue := findInt64MetricValue(t, rm, "websocket.clients.active", map[string]string{
		"session_name":   "LIVE",
		"airport":        "EKCH",
		"source":         "frontend",
		"callsign":       "EKCH_DEL",
		"client_version": "0.16.0",
	})
	if clientValue != 0 {
		t.Fatalf("expected client up/down counter to net to 0, got %d", clientValue)
	}
}

func TestMasterClientMetricTracksCurrentCallsign(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	resetInstrumentsForTest()
	t.Cleanup(func() {
		otel.SetMeterProvider(previousProvider)
		resetInstrumentsForTest()
	})

	MasterClientAssigned(context.Background(), "live", "ekch", "ekch_a_twr", "0.16.0")
	MasterClientCleared(context.Background(), "live", "ekch", "ekch_a_twr", "0.16.0")
	MasterClientAssigned(context.Background(), "live", "ekch", "ekch_d_twr", "0.16.1")

	rm := collectMetrics(t, reader)

	if got := findInt64MetricValue(t, rm, "euroscope.master_client.active", map[string]string{
		"session_name":   "LIVE",
		"airport":        "EKCH",
		"callsign":       "EKCH_A_TWR",
		"client_version": "0.16.0",
	}); got != 0 {
		t.Fatalf("expected previous master gauge to net to 0, got %d", got)
	}

	if got := findInt64MetricValue(t, rm, "euroscope.master_client.active", map[string]string{
		"session_name":   "LIVE",
		"airport":        "EKCH",
		"callsign":       "EKCH_D_TWR",
		"client_version": "0.16.1",
	}); got != 1 {
		t.Fatalf("expected current master gauge to be 1, got %d", got)
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

	RecordEuroscopeSync(context.Background(), "live", "ekch", "0.16.0", 12, 4, 3, 1, 9, 150*time.Millisecond)

	rm := collectMetrics(t, reader)

	attrs := map[string]string{
		"session_name":   "LIVE",
		"airport":        "EKCH",
		"client_version": "0.16.0",
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

func TestSATMetricsAvoidPersonalDataDimensions(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	resetInstrumentsForTest()
	t.Cleanup(func() { otel.SetMeterProvider(previousProvider); resetInstrumentsForTest() })

	ctx := context.Background()
	RecordSATFeedSnapshot(ctx, 5*time.Second, 7, 3)
	RecordSATAssignment(ctx, "ASSIGNED", "AUTOMATIC", "airline_rule", 2)
	RecordSATOutcome(ctx, "no_compatible_stand", "ARRIVAL")
	RecordSATConflict(ctx, "database_contention")
	RecordSATExpiration(ctx, "DEPARTURE", "RESERVED")
	rm := collectMetrics(t, reader)

	if got := findInt64MetricValue(t, rm, "sat.assignments", map[string]string{"stage": "ASSIGNED", "source": "AUTOMATIC", "category": "airline_rule"}); got != 1 {
		t.Fatalf("expected assignment counter 1, got %d", got)
	}
	if got := findInt64MetricValue(t, rm, "sat.allocation.outcomes", map[string]string{"outcome": "no_compatible_stand", "category": "ARRIVAL"}); got != 1 {
		t.Fatalf("expected outcome counter 1, got %d", got)
	}
	if got := findInt64MetricValue(t, rm, "sat.allocation.conflicts", map[string]string{"kind": "database_contention"}); got != 1 {
		t.Fatalf("expected conflict counter 1, got %d", got)
	}
	if got := findInt64MetricValue(t, rm, "sat.assignments.expired", map[string]string{"direction": "DEPARTURE", "stage": "RESERVED"}); got != 1 {
		t.Fatalf("expected expiration counter 1, got %d", got)
	}
}

func TestAMANMetricsBoundLabelsAndExcludeFlightIdentifiers(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	resetInstrumentsForTest()
	t.Cleanup(func() { otel.SetMeterProvider(previousProvider); resetInstrumentsForTest() })

	ctx := context.Background()
	RecordAMANObservation(ctx, 3*time.Second, "SAS123")
	RecordAMANGeometryCache(ctx, "hit")
	RecordAMANRouteMaterialization(ctx, 20*time.Millisecond, "success")
	RecordAMANPredictor(ctx, 10*time.Millisecond, "high", "weather")
	RecordAMANPredictionDrift(ctx, "superstable", 2*time.Second)
	RecordAMANSequenceRevision(ctx)
	RecordAMANSequenceConflict(ctx, "revision")
	RecordAMANCommand(ctx, "accepted")
	RecordAMANFlightDegradation(ctx, "disconnected")
	RecordAMANSourceRefresh(ctx, "vatsim", "stale")
	RecordAMANPublicationFailure(ctx, "frontend")
	rm := collectMetrics(t, reader)

	if got := findInt64MetricValue(t, rm, "aman.geometry.cache", map[string]string{"outcome": "hit"}); got != 1 {
		t.Fatalf("geometry cache = %d, want 1", got)
	}
	if got := findInt64MetricValue(t, rm, "aman.commands", map[string]string{"outcome": "accepted"}); got != 1 {
		t.Fatalf("commands = %d, want 1", got)
	}
	if got := findFloat64HistogramSum(t, rm, "aman.observation.age", map[string]string{"state": "other"}); got != 3 {
		t.Fatalf("observation age = %f, want 3", got)
	}

	for _, scope := range rm.ScopeMetrics {
		for _, recorded := range scope.Metrics {
			if len(recorded.Name) < 5 || recorded.Name[:5] != "aman." {
				continue
			}
			for _, attrs := range metricAttributeSets(recorded.Data) {
				for _, attribute := range attrs.ToSlice() {
					if attribute.Value.AsString() == "SAS123" || string(attribute.Key) == "callsign" || string(attribute.Key) == "route" || string(attribute.Key) == "command_id" {
						t.Fatalf("AMAN metric %s contains unbounded attribute %s=%s", recorded.Name, attribute.Key, attribute.Value.AsString())
					}
				}
			}
		}
	}
}

func metricAttributeSets(data metricdata.Aggregation) []attribute.Set {
	var result []attribute.Set
	switch values := data.(type) {
	case metricdata.Sum[int64]:
		for _, point := range values.DataPoints {
			result = append(result, point.Attributes)
		}
	case metricdata.Histogram[float64]:
		for _, point := range values.DataPoints {
			result = append(result, point.Attributes)
		}
	}
	return result
}
