package app

import (
	"fmt"
	"testing"

	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestBuildNATSDuplicateDisconnectKeepsSocketAndTombstone(t *testing.T) {
	f := newRuntimeFixture(t, nil)
	master := f.socket(0, "111111", "EKCH_A_TWR", "118.100")
	f.sync(master, &es.Strip{Callsign: "SAS1", Origin: "EKCH", Destination: "EGLL", HasFp: true, AssignedSquawk: "1001", Position: &es.Position{Lat: 55.6, Lon: 12.65}})
	disconnect := func() {
		f.send(master, &es.Envelope{Event: &es.Envelope_AircraftDisconnect{AircraftDisconnect: &es.AircraftDisconnectEvent{Callsign: "SAS1"}}})
	}
	tombstone := func() bool {
		entries, _, err := f.apps[f.owner(sessionNATSRef(f.session))].natsRuntime.projection.ObservationSnapshot(f.session)
		if err != nil {
			return false
		}
		for _, entry := range entries {
			if entry.Value.AircraftKey == "SAS1" {
				return entry.Value.GetTombstone() != nil
			}
		}
		return false
	}
	disconnect()
	f.await("disconnect accepted", tombstone)
	disconnect()
	f.send(master, &es.Envelope{Event: &es.Envelope_AircraftPositionUpdate{AircraftPositionUpdate: &es.AircraftPositionUpdateEvent{Callsign: "SAS1", Lat: 55.6, Lon: 12.65}}})
	// Stateful runway admission drains preceding position events on this socket.
	f.send(master, &es.Envelope{Event: &es.Envelope_Runway{Runway: &es.RunwayEvent{Runways: []*es.Runway{{Name: "18", Departure: true}}}}})
	f.await("socket processes events after duplicate disconnect", func() bool {
		master.mu.Lock()
		defer master.mu.Unlock()
		require.NoError(t, master.err)
		session := f.state().Indexes[pb.EntityKind_SESSION][fmt.Sprint(f.session)].Value.GetSession()
		return len(session.Runways) == 1 && session.Runways[0].Name == "18"
	})
	require.True(t, tombstone(), "late position must not resurrect disconnected aircraft")
}

func TestBuildNATSSyncCreatesStripAfterRetainedCDMRecord(t *testing.T) {
	f := newRuntimeFixture(t, nil)
	master := f.socket(0, "111111", "EKCH_A_TWR", "118.100")
	ref := sessionNATSRef(f.session)
	runtime := f.apps[f.owner(ref)].natsRuntime
	zero := uint64(0)
	reply := runtime.router.RouteDurable(f.ctx, &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test-cdm"}, ExpectedEntityRevision: &zero,
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "SAS1", Value: &pb.EntityRecord{Value: &pb.EntityRecord_CdmState{CdmState: &pb.CdmState{Callsign: "SAS1"}}}}}}}})
	require.Equal(t, pb.CommandReply_COMMITTED, reply.Status, "%+v", reply)
	require.NotEqual(t, pb.CommandOutcome_FAILED, reply.GetOutcome().GetStatus(), "%+v", reply)
	f.sync(master, &es.Strip{Callsign: "SAS1", Origin: "EKCH", Destination: "EGLL", HasFp: true, AssignedSquawk: "1001"})
	f.await("both typed records retained", func() bool {
		state := f.state()
		return state.Indexes[pb.EntityKind_STRIP]["SAS1"] != nil && state.Indexes[pb.EntityKind_CDM_STATE]["SAS1"] != nil
	})
	master.mu.Lock()
	defer master.mu.Unlock()
	require.NoError(t, master.err)
}
