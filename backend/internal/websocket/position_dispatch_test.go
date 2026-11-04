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
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type dispatchClient struct {
	testClient
	conn       *gorilla.Conn
	dispatcher *shared.PositionDispatcher
	auth       func() bool
	receipt    func()
}

func (c *dispatchClient) GetConnection() *gorilla.Conn                   { return c.conn }
func (c *dispatchClient) PositionDispatcher() *shared.PositionDispatcher { return c.dispatcher }
func (c *dispatchClient) IsAuthenticated() bool {
	if c.auth != nil {
		return c.auth()
	}
	return true
}
func (c *dispatchClient) PositionFence() func(context.Context) context.Context {
	if c.receipt != nil {
		c.receipt()
	}
	return func(ctx context.Context) context.Context { return ctx }
}

type dispatchHub struct {
	handlers shared.MessageHandlers[es.EventType, *dispatchClient]
	done     chan struct{}
}

func TestQueuedFrameRechecksAuthenticationAtApplication(t *testing.T) {
	h := &dispatchHub{handlers: shared.NewMessageHandlers[es.EventType, *dispatchClient](), done: make(chan struct{})}
	var valid atomic.Bool
	valid.Store(true)
	var received, writes atomic.Int32
	entered, release, queued, expired := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var expiredOnce sync.Once
	h.handlers.Add(es.PositionUpdate, func(_ context.Context, _ *dispatchClient, m shared.Message[es.EventType]) error {
		var event es.AircraftPositionUpdateEvent
		if err := m.ProtoUnmarshal(&event); err != nil {
			return err
		}
		if event.Altitude == 1 {
			close(entered)
			<-release
		} else {
			writes.Add(1)
		}
		return nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&gorilla.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		client := &dispatchClient{conn: conn, dispatcher: shared.NewPositionDispatcher(1, 16, nil), auth: func() bool {
			v := valid.Load()
			if !v {
				expiredOnce.Do(func() { close(expired) })
			}
			return v
		}, receipt: func() {
			if received.Add(1) == 2 {
				close(queued)
			}
		}}
		ReadPump[es.EventType](h, client)
	}))
	defer server.Close()
	conn, _, err := gorilla.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer conn.Close()
	for _, altitude := range []int64{1, 2} {
		frame, err := es.MarshalEnvelope(&es.AircraftPositionUpdateEvent{Callsign: "SAS123", Altitude: altitude}, es.PositionUpdate)
		require.NoError(t, err)
		require.NoError(t, conn.WriteMessage(gorilla.BinaryMessage, frame))
		if altitude == 1 {
			<-entered
		}
	}
	<-queued
	valid.Store(false)
	close(release)
	select {
	case <-expired:
	case <-time.After(time.Second):
		t.Fatal("queued frame did not validate expired authentication")
	}
	conn.Close()
	select {
	case <-h.done:
	case <-time.After(time.Second):
		t.Fatal("reader did not finish")
	}
	require.Zero(t, writes.Load(), "expired queued authority must never reach its handler")
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

func TestReadPumpOnlyDispatchesPositionsAndPreservesLifecycleOrdering(t *testing.T) {
	h := &dispatchHub{handlers: shared.NewMessageHandlers[es.EventType, *dispatchClient](), done: make(chan struct{})}
	entered, release, other, operational, disconnected, finished := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
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
	h.handlers.Add(es.SetHeading, func(context.Context, *dispatchClient, shared.Message[es.EventType]) error {
		add("heading")
		close(operational)
		return nil
	})
	h.handlers.Add(es.AircraftDisconnected, func(context.Context, *dispatchClient, shared.Message[es.EventType]) error {
		add("disconnect")
		close(disconnected)
		return nil
	})
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
	select {
	case <-other:
	case <-time.After(time.Second):
		t.Fatal("B blocked behind A")
	}
	select {
	case <-operational:
	case <-time.After(time.Second):
		t.Fatal("operational message blocked behind position backlog")
	}
	mu.Lock()
	require.Contains(t, order, "B1")
	require.Contains(t, order, "heading")
	require.NotContains(t, order, "A1")
	mu.Unlock()
	send(&es.AircraftDisconnectEvent{Callsign: "A"}, es.AircraftDisconnected)
	select {
	case <-disconnected:
		t.Fatal("disconnect overtook an earlier position")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("disconnect did not run after the position backlog drained")
	}
	send(&es.AircraftPositionUpdateEvent{Callsign: "A", Altitude: 3}, es.PositionUpdate)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("reader did not finish")
	}
	mu.Lock()
	var aircraftA []string
	for _, item := range order {
		if strings.HasPrefix(item, "A") {
			aircraftA = append(aircraftA, item)
		}
	}
	require.Equal(t, []string{"A1", "A2", "A3"}, aircraftA)
	require.Less(t, slices.Index(order, "A2"), slices.Index(order, "disconnect"))
	require.Less(t, slices.Index(order, "disconnect"), slices.Index(order, "A3"))
	mu.Unlock()
	conn.Close()
	select {
	case <-h.done:
	case <-time.After(time.Second):
		t.Fatal("workers did not clean up")
	}
}
