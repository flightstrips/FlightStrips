package metrics

import (
	"testing"

	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestStandPublicationSeparatesSourceMessagesAndFanout(t *testing.T) {
	reader := newTestReader(t)
	RecordStandPublication(t.Context(), "snapshot", 100)
	RecordStandPublication(t.Context(), "block_fallback", 100)
	RecordStandPublication(t.Context(), "session-123", -1)
	RecordStandWebsocketFanout(t.Context(), "stand_assignment_update", 12)
	RecordStandWebsocketFanout(t.Context(), "stand_assignment_update", 0)
	RecordStandWebsocketFanout(t.Context(), "arbitrary-callsign", 500)
	rm := collectMetrics(t, reader)
	if got := findInt64HistogramSum(t, rm, "sat.publication.assignments", map[string]string{"mode": "snapshot"}); got != 100 {
		t.Fatalf("snapshot assignments = %d, want 100", got)
	}
	if got := findInt64HistogramSum(t, rm, "sat.publication.assignments", map[string]string{"mode": "block_fallback"}); got != 100 {
		t.Fatalf("fallback assignments = %d, want 100", got)
	}
	if got := findInt64HistogramSum(t, rm, "sat.publication.assignments", map[string]string{"mode": "other"}); got != 0 {
		t.Fatalf("unknown mode assignments = %d, want 0", got)
	}
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "sat.websocket.fanout" {
				continue
			}
			data := m.Data.(metricdata.Histogram[int64])
			if len(data.DataPoints) != 1 {
				t.Fatalf("unexpected event type cardinality: %d", len(data.DataPoints))
			}
			point := data.DataPoints[0]
			if point.Count != 2 || point.Sum != 12 || point.Attributes.Len() != 1 || !attributesMatch(point.Attributes, map[string]string{"type": "stand_assignment_update"}) {
				t.Fatalf("expected two source messages and 12 enqueue attempts with only a type label: %+v", point)
			}
			return
		}
	}
	t.Fatal("stand fanout metric missing")
}
