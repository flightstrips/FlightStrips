package app

import (
	"FlightStrips/internal/frontendbinary"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"strings"
	"testing"
	"time"
)

// Exercise browser command-to-delta delivery on both nodes while ES sends a
// full batch of position observations for a populated session.
func TestBuildNATSStripMoveLatencyDuringPositionTraffic(t *testing.T) {
	f := newRuntimeFixture(t, nil)
	socket := f.socket(0, "111111", "EKCH_A_TWR", "118.100")
	strips := make([]*es.Strip, 300)
	for i := range strips {
		strips[i] = &es.Strip{Callsign: fmt.Sprintf("LAG%03d", i), Origin: "EKCH", Destination: "EGLL", AircraftType: "A320", AssignedSquawk: "1001", Runway: "22R", Sid: "ODN1C", Route: "ODN DCT", HasFp: true}
	}
	f.sync(socket, strips...)
	f.await("all strips present on both nodes", func() bool {
		for _, app := range f.apps {
			state, err := app.natsRuntime.projection.ReadEntityKinds(sessionNATSRef(f.session), pb.EntityKind_STRIP)
			if err != nil || len(state.Indexes[pb.EntityKind_STRIP]) != len(strips) {
				return false
			}
		}
		return true
	})
	owner := f.owner(sessionNATSRef(f.session))
	for _, node := range []int{owner, 1 - owner} {
		dialer := websocket.Dialer{Subprotocols: []string{frontendbinary.Subprotocol}}
		conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(f.servers[node].URL, "http")+"/frontEndEvents", nil)
		require.NoError(t, err)
		defer conn.Close()
		write := func(frame *pb.FrontendFrame) {
			data, err := proto.Marshal(frame)
			require.NoError(t, err)
			require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, data))
		}
		read := func() *pb.FrontendFrame {
			require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
			_, data, err := conn.ReadMessage()
			require.NoError(t, err)
			frame := &pb.FrontendFrame{}
			require.NoError(t, pb.UnmarshalStrict(data, frame))
			return frame
		}
		write(&pb.FrontendFrame{ProtocolRevision: 2, Frame: &pb.FrontendFrame_Authenticate{Authenticate: &pb.FrontendAuthenticate{BearerToken: "111111", Airport: "EKCH", SessionName: f.name}}})
		require.NotNil(t, read().GetInitial())
		var worst time.Duration
		for i := 0; i < 10; i++ {
			state := f.state()
			for _, strip := range strips {
				socket.send(t, &es.Envelope{CommandId: uuid.NewString(), SessionId: f.session, OwnerEpoch: state.Owner.Epoch, MasterEpoch: state.Master.Epoch,
					Event: &es.Envelope_AircraftPositionUpdate{AircraftPositionUpdate: &es.AircraftPositionUpdateEvent{Callsign: strip.Callsign, Lat: 55.6, Lon: 12.6, Altitude: 100, GroundSpeedKnots: 5}}})
			}
			id := uuid.NewString()
			bay := "CLEARED"
			if i%2 == 1 {
				bay = "TAXI"
			}
			start := time.Now()
			write(&pb.FrontendFrame{ProtocolRevision: 2, Frame: &pb.FrontendFrame_Command{Command: &pb.FrontendCommand{RequestId: id,
				Action: &pb.ClientCommand{Action: &pb.ClientCommand_Strip{Strip: &pb.StripAction{Callsign: strips[0].Callsign, Change: &pb.StripAction_Move{Move: &pb.MoveStrip{Bay: bay, Clearance: proto.Bool(i == 0), ConfirmedRemoval: proto.Bool(false)}}}}}}}})
			delivered := false
			for !delivered {
				frame := read()
				if result := frame.GetActionResult(); result != nil && result.RequestId == id {
					require.NotEqual(t, pb.CommandOutcome_FAILED, result.Status, "%s: %s", result.ReasonCode, result.Detail)
				}
				for _, change := range frame.GetDelta().GetChanges() {
					strip := change.GetUpsert().GetStrip()
					if strip.GetCallsign() == strips[0].Callsign && strip.GetBay() == bay {
						delivered = true
					}
				}
			}
			elapsed := time.Since(start)
			if elapsed > worst {
				worst = elapsed
			}
			require.Less(t, elapsed, time.Second, "strip moves must not wait seconds behind observations")
		}
		// One browser can issue many commands before receiving any delta.
		// Every intention must succeed despite the unchanged browser revision.
		pending := map[string]bool{}
		for i := 0; i < 20; i++ {
			id := uuid.NewString()
			pending[id] = true
			write(&pb.FrontendFrame{ProtocolRevision: 2, Frame: &pb.FrontendFrame_Command{Command: &pb.FrontendCommand{RequestId: id,
				Action: &pb.ClientCommand{Action: &pb.ClientCommand_Strip{Strip: &pb.StripAction{Callsign: strips[0].Callsign,
					Change: &pb.StripAction_SetMarked{SetMarked: &pb.SetMarked{Marked: i%2 == 1}}}}}}}})
		}
		for len(pending) > 0 {
			frame := read()
			if result := frame.GetActionResult(); result != nil && pending[result.RequestId] {
				require.Equal(t, pb.CommandOutcome_SUCCEEDED, result.Status, "%s: %s", result.ReasonCode, result.Detail)
				delete(pending, result.RequestId)
			}
		}

		t.Logf("node=%d owner=%d 300 strips, position batches: worst move delivery %s", node, owner, worst)
	}
}
