package euroscope

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events"

	gorilla "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleNewConnection_SeedsPendingOnlineOrchestrationForOperationalLogin(t *testing.T) {
	const session = int32(42)
	const callsign = "EKCH_C_TWR"
	const position = "118.580"

	updateLayoutsCalls := 0
	controllerRepo := &testutil.MockControllerRepository{
		GetFn: func(_ context.Context, gotCallsign string, gotSession int32) (*models.Controller, error) {
			assert.Equal(t, callsign, gotCallsign)
			assert.Equal(t, session, gotSession)
			return &models.Controller{
				Callsign: callsign,
				Session:  session,
				Position: position,
			}, nil
		},
		SetCidFn: func(_ context.Context, gotSession int32, gotCallsign string, cid *string) (int64, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			require.NotNil(t, cid)
			assert.Equal(t, "1234567", *cid)
			return 1, nil
		},
		SetObserverFn: func(_ context.Context, gotSession int32, gotCallsign string, observer bool) (int64, error) {
			assert.Equal(t, session, gotSession)
			assert.Equal(t, callsign, gotCallsign)
			assert.False(t, observer)
			return 1, nil
		},
	}

	server := &testutil.MockServer{
		ControllerRepoVal: controllerRepo,
		GetOrCreateSessionFn: func(airport string, name string) (shared.Session, error) {
			assert.Equal(t, "EKCH", airport)
			assert.Equal(t, "LIVE", name)
			return shared.Session{Id: session}, nil
		},
		UpdateLayoutsFn: func(gotSession int32) error {
			assert.Equal(t, session, gotSession)
			updateLayoutsCalls++
			return nil
		},
	}

	hub := &Hub{
		server:                      server,
		register:                    make(chan *Client, 1),
		pendingOnlineOrchestrations: make(map[string]struct{}),
		observerByCid:               make(map[string]bool),
		localIPByClient:             make(map[string]string),
	}

	user := shared.NewAuthenticatedUser("1234567", 0, nil)
	clientCh := make(chan *Client, 1)
	errCh := make(chan error, 1)

	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := gorilla.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			errCh <- err
			return
		}

		client, err := hub.HandleNewConnection(conn, user, events.AuthenticationEvent{})
		if err != nil {
			errCh <- err
			return
		}

		clientCh <- client
	}))
	defer wsServer.Close()

	wsURL := "ws" + strings.TrimPrefix(wsServer.URL, "http")
	wsConn, _, err := gorilla.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer wsConn.Close()

	err = wsConn.WriteMessage(gorilla.TextMessage, buildLoginPayload(t, callsign, position, "EKCH"))
	require.NoError(t, err)

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case client := <-clientCh:
		t.Cleanup(func() { _ = client.Close() })
		assert.Equal(t, callsign, client.callsign)
		assert.Equal(t, position, client.position)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for HandleNewConnection")
	}

	assert.Equal(t, 1, updateLayoutsCalls)
	assert.True(t, hub.consumePendingOnlineOrchestration(session, callsign))
	assert.False(t, hub.consumePendingOnlineOrchestration(session, callsign))
}
