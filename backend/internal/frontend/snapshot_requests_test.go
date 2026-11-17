package frontend

import (
	"context"
	"sync"
	"testing"

	"FlightStrips/pkg/events"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"github.com/stretchr/testify/require"
)

func TestSnapshotRequestsCoalesceUntilRefreshFinishes(t *testing.T) {
	client := &Client{session: WaitingForEuroscopeConnectionSessionId, closed: make(chan struct{}), send: make(chan events.OutgoingMessage, 1)}
	hub := &Hub{snapshotRequests: make(chan *Client, 128), clients: map[*Client]bool{client: true}}
	requestConcurrently := func() {
		var workers sync.WaitGroup
		for range 64 {
			workers.Add(1)
			go func() { defer workers.Done(); hub.RequestSnapshot(client) }()
		}
		workers.Wait()
	}
	requestConcurrently()
	require.Len(t, hub.snapshotRequests, 1)
	queued := <-hub.snapshotRequests
	requestConcurrently() // Dequeued refresh is still in progress.
	require.Empty(t, hub.snapshotRequests)
	hub.handleSnapshotRequest(context.Background(), queued)
	require.IsType(t, frontendEvents.DisconnectEvent{}, <-client.send)
	hub.RequestSnapshot(client)
	require.Len(t, hub.snapshotRequests, 1, "completed refresh must allow another request")
}

func TestSnapshotRequestCanRetryAfterQueueFullOrUnregister(t *testing.T) {
	hub := &Hub{snapshotRequests: make(chan *Client, 1)}
	first, second := &Client{}, &Client{}
	hub.RequestSnapshot(first)
	hub.RequestSnapshot(second)
	require.False(t, second.snapshotRefreshPending.Load(), "dropped request must not remain pending")
	hub.handleSnapshotRequest(context.Background(), <-hub.snapshotRequests) // Unregistered client.
	require.False(t, first.snapshotRefreshPending.Load())
	hub.RequestSnapshot(second)
	require.Same(t, second, <-hub.snapshotRequests)
}
