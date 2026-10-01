package main

import (
	"os"
	"testing"
	"time"

	"FlightStrips/internal/frontendbinary"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestServerNATSArchivedCommandAfterRestart(t *testing.T) {
	if os.Getenv("NATS_TASK22") != "1" {
		t.Skip("requires disposable Task22 fixture")
	}
	f := newEntrypointFixture(t, true)
	name, ref, _ := f.seededSession()
	connect := func(node int) *websocket.Conn {
		c := f.dial(node, "/frontEndEvents", frontendbinary.Subprotocol)
		sendEntrypointFrame(t, c, &pb.FrontendFrame{ProtocolRevision: 2, Frame: &pb.FrontendFrame_Authenticate{Authenticate: &pb.FrontendAuthenticate{BearerToken: f.token, Airport: "EKCH", SessionName: name}}})
		_ = c.SetReadDeadline(time.Now().Add(20 * time.Second))
		kind, data, err := c.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, websocket.BinaryMessage, kind)
		frame := &pb.FrontendFrame{}
		require.NoError(t, pb.UnmarshalStrict(data, frame))
		require.NotNil(t, frame.GetInitial())
		return c
	}
	send := func(c *websocket.Conn, request *pb.FrontendFrame) *pb.FrontendActionResult {
		sendEntrypointFrame(t, c, request)
		_ = c.SetReadDeadline(time.Now().Add(20 * time.Second))
		for {
			kind, data, err := c.ReadMessage()
			require.NoError(t, err)
			require.Equal(t, websocket.BinaryMessage, kind)
			frame := &pb.FrontendFrame{}
			require.NoError(t, pb.UnmarshalStrict(data, frame))
			if result := frame.GetActionResult(); result != nil && result.RequestId == request.GetCommand().RequestId {
				return result
			}
		}
	}
	c := connect(0)
	oldID := uuid.NewString()
	oldRequest := markedCommand(oldID, "SAS123", f.state(ref).Indexes[pb.EntityKind_STRIP]["SAS123"].Revision, true)
	original := send(c, oldRequest)
	require.Equal(t, pb.CommandOutcome_SUCCEEDED, original.Status)
	for i, attempts := 0, 0; i < 640; attempts++ {
		require.Less(t, attempts, 2000, "background worker revision races must not prevent history qualification")
		state := f.state(ref)
		request := markedCommand(uuid.NewString(), "SAS123", state.Indexes[pb.EntityKind_STRIP]["SAS123"].Revision, i%2 == 0)
		result := send(c, request)
		if result.Status == pb.CommandOutcome_FAILED && result.ReasonCode == "REVISION_CONFLICT" {
			continue
		}
		require.Equal(t, pb.CommandOutcome_SUCCEEDED, result.Status, "reason=%s detail=%s", result.ReasonCode, result.Detail)
		f.await("command applied by independent projection", func() bool {
			record, err := f.state(ref).LookupOutcome(request.GetCommand().RequestId)
			return err == nil && record != nil
		})
		i++
	}
	state := f.state(ref)
	require.Nil(t, state.Ledger[oldID], "original result must have left the resident working set")
	retained, err := state.LookupOutcome(oldID)
	require.NoError(t, err)
	require.Equal(t, original.AggregateRevision, retained.AggregateRevision)
	require.NoError(t, f.projection.Snapshots.Save(state))
	require.NoError(t, c.Close())
	f.apps[1].stop()
	f.restart(1)
	restarted := connect(1)
	before := f.state(ref).Indexes[pb.EntityKind_STRIP]["SAS123"].Revision
	retry := send(restarted, oldRequest)
	require.Equal(t, pb.CommandOutcome_SUCCEEDED, retry.Status)
	require.Equal(t, original.AggregateRevision, retry.AggregateRevision)
	changed := proto.Clone(oldRequest).(*pb.FrontendFrame)
	changed.GetCommand().GetAction().GetStrip().GetSetMarked().Marked = false
	rejected := send(restarted, changed)
	require.Equal(t, pb.CommandOutcome_FAILED, rejected.Status)
	require.Equal(t, before, f.state(ref).Indexes[pb.EntityKind_STRIP]["SAS123"].Revision, "old/changed retries must not mutate the strip")
	for node := range f.apps {
		require.Equal(t, "succeeded", f.outcome(node, oldID))
	}
	t.Logf("HISTORY_RETRY retained_commands=641 hot_outcomes=%d original_sequence=%d original_revision=%d retry_revision=%d changed_payload_status=%s", len(state.Ledger), retained.CommittedStreamSequence, original.AggregateRevision, retry.AggregateRevision, rejected.Status)
}
