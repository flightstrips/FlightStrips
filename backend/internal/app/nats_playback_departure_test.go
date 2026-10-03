package app

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	appconfig "FlightStrips/internal/config"
	"FlightStrips/internal/frontendbinary"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestBuildNATSPlaybackDepartureStandAndCDMReachFrontend(t *testing.T) {
	f := newRuntimeFixture(t, func(cfg *Config, _ *Dependencies) {
		root, err := filepath.Abs("../..")
		require.NoError(t, err)
		cfg.EnableStandAssignment = true
		cfg.StandAssignmentAircraftJSON = filepath.Join(root, "config/test/ICAO_Aircraft.json")
	})
	master := f.socket(0, "111111", "EKCH_A_TWR", "118.100")
	stand, found := appconfig.GetStandCapabilities().Lookup("EKCH", "A17")
	require.True(t, found)
	clock := time.Now().UTC().Add(10 * time.Minute).Format("1504")
	strip := &es.Strip{Callsign: "SAS1", Origin: "EKCH", Destination: "EGLL", AircraftType: "A320", HasFp: true, AssignedSquawk: "1001", Runway: "22R", Sid: "ODN1C", Eobt: clock, Stand: "WRONG", Position: &es.Position{Lat: stand.Latitude, Lon: stand.Longitude}}
	f.sync(master, strip)
	f.await("backend detects stand and calculates CDM without VATSIM or ES stand", func() bool {
		for _, app := range f.apps {
			state, err := app.natsRuntime.projection.Read(sessionNATSRef(f.session))
			if err != nil {
				return false
			}
			s := state.Indexes[pb.EntityKind_STRIP]["SAS1"].GetValue().GetStrip()
			c := state.Indexes[pb.EntityKind_CDM_STATE]["SAS1"].GetValue().GetCdmState()
			if s.GetStand() != "A17" || c.GetTobt() == nil || c.GetTsat() == nil || c.GetTtot() == nil || s.GetTsat() == nil {
				return false
			}
		}
		return true
	})
	// ES echoes and subsequent complete reports cannot erase backend geometry.
	f.send(master, &es.Envelope{Event: &es.Envelope_Stand{Stand: &es.StandEvent{Callsign: "SAS1", Stand: "WRONG"}}})
	f.sync(master, strip)
	f.await("resync preserves detected stand and calculated times", func() bool {
		s := f.state().Indexes[pb.EntityKind_STRIP]["SAS1"].GetValue().GetStrip()
		return s.GetStand() == "A17" && s.GetTsat() != nil
	})
	dialer := websocket.Dialer{Subprotocols: []string{frontendbinary.Subprotocol}}
	conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(f.servers[1].URL, "http")+"/frontEndEvents", nil)
	require.NoError(t, err)
	defer conn.Close()
	request := &pb.FrontendFrame{ProtocolRevision: 2, Frame: &pb.FrontendFrame_Authenticate{Authenticate: &pb.FrontendAuthenticate{BearerToken: "111111", Airport: "EKCH", SessionName: f.name}}}
	data, err := proto.Marshal(request)
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, data))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(10*time.Second)))
	_, data, err = conn.ReadMessage()
	require.NoError(t, err)
	frame := &pb.FrontendFrame{}
	require.NoError(t, pb.UnmarshalStrict(data, frame))
	var received *pb.Strip
	for _, entity := range frame.GetInitial().GetEntities() {
		if entity.GetValue().GetStrip().GetCallsign() == "SAS1" {
			received = entity.GetValue().GetStrip()
		}
	}
	require.NotNil(t, received)
	require.Equal(t, "A17", received.Stand)
	require.Equal(t, clock, received.Tobt.AsTime().UTC().Format("1504"))
	require.NotNil(t, received.Tsat)
}
