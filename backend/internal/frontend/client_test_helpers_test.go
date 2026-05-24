package frontend

import (
	"FlightStrips/pkg/events"
	"testing"
	"time"
)

func startQueuedTestClient(client *Client) *Client {
	if client.send == nil {
		client.send = make(chan events.OutgoingMessage, 1)
	}
	client.closed = make(chan struct{})
	return client
}

func waitForOutgoingMessage(t *testing.T, ch <-chan events.OutgoingMessage) events.OutgoingMessage {
	t.Helper()
	select {
	case message := <-ch:
		return message
	case <-time.After(time.Second):
		t.Fatal("expected outgoing message")
		return nil
	}
}
