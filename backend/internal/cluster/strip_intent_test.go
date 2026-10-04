package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestStripIntentUsesCurrentStateAndRetainsAuthorizationAndIdempotency(t *testing.T) {
	store, strips := stripFixture(t)
	ctx := context.Background()
	_, err := strips.Put(ctx, 1, &pb.Strip{Callsign: "SAS101", Bay: "CLEARED", Departure: "EKCH", Destination: "ESSA", Route: "OLD"}, 0)
	require.NoError(t, err)
	writer := Writer{Store: store, NodeID: "node-a", Plan: PlanStrip}
	controller := ControllerSector{Store: LocalLifecycleStore{Writer: writer}}
	_, err = controller.PutController(ctx, 1, &pb.Controller{Cid: "cid-1", Position: "118.100", Callsign: "EKCH_A_TWR"}, 0)
	require.NoError(t, err)
	current, _, err := strips.ByCallsign(ctx, 1, "SAS101")
	require.NoError(t, err)
	_, err = strips.Observe(ctx, 1, &pb.Strip{Callsign: "SAS101", Route: "LATEST", Departure: "EKCH", Destination: "ESSA"}, current.Revision)
	require.NoError(t, err)
	id := int32(1)
	req := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(1), Actor: &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "cid-1", SessionId: &id},
		Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Strip{Strip: &pb.StripAction{Callsign: "SAS101", Change: &pb.StripAction_Move{Move: &pb.MoveStrip{Bay: "TAXI"}}}}}}}
	reply := writer.Execute(ctx, req)
	require.Equal(t, pb.CommandOutcome_SUCCEEDED, reply.GetOutcome().GetStatus(), reply.String())
	after, _, err := strips.ByCallsign(ctx, 1, "SAS101")
	require.NoError(t, err)
	require.Equal(t, "TAXI", after.Bay)
	require.Equal(t, "LATEST", after.Route)
	require.Equal(t, reply.GetOutcome().GetAggregateRevision(), writer.Execute(ctx, req).GetOutcome().GetAggregateRevision())
	req.CommandId = uuid.NewString()
	req.Actor.Id = "unknown-controller"
	require.Equal(t, pb.CommandReply_UNAUTHORIZED, writer.Execute(ctx, req).Status)
	req.Actor.Id = "cid-1"
	req.CommandId = uuid.NewString()
	req.GetClient().GetStrip().Change = &pb.StripAction_MissedApproach{MissedApproach: &pb.MissedApproach{}}
	require.Equal(t, "REVISION_CONFLICT", writer.Execute(ctx, req).GetOutcome().GetReasonCode())
}
