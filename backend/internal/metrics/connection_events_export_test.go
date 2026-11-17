package metrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	collectormetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"
)

func TestConnectionEventBaselineSurvivesOTLPExportBeforeFirstBurst(t *testing.T) {
	requests := make(chan *collectormetrics.ExportMetricsServiceRequest, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		request := &collectormetrics.ExportMetricsServiceRequest{}
		if err := proto.Unmarshal(data, request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- request
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	exporter, err := otlpmetrichttp.New(context.Background(), otlpmetrichttp.WithEndpointURL(server.URL), otlpmetrichttp.WithTimeout(time.Second))
	require.NoError(t, err)
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(time.Hour))))
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	resetInstrumentsForTest()
	t.Cleanup(func() {
		otel.SetMeterProvider(previous)
		resetInstrumentsForTest()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = provider.Shutdown(ctx)
	})
	collect := func() map[string]int64 {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		require.NoError(t, provider.ForceFlush(ctx))
		var request *collectormetrics.ExportMetricsServiceRequest
		select {
		case request = <-requests:
		case <-ctx.Done():
			t.Fatal("no OTLP export received")
		}
		points := map[string]int64{}
		for _, resource := range request.GetResourceMetrics() {
			for _, scope := range resource.GetScopeMetrics() {
				for _, instrument := range scope.GetMetrics() {
					if instrument.GetName() != "websocket.connection.events" {
						continue
					}
					sum := instrument.GetSum()
					require.True(t, sum.GetIsMonotonic())
					require.Equal(t, metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, sum.GetAggregationTemporality())
					for _, point := range sum.GetDataPoints() {
						require.Len(t, point.GetAttributes(), 2)
						labels := map[string]string{}
						for _, attr := range point.GetAttributes() {
							labels[attr.GetKey()] = attr.GetValue().GetStringValue()
						}
						points[labels["source"]+"/"+labels["event"]] = point.GetAsInt()
					}
				}
			}
		}
		return points
	}
	InitializeConnectionEvents()
	baseline := collect()
	require.Len(t, baseline, len(connectionEventSources)*len(connectionEventNames))
	for labels, value := range baseline {
		require.Zero(t, value, labels)
	}
	for range 5 {
		RecordConnectionEvent(context.Background(), "frontend", "read_timeout")
	}
	for range 3 {
		RecordConnectionEvent(context.Background(), "euroscope", "master_assigned")
	}
	after := collect()
	require.EqualValues(t, 5, after["frontend/read_timeout"])
	require.EqualValues(t, 3, after["euroscope/master_assigned"])
}
