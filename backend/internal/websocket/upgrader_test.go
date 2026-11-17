package websocket

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	fe "FlightStrips/pkg/events/frontend"
	gorilla "github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type upgradeTestClient struct {
	testClient
	conn *gorilla.Conn
	send chan events.OutgoingMessage
}

func (c *upgradeTestClient) GetConnection() *gorilla.Conn                { return c.conn }
func (c *upgradeTestClient) GetSource() string                           { return "frontend" }
func (c *upgradeTestClient) GetSendChannel() chan events.OutgoingMessage { return c.send }
func (c *upgradeTestClient) Enqueue(message events.OutgoingMessage) bool {
	select {
	case c.send <- message:
		return true
	default:
		return false
	}
}

type upgradeTestHub struct {
	decodeErr, admissionErr error
	snapshotRequests        chan *upgradeTestClient
}

func (h *upgradeTestHub) RequestSnapshot(client *upgradeTestClient) {
	if h.snapshotRequests != nil {
		h.snapshotRequests <- client
	}
}

func (h *upgradeTestHub) DecodeAuthentication(int, []byte) (events.AuthenticationEvent, error) {
	return events.AuthenticationEvent{Token: "token"}, h.decodeErr
}
func (h *upgradeTestHub) HandleNewConnection(conn *gorilla.Conn, _ shared.AuthenticatedUser, _ events.AuthenticationEvent) (*upgradeTestClient, error) {
	if h.admissionErr != nil {
		return nil, h.admissionErr
	}
	client := &upgradeTestClient{conn: conn, send: make(chan events.OutgoingMessage, 8)}
	client.send <- fe.DisconnectEvent{}
	return client, nil
}
func (*upgradeTestHub) Unregister(client *upgradeTestClient) { close(client.send) }
func (*upgradeTestHub) GetMessageHandlers() shared.MessageHandlers[string, *upgradeTestClient] {
	return shared.NewMessageHandlers[string, *upgradeTestClient]()
}

type upgradeTestAuth struct{ err error }

func (a upgradeTestAuth) Validate(string) (shared.AuthenticatedUser, error) {
	return shared.NewAuthenticatedUser("123", 0, nil), a.err
}

func TestAdmittedFrontendNegotiatesHealthAndRefreshesItsExistingSubscription(t *testing.T) {
	hub := &upgradeTestHub{snapshotRequests: make(chan *upgradeTestClient, 1)}
	upgrader := NewConnectionUpgrader[string, *upgradeTestClient](hub, upgradeTestAuth{})
	server := httptest.NewServer(http.HandlerFunc(upgrader.Upgrade))
	defer server.Close()
	conn, response, err := gorilla.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
	require.NoError(t, conn.WriteMessage(gorilla.TextMessage, []byte(`{"type":"token","token":"token"}`)))
	_, _, err = conn.ReadMessage() // The existing initial/disconnect frame.
	require.NoError(t, err)
	var welcome connectionHealth
	require.NoError(t, conn.ReadJSON(&welcome))
	require.Equal(t, "connection_health", welcome.Type)
	require.Equal(t, response.Header.Get("X-Flightstrips-Connection-Id"), welcome.ConnectionID)
	require.NoError(t, conn.WriteMessage(gorilla.TextMessage, []byte(`{"type":"connection_health","sequence":"123","stage":"ready","request_snapshot":true,"board":{"session_id":999}}`)))
	var acknowledgement connectionHealth
	require.NoError(t, conn.ReadJSON(&acknowledgement))
	require.Equal(t, "123", acknowledgement.Sequence)
	require.Equal(t, welcome.ConnectionID, acknowledgement.ConnectionID)
	select {
	case existing := <-hub.snapshotRequests:
		require.Equal(t, welcome.ConnectionID, diagnostics(existing.GetConnection()).id)
		require.EqualValues(t, 42, existing.GetSession(), "a health payload cannot redirect the subscription")
	case <-time.After(time.Second):
		t.Fatal("snapshot was not requested")
	}
}

func TestUpgradeBoundsMissingAuthenticationAndReturnsPhysicalID(t *testing.T) {
	upgrader := NewConnectionUpgrader[string, *upgradeTestClient](&upgradeTestHub{}, upgradeTestAuth{})
	upgrader.authenticationTimeout = 30 * time.Millisecond
	finished := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader.Upgrade(w, r)
		close(finished)
	}))
	defer server.Close()
	conn, response, err := gorilla.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer conn.Close()
	require.NotEmpty(t, response.Header.Get("X-Flightstrips-Connection-Id"))
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("authentication deadline was not enforced")
	}
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
	_, _, err = conn.ReadMessage()
	require.Error(t, err)
}

