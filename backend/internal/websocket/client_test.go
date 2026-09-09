package websocket

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	frontend "FlightStrips/pkg/events/frontend"
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	gorilla "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type recordingMessageWriter struct {
	messageType int
	payload     []byte
	writeErr    error
}

func (w *recordingMessageWriter) WriteMessage(messageType int, payload []byte) error {
	w.messageType = messageType
	w.payload = append([]byte(nil), payload...)
	return w.writeErr
}

func TestWriteOutboundMessagePassesSerializedPayloadUnchanged(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previousProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() { otel.SetMeterProvider(previousProvider) })

	writer := &recordingMessageWriter{}
	payload := []byte(`{"type":"strip_update","callsign":"SAS123"}`)

	err := writeOutboundMessage(context.Background(), writer, "frontend", "strip_update", payload)

	assert.NoError(t, err)
	assert.Equal(t, gorilla.TextMessage, writer.messageType)
	assert.Equal(t, payload, writer.payload)
	assert.Len(t, writer.payload, len(payload))

	var rm metricdata.ResourceMetrics
	assert.NoError(t, reader.Collect(context.Background(), &rm))
	var recordedBytes int64
	for _, scope := range rm.ScopeMetrics {
		for _, recorded := range scope.Metrics {
			if recorded.Name != "websocket.message.bytes.sent" {
				continue
			}
			for _, point := range recorded.Data.(metricdata.Sum[int64]).DataPoints {
				recordedBytes += point.Value
			}
		}
	}
	assert.Equal(t, int64(len(writer.payload)), recordedBytes)
}

type testClient struct{}

func (testClient) Close() error { return nil }

func (testClient) GetCid() string { return "10000001" }

func (testClient) GetCallsign() string { return "EKCH_APP" }

func (testClient) GetAirport() string { return "EKCH" }

func (testClient) GetPosition() string { return "APP" }

func (testClient) GetSession() int32 { return 42 }

func (testClient) GetSessionName() string { return "LIVE" }

func (testClient) GetSource() string { return "euroscope" }

func (testClient) GetVersion() string { return "0.16.0" }

func (testClient) GetConnection() *gorilla.Conn { return nil }

func (testClient) IsAuthenticated() bool { return true }

func (testClient) SetUser(shared.AuthenticatedUser) {}

func (testClient) CanHandleMessage(string) error { return nil }

func (testClient) HandlePong() error { return nil }

func (testClient) GetSendChannel() chan events.OutgoingMessage { return nil }

func (testClient) Enqueue(events.OutgoingMessage) bool { return true }

func (testClient) RecordMessage([]byte) {}

func TestLogReadError_LogsCloseReason(t *testing.T) {
	var buffer bytes.Buffer
	previousLogger := slog.Default()
	logger := slog.New(slog.NewJSONHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	logReadError(testClient{}, &gorilla.CloseError{
		Code: gorilla.CloseNormalClosure,
		Text: "CloudFlare WebSocket proxy restarting",
	})

	output := strings.TrimSpace(buffer.String())
	assert.Contains(t, output, `"msg":"Websocket connection closed"`)
	assert.Contains(t, output, `"level":"INFO"`)
	assert.Contains(t, output, `"source":"euroscope"`)
	assert.Contains(t, output, `"close_code":1000`)
	assert.Contains(t, output, `"reason":"CloudFlare WebSocket proxy restarting"`)
	assert.Contains(t, output, `"callsign":"EKCH_APP"`)
}

func TestLogReadError_LogsNonCloseErrorWithClientDetails(t *testing.T) {
	var buffer bytes.Buffer
	previousLogger := slog.Default()
	logger := slog.New(slog.NewJSONHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
	})

	logReadError(testClient{}, errors.New("unexpected EOF"))

	output := strings.TrimSpace(buffer.String())
	assert.Contains(t, output, `"msg":"Websocket connection closed"`)
	assert.Contains(t, output, `"level":"WARN"`)
	assert.Contains(t, output, `"source":"euroscope"`)
	assert.Contains(t, output, `"error":"unexpected EOF"`)
	assert.Contains(t, output, `"callsign":"EKCH_APP"`)
}

func TestActionRejectedEvent_PreservesForceAssumeRequestID(t *testing.T) {
	payload := []byte(`{"type":"coordination_force_assume_request","callsign":"SAS123","request_id":"SAS123-1"}`)

	rejection := actionRejectedEvent(string(frontend.CoordinationForceAssumeRequestType), payload, errors.New("not allowed"))

	assert.Equal(t, string(frontend.CoordinationForceAssumeRequestType), rejection.Action)
	assert.Equal(t, "not allowed", rejection.Reason)
	assert.Equal(t, "SAS123-1", rejection.RequestID)
}

func TestActionRejectedEvent_DoesNotParseOtherActions(t *testing.T) {
	rejection := actionRejectedEvent("move", []byte(`{"request_id":"SAS123-1"}`), errors.New("not allowed"))

	assert.Empty(t, rejection.RequestID)
}

var _ Client = testClient{}
var _ Client = testClient{}
