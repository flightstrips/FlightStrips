package websocket

import (
	"errors"
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
