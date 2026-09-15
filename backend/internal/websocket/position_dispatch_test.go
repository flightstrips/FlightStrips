package websocket

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	es "FlightStrips/pkg/events/euroscope"
	"context"
	gorilla "github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type dispatchClient struct {
	testClient
	conn       *gorilla.Conn
	dispatcher *shared.PositionDispatcher
}

func (c *dispatchClient) GetConnection() *gorilla.Conn                   { return c.conn }
func (c *dispatchClient) PositionDispatcher() *shared.PositionDispatcher { return c.dispatcher }

type dispatchHub struct {
	handlers shared.MessageHandlers[es.EventType, *dispatchClient]
	done     chan struct{}
}

func (h *dispatchHub) Unregister(*dispatchClient) { close(h.done) }
func (h *dispatchHub) GetMessageHandlers() shared.MessageHandlers[es.EventType, *dispatchClient] {
	return h.handlers
}
func (*dispatchHub) DecodeAuthentication(int, []byte) (events.AuthenticationEvent, error) {
	return events.AuthenticationEvent{}, nil
}
func (*dispatchHub) HandleNewConnection(*gorilla.Conn, shared.AuthenticatedUser, events.AuthenticationEvent) (*dispatchClient, error) {
	panic("unused")
}

func TestReadPumpPositionBarrierAndFIFO(t *testing.T) {
	h := &dispatchHub{handlers: shared.NewMessageHandlers[es.EventType, *dispatchClient](), done: make(chan struct{})}
	entered, release, other, finished := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	var order []string
	add := func(s string) { mu.Lock(); order = append(order, s); mu.Unlock() }
	h.handlers.Add(es.PositionUpdate, func(ctx context.Context, c *dispatchClient, m shared.Message[es.EventType]) error {
		var p es.AircraftPositionUpdateEvent
		if err := m.ProtoUnmarshal(&p); err != nil {
			return err
		}
		if p.Callsign == "A" && p.Altitude == 1 {
			close(entered)
			<-release
		}
		add(p.Callsign + string(rune('0'+p.Altitude)))
		if p.Callsign == "B" {
			close(other)
		}
		if p.Altitude == 3 {
			close(finished)
		}
		return nil
	})
	h.handlers.Add(es.SetHeading, func(context.Context, *dispatchClient, shared.Message[es.EventType]) error { add("barrier"); return nil })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&gorilla.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		c := &dispatchClient{conn: conn, dispatcher: shared.NewPositionDispatcher(4, 256, nil)}
		ReadPump[es.EventType](h, c)
	}))
	defer server.Close()
	conn, _, err := gorilla.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer conn.Close()
	send := func(m proto.Message, kind es.EventType) {
		raw, err := es.MarshalEnvelope(m, kind)
		require.NoError(t, err)
		require.NoError(t, conn.WriteMessage(gorilla.BinaryMessage, raw))
	}
	send(&es.AircraftPositionUpdateEvent{Callsign: "A", Altitude: 1}, es.PositionUpdate)
	<-entered
	send(&es.AircraftPositionUpdateEvent{Callsign: "A", Altitude: 2}, es.PositionUpdate)
	send(&es.AircraftPositionUpdateEvent{Callsign: "B", Altitude: 1}, es.PositionUpdate)
	send(&es.HeadingEvent{Callsign: "A"}, es.SetHeading)
	send(&es.AircraftPositionUpdateEvent{Callsign: "A", Altitude: 3}, es.PositionUpdate)
	select {
	case <-other:
	case <-time.After(time.Second):
		t.Fatal("B blocked behind A")
	}
	mu.Lock()
	require.Equal(t, []string{"B1"}, order)
	mu.Unlock()
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("reader did not finish")
	}
	mu.Lock()
	require.Equal(t, []string{"B1", "A1", "A2", "barrier", "A3"}, order)
	mu.Unlock()
	conn.Close()
	select {
	case <-h.done:
	case <-time.After(time.Second):
		t.Fatal("workers did not clean up")
	}
}
