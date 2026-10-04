package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/cluster"
	cr "FlightStrips/internal/coordinationrequest"
	"FlightStrips/internal/frontendbinary"
	"FlightStrips/internal/navigation"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestBuildNATSAIRACWindAndAMANPolicy(t *testing.T) {
	from := time.Now().UTC().Truncate(24 * time.Hour).Add(-24 * time.Hour)
	until := from.Add(28 * 24 * time.Hour)
	course := 221.2
	minute := int64(60)
	provenance := terminal.ProvenanceDefinition{SourceID: "fixture-aip", SourceRevision: "fixture-r1", ImportedAt: from, EffectiveFrom: from, EffectiveUntil: until}
	config := terminal.Configuration{SchemaVersion: terminal.SchemaVersion, ConfigVersion: "runtime-" + uuid.NewString(), Airport: "EKCH", ApplicabilityFrom: from, ApplicabilityUntil: until,
		Dataset: terminal.DatasetCompatibility{Cycle: "2610", EffectiveFrom: from, EffectiveUntil: until}, Sources: []terminal.Source{{ID: "fixture-aip", Document: "local HTTP fixture", EffectiveFrom: from, EffectiveUntil: until}},
		RunwayGroups: []terminal.RunwayGroup{{ID: "SOUTH", Aliases: []aman.RunwayGroupID{"22L"}, Runways: []navdata.RunwayID{"22L"}, FinalApproaches: []terminal.FinalApproachDefinition{{Runway: "22L", FinalApproachFix: "KEMAX", Threshold: terminal.ThresholdDefinition{Position: terminal.CoordinateDefinition{LatitudeDeg: 55.6254, LongitudeDeg: 12.6676}, CourseTrueDeg: &course}, CourseTrueDeg: course, PhysicalLengthM: 3302, Provenance: provenance}}}},
		Feeders:      []terminal.Feeder{{ID: "SOK"}}, Paths: []terminal.Path{{Feeder: "SOK", RunwayGroup: "SOUTH", Fixes: []navdata.FixID{"SOK", "KEMAX"}, MergeFix: "KEMAX", SelectedHolding: "SOK-HF"}},
		OverlayHoldings: []terminal.HoldingDefinition{{ID: "SOK-HF", Fix: "SOK", InboundCourseTrueDeg: 90, TurnDirection: navdata.TurnRight, LegTimeSeconds: &minute, Termination: navdata.HoldingManual, Provenance: provenance}}}
	data, err := json.Marshal(config)
	require.NoError(t, err)
	filename := filepath.Join(t.TempDir(), "terminal.json")
	require.NoError(t, os.WriteFile(filename, data, 0600))
	var airacCalls, windCalls atomic.Int32
	point := func(id string, lat, lon float64) any {
		return map[string]any{"identifier": id, "coordinates": map[string]any{"lat": lat, "lon": lon}}
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var value any
		switch {
		case r.URL.Path == "/api/v1/airac/current":
			airacCalls.Add(1)
			value = map[string]any{"data": map[string]any{"cycle": "2610", "effective_date": from.Format(time.RFC3339), "expiration_date": until.Format(time.RFC3339)}}
		case r.URL.Path == "/api/v1/airports/EKCH":
			value = map[string]any{"data": map[string]any{"icao": "EKCH", "name": "Copenhagen", "coordinates": map[string]any{"lat": 55.618, "lon": 12.656}}}
		case r.URL.Path == "/api/v1/procedures":
			kind := r.URL.Query().Get("type")
			id := map[string]string{"SID": "KEMAX3A", "STAR": "SOK1P", "APP": "ILS22L"}[kind]
			if id == "" {
				id = "SOK1P"
				kind = "STAR"
			}
			value = map[string]any{"data": []any{map[string]any{"airport": "EKCH", "identifier": id, "type": map[string]any{"code": kind}, "runway": "22L"}}, "pagination": map[string]any{"has_more": false}}
		case strings.HasPrefix(r.URL.Path, "/api/v1/procedures/EKCH/"):
			id := path.Base(r.URL.Path)
			kind := map[string]string{"KEMAX3A": "SID", "SOK1P": "STAR", "ILS22L": "APP"}[id]
			value = map[string]any{"data": map[string]any{"airport": "EKCH", "identifier": id, "type": map[string]any{"code": kind}, "available_runways": []string{"22L"}, "segments": []any{map[string]any{"sequence": 10, "path_terminator": "TF", "fix_identifier": "SOK"}, map[string]any{"sequence": 20, "path_terminator": "TF", "fix_identifier": "KEMAX"}}}}
		case strings.HasPrefix(r.URL.Path, "/api/v1/waypoints/"):
			id := path.Base(r.URL.Path)
			lat, lon := 55.4, 11.5
			if id == "KEMAX" {
				lat, lon = 55.71, 12.8
			}
			value = map[string]any{"data": point(id, lat, lon)}
		case r.URL.Path == "/api/v1/routes/parse":
			value = map[string]any{"data": map[string]any{"total_distance": 40, "segments": []any{map[string]any{"from": point("EHAM", 52.3086, 4.7639), "to": point("SOK", 55.4, 11.5), "distance": 30, "bearing": 45}, map[string]any{"from": point("SOK", 55.4, 11.5), "to": point("KEMAX", 55.71, 12.8), "distance": 10, "bearing": 90}}, "errors": []any{}}}
		case r.URL.Path == "/wind":
			windCalls.Add(1)
			hourly := map[string]any{"time": []string{time.Now().UTC().Truncate(time.Hour).Format("2006-01-02T15:04"), time.Now().UTC().Truncate(time.Hour).Add(time.Hour).Format("2006-01-02T15:04")}}
			for i, level := range []int{1000, 850, 700, 500, 300, 250, 200, 150} {
				hourly[fmt.Sprintf("wind_speed_%dhPa", level)] = []float64{20, 20}
				hourly[fmt.Sprintf("wind_direction_%dhPa", level)] = []float64{270, 270}
				hourly[fmt.Sprintf("geopotential_height_%dhPa", level)] = []float64{float64(100 + i*1800), float64(100 + i*1800)}
			}
			samples := strings.Count(r.URL.Query().Get("latitude"), ",") + 1
			values := make([]any, samples)
			for i := range values {
				values[i] = map[string]any{"hourly": hourly}
			}
			if samples == 1 {
				value = values[0]
			} else {
				value = values
			}
		default:
			http.NotFound(w, r)
			return
		}
		if envelope, ok := value.(map[string]any); ok && r.URL.Path != "/wind" {
			envelope["status"] = "success"
		}
		_ = json.NewEncoder(w).Encode(value)
	}))
	t.Cleanup(provider.Close)
	f := newRuntimeFixture(t, func(cfg *Config, deps *Dependencies) {
		cfg.Navigation = navigation.Config{Source: navigation.SourceAIRACNet, TerminalGeometryPath: filename}
		cfg.AMAN = aman.RuntimeConfig{EnableEuroScopeGainLoseTags: true, Mode: aman.ModeAuthoritative, SourceMode: aman.ObservationSourceEuroScope, EnabledAirports: []string{"EKCH"}, ReconciliationInterval: time.Second, SurveillanceInterval: time.Second}
		deps.NATS = NATSDependencies{HTTPClient: provider.Client(), AIRACBaseURL: provider.URL + "/api/v1", OpenMeteoBaseURL: provider.URL + "/wind"}
	})
	socket := f.socket(0, "111111", "EKCH_FMP", "119.905")
	callsign := "A" + strings.ToUpper(uuid.NewString()[:6])
	f.sync(socket, &es.Strip{Callsign: callsign, Origin: "EHAM", Destination: "EKCH", AircraftType: "A320", AssignedSquawk: "1001", Runway: "22L", Star: "SOK1P", Route: "DCT SOK", HasFp: true, Position: &es.Position{Lat: 55.3, Lon: 11.3, Altitude: 12000, GroundSpeedKnots: 250, TrackDegrees: 90}, TrackingController: "EKCH_FMP"})
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		state, err := f.apps[0].natsRuntime.projection.Read(airportNATSRef("EKCH"))
		t.Logf("AIRAC=%d wind=%d projection=%v", airacCalls.Load(), windCalls.Load(), err)
		if state != nil {
			board, err := cluster.ReadAmanBoard(state)
			t.Logf("AMAN revision=%d flights=%d error=%v", board.Airport.GetRevision(), len(board.Flights), err)
		}
	})
	f.await("real AIRAC and surveillance produce authoritative AMAN", func() bool {
		for _, app := range f.apps {
			state, e := app.natsRuntime.projection.Read(airportNATSRef("EKCH"))
			if e != nil {
				return false
			}
			board, e := cluster.ReadAmanBoard(state)
			if e != nil || !board.Airport.GetAuthoritative() || !board.Airport.GetHealth().GetReady() {
				return false
			}
			found := false
			for _, flight := range board.Flights {
				found = found || flight.Callsign == callsign && flight.LatestObservation.GetSurveillance().GetGroundspeedKnots() == 250 && flight.Prediction != nil
			}
			if !found {
				return false
			}
		}
		return true
	})
	f.get(1, "/api/aman/airports/EKCH/flights/"+callsign+"/detail", 200)
	f.send(socket, &es.Envelope{Event: &es.Envelope_AircraftPositionUpdate{AircraftPositionUpdate: &es.AircraftPositionUpdateEvent{Callsign: callsign, Lat: 55.29, Lon: 11.3, Altitude: 12000, GroundSpeedKnots: 260, TrackDegrees: -1}}})
	f.await("backend derives southbound track despite an invalid transmitted heading", func() bool {
		for _, app := range f.apps {
			state, err := app.natsRuntime.projection.Read(airportNATSRef("EKCH"))
			if err != nil {
				return false
			}
			flight := state.Indexes[pb.EntityKind_AMAN_FLIGHT][callsign].GetValue().GetAmanFlight()
			observation := flight.GetLatestObservation().GetSurveillance()
			if observation.GetGroundspeedKnots() != 260 || observation.TrackTrueDegrees == nil || observation.GetTrackTrueDegrees() != 180 {
				return false
			}
		}
		return true
	})

	f.send(socket, &es.Envelope{Event: &es.Envelope_AircraftPositionUpdate{AircraftPositionUpdate: &es.AircraftPositionUpdateEvent{Callsign: callsign, Lat: 55.291, Lon: 11.3, Altitude: 12000, GroundSpeedKnots: 0}}})
	f.await("zero-speed ES motion gets a backend-derived speed on both nodes", func() bool {
		for _, app := range f.apps {
			state, err := app.natsRuntime.projection.ReadEntityKinds(airportNATSRef("EKCH"), pb.EntityKind_AMAN_FLIGHT)
			if err != nil {
				return false
			}
			observation := state.Indexes[pb.EntityKind_AMAN_FLIGHT][callsign].GetValue().GetAmanFlight().GetLatestObservation().GetSurveillance()
			if observation.GetLatitudeDegrees() != 55.291 || observation.GetGroundspeedKnots() <= 0 {
				return false
			}
		}
		return true
	})
	f.await("authoritative AMAN replacement delivered over ES socket", func() bool {
		socket.mu.Lock()
		defer socket.mu.Unlock()
		for _, frame := range socket.frames {
			event := frame.GetAmanGainLoss()
			if event == nil || !event.Authoritative {
				continue
			}
			for _, value := range event.Values {
				if value.Callsign == callsign && value.TargetTime != nil && value.PredictedTime != nil && value.GainLossSeconds != nil {
					return true
				}
			}
		}
		return false
	})
	observer := f.socket(1, "222222", "EKCH_OBS", "", true)
	f.await("AMAN initial replacement reaches observer on the other node", func() bool {
		observer.mu.Lock()
		defer observer.mu.Unlock()
		for _, frame := range observer.frames {
			if frame.GetAmanGainLoss().GetAuthoritative() && len(frame.GetAmanGainLoss().GetValues()) > 0 {
				return true
			}
		}
		return false
	})
	dialer := websocket.Dialer{Subprotocols: []string{frontendbinary.Subprotocol}}
	front, _, frontErr := dialer.Dial("ws"+strings.TrimPrefix(f.servers[1].URL, "http")+"/frontEndEvents", nil)
	require.NoError(t, frontErr)
	defer front.Close()
	authFrame, frontErr := proto.Marshal(&pb.FrontendFrame{ProtocolRevision: 2, Frame: &pb.FrontendFrame_Authenticate{Authenticate: &pb.FrontendAuthenticate{BearerToken: "111111", Airport: "EKCH", SessionName: f.name}}})
	require.NoError(t, frontErr)
	require.NoError(t, front.WriteMessage(websocket.BinaryMessage, authFrame))
	require.NoError(t, front.SetReadDeadline(time.Now().Add(10*time.Second)))
	_, frontData, frontErr := front.ReadMessage()
	require.NoError(t, frontErr)
	frontFrame := &pb.FrontendFrame{}
	require.NoError(t, pb.UnmarshalStrict(frontData, frontFrame))
	var delivered *pb.AmanAirport
	for _, entity := range frontFrame.GetInitial().GetEntities() {
		if entity.GetValue().GetAmanAirport() != nil {
			delivered = entity.Value.GetAmanAirport()
		}
	}
	require.NotNil(t, delivered)
	require.NotNil(t, delivered.Header)
	require.NotNil(t, delivered.TrafficPrediction)
	require.True(t, delivered.Header.GetReadiness().GetReady())
	require.NotEmpty(t, delivered.TrafficPrediction.Buckets)

	require.Positive(t, airacCalls.Load())
	require.Positive(t, windCalls.Load())
	state, err := f.apps[1].natsRuntime.projection.Read(airportNATSRef("EKCH"))
	require.NoError(t, err)
	board, err := cluster.ReadAmanBoard(state)
	require.NoError(t, err)
	id := uuid.NewString()
	revision := board.Airport.Revision
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: state.Ref, Actor: &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "111111", SessionId: &f.session}, Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Aman{Aman: &pb.AmanAction{ExpectedAirportRevision: revision, Change: &pb.AmanAction_SetRate{SetRate: &pb.AmanRate{RunwayGroupId: "SOUTH", ArrivalsPerHour: 24, EffectiveAt: timestamppb.Now()}}}}}}}
	reply := f.apps[1].natsRuntime.Route(f.ctx, request)
	require.NoError(t, natsReply(reply), reply)
	f.await("actual AMAN command outcome replicated", func() bool {
		a, e := f.apps[0].natsRuntime.projection.Read(state.Ref)
		return e == nil && a.Ledger[id].GetStatus() == pb.CommandOutcome_SUCCEEDED
	})
	amanCommand := func(makeAction func(string) *pb.AmanAction) string {
		command := uuid.NewString()
		f.await("accepted AMAN coordination command", func() bool {
			current, err := f.apps[1].natsRuntime.projection.Read(airportNATSRef("EKCH"))
			if err != nil {
				return false
			}
			board, err := cluster.ReadAmanBoard(current)
			if err != nil {
				return false
			}
			action := makeAction(command)
			action.ExpectedAirportRevision = board.Airport.Revision
			request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: command, Aggregate: current.Ref, Actor: &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "111111", SessionId: &f.session}, Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Aman{Aman: action}}}}
			response := f.apps[1].natsRuntime.Route(f.ctx, request)
			if response.GetStatus() == pb.CommandReply_REVISION_CONFLICT || response.GetOutcome().GetReasonCode() == "REVISION_CONFLICT" {
				command = uuid.NewString()
				return false
			}
			return natsReply(response) == nil
		})
		return command
	}
	submit := amanCommand(func(command string) *pb.AmanAction {
		return &pb.AmanAction{Change: &pb.AmanAction_SubmitCoordinationRequest{SubmitCoordinationRequest: &pb.AmanSubmitCoordination{Callsign: callsign, CoordinationRequestId: string(cr.IDForCommand(command)), Request: &pb.AmanSubmitCoordination_Speed{Speed: &pb.AmanSpeed{Requested: "210 KT"}}}}}
	})
	coordination := string(cr.IDForCommand(submit))
	amanCommand(func(string) *pb.AmanAction {
		return &pb.AmanAction{Change: &pb.AmanAction_AcceptCoordinationRequest{AcceptCoordinationRequest: &pb.AmanDecideCoordination{CoordinationRequestId: coordination}}}
	})
	f.send(socket, &es.Envelope{Event: &es.Envelope_AmanRouteFact{AmanRouteFact: &es.AMANRouteFactEvent{Version: 1, Data: &es.AMANRouteFactData{Callsign: callsign, Kind: "speed", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), AssignedSpeed: &es.AssignedSpeed{Value: &es.AssignedSpeed_Knots{Knots: 210}}}}}})
	f.await("real route-fact policy correlates accepted coordination", func() bool {
		state, err := f.apps[1].natsRuntime.projection.Read(airportNATSRef("EKCH"))
		if err != nil {
			return false
		}
		board, err := cluster.ReadAmanBoard(state)
		if err != nil {
			return false
		}
		for _, request := range board.Coordinations {
			if request.Id == coordination {
				return request.Clearance != nil && request.Clearance.Value == "210 KT"
			}
		}
		return false
	})

}
