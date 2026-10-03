package cluster

import (
	"context"
	"testing"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestStripLifecyclePreservesCDMRecordWithSameCallsign(t *testing.T) {
	store, strips := stripFixture(t)
	ctx := context.Background()
	zero := uint64(0)
	writer := Writer{Store: store, NodeID: "node-a", Plan: PlanSystemEntity}
	reply := writer.Execute(ctx, &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(1), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test-cdm"}, ExpectedEntityRevision: &zero,
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "SAS101", Value: &pb.EntityRecord{Value: &pb.EntityRecord_CdmState{CdmState: &pb.CdmState{Callsign: "SAS101"}}}}}}}})
	require.Equal(t, pb.CommandReply_COMMITTED, reply.Status, "%+v", reply)
	_, err := strips.Put(ctx, 1, &pb.Strip{Callsign: "SAS101", Bay: "NOT_CLEARED"}, 0)
	require.NoError(t, err)
	strip, _, err := strips.ByCallsign(ctx, 1, "SAS101")
	require.NoError(t, err)
	strip.Route = "NEW ROUTE"
	_, err = strips.Put(ctx, 1, strip, strip.Revision)
	require.NoError(t, err)
	updated, _, err := strips.ByCallsign(ctx, 1, "SAS101")
	require.NoError(t, err)
	require.Equal(t, "NEW ROUTE", updated.Route)
	_, err = strips.Delete(ctx, 1, "SAS101", updated.Revision)
	require.NoError(t, err)
	_, _, err = strips.ByCallsign(ctx, 1, "SAS101")
	require.Error(t, err)
	state, err := writer.load(ctx, "fs.v1.state.session.1", sessionRef(1))
	require.NoError(t, err)
	require.NotNil(t, state.Indexes[pb.EntityKind_CDM_STATE]["SAS101"])
	require.Nil(t, state.Indexes[pb.EntityKind_STRIP]["SAS101"])
	_, err = strips.Put(ctx, 1, &pb.Strip{Callsign: "SAS101", Bay: "NOT_CLEARED"}, 0)
	require.NoError(t, err)
	state, err = writer.load(ctx, "fs.v1.state.session.1", sessionRef(1))
	require.NoError(t, err)
	snapshot, err := state.Snapshot()
	require.NoError(t, err)
	restored, err := aggregateFromSnapshot(snapshot)
	require.NoError(t, err)
	require.NotNil(t, restored.Indexes[pb.EntityKind_CDM_STATE]["SAS101"])
	require.NotNil(t, restored.Indexes[pb.EntityKind_STRIP]["SAS101"])
}
