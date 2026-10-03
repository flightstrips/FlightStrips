package cluster

import (
	"context"
	"fmt"
	"testing"
	"time"

	"FlightStrips/internal/natsresources"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

type queuedStateMessages struct{ messages chan *nats.Msg }

func (s queuedStateMessages) NextMsgWithContext(ctx context.Context) (*nats.Msg, error) {
	select {
	case msg := <-s.messages:
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestStateReplayAdvancesWhileMetadataProofIsBlocked(t *testing.T) {
	nc := asyncConnectedTransport(t)
	require.NoError(t, nc.Flush())
	before := nc.Stats().OutMsgs
	store, writer, ref := fixture(t)
	writer.Execute(context.Background(), command(ref, "Copenhagen", 0))
	subject, err := Subject(ref)
	require.NoError(t, err)
	entries, err := store.Replay(context.Background(), subject)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	p := &Projection{NC: nc, Config: natsresources.Config{Names: natsresources.RequiredNames, URLs: []string{nc.ConnectedUrl()}, ConnectTimeout: time.Second, RequestTimeout: time.Second},
		states: map[string]*Aggregate{}, listeners: map[uint64]*projectionListener{}, lastSnapshot: map[string]time.Time{}, sinceSnapshot: map[string]uint64{},
		started: true, positionReady: true, presenceReady: true, highWater: 2, metadataGeneration: 1}
	ctx, cancel := context.WithCancel(context.Background())
	proofDone := make(chan struct{})
	go func() { defer close(proofDone); p.watchStateMetadata(ctx, 1) }()
	replayDone := make(chan error, 1)
	messages := queuedStateMessages{make(chan *nats.Msg, 2)}
	go func() { replayDone <- p.consumeState(ctx, messages) }()
	t.Cleanup(func() { cancel(); <-proofDone; <-replayDone })
	// The real connected peer deliberately drops metadata responses. A request is
	// demonstrably in flight before either ordered event is handed to replay.
	require.Eventually(t, func() bool { return nc.Stats().OutMsgs > before }, 500*time.Millisecond, time.Millisecond)
	for _, entry := range entries {
		messages.messages <- &nats.Msg{Sub: &nats.Subscription{}, Subject: entry.Subject, Data: entry.Data,
			Reply: fmt.Sprintf("$JS.ACK.FS_STATE.unit.1.%d.%d.%d.0", entry.StreamSequence, entry.StreamSequence, entry.ServerTime.UnixNano())}
	}
	require.Eventually(t, func() bool {
		p.mu.RLock()
		defer p.mu.RUnlock()
		return p.applied == 2 && p.states[subject].Revision == 1
	}, 250*time.Millisecond, time.Millisecond,
		"broker metadata latency must not stall ordered reducer application")
	p.mu.RLock()
	checked := p.checked
	p.mu.RUnlock()
	require.True(t, checked.IsZero(), "replay alone cannot fabricate a metadata proof")
	require.ErrorContains(t, p.Ready(), "metadata is stale")
	_, err = p.Read(ref)
	require.ErrorContains(t, err, "metadata is stale", "accepted state remains unavailable without a fresh proof")
}

func TestStoppedStateReplayCannotBeRevivedByInFlightMetadata(t *testing.T) {
	nc := asyncConnectedTransport(t)
	require.NoError(t, nc.Flush())
	before := nc.Stats().OutMsgs
	p := &Projection{NC: nc, Config: natsresources.Config{Names: natsresources.RequiredNames, URLs: []string{nc.ConnectedUrl()}, ConnectTimeout: time.Second, RequestTimeout: 50 * time.Millisecond}, metadataGeneration: 1}
	done := make(chan struct{})
	go func() { defer close(done); p.refresh(context.Background(), 1) }()
	require.Eventually(t, func() bool { return nc.Stats().OutMsgs > before }, 500*time.Millisecond, time.Millisecond)
	messages := queuedStateMessages{make(chan *nats.Msg, 1)}
	messages.messages <- &nats.Msg{} // no JetStream metadata: a terminal reader error
	failedReplay := p.consumeState(context.Background(), messages)
	require.Error(t, failedReplay)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("metadata request exceeded its bounded lifetime")
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	require.ErrorIs(t, p.healthErr, failedReplay)
	require.True(t, p.checked.IsZero())
}
