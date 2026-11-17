package frontend

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"github.com/stretchr/testify/require"
)

func requestSnapshotsConcurrently(hub *Hub, client *Client) {
	var workers sync.WaitGroup
	for range 64 {
		workers.Add(1)
		go func() { defer workers.Done(); hub.RequestSnapshot(client) }()
	}
	workers.Wait()
}

func TestSnapshotRequestsCoalesceWhileQueued(t *testing.T) {
	client := &Client{session: WaitingForEuroscopeConnectionSessionId, closed: make(chan struct{}), send: make(chan events.OutgoingMessage, 1)}
	hub := &Hub{snapshotRequests: make(chan *Client, 128), clients: map[*Client]bool{client: true}}
	requestSnapshotsConcurrently(hub, client)
	require.Len(t, hub.snapshotRequests, 1)
	queued := <-hub.snapshotRequests
	requestSnapshotsConcurrently(hub, client) // Still queued for processing by the hub.
	require.Empty(t, hub.snapshotRequests)
	hub.handleSnapshotRequest(context.Background(), queued)
	require.IsType(t, frontendEvents.DisconnectEvent{}, <-client.send)
	client.snapshotRefreshNotBefore.Store(nil) // Advance past the cooldown.
	hub.RequestSnapshot(client)
	require.Len(t, hub.snapshotRequests, 1, "completed refresh must allow another request")
}

func TestSnapshotRequestsDuringBuildScheduleExactlyOneFollowUp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	hub := newAMANInitialTestHub(t, nil)
	hub.snapshotRequests = make(chan *Client, 128)
	client := &Client{session: 42, airport: "EKCH", user: validFrontendUser("1234567"), readOnly: true, closed: make(chan struct{}), send: make(chan events.OutgoingMessage, 4)}
	hub.clients = map[*Client]bool{client: true}
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var builds atomic.Int32
	hub.server.(*testutil.MockServer).ControllerRepoVal.(*testutil.MockControllerRepository).ListBySessionFn = func(ctx context.Context, _ int32) ([]*internalModels.Controller, error) {
		if builds.Add(1) == 1 {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return []*internalModels.Controller{}, nil
	}
	hub.RequestSnapshot(client)
	first := <-hub.snapshotRequests
	go func() { hub.handleSnapshotRequest(ctx, first); close(finished) }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("snapshot build did not start")
	}
	deadline := client.snapshotRefreshNotBefore.Load()
	require.NotNil(t, deadline)
	require.True(t, time.Until(*deadline) > 0)
	soon := time.Now().Add(50 * time.Millisecond)
	client.snapshotRefreshNotBefore.Store(&soon) // Advance near the end of the cooldown.
	requestSnapshotsConcurrently(hub, client)
	require.True(t, client.snapshotRefreshPending.Load())
	require.Eventually(t, func() bool { return len(hub.snapshotRequests) == 1 }, time.Second, time.Millisecond, "requests during a build must schedule one follow-up")
	releaseOnce.Do(func() { close(release) })
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("snapshot build did not finish")
	}
	require.True(t, client.snapshotRefreshPending.Load(), "finishing a build must preserve the queued follow-up flag")
	requestSnapshotsConcurrently(hub, client)
	require.Len(t, hub.snapshotRequests, 1)
	hub.handleSnapshotRequest(ctx, <-hub.snapshotRequests)
	require.EqualValues(t, 2, builds.Load())
	require.Empty(t, hub.snapshotRequests)
	require.False(t, client.snapshotRefreshPending.Load())
	for range 2 {
		require.IsType(t, frontendEvents.InitialEvent{}, <-client.send)
	}
}

func TestSnapshotRequestsDuringCooldownCoalesceAndIgnoreClosedClients(t *testing.T) {
	hub := &Hub{snapshotRequests: make(chan *Client, 128)}
	client := &Client{closed: make(chan struct{})}
	deadline := time.Now().Add(50 * time.Millisecond)
	client.snapshotRefreshNotBefore.Store(&deadline)
	requestSnapshotsConcurrently(hub, client)
	if time.Now().Before(deadline) {
		require.Empty(t, hub.snapshotRequests, "cooldown must defer the build")
	}
	require.True(t, client.snapshotRefreshPending.Load())
	require.Eventually(t, func() bool { return len(hub.snapshotRequests) == 1 }, time.Second, time.Millisecond)
	require.Same(t, client, <-hub.snapshotRequests)
	client.snapshotRefreshPending.Store(false)
	deadline = time.Now().Add(50 * time.Millisecond)
	client.snapshotRefreshNotBefore.Store(&deadline)
	hub.RequestSnapshot(client)
	close(client.closed)
	require.Eventually(t, func() bool { return !client.snapshotRefreshPending.Load() }, time.Second, time.Millisecond)
	require.Empty(t, hub.snapshotRequests, "a deferred request must not requeue a closed client")
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
