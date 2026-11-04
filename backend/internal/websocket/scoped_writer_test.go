package websocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"FlightStrips/pkg/events"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type scopedWriterClient struct {
	testClient
	conn *websocket.Conn
	send chan events.OutgoingMessage
}

func (c *scopedWriterClient) GetConnection() *websocket.Conn              { return c.conn }
func (c *scopedWriterClient) GetSendChannel() chan events.OutgoingMessage { return c.send }

func TestWritePumpSendsScopedControlAndOperationalFrames(t *testing.T) {
	scope := &es.MembershipContext{InstanceId: "instance", TransportGeneration: 7, MembershipId: "member", SessionId: 42, Airport: "EKCH", Environment: "LIVE", Generation: 8, MasterEpoch: 9}
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		client := &scopedWriterClient{conn: conn, send: make(chan events.OutgoingMessage, 2)}
		client.send <- es.NewScopedMessage(&es.MembershipContext{InstanceId: "instance", TransportGeneration: 7}, es.ControlEvent{})
		client.send <- es.NewScopedMessage(scope, es.SessionInfoEvent{Role: es.SessionInfoMaster})
		close(client.send)
		WritePump(client)
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
	for _, kind := range []es.EventType{es.Control, es.SessionInfo} {
		frame, data, err := conn.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, websocket.BinaryMessage, frame)
		got, _, err := es.UnmarshalEnvelope(data)
		require.NoError(t, err)
		require.Equal(t, kind, got)
		var envelope es.Envelope
		require.NoError(t, proto.Unmarshal(data, &envelope))
		require.Equal(t, "instance", envelope.Context.InstanceId)
		if kind == es.SessionInfo {
			require.True(t, proto.Equal(scope, envelope.Context))
		}
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("writer did not exit")
	}
}
