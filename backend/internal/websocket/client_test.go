package websocket

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	gorilla "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
)

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

var _ Client = testClient{}
var _ Client = testClient{}