func TestUpgradeClosesUnadmittedSocketAndAllowsRetry(t *testing.T) {
	for _, stage := range []string{"decode", "authentication", "stale membership", "transient repository"} {
		t.Run(stage, func(t *testing.T) {
			hub := &upgradeTestHub{}
			auth := upgradeTestAuth{}
			failure := errors.New(stage)
			switch stage {
			case "decode":
				hub.decodeErr = failure
			case "authentication":
				auth.err = failure
			default:
				hub.admissionErr = failure
			}
			upgrader := NewConnectionUpgrader[string, *upgradeTestClient](hub, auth)
			completed := make(chan struct{}, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upgrader.Upgrade(w, r)
				completed <- struct{}{}
			}))
			defer server.Close()
			connect := func() *gorilla.Conn {
				conn, _, err := gorilla.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
				require.NoError(t, err)
				t.Cleanup(func() { _ = conn.Close() })
				require.NoError(t, conn.WriteMessage(gorilla.TextMessage, []byte(`{"type":"token","token":"token"}`)))
				select {
				case <-completed:
				case <-time.After(time.Second):
					t.Fatal("upgrade did not finish")
				}
				require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
				return conn
			}
			conn := connect()
			_, _, err := conn.ReadMessage()
			require.Error(t, err)
			var networkErr net.Error
			require.False(t, errors.As(err, &networkErr) && networkErr.Timeout(), "failed admission left an open socket with no reader or writer")

			// The next attempt succeeds with the same identity once the transient
			// condition clears. Successful handoff must keep the pumps alive.
			hub.decodeErr, hub.admissionErr, upgrader.authenticationService = nil, nil, upgradeTestAuth{}
			retry := connect()
			_, payload, err := retry.ReadMessage()
			require.NoError(t, err)
			require.Contains(t, string(payload), "disconnect")
		})
	}
}

type upgradeLogHandler struct {
	slog.Handler
	records chan slog.Record
}

func (h upgradeLogHandler) Handle(_ context.Context, record slog.Record) error {
	if record.Message == "Websocket connection rejected" || record.Message == "Failed to handle new connection" {
		h.records <- record.Clone()
	}
	return nil
}

func TestUpgradeAdmissionRejectionTelemetry(t *testing.T) {
	rejection := shared.RejectAction("selected own membership is unavailable or stale")
	for _, tc := range []struct {
		name  string
		err   error
		level slog.Level
	}{
		{"stale membership", rejection, slog.LevelInfo},
		{"wrapped rejection", fmt.Errorf("authorize board: %w", rejection), slog.LevelInfo},
		{"repository failure", io.ErrUnexpectedEOF, slog.LevelWarn},
		{"untyped lookalike", errors.New(rejection.Error()), slog.LevelWarn},
		{"rejection joined with failure", errors.Join(rejection, io.ErrUnexpectedEOF), slog.LevelWarn},
	} {
		t.Run(tc.name, func(t *testing.T) {
			records := make(chan slog.Record, 1)
			previousLogger := slog.Default()
			slog.SetDefault(slog.New(upgradeLogHandler{
				Handler: slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}),
				records: records,
			}))
			t.Cleanup(func() { slog.SetDefault(previousLogger) })
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			previousProvider := otel.GetTracerProvider()
			otel.SetTracerProvider(provider)
			t.Cleanup(func() {
				otel.SetTracerProvider(previousProvider)
				_ = provider.Shutdown(context.Background())
			})

			upgrader := NewConnectionUpgrader[string, *upgradeTestClient](&upgradeTestHub{admissionErr: tc.err}, upgradeTestAuth{})
			completed := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upgrader.Upgrade(w, r)
				close(completed)
			}))
			defer server.Close()
			conn, _, err := gorilla.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			defer conn.Close()
			require.NoError(t, conn.WriteMessage(gorilla.TextMessage, []byte(`{"type":"token","token":"token"}`)))
			select {
			case <-completed:
			case <-time.After(time.Second):
				t.Fatal("upgrade did not finish")
			}
			require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
			_, _, err = conn.ReadMessage()
			require.Error(t, err)
			var networkErr net.Error
			require.False(t, errors.As(err, &networkErr) && networkErr.Timeout(), "rejection must still close the socket")

			var record slog.Record
			select {
			case record = <-records:
			default:
				t.Fatal("upgrade completed without an admission log record")
			}
			require.Equal(t, tc.level, record.Level)
			attrs := map[string]any{}
			record.Attrs(func(attr slog.Attr) bool { attrs[attr.Key] = attr.Value.Any(); return true })
			require.Equal(t, "123", attrs["cid"])
			require.Equal(t, tc.err, attrs["error"])
			spans := recorder.Ended()
			require.Len(t, spans, 1)
			if tc.level == slog.LevelInfo {
				require.Equal(t, "Websocket connection rejected", record.Message)
				require.Empty(t, spans[0].Events(), "expected rejection must not emit a trace exception")
			} else {
				require.Equal(t, "Failed to handle new connection", record.Message)
				require.Len(t, spans[0].Events(), 1, "unexpected failures must retain their trace exception")
			}
		})
	}
}
