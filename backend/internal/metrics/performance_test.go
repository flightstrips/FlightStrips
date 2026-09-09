package metrics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestMessageErrorClass(t *testing.T) {
	tests := []struct {
		name, messageType, want string
		err                     error
	}{
		{name: "success", want: "none"},
		{name: "serialization", err: &pgconn.PgError{Code: "40001"}, want: "serialization_conflict"},
		{name: "deadlock", err: &pgconn.PgError{Code: "40P01"}, want: "deadlock"},
		{name: "missing row", err: pgx.ErrNoRows, want: "missing_row"},
		{name: "coordination", messageType: "coordination_assume_request", err: errors.New("invalid request"), want: "coordination"},
		{name: "other", messageType: "strip_update", err: errors.New("failed"), want: "other"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := messageErrorClass(test.messageType, test.err); got != test.want {
				t.Fatalf("messageErrorClass() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestMessageDBRetriesRecordsBoundedConflictClass(t *testing.T) {
	reader := newTestReader(t)

	MessageDBRetries(context.Background(), "live", "ekch", "euroscope", "aircraft_position_update", "0.16.0", map[string]int{
		"serialization_conflict": 2,
	})

	rm := collectMetrics(t, reader)
	if got := findInt64MetricValue(t, rm, "websocket.message.db_retries", map[string]string{
		"type": "aircraft_position_update", "error_class": "serialization_conflict",
	}); got != 2 {
		t.Fatalf("expected 2 serialization retries, got %d", got)
	}
}

func newTestReader(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	resetInstrumentsForTest()
	t.Cleanup(func() {
		otel.SetMeterProvider(previousProvider)
		resetInstrumentsForTest()
	})
	return reader
}

func findInt64HistogramSum(t *testing.T, rm metricdata.ResourceMetrics, metricName string, want map[string]string) int64 {
	t.Helper()

	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != metricName {
				continue
			}
			if data, ok := metric.Data.(metricdata.Histogram[int64]); ok {
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

func TestSyncPhaseDurationIsLabelledByPhase(t *testing.T) {
	reader := newTestReader(t)

	RecordEuroscopeSyncPhase(context.Background(), "live", "ekch", SyncPhaseStrips, 250*time.Millisecond)
	RecordEuroscopeSyncPhase(context.Background(), "live", "ekch", SyncPhaseFinalize, 500*time.Millisecond)

	rm := collectMetrics(t, reader)

	strips := findFloat64HistogramSum(t, rm, "euroscope.sync.phase.duration", map[string]string{
		"session_name": "LIVE",
		"airport":      "EKCH",
		"phase":        SyncPhaseStrips,
	})
	if strips != 0.25 {
		t.Fatalf("expected strips phase to record 0.25s, got %v", strips)
	}

	finalize := findFloat64HistogramSum(t, rm, "euroscope.sync.phase.duration", map[string]string{
		"session_name": "LIVE",
		"airport":      "EKCH",
		"phase":        SyncPhaseFinalize,
	})
	if finalize != 0.5 {
		t.Fatalf("expected finalize phase to record 0.5s, got %v", finalize)
	}
}

func TestSyncPhaseRejectsUnknownPhaseLabel(t *testing.T) {
	reader := newTestReader(t)

	RecordEuroscopeSyncPhase(context.Background(), "live", "ekch", "EKCH_TWR", time.Second)

	rm := collectMetrics(t, reader)

	value := findFloat64HistogramSum(t, rm, "euroscope.sync.phase.duration", map[string]string{
		"phase": "other",
	})
	if value != 1 {
		t.Fatalf("expected an unknown phase to collapse to \"other\", got %v", value)
	}
}

func TestSyncOutcomeSeparatesChangedFromUnchanged(t *testing.T) {
	reader := newTestReader(t)

	RecordEuroscopeSyncOutcome(context.Background(), "live", "ekch", true)
	RecordEuroscopeSyncOutcome(context.Background(), "live", "ekch", false)
	RecordEuroscopeSyncOutcome(context.Background(), "live", "ekch", false)

	rm := collectMetrics(t, reader)

	changed := findInt64MetricValue(t, rm, "euroscope.sync.outcomes", map[string]string{
		"session_name": "LIVE",
		"airport":      "EKCH",
		"outcome":      "changed",
	})
	if changed != 1 {
		t.Fatalf("expected 1 changed sync, got %d", changed)
	}

	unchanged := findInt64MetricValue(t, rm, "euroscope.sync.outcomes", map[string]string{
		"session_name": "LIVE",
		"airport":      "EKCH",
		"outcome":      "unchanged",
	})
	if unchanged != 2 {
		t.Fatalf("expected 2 unchanged syncs, got %d", unchanged)
	}
}

func TestSyncFollowUpWorkIgnoresEmptyWork(t *testing.T) {
	reader := newTestReader(t)

	RecordEuroscopeSyncFollowUpWork(context.Background(), "live", "ekch", SyncWorkRouteRecalc, 12)
	RecordEuroscopeSyncFollowUpWork(context.Background(), "live", "ekch", SyncWorkBayUpdate, 0)

	rm := collectMetrics(t, reader)

	recalculations := findInt64MetricValue(t, rm, "euroscope.sync.follow_up_work", map[string]string{
		"session_name": "LIVE",
		"airport":      "EKCH",
		"kind":         SyncWorkRouteRecalc,
	})
	if recalculations != 12 {
		t.Fatalf("expected 12 route recalculations, got %d", recalculations)
	}

	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "euroscope.sync.follow_up_work" {
				continue
			}
			data, ok := metric.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, point := range data.DataPoints {
				if attributesMatch(point.Attributes, map[string]string{"kind": SyncWorkBayUpdate}) {
					t.Fatal("expected no series for follow-up work with a zero count")
				}
			}
		}
	}
}

func TestCDMRecalculationRecordsSizeAndOutcome(t *testing.T) {
	reader := newTestReader(t)

	RecordCDMRecalculation(context.Background(), "ekch", 42, 100*time.Millisecond, true, true)

	rm := collectMetrics(t, reader)

	count := findInt64MetricValue(t, rm, "cdm.recalculations", map[string]string{
		"airport": "EKCH",
		"outcome": "success",
		"notify":  "true",
	})
	if count != 1 {
		t.Fatalf("expected 1 recalculation, got %d", count)
	}

	duration := findFloat64HistogramSum(t, rm, "cdm.recalculation.duration", map[string]string{
		"airport": "EKCH",
		"outcome": "success",
	})
	if duration != 0.1 {
		t.Fatalf("expected 0.1s recalculation duration, got %v", duration)
	}

	strips := findInt64HistogramSum(t, rm, "cdm.recalculation.strips", map[string]string{"airport": "EKCH"})
	if strips != 42 {
		t.Fatalf("expected 42 strips, got %d", strips)
	}
}

func TestCDMRecalculationSkipsUnknownStripCount(t *testing.T) {
	reader := newTestReader(t)

	RecordCDMRecalculation(context.Background(), "ekch", -1, time.Millisecond, false, false)

	rm := collectMetrics(t, reader)

	failures := findInt64MetricValue(t, rm, "cdm.recalculations", map[string]string{
		"airport": "EKCH",
		"outcome": "failure",
	})
	if failures != 1 {
		t.Fatalf("expected 1 failed recalculation, got %d", failures)
	}

	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == "cdm.recalculation.strips" {
				t.Fatal("expected no strip histogram when the strip count is unknown")
			}
		}
	}
}

func TestHubDispatchRecordsDepthFanoutAndDuration(t *testing.T) {
	reader := newTestReader(t)

	RecordHubDispatch(context.Background(), "frontend", "broadcast", 7, 25, 2*time.Millisecond)

	rm := collectMetrics(t, reader)
	want := map[string]string{"source": "frontend", "kind": "broadcast"}

	if depth := findInt64HistogramSum(t, rm, "websocket.hub.queue.depth", want); depth != 7 {
		t.Fatalf("expected queue depth 7, got %d", depth)
	}
	if fanout := findInt64HistogramSum(t, rm, "websocket.hub.broadcast.fanout", want); fanout != 25 {
		t.Fatalf("expected fanout 25, got %d", fanout)
	}
	if duration := findFloat64HistogramSum(t, rm, "websocket.hub.dispatch.duration", want); duration != 0.002 {
		t.Fatalf("expected 0.002s dispatch duration, got %v", duration)
	}
}

func TestHubMetricsCollapseUnknownSourceAndKind(t *testing.T) {
	reader := newTestReader(t)

	RecordHubDispatch(context.Background(), "EKCH_TWR", "session-42", 1, 1, time.Millisecond)

	rm := collectMetrics(t, reader)

	if value := findInt64HistogramSum(t, rm, "websocket.hub.queue.depth", map[string]string{
		"source": "other",
		"kind":   "other",
	}); value != 1 {
		t.Fatalf("expected unknown source and kind to collapse to \"other\", got %d", value)
	}
}

func TestHubPublishBlockedRecordsWait(t *testing.T) {
	reader := newTestReader(t)

	RecordHubPublishBlocked(context.Background(), "euroscope", 50*time.Millisecond)

	rm := collectMetrics(t, reader)

	count := findInt64MetricValue(t, rm, "websocket.hub.publish.blocked", map[string]string{"source": "euroscope"})
	if count != 1 {
		t.Fatalf("expected 1 blocked publish, got %d", count)
	}

	wait := findFloat64HistogramSum(t, rm, "websocket.hub.publish.blocked.duration", map[string]string{"source": "euroscope"})
	if wait != 0.05 {
		t.Fatalf("expected 0.05s blocked duration, got %v", wait)
	}
}

func TestSlowConsumerDisconnectIsLabelledBySession(t *testing.T) {
	reader := newTestReader(t)

	RecordSlowConsumerDisconnect(context.Background(), "live", "ekch", "frontend")

	rm := collectMetrics(t, reader)

	count := findInt64MetricValue(t, rm, "websocket.clients.slow_disconnects", map[string]string{
		"session_name": "LIVE",
		"airport":      "EKCH",
		"source":       "frontend",
	})
	if count != 1 {
		t.Fatalf("expected 1 slow consumer disconnect, got %d", count)
	}
}
