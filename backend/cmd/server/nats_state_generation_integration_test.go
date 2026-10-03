package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

// This destructive restore scenario may only use this test's private brokers.
// Matching event bytes and sequence numbers do not establish stream identity.
func TestServerNATSStateStreamRecreationFencesReusedProjection(t *testing.T) {
	if os.Getenv("NATS_TASK22") != "1" || os.Getenv("NATS_SERVER_BINARY") == "" {
		t.Skip("requires explicit disposable native Task22 fixture")
	}
	f := newEntrypointFixture(t, true)
	for _, app := range f.apps {
		app.stop()
	}
	f.stopProjection()
	adminConfig := f.resources
	adminConfig.URLs = append([]string(nil), f.resources.URLs...)
	for i, url := range adminConfig.URLs {
		adminConfig.URLs[i] = strings.Replace(url, "backend:backend-local-only", "bootstrap:bootstrap-local-only", 1)
	}
	admin, err := natsresources.Connect(adminConfig)
	require.NoError(t, err)
	t.Cleanup(admin.Close)
	adminJS, err := admin.JetStream(nats.MaxWait(f.resources.RequestTimeout))
	require.NoError(t, err)
	originalInfo, err := adminJS.StreamInfo(f.resources.Names.State, nats.Context(f.ctx))
	require.NoError(t, err)
	require.Greater(t, originalInfo.State.Msgs, uint64(0), "the restore must contain genuinely replayed state")
	require.Equal(t, uint64(1), originalInfo.State.FirstSeq)
	require.Equal(t, originalInfo.State.LastSeq, originalInfo.State.Msgs)
	require.Zero(t, originalInfo.State.NumDeleted)
	messages := make([]*nats.RawStreamMsg, 0, originalInfo.State.Msgs)
	refs := map[string]*pb.AggregateRef{}
	for seq := uint64(1); seq <= originalInfo.State.LastSeq; seq++ {
		msg, err := adminJS.GetMsg(f.resources.Names.State, seq, nats.Context(f.ctx))
		require.NoError(t, err)
		messages = append(messages, msg)
		event := &pb.StateEvent{}
		require.NoError(t, pb.UnmarshalStrict(msg.Data, event))
		refs[msg.Subject] = event.Aggregate
	}
	start := func(p *cluster.Projection) (context.CancelFunc, <-chan error) {
		ctx, cancel := context.WithCancel(f.ctx)
		done := make(chan error, 1)
		go func() { done <- p.Run(ctx) }()
		t.Cleanup(func() { cancel(); <-done })
		return cancel, done
	}
	reused, err := cluster.NewProjection(f.nc, f.resources)
	require.NoError(t, err)
	// Use explicit stop/join ownership; its cleanup must not consume the result a
	// second time when this same object is intentionally run again below.
	runCtx, stop := context.WithCancel(f.ctx)
	stopped := make(chan error, 1)
	go func() { stopped <- reused.Run(runCtx) }()
	joined := false
	t.Cleanup(func() {
		stop()
		if !joined {
			<-stopped
		}
	})
	require.Eventually(t, func() bool { return reused.Ready() == nil }, 15*time.Second, 10*time.Millisecond)
	require.NoError(t, reused.WaitApplied(f.ctx, originalInfo.State.LastSeq))
	expected := map[string]string{}
	for subject, ref := range refs {
		state, err := reused.Read(ref)
		require.NoError(t, err)
		snapshot, err := state.Snapshot()
		require.NoError(t, err)
		expected[subject] = snapshot.Sha256
		// Persist every aggregate's valid original checkpoint. A native restore
		// retains those checkpoints and the complete event log, never a guessed tail.
		require.NoError(t, reused.Snapshots.Save(state))
	}
	stop()
	<-stopped
	joined = true
	stableInfo, err := adminJS.StreamInfo(f.resources.Names.State, nats.Context(f.ctx))
	require.NoError(t, err)
	require.Equal(t, originalInfo.State.LastSeq, stableInfo.State.LastSeq, "owned writers must be stopped before capturing and restoring the full history")
	require.Equal(t, originalInfo.State.Msgs, stableInfo.State.Msgs)
	require.NoError(t, adminJS.DeleteStream(f.resources.Names.State, nats.Context(f.ctx)))
	restoredInfo, err := adminJS.AddStream(&originalInfo.Config, nats.Context(f.ctx))
	require.NoError(t, err)
	require.False(t, restoredInfo.Created.Equal(originalInfo.Created), "native server must create a distinct stream generation")
	for _, msg := range messages {
		// Restore the immutable domain bytes in original global order. Publication
		// preconditions/message dedup IDs belong to the former stream transport.
		ack, err := adminJS.Publish(msg.Subject, msg.Data, nats.Context(f.ctx))
		require.NoError(t, err)
		require.Equal(t, msg.Sequence, ack.Sequence)
	}
	require.NoError(t, natscluster.WaitForQuorum(f.ctx, admin))
	restoredInfo, err = adminJS.StreamInfo(f.resources.Names.State, nats.Context(f.ctx))
	require.NoError(t, err)
	require.Equal(t, originalInfo.State.LastSeq, restoredInfo.State.LastSeq)
	require.Equal(t, originalInfo.State.Msgs, restoredInfo.State.Msgs)
	require.Equal(t, uint64(1), restoredInfo.State.FirstSeq)
	for _, original := range messages {
		restored, err := adminJS.GetMsg(f.resources.Names.State, original.Sequence, nats.Context(f.ctx))
		require.NoError(t, err)
		require.Equal(t, original.Subject, restored.Subject)
		require.Equal(t, original.Data, restored.Data)
	}
	err = reused.Run(f.ctx)
	require.ErrorContains(t, err, "FS_STATE stream identity changed")
	require.Error(t, reused.Ready(), "cached checkpoints cannot make the reused reader ready")
	for _, ref := range refs {
		_, err := reused.Read(ref)
		require.Error(t, err)
	}
	fresh, err := cluster.NewProjection(f.nc, f.resources)
	require.NoError(t, err)
	start(fresh)
	require.Eventually(t, func() bool { return fresh.Ready() == nil }, 15*time.Second, 10*time.Millisecond,
		"a fresh projection must independently prove the restored generation")
	require.NoError(t, fresh.WaitApplied(f.ctx, restoredInfo.State.LastSeq))
	for subject, ref := range refs {
		state, err := fresh.Read(ref)
		require.NoError(t, err)
		snapshot, err := state.Snapshot()
		require.NoError(t, err)
		require.Equal(t, expected[subject], snapshot.Sha256, "restored canonical state must match for %s", subject)
	}
	t.Logf("STATE_GENERATION_RESTORE old_created=%s new_created=%s retained=%d aggregates=%d reused_rejected=true fresh_proved=true", originalInfo.Created.UTC().Format(time.RFC3339Nano), restoredInfo.Created.UTC().Format(time.RFC3339Nano), restoredInfo.State.Msgs, len(refs))
}
