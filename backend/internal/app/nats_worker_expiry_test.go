package app

import (
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"testing"
	"time"
)

func TestBuildNATSAircraftExpiryWaitsForSyncAfterMasterDisconnect(t *testing.T) {
	f := newRuntimeFixture(t, nil)
	socket := f.socket(0, "111111", "EKCH_A_TWR", "118.100")
	f.sync(socket, &es.Strip{Callsign: "SAS1", Origin: "EKCH", Destination: "EGLL", HasFp: true, AssignedSquawk: "1001", Position: &es.Position{Lat: 55.63, Lon: 12.65}})
	f.send(socket, &es.Envelope{Event: &es.Envelope_AircraftDisconnect{AircraftDisconnect: &es.AircraftDisconnectEvent{Callsign: "SAS1"}}})
	var deadline *pb.EntitySnapshot
	f.await("disconnect deadline", func() bool {
		for _, entity := range f.state().EntitiesByKind(pb.EntityKind_SESSION_DEADLINE) {
			if entity.GetValue().GetSessionDeadline().Kind == "aircraft-disconnect" {
				deadline = entity
				return true
			}
		}
		return false
	})
	runtime := f.apps[f.owner(sessionNATSRef(f.session))].natsRuntime
	d := proto.Clone(deadline.GetValue().GetSessionDeadline()).(*pb.SessionDeadline)
	d.DueAt = timestamppb.New(time.Now().Add(3 * time.Second))
	_, err := (cluster.SessionObservations{Store: cluster.RoutedLifecycleStore{Router: runtime.router, Projection: runtime.projection}}).PutDeadline(f.ctx, f.session, uuid.NewString(), d, deadline.Revision)
	require.NoError(t, err)
	require.NoError(t, socket.conn.Close())
	f.await("master socket retired", func() bool { return f.state().Master.GetConnectionId() == "" })
	f.await("expiry is due", func() bool { return !time.Now().Before(d.DueAt.AsTime()) })
	require.NoError(t, runtime.work.ReconcileSession(f.ctx, f.session))
	f.await("pending expiry and strip survive master disconnect", func() bool {
		for _, app := range f.apps {
			state, err := app.natsRuntime.projection.Read(sessionNATSRef(f.session))
			if err != nil || state.Indexes[pb.EntityKind_SESSION_DEADLINE][d.Id] == nil || state.Indexes[pb.EntityKind_STRIP]["SAS1"] == nil {
				return false
			}
		}
		return true
	})
}
