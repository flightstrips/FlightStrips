package positionload

import (
	"bytes"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestCaptureWaitDiagnosticsKeepsCountsDistinctFromMilliseconds(t *testing.T) {
	at := time.Now()
	request := &collector.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{
		Name: "euroscope.position.wait_applied", StartTimeUnixNano: uint64(at.UnixNano()), EndTimeUnixNano: uint64(at.Add(9 * time.Millisecond).UnixNano()),
		Attributes: []*common.KeyValue{
			{Key: "position.wait_rechecks", Value: &common.AnyValue{Value: &common.AnyValue_IntValue{IntValue: 3}}},
			{Key: "position.wait_notification_ms", Value: &common.AnyValue{Value: &common.AnyValue_DoubleValue{DoubleValue: 7.5}}},
		},
	}}}}}}}
	data, err := proto.Marshal(request)
	require.NoError(t, err)
	capture := New()
	response := httptest.NewRecorder()
	capture.ServeHTTP(response, httptest.NewRequest("POST", "/v1/traces", bytes.NewReader(data)))
	require.Equal(t, 200, response.Code)
	report := capture.StageReport(at, at.Add(time.Second))
	rechecks := report["position.wait_rechecks"].(map[string]any)
	require.Equal(t, "count", rechecks["unit"])
	require.Equal(t, 3., rechecks["p95"])
	require.NotContains(t, rechecks, "p95_ms")
	wait := report["position.wait_notification_ms"].(map[string]any)
	require.Equal(t, 7.5, wait["p95_ms"])
	require.Equal(t, 9., report["euroscope.position.wait_applied.total_ms"].(map[string]any)["p95_ms"])
	require.Equal(t, 0, capture.StageReport(at.Add(time.Second), at.Add(2*time.Second))["position.wait_rechecks"].(map[string]any)["count"])
}
