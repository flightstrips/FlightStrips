package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// Exercise more than one ordered-consumer flow-control window using typed
// control events. Short assembly tests can pass even when the backend account
// cannot publish JetStream's flow-control replies and replay later stalls.
func TestBuildNATSProjectionFlowControl(t *testing.T) {
	f := newRuntimeFixture(t, nil)
	js, err := f.admin.JetStream()
	require.NoError(t, err)
	id := uuid.New()
	icao := string([]byte{'A' + id[0]%26, 'A' + id[1]%26, 'A' + id[2]%26, 'A' + id[3]%26})
	ref := airportNATSRef(icao)
	subject, err := cluster.Subject(ref)
	require.NoError(t, err)
	node := uuid.NewString()
	store := cluster.NATSStore{JS: js}
	var sequence uint64
	for i := 0; i < 128; i++ {
		event := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref,
			Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: strings.Repeat("flow-fixture", 32768)},
			Fact:  &pb.StateEvent_OwnerRenewed{OwnerRenewed: &pb.OwnerTerm{NodeId: node, Epoch: 1}}}
		if i == 0 {
			event.Fact = &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: node, Epoch: 1}}
		}
		data, e := proto.Marshal(event)
		require.NoError(t, e)
		sequence, err = store.Publish(f.ctx, subject, sequence, data)
		require.NoError(t, err)
	}
	for _, app := range f.apps {
		wait, cancel := context.WithTimeout(f.ctx, 15*time.Second)
		err := app.natsRuntime.projection.WaitApplied(wait, sequence)
		cancel()
		require.NoError(t, err, "backend credential must sustain ordered replay through flow-control replies")
		state, err := app.natsRuntime.projection.Read(ref)
		require.NoError(t, err)
		require.Equal(t, node, state.Owner.NodeId)
	}
	f.await("both applications caught up after sustained replay", func() bool {
		return f.status(0, "/readyz") == 200 && f.status(1, "/readyz") == 200
	})
}
