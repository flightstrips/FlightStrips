package cluster

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

func TestPositionReplayStatusReportsIncompleteProofWithoutAdvancingReadiness(t *testing.T) {
	created := time.Now()
	sub := &nats.Subscription{}
	p := &Projection{presenceReady: true, positionReplayProblem: "position consumer generation changed", positionCursor: positionCursor{sub: sub, consumer: "current", created: created, appliedConsumer: 1, appliedStream: 5, retained: map[string]uint64{"redacted-key": 5}}}
	info := &nats.StreamInfo{Created: created, Config: nats.StreamConfig{MaxMsgsPerSubject: 1, MaxMsgs: -1, MaxBytes: -1}, State: nats.StreamState{FirstSeq: 5, LastSeq: 9, Msgs: 2}}
	ci := &nats.ConsumerInfo{Name: "current", Delivered: nats.SequenceInfo{Consumer: 2, Stream: 9}, NumPending: 0}
	p.mu.Lock()
	caught, err := p.verifyPositionCursorLocked(sub, info, ci)
	p.mu.Unlock()
	require.NoError(t, err)
	require.False(t, caught)
	require.False(t, p.positionReady)
	// JS is intentionally nil: diagnostics must read only captured local data,
	// including the failed proof's broker frontiers, without making requests.
	status := p.PositionReplayStatus()
	require.Contains(t, status, "position_ready=false presence_ready=true consumer=current")
	require.Contains(t, status, "applied_consumer=1 applied_stream=5")
	require.Contains(t, status, "stream_first=5 stream_head=9 stream_messages=2")
	require.Contains(t, status, "delivered_consumer=2 delivered_stream=9 pending=0")
	require.Contains(t, status, "position consumer generation changed")
	require.NotContains(t, status, "redacted-key")
}
