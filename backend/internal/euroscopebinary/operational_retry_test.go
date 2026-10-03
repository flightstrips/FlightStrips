package euroscopebinary

import (
	"FlightStrips/internal/cluster"
	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"context"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"testing"
	"time"
)

func TestStripObservationReplansAgainstConcurrentFields(t *testing.T) {
	req := &pb.CommandRequest{ProtocolRevision: 1, Aggregate: candidateRef(1), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "euroscope-strip"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "SAS1", Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Callsign: "SAS1"}}}}}}}}
	original := proto.Clone(req)
	state := cluster.NewAggregate(candidateRef(1))
	landedAt := timestamppb.New(time.Now())
	strip := &pb.Strip{Callsign: "SAS1", Bay: "FINAL", Stand: "A1", Aldt: landedAt}
	old := &pb.EntitySnapshot{Key: "SAS1", Revision: 4, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: strip}}}
	state.Indexes[pb.EntityKind_STRIP] = map[string]*pb.EntitySnapshot{"SAS1": old}
	heading := int32(123)
	apply := func(s *pb.Strip) { s.Heading = &heading }
	next := func(_ context.Context, planned *pb.CommandRequest, _ *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		require.Equal(t, old.Revision, planned.GetExpectedEntityRevision())
		observed := planned.GetSystem().GetUpdateEntity().GetValue().GetStrip()
		require.Equal(t, strip.Bay, observed.Bay)
		require.Equal(t, strip.Stand, observed.Stand)
		require.True(t, proto.Equal(landedAt, observed.Aldt))
		require.Equal(t, heading, observed.GetHeading())
		require.Nil(t, strip.Heading)
		return &pb.DomainChange{}, pb.CommandReply_COMMITTED, old.Revision, nil
	}
	_, _, _, err := planStripObservation(context.Background(), req, state, "SAS1", apply, next)
	require.NoError(t, err)
	old.Revision = 5
	strip.Stand = "B2"
	strip.Bay = "TWY_ARR"
	_, _, _, err = planStripObservation(context.Background(), req, state, "SAS1", apply, next)
	require.NoError(t, err)
	require.True(t, proto.Equal(original, req))
}

func TestConditionalObservationHashRetainsOriginalBoolean(t *testing.T) {
	apply := func(s *pb.Strip) {
		if s.Bay == shared.BAY_NOT_CLEARED {
			s.Bay = shared.BAY_CLEARED
		}
	}
	frame := &es.Envelope{Event: &es.Envelope_ClearedFlag{ClearedFlag: &es.ClearedFlagEvent{Callsign: "SAS1", Cleared: true}}}
	yes := stripObservationPatch(frame, "SAS1", apply)
	frame.GetClearedFlag().Cleared = false
	no := stripObservationPatch(frame, "SAS1", apply)
	request := func(patch *pb.Strip) *pb.CommandRequest {
		return &pb.CommandRequest{ProtocolRevision: 1, CommandId: "7f8b5915-a62d-4939-8fe0-9cd2e494587a", Aggregate: candidateRef(1), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "euroscope-strip"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "SAS1", Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: patch}}}}}}}
	}
	yesHash, err := cluster.RequestHash(request(yes))
	require.NoError(t, err)
	noHash, err := cluster.RequestHash(request(no))
	require.NoError(t, err)
	require.NotEqual(t, yesHash, noHash, "reusing a command ID with changed clearance must not reuse its outcome")
}

