package websocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	es "FlightStrips/pkg/events/euroscope"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestRequiredEuroScopeSubprotocolRejectsOldClient(t *testing.T) {
	hub := &dispatchHub{}
	u := NewConnectionUpgrader[es.EventType, *dispatchClient](hub, nil)
	u.RequireSubprotocol("flightstrips.euroscope.pb.v2")
	server := httptest.NewServer(http.HandlerFunc(u.Upgrade))
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http")
	_, response, err := websocket.DefaultDialer.Dial(url, nil)
	require.Error(t, err)
	require.Equal(t, http.StatusForbidden, response.StatusCode)

	dialer := websocket.Dialer{Subprotocols: []string{"flightstrips.euroscope.pb.v1"}}
	_, response, err = dialer.Dial(url, nil)
	require.Error(t, err)
	require.Equal(t, http.StatusForbidden, response.StatusCode)
}
