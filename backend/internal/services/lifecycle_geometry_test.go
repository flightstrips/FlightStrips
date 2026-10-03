package services

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/sat"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestLifecycleGeometryReusesExactPositionsAndInvalidatesChangedInputs(t *testing.T) {
	registry, _ := lifecyclePolicyFixture(t)
	otherRegistry, _ := lifecyclePolicyFixture(t)
	stand, found := registry.Lookup("EKCH", "A1")
	require.True(t, found)
	calls := 0
	geometry := &lifecycleGeometry{lookup: func(r *sat.StandCapabilityRegistry, airport string, lat, lon float64) (string, bool) {
		calls++
		return physicalStandAt(r, airport, lat, lon)
	}}
	positions := []cluster.KVPosition{{Value: &pb.PositionValue{AircraftKey: "SAS123"}}, {Value: &pb.PositionValue{AircraftKey: "SAS456"}}}
	geometry.prepare(registry, "EKCH", positions)
	for i := 0; i < 200; i++ {
		name, ok := geometry.standAt("SAS123", stand.Latitude, stand.Longitude)
		require.True(t, ok)
		require.Equal(t, "A1", name)
	}
	require.Equal(t, 1, calls, "other callsign plans must reuse identical fleet geometry")
	_, found = geometry.standAt("SAS456", 56, 13)
	require.False(t, found)
	_, found = geometry.standAt("SAS456", 56, 13)
	require.False(t, found)
	require.Equal(t, 2, calls, "negative geometry results are equally reusable")
	_, found = geometry.standAt("SAS123", 56, 13)
	require.False(t, found)
	require.Equal(t, 3, calls, "a moved observation must recompute immediately")
	geometry.prepare(otherRegistry, "EKCH", positions)
	geometry.standAt("SAS123", stand.Latitude, stand.Longitude)
	require.Equal(t, 4, calls, "a new validated registry invalidates old geometry")
	geometry.prepare(otherRegistry, "EDDF", positions)
	_, found = geometry.standAt("SAS123", stand.Latitude, stand.Longitude)
	require.False(t, found)
	require.Equal(t, 5, calls, "airport is part of the geometry identity")
	geometry.prepare(otherRegistry, "EDDF", positions[1:])
	require.Empty(t, geometry.entries, "removed fleet keys cannot accumulate in the invocation cache")
}

func TestLifecycleGeometryCachePreservesFreshPlanningEligibility(t *testing.T) {
	stands, policy := lifecyclePolicyFixture(t)
	now := time.Now().UTC()
	stand, _ := stands.Lookup("EKCH", "A1")
	ref := sessionRef(42)
	state := cluster.NewAggregate(ref)
	state.Indexes[pb.EntityKind_SESSION] = map[string]*pb.EntitySnapshot{"42": {Key: "42", Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: 42, Airport: "EKCH", Name: "LIVE"}}}}}
	strip := &pb.Strip{Callsign: "SAS123", Departure: "EKCH", Destination: "EDDF", AircraftType: "A320", Bay: "DEP_HIDDEN", VatsimOnly: true, VatsimCid: "12345"}
	state.Indexes[pb.EntityKind_STRIP] = map[string]*pb.EntitySnapshot{"SAS123": {Key: "SAS123", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: strip}}}}
	candidate := &VatsimLifecycleCandidate{Writer: cluster.Writer{Projection: &cluster.Projection{}}, Stands: cluster.StandState{Stands: stands, Policy: policy, Now: func() time.Time { return now }}, AllowPrefiles: true}
	request := &pb.CommandRequest{CommandId: "geometry-regression", Aggregate: ref}
	page := &pb.VatsimPage{SnapshotAt: timestamppb.New(now), Flights: []*pb.VatsimFlight{lifecycleFlight("SAS123", "EKCH", "EDDF", "prefile")}}
	position := cluster.KVPosition{Revision: 1, Value: &pb.PositionValue{AircraftKey: "SAS123", SessionId: 42, OwnerEpoch: 1, SourceConnectionId: "master", ObservedAt: timestamppb.New(now), Observation: &pb.PositionValue_Position{Position: &pb.AircraftPosition{Latitude: stand.Latitude, Longitude: stand.Longitude, AltitudeFeet: 20}}}}
	geometry := &lifecycleGeometry{}
	ctx := context.WithValue(context.Background(), lifecycleGeometryContextKey{}, geometry)
	for step := 0; step < 6; step++ {
		if step == 1 {
			position.Value.GetPosition().Latitude = 56
			position.Revision++
		} else if step == 2 {
			position.Stale = true
		} else if step == 3 {
			position.Stale = false
			position.Value.GetPosition().AltitudeFeet = 2000
		} else if step == 4 {
			position.Value.GetPosition().Latitude = stand.Latitude
			position.Value.GetPosition().AltitudeFeet = 20
			state.Indexes[pb.EntityKind_STAND_ASSIGNMENT] = map[string]*pb.EntitySnapshot{"SAS123": {Key: "SAS123", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_StandAssignment{StandAssignment: &pb.StandAssignment{Callsign: "SAS123", Stand: "A1", Direction: "DEPARTURE", Stage: StageDepartureBlock, Revision: 1, CreatedAt: timestamppb.New(now), UpdatedAt: timestamppb.New(now)}}}}}
		} else if step == 5 {
			delete(state.Indexes[pb.EntityKind_STAND_ASSIGNMENT], "SAS123")
		}
		before := proto.Clone(strip)
		cached, err := candidate.plan(ctx, request, state, "SAS123", true, 1, "source", page, []cluster.KVPosition{position}, false)
		require.NoError(t, err)
		fresh, err := candidate.plan(context.Background(), request, state, "SAS123", true, 1, "source", page, []cluster.KVPosition{position}, false)
		require.NoError(t, err)
		require.True(t, proto.Equal(cached, fresh), "geometry reuse must preserve the fresh plan at step %d", step)
		require.True(t, proto.Equal(before, strip), "cached planning must not mutate accepted records")
	}
}
