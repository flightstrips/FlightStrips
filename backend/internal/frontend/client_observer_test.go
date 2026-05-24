package frontend

import (
	"testing"
	"time"

	"FlightStrips/pkg/events"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientCanHandleMessage_ReadOnlyAllowsTokenOnly(t *testing.T) {
	client := &Client{readOnly: true}

	assert.NoError(t, client.CanHandleMessage("token"))
	assert.Error(t, client.CanHandleMessage("move"))
}

func TestClientCanHandleMessage_WritableAllowsMutations(t *testing.T) {
	client := &Client{readOnly: false}

	assert.NoError(t, client.CanHandleMessage("move"))
}

func TestClientEnqueue_DisconnectsSlowConsumer(t *testing.T) {
	hub := &Hub{unregister: make(chan *Client, 1)}
	client := &Client{
		hub:    hub,
		send:   make(chan events.OutgoingMessage, 1),
		closed: make(chan struct{}),
	}

	client.send <- frontendEvents.DisconnectEvent{}

	ok := client.Enqueue(frontendEvents.DisconnectEvent{})

	require.False(t, ok)
	select {
	case got := <-hub.unregister:
		assert.Same(t, client, got)
	case <-time.After(time.Second):
		t.Fatal("expected slow client to be unregistered")
	}
}
