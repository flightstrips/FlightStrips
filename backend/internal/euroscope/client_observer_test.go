package euroscope

import (
	"testing"
	"time"

	"FlightStrips/pkg/events"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientCanHandleMessage_ObserverAllowsRunwayValidation(t *testing.T) {
	client := &Client{observer: true}

	assert.NoError(t, client.CanHandleMessage("token"))
	assert.NoError(t, client.CanHandleMessage("login"))
	assert.NoError(t, client.CanHandleMessage("runway"))
	assert.Error(t, client.CanHandleMessage("sync"))
}

func TestClientCanHandleMessage_ActiveClientAllowsTelemetry(t *testing.T) {
	client := &Client{observer: false}

	assert.NoError(t, client.CanHandleMessage("sync"))
}

func TestClientEnqueue_DisconnectsSlowConsumer(t *testing.T) {
	hub := &Hub{unregister: make(chan *Client, 1)}
	client := &Client{
		hub:    hub,
		send:   make(chan events.OutgoingMessage, 1),
		closed: make(chan struct{}),
	}

	client.send <- euroscopeEvents.SessionInfoEvent{}

	ok := client.Enqueue(euroscopeEvents.SessionInfoEvent{})

	require.False(t, ok)
	select {
	case got := <-hub.unregister:
		assert.Same(t, client, got)
	case <-time.After(time.Second):
		t.Fatal("expected slow client to be unregistered")
	}
}