func TestRunwayObservationReplansPreservingConcurrentSIDs(t *testing.T) {
	patch := &pb.Session{Id: 1, Runways: []*pb.Runway{{Name: "22L", Arrival: true}}}
	req := &pb.CommandRequest{ProtocolRevision: 1, CommandId: "7f8b5915-a62d-4939-8fe0-9cd2e494587a", Aggregate: candidateRef(1), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "euroscope-session"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "1", Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: patch}}}}}}}
	original := proto.Clone(req)
	hash, err := cluster.RequestHash(req)
	require.NoError(t, err)
	state := cluster.NewAggregate(candidateRef(1))
	session := &pb.Session{Id: 1, Airport: "EKCH", Name: "LIVE", AvailableSids: []*pb.SidInfo{{Name: "BETUD1A", Runway: "22R"}}}
	old := &pb.EntitySnapshot{Key: "1", Revision: 7, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: session}}}
	state.Indexes[pb.EntityKind_SESSION] = map[string]*pb.EntitySnapshot{"1": old}
	next := func(_ context.Context, planned *pb.CommandRequest, _ *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		observed := planned.GetSystem().GetUpdateEntity().GetValue().GetSession()
		require.Equal(t, old.Revision, planned.GetExpectedEntityRevision())
		require.Equal(t, session.Name, observed.Name)
		require.Equal(t, "EKCH", observed.Airport)
		require.True(t, proto.Equal(session.AvailableSids[0], observed.AvailableSids[0]))
		require.True(t, proto.Equal(patch.Runways[0], observed.Runways[0]))
		require.Empty(t, session.Runways, "planning must not mutate published session")
		return &pb.DomainChange{}, pb.CommandReply_COMMITTED, old.Revision, nil
	}
	_, _, _, err = planRunwayObservation(context.Background(), req, state, "1", next)
	require.NoError(t, err)
	old.Revision = 8
	session.Name = "UPDATED"
	session.AvailableSids[0] = &pb.SidInfo{Name: "BETUD2B", Runway: "04L"}
	_, _, _, err = planRunwayObservation(context.Background(), req, state, "1", next)
	require.NoError(t, err)
	require.True(t, proto.Equal(original, req))
	after, err := cluster.RequestHash(req)
	require.NoError(t, err)
	require.Equal(t, hash, after)
}

func TestDistinctClearOperationsNeverShareAnObservationHash(t *testing.T) {
	frames := []*es.Envelope{
		{CommandId: "7f8b5915-a62d-4939-8fe0-9cd2e494587a", Event: &es.Envelope_Stand{Stand: &es.StandEvent{Callsign: "SAS1", Stand: ""}}},
		{CommandId: "7f8b5915-a62d-4939-8fe0-9cd2e494587a", Event: &es.Envelope_Route{Route: &es.RouteEvent{Callsign: "SAS1", Route: ""}}},
		{CommandId: "7f8b5915-a62d-4939-8fe0-9cd2e494587a", Event: &es.Envelope_Remarks{Remarks: &es.RemarksEvent{Callsign: "SAS1", Remarks: ""}}},
	}
	var previous *pb.CommandRequest
	hashes := map[string]bool{}
	for _, f := range frames {
		request := stripObservationRequest(f, 1, "connection", "SAS1", func(*pb.Strip) {})
		hash, err := cluster.RequestHash(request)
		require.NoError(t, err)
		require.False(t, hashes[hash], "different clear operations must reject changed-content command reuse")
		hashes[hash] = true
		if previous != nil {
			require.Equal(t, previous.CommandId, request.CommandId)
			require.True(t, proto.Equal(previous.GetSystem(), request.GetSystem()), "empty sparse patches alone cannot distinguish operations")
		}
		previous = request
	}
}
func TestEventSpecificActorRetainsEstablishedPlanningPolicy(t *testing.T) {
	f := &es.Envelope{CommandId: "7f8b5915-a62d-4939-8fe0-9cd2e494587a", Event: &es.Envelope_Stand{Stand: &es.StandEvent{Callsign: "SAS1", Stand: ""}}}
	apply := func(s *pb.Strip) { s.Stand = "" }
	req := stripObservationRequest(f, 1, "connection", "SAS1", apply)
	original := proto.Clone(req)
	state := cluster.NewAggregate(candidateRef(1))
	state.Indexes[pb.EntityKind_STRIP] = map[string]*pb.EntitySnapshot{"SAS1": {Key: "SAS1", Revision: 7, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Callsign: "SAS1", Stand: "A1", Bay: "FINAL"}}}}}
	next := func(_ context.Context, planned *pb.CommandRequest, _ *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		require.Equal(t, "euroscope-strip", planned.Actor.Id, "preserve merge and disconnect-cancellation policy")
		require.Empty(t, planned.GetSystem().GetUpdateEntity().GetValue().GetStrip().Stand)
		require.Equal(t, "FINAL", planned.GetSystem().GetUpdateEntity().GetValue().GetStrip().Bay)
		return &pb.DomainChange{}, pb.CommandReply_COMMITTED, 7, nil
	}
	_, _, _, err := planStripObservation(context.Background(), req, state, "SAS1", apply, next)
	require.NoError(t, err)
	require.True(t, proto.Equal(original, req))
	require.Equal(t, "euroscope-strip/stand", req.Actor.Id)
}
