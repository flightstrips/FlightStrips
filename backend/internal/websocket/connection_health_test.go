package websocket

import (
	"net/http/httptest"
	"strings"
	"testing"

	"FlightStrips/pkg/events"
	"github.com/stretchr/testify/require"
)

type healthTestClient struct {
	testClient
	authenticated bool
	session       int32
	response      events.OutgoingMessage
}

func (c *healthTestClient) IsAuthenticated() bool { return c.authenticated }
func (c *healthTestClient) GetSession() int32     { return c.session }
func (c *healthTestClient) Enqueue(message events.OutgoingMessage) bool {
	c.response = message
	return true
}

func TestConnectionHealthRequiresAuthenticationAndDoesNotChangeAuthority(t *testing.T) {
	client := &healthTestClient{session: -1}
	request := []byte(`{"type":"connection_health","sequence":"1","stage":"ready","request_snapshot":true,"board":{"session_id":123}}`)
	_, err := handleConnectionHealth(client, request)
	require.Error(t, err)
	require.Nil(t, client.response)
	client.authenticated = true
	snapshot, err := handleConnectionHealth(client, request)
	require.NoError(t, err)
	require.True(t, snapshot)
	require.EqualValues(t, -1, client.session, "a reported stage cannot associate or authorize a board")
	require.Equal(t, connectionHealth{Type: "connection_health", Protocol: 1, Sequence: "1", WaitingForPlugin: true}, client.response)
	for _, payload := range [][]byte{[]byte(`{"sequence":""}`), []byte(`{"sequence":"` + strings.Repeat("x", 65) + `"}`), []byte(strings.Repeat("x", 2049))} {
		_, err := handleConnectionHealth(client, payload)
		require.Error(t, err)
	}
}

func TestDiagnosticsHavePhysicalIDsAndBoundUntrustedHeaders(t *testing.T) {
	request := httptest.NewRequest("GET", "/frontEndEvents", nil)
	request.RemoteAddr = "192.0.2.1:1234"
	request.Header.Set("CF-Ray", strings.Repeat("x", 200)+"\nspoofed")
	request.Header.Set("CF-Connecting-IP", "198.51.100.1")
	first, second := newConnectionDiagnostics(request), newConnectionDiagnostics(request)
	require.NotEqual(t, first.id, second.id)
	require.Len(t, first.ray, 128)
	require.Equal(t, "192.0.2.1", first.peer)
	require.Equal(t, "198.51.100.1", first.reportedIP)
}
