package cluster

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/natsresources"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

func TestPositionMetadataLostReplyDoesNotBlockWatcherRetry(t *testing.T) {
	// This connected protocol peer answers PING but deliberately never answers
	// JetStream metadata. Exercise the real client's context handling, including
	// the long parent deadline that otherwise overrides its request timeout.
	nc := asyncConnectedTransport(t)
	var requests atomic.Int32
	js, err := nc.JetStream(nats.MaxWait(time.Second), nats.ClientTrace{RequestSent: func(string, []byte) { requests.Add(1) }})
	require.NoError(t, err)
	p := &Projection{JS: js, Config: natsresources.Config{Names: natsresources.RequiredNames, RequestTimeout: 20 * time.Millisecond}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); p.watchPositions(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	require.Eventually(t, func() bool { return requests.Load() >= 2 }, 500*time.Millisecond, time.Millisecond,
		"a lost initial StreamInfo reply must time out and rebuild while the parent remains live")
	require.NoError(t, ctx.Err())
	p.mu.RLock()
	ready, failure, problem := p.positionReady, p.observationErr, p.positionReplayProblem
	p.mu.RUnlock()
	require.False(t, ready, "no replay proof means no readiness")
	require.Nil(t, failure, "transport timeout remains retryable")
	require.Contains(t, problem, "initial stream metadata")
}

func TestPositionProofLostMetadataReplyUsesRequestTimeout(t *testing.T) {
	nc := asyncConnectedTransport(t)
	js, err := nc.JetStream(nats.MaxWait(time.Second))
	require.NoError(t, err)
	p := &Projection{JS: js, Config: natsresources.Config{Names: natsresources.RequiredNames, RequestTimeout: 20 * time.Millisecond}, positionCursor: positionCursor{sub: &nats.Subscription{}}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := time.Now()
	caught, err := p.provePositionCursor(ctx)
	require.False(t, caught)
	require.True(t, errors.Is(err, context.DeadlineExceeded), "metadata timeout must remain identifiable: %v", err)
	require.Less(t, time.Since(started), 500*time.Millisecond)
	require.NoError(t, ctx.Err(), "per-request timeout must not cancel the watcher")
	require.False(t, p.positionReady)
}
