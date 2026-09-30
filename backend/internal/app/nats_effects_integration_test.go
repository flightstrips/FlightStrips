package app

import (
	"strings"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestBuildNATSDeadlineAndSquawkEffects(t *testing.T) {
	f := newRuntimeFixture(t, nil)
	callsign := "S" + strings.ToUpper(uuid.NewString()[:6])
	socket := f.socket(0, "111111", "EKCH_DEL", "119.905")
	f.sync(socket, &es.Strip{Callsign: callsign, Origin: "EKCH", Destination: "ENGM", AircraftType: "A320", Sid: "ODN1C", Runway: "22R", HasFp: true})
	state := f.state()
	var id string
	var delivered *es.Envelope
	f.await("accepted effect reaches actual binary socket", func() bool {
		socket.mu.Lock()
		defer socket.mu.Unlock()
		for _, frame := range socket.frames {
			if frame.GetGenerateSquawk().GetCallsign() == callsign {
				delivered = frame
				id = frame.CommandId
				return true
			}
		}
		return false
	})
	socket.send(t, &es.Envelope{CommandId: id, SessionId: delivered.SessionId, OwnerEpoch: delivered.OwnerEpoch, MasterEpoch: delivered.MasterEpoch,
		Event: &es.Envelope_CommandResult{CommandResult: &es.CommandResultEvent{CommandId: id, Status: es.CommandResultEvent_EXECUTED, Reason: es.CommandResultEvent_OK}}})
	f.await("plugin result becomes durable on both replicas", func() bool {
		for _, app := range f.apps {
			accepted, err := app.natsRuntime.projection.Read(state.Ref)
			if err != nil || accepted.Effects[id].GetStatus() != pb.EffectRecord_EXECUTED {
				return false
			}
		}
		return true
	})
	f.send(socket, &es.Envelope{Event: &es.Envelope_AssignedSquawk{AssignedSquawk: &es.AssignedSquawkEvent{Callsign: callsign, Squawk: "2104"}}})
	f.await("observed squawk enters accepted strip", func() bool {
		return f.state().Indexes[pb.EntityKind_STRIP][callsign].GetValue().GetStrip().GetAssignedSquawk() == "2104"
	})
	for _, action := range []*pb.FlightPlanAction{
		{Callsign: callsign, Create: &pb.FlightPlanAction_Manual{Manual: &pb.CreateManualFlightPlan{Destination: "EGLL", Sid: "ODN1C", Squawk: "2104", Eobt: timestamppb.New(time.Now().UTC().Add(time.Hour)), AircraftType: "A320", FlightLevel: "330", Route: "ODN DCT", Stand: "A17", DepartureRunway: "22R"}}},
		{Callsign: callsign, Create: &pb.FlightPlanAction_Vfr{Vfr: &pb.CreateVfrFlightPlan{AircraftType: "C172", PersonsOnBoard: 2, FlightPlanType: "V", Language: "EN", Remarks: "LOCAL"}}},
	} {
		command := uuid.NewString()
		f.await("manual flight plan reaches owner policy", func() bool {
			state := f.state()
			revision := state.Indexes[pb.EntityKind_STRIP][callsign].Revision
			reply := f.apps[1].natsRuntime.Route(f.ctx, &pb.CommandRequest{ProtocolRevision: 1, CommandId: command, Aggregate: state.Ref, Actor: &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: socket.cid, SessionId: &f.session}, ExpectedEntityRevision: &revision, Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_FlightPlan{FlightPlan: action}}}})
			if reply.GetOutcome().GetReasonCode() == "REVISION_CONFLICT" {
				command = uuid.NewString()
			}
			return natsReply(reply) == nil
		})
		var frame *es.Envelope
		f.await("typed flight-plan effect reaches real plugin transport", func() bool {
			socket.mu.Lock()
			defer socket.mu.Unlock()
			for _, received := range socket.frames {
				if received.CommandId == command && received.GetCreateFpl() != nil {
					frame = received
					return true
				}
			}
			return false
		})
		flight := frame.GetCreateFpl()
		require.Equal(t, callsign, flight.Callsign)
		if action.GetManual() != nil {
			require.Equal(t, "EGLL", flight.Destination)
			require.Equal(t, int32(33000), flight.RequestedAltitude)
			require.Equal(t, "ODN DCT", flight.Route)
			require.Equal(t, "A17", flight.Stand)
		} else {
			require.Equal(t, "7000", flight.AssignedSquawk)
			require.Equal(t, int32(2), flight.PersonsOnBoard)
			require.Equal(t, "LOCAL", flight.Remarks)
		}
		socket.send(t, &es.Envelope{CommandId: command, SessionId: frame.SessionId, OwnerEpoch: frame.OwnerEpoch, MasterEpoch: frame.MasterEpoch, Event: &es.Envelope_CommandResult{CommandResult: &es.CommandResultEvent{CommandId: command, Status: es.CommandResultEvent_EXECUTED, Reason: es.CommandResultEvent_OK}}})
		f.await("flight-plan result persists on both replicas", func() bool {
			for _, app := range f.apps {
				state, err := app.natsRuntime.projection.Read(sessionNATSRef(f.session))
				if err != nil || state.Effects[command].GetStatus() != pb.EffectRecord_EXECUTED {
					return false
				}
			}
			return true
		})
	}
	require.NoError(t, socket.conn.Close())
	f.await("offline grace is persisted and consumed by assembled worker", func() bool { return f.state().Indexes[pb.EntityKind_CONTROLLER][socket.cid] == nil })
}
