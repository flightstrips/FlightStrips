package amancandidate

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
)

func TestOperationalSharedEuroScopeNATS(t *testing.T) {
	f := newFixture(t)
	f.seed(t)
	w := f.nodes[f.owner(t, airportRef(f.airport), "")].worker
	committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.ObserveVatsim(f.ctx, f.airport, "SAS123"), nil }))
	n := f.nodes[f.owner(t, sessionRef(f.session), "")]
	state, err := (cluster.LocalLifecycleStore{Writer: n.writer}).Read(f.ctx, sessionRef(f.session))
	require.NoError(t, err)
	master := state.Master
	for key, value := range map[string]*pb.PresenceValue{
		"node." + n.owner.NodeID:        {SchemaVersion: 1, Present: &pb.PresenceValue_Node{Node: &pb.NodePresence{NodeId: n.owner.NodeID, StartedAt: timestamp(time.Now()), Ready: true}}},
		"client." + master.ConnectionId: {SchemaVersion: 1, Present: &pb.PresenceValue_Client{Client: &pb.ClientPresence{ConnectionId: master.ConnectionId, NodeId: n.owner.NodeID, SessionId: f.session, Cid: master.Cid, Kind: pb.ClientPresence_EUROSCOPE, ConnectedAt: timestamp(time.Now())}}},
	} {
		data, err := proto.Marshal(value)
		require.NoError(t, err)
		_, err = n.p.Presence.Put(key, data)
		require.NoError(t, err)
	}
	require.Eventually(t, func() bool {
		return n.p.RequireLiveSocket(f.session, master.ConnectionId, master.Cid, pb.ClientPresence_EUROSCOPE) == nil
	}, time.Second, 10*time.Millisecond)
	adapter := cluster.SessionObservations{Store: cluster.RoutedLifecycleStore{Router: n.router, Projection: n.p}}
	_, err = adapter.RecordSync(f.ctx, f.session, uuid.NewString(), &pb.SessionSync{ConnectionId: master.ConnectionId, MasterEpoch: master.Epoch, CompletedAt: timestamp(f.now)})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		sync, e := w.options.Projection.OperationalSync(sessionRef(f.session))
		return e == nil && sync != nil
	}, time.Second, 10*time.Millisecond)
	s := proto.Clone(state.Indexes[pb.EntityKind_STRIP]["SAS123"].GetValue().GetStrip()).(*pb.Strip)
	s.VatsimOnly = false
	s.AircraftType = "C172"
	s.EuroscopeObservedAt = timestamp(f.now.Add(time.Second))
	f.entity(t, sessionRef(f.session), s.Callsign, &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: s}})
	positions, err := cluster.NewPositionWriter(n.p.Positions, f.session, state.Owner.Epoch, master.ConnectionId, n.p.PositionAuthority(n.owner.NodeID), 1, 8)
	require.NoError(t, err)
	defer positions.Close(f.ctx)
	report, err := positions.QueuePosition(f.ctx, s.Callsign, &pb.AircraftPosition{Latitude: 56.005, Longitude: 12, AltitudeFeet: 9000, GroundSpeedKnots: 170, TrackDegrees: 180}, f.now.Add(time.Second))
	require.NoError(t, err)
	require.NoError(t, (<-report).Err)
	require.Eventually(t, func() bool {
		p, _, e := w.options.Projection.ObservationSnapshot(f.session)
		return e == nil && len(p) == 1
	}, time.Second, 10*time.Millisecond)
	f.now = f.now.Add(2 * time.Second)
	f.vatsim(t, true)
	committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.Reconcile(f.ctx, f.airport, f.now), nil }))
	b, err := w.options.State.Read(f.ctx, f.airport)
	require.NoError(t, err)
	require.Len(t, b.Flights[0].SourceObservations, 2)
	require.Equal(t, string(aman.SurveillanceSourceEuroScope), b.Flights[0].LatestObservation.SurveillanceSource)
	require.Equal(t, int32(9000), b.Flights[0].LatestObservation.Surveillance.GetAltitudeFeet())
	require.Equal(t, "TESPI", b.Flights[0].HoldingClearance.Hold)
	// A fresh VATSIM absence retracts only VATSIM; the accepted ES report survives.
	f.now = f.now.Add(time.Second)
	f.vatsim(t, false)
	committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.ObserveMissingVatsim(f.ctx, f.airport, "SAS123"), nil }))
	b, err = w.options.State.Read(f.ctx, f.airport)
	require.NoError(t, err)
	require.False(t, b.Flights[0].LatestObservation.Missing)
	require.Equal(t, string(aman.SurveillanceSourceEuroScope), b.Flights[0].LatestObservation.SurveillanceSource)
	require.NotEqual(t, "removed", b.Flights[0].State)
	// A later accepted holding clearance must carry its own fact timestamp.
	s.Hold = "TNO"
	s.EuroscopeObservedAt = timestamp(f.now.Add(time.Second))
	f.entity(t, sessionRef(f.session), s.Callsign, &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: s}})
	f.now = f.now.Add(2 * time.Second)
	committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.Reconcile(f.ctx, f.airport, f.now), nil }))
	b, err = w.options.State.Read(f.ctx, f.airport)
	require.NoError(t, err)
	require.Equal(t, "TNO", b.Flights[0].HoldingClearance.Hold)
}
