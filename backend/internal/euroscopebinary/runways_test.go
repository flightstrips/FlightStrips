package euroscopebinary

import (
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSocketRunwayReportsCompareAndReplaceWithoutChangingCanonicalRunways(t *testing.T) {
	state := cluster.NewAggregate(candidateRef(1))
	state.Owner = &pb.OwnerTerm{Epoch: 2}
	state.Master = &pb.MasterTerm{ConnectionId: "master", OwnerEpoch: 2, Epoch: 3}
	state.Sync = &pb.SessionSync{ConnectionId: "master", MasterEpoch: 3}
	session := &pb.Session{Id: 1, Runways: []*pb.Runway{{Name: "22L", Arrival: true}, {Name: "22R", Departure: true}}}
	state.Indexes[pb.EntityKind_SESSION] = map[string]*pb.EntitySnapshot{"1": {Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: session}}}}
	reports := &socketRunwayReports{}
	wrong := &es.RunwayEvent{Runways: []*es.Runway{{Name: "04L", Departure: true}, {Name: "04R", Arrival: true}}}
	reports.replace(wrong)
	wrong.Runways[0].Name = "mutated"
	alert := reports.evaluate(state, "slave").GetRunwayMismatchAlert()
	require.Equal(t, []string{"22R"}, alert.ExpectedDeparture)
	require.Equal(t, []string{"04L"}, alert.CurrentDeparture)
	require.Equal(t, []string{"22L"}, alert.ExpectedArrival)
	require.Equal(t, []string{"04R"}, alert.CurrentArrival)
	require.Nil(t, reports.evaluate(state, "slave"), "identical mismatches are not repeated")
	require.Equal(t, "22L", session.Runways[0].Name, "reports cannot change canonical configuration")
	reports.replace(&es.RunwayEvent{Runways: []*es.Runway{{Name: " 22r ", Departure: true}, {Name: "22L", Arrival: true}}})
	require.Nil(t, reports.evaluate(state, "slave"), "activation sets ignore order and case")
	reports.replace(&es.RunwayEvent{})
	require.NotNil(t, reports.evaluate(state, "observer"), "observers report mismatches too")
	require.Nil(t, reports.evaluate(state, "master"), "master never receives its own mismatch")
	state.Sync = nil
	require.Nil(t, reports.evaluate(state, "slave"), "await authoritative initial sync")
	state.Sync = &pb.SessionSync{ConnectionId: "master", MasterEpoch: 3}
	require.NotNil(t, reports.evaluate(state, "slave"))
	session.Runways = nil
	require.Nil(t, reports.evaluate(state, "slave"), "master changes also resolve mismatches")
	session.Runways = []*pb.Runway{{Name: "22L", Arrival: true}}
	require.NotNil(t, reports.evaluate(state, "slave"), "master changes re-evaluate the latest report")
	require.True(t, runwayReportIsMaster(state, "master", 3))
	require.False(t, runwayReportIsMaster(state, "slave", 3))
	require.False(t, runwayReportIsMaster(state, "master", 2))
}
