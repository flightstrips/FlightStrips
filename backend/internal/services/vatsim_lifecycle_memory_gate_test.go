package services

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type lifecyclePendingOwner struct{ healthy, pending bool }

func (o lifecyclePendingOwner) CanWrite(*pb.AggregateRef) bool       { return o.healthy && !o.pending }
func (o lifecyclePendingOwner) CanCommitLocal(*pb.AggregateRef) bool { return o.healthy }

type lifecycleDurableOnlyOwner struct{}

func (lifecycleDurableOnlyOwner) CanWrite(*pb.AggregateRef) bool { return true }

func TestLifecycleMemoryPlanningOwnerEligibility(t *testing.T) {
	ref := sessionRef(42)
	require.True(t, lifecycleOwnerCanPlan(lifecyclePendingOwner{healthy: true, pending: true}, true, ref), "healthy owner may plan while replication is pending")
	require.False(t, lifecycleOwnerCanPlan(lifecyclePendingOwner{healthy: false, pending: true}, true, ref), "stale owner cannot plan")
	require.False(t, lifecycleOwnerCanPlan(lifecyclePendingOwner{healthy: true, pending: true}, false, ref), "legacy planner retains durable guard")
	require.False(t, lifecycleOwnerCanPlan(lifecycleDurableOnlyOwner{}, true, ref), "RAM admission requires explicit local ownership support")
	require.True(t, lifecycleOwnerCanPlan(lifecycleDurableOnlyOwner{}, false, ref))
}

func TestLifecyclePendingOwnerRemovesDepartureBlockFromAcceptedPosition(t *testing.T) {
	stands, policy := lifecyclePolicyFixture(t)
	now := time.Now().UTC()
	ref := sessionRef(42)
	state := cluster.NewAggregate(ref)
	strip := &pb.Strip{Callsign: "SAS123", Departure: "EKCH", Destination: "EDDF", AircraftType: "A320", Bay: "AIRBORNE", Stand: "A1", EuroscopeObservedAt: timestamppb.New(now)}
	assignment := &pb.StandAssignment{Callsign: "SAS123", Stand: "A1", Direction: "DEPARTURE", Stage: StageDepartureBlock, ObservedStand: proto.String("A1"), Revision: 1, CreatedAt: timestamppb.New(now), UpdatedAt: timestamppb.New(now)}
	state.Indexes[pb.EntityKind_SESSION] = map[string]*pb.EntitySnapshot{"42": {Key: "42", Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: 42, Airport: "EKCH", Name: "LIVE"}}}}}
	state.Indexes[pb.EntityKind_STRIP] = map[string]*pb.EntitySnapshot{"SAS123": {Key: "SAS123", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: strip}}}}
	state.Indexes[pb.EntityKind_STAND_ASSIGNMENT] = map[string]*pb.EntitySnapshot{"SAS123": {Key: "SAS123", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_StandAssignment{StandAssignment: assignment}}}}
	candidate := &VatsimLifecycleCandidate{Writer: cluster.Writer{Projection: &cluster.Projection{}}, Stands: cluster.StandState{Stands: stands, Policy: policy, Now: func() time.Time { return now }}}
	page := &pb.VatsimPage{SnapshotAt: timestamppb.New(now), Flights: []*pb.VatsimFlight{lifecycleFlight("SAS123", "EKCH", "EDDF", "online")}}
	// The accepted live position is authoritative over a provider's stale location.
	stand, _ := stands.Lookup("EKCH", "A1")
	page.Flights[0].Latitude, page.Flights[0].Longitude = stand.Latitude, stand.Longitude
	position := cluster.KVPosition{Revision: 1<<63 | 10, Value: &pb.PositionValue{SessionId: 42, AircraftKey: "SAS123", OwnerEpoch: 1, SourceConnectionId: "master", ObservedAt: timestamppb.New(now), Value: &pb.PositionValue_Position{Position: &pb.AircraftPosition{Latitude: stand.Latitude, Longitude: stand.Longitude, AltitudeFeet: 20}}}}
	require.True(t, lifecycleOwnerCanPlan(lifecyclePendingOwner{healthy: true, pending: true}, true, ref))
	before := proto.Clone(assignment)
	request := &pb.CommandRequest{CommandId: uuid.NewString(), Aggregate: ref}
	removesAssignment := func(change *pb.DomainChange) bool {
		for _, entity := range change.Changes {
			if entity.Key == "SAS123" && entity.GetDelete().GetKind() == pb.EntityKind_STAND_ASSIGNMENT {
				return true
			}
		}
		return false
	}
	control, err := candidate.plan(context.Background(), request, state, "SAS123", true, 1, "source", page, []cluster.KVPosition{position}, false)
	require.NoError(t, err)
	require.False(t, removesAssignment(control), "the same strip and assignment must remain blocked while the accepted position is still at A1")
	require.True(t, proto.Equal(before, assignment), "control planning must leave accepted input immutable")
	// Keep owner, source, strip, assignment, timestamps and altitude identical.
	// Only the accepted position coordinates now move away from every stand.
	position.Value = proto.Clone(position.Value).(*pb.PositionValue)
	position.Value.GetPosition().Latitude, position.Value.GetPosition().Longitude = 56, 13
	change, err := candidate.plan(context.Background(), request, state, "SAS123", true, 1, "source", page, []cluster.KVPosition{position}, false)
	require.NoError(t, err)
	require.True(t, removesAssignment(change), "departure away from configured stands must release the accepted block despite pending replication")
	require.True(t, proto.Equal(before, assignment), "planner must leave accepted input immutable")
}
