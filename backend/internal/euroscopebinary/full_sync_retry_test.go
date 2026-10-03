package euroscopebinary

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestFullSyncObservationPreservesConcurrentCDMUpdates(t *testing.T) {
	state := cluster.NewAggregate(candidateRef(1))
	latest := &pb.Strip{Callsign: "SAS1", Eobt: timestamppb.New(time.Unix(20, 0)), Eldt: timestamppb.New(time.Unix(30, 0))}
	old := &pb.EntitySnapshot{Key: "SAS1", Revision: 7, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: latest}}}
	state.Indexes[pb.EntityKind_STRIP] = map[string]*pb.EntitySnapshot{"SAS1": old}
	expected := uint64(1)
	req := &pb.CommandRequest{ProtocolRevision: 1, CommandId: "7f8b5915-a62d-4939-8fe0-9cd2e494587a", Aggregate: state.Ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "euroscope-strip"}, ExpectedEntityRevision: &expected,
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "SAS1", Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Callsign: "SAS1", Route: "OBSERVED", Eobt: timestamppb.New(time.Unix(10, 0))}}}}}}}}
	original := proto.Clone(req)
	next := func(_ context.Context, planned *pb.CommandRequest, _ *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		require.Equal(t, old.Revision, planned.GetExpectedEntityRevision())
		got := planned.GetSystem().GetUpdateEntity().GetValue().GetStrip()
		require.Equal(t, "OBSERVED", got.Route)
		require.True(t, proto.Equal(latest.Eobt, got.Eobt))
		require.True(t, proto.Equal(latest.Eldt, got.Eldt))
		return &pb.DomainChange{}, pb.CommandReply_COMMITTED, old.Revision, nil
	}
	for i := 0; i < 2; i++ {
		_, _, _, err := planFrameObservation(context.Background(), req, state, next)
		require.NoError(t, err)
		old.Revision++
		latest.Eobt = timestamppb.New(time.Unix(40, 0))
	}
	require.True(t, proto.Equal(original, req), "retries must preserve the request hash input")
}

func TestFullSyncSessionPreservesConcurrentAllocationAndCleanup(t *testing.T) {
	state := cluster.NewAggregate(candidateRef(1))
	latest := &pb.Session{Id: 1, Name: "NEW", NextStripId: 50, FirstNoControllerAt: timestamppb.New(time.Unix(20, 0))}
	old := &pb.EntitySnapshot{Key: "1", Revision: 7, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: latest}}}
	state.Indexes[pb.EntityKind_SESSION] = map[string]*pb.EntitySnapshot{"1": old}
	expected := uint64(1)
	req := &pb.CommandRequest{ProtocolRevision: 1, Aggregate: state.Ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "euroscope-session"}, ExpectedEntityRevision: &expected,
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "1", Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: 1, Name: "OLD", NextStripId: 2, Runways: []*pb.Runway{{Name: "22R", Departure: true}}}}}}}}}}
	original := proto.Clone(req)
	next := func(_ context.Context, planned *pb.CommandRequest, _ *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		require.Equal(t, old.Revision, planned.GetExpectedEntityRevision())
		got := planned.GetSystem().GetUpdateEntity().GetValue().GetSession()
		require.Equal(t, latest.Name, got.Name)
		require.Equal(t, latest.NextStripId, got.NextStripId)
		require.True(t, proto.Equal(latest.FirstNoControllerAt, got.FirstNoControllerAt))
		require.Equal(t, "22R", got.Runways[0].Name)
		return &pb.DomainChange{}, pb.CommandReply_COMMITTED, old.Revision, nil
	}
	for i := 0; i < 2; i++ {
		_, _, _, err := planFrameObservation(context.Background(), req, state, next)
		require.NoError(t, err)
		old.Revision++
		latest.NextStripId++
	}
	require.True(t, proto.Equal(original, req))
}
