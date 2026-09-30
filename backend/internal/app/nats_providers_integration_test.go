package app

import (
	appconfig "FlightStrips/internal/config"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBuildNATSProductionProvidersCDMSATAndPDC(t *testing.T) {
	callsign := "S" + strings.ToUpper(uuid.NewString()[:6])
	cpdlc := fmt.Sprintf("TST%04d", time.Now().UnixNano()%10000)
	clock := time.Now().UTC().Add(30 * time.Minute).Format("1504")
	var mu sync.Mutex
	polls, sends := 0, 0
	readyToPoll, requestDelivered := false, false
	paths := map[string]int{}
	var provider *httptest.Server
	provider = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		mu.Lock()
		paths[request.URL.Path]++
		mu.Unlock()
		switch request.URL.Path {
		case "/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"v3": []string{provider.URL + "/network"}}})
		case "/network":
			stand, _ := appconfig.GetStandCapabilities().Lookup("EKCH", "A17")
			_ = json.NewEncoder(w).Encode(map[string]any{"general": map[string]any{"update_timestamp": time.Now().UTC().Format(time.RFC3339)}, "pilots": []any{map[string]any{"cid": 111111, "callsign": callsign, "latitude": stand.Latitude, "longitude": stand.Longitude, "altitude": 0, "groundspeed": 0, "logon_time": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), "last_updated": time.Now().UTC().Format(time.RFC3339), "flight_plan": map[string]any{"flight_rules": "I", "aircraft_short": "A320", "departure": "EKCH", "arrival": "ENGM", "deptime": clock, "route": "ODN DCT", "altitude": "35000", "revision_id": 1}}}, "prefiles": []any{}})
		case "/transceivers":
			_ = json.NewEncoder(w).Encode([]any{map[string]any{"callsign": "EKCH_DEL", "frequency": 119905000}, map[string]any{"callsign": "EKCH_A_TWR", "frequency": 118100000}})
		case "/atis":
			_ = json.NewEncoder(w).Encode([]any{map[string]any{"callsign": "EKCH_D_ATIS", "frequency": "122.750", "atis_code": "A", "text_atis": []string{"COPENHAGEN INFORMATION ALPHA"}, "last_updated": time.Now().UTC().Format(time.RFC3339)}})
		case "/metar/EKCH":
			fmt.Fprint(w, "EKCH 302050Z 22005KT CAVOK 12/09 Q1015")
		case "/flow-measure", "/etfms/restrictions", "/ifps/depAirport", "/airport":
			fmt.Fprint(w, "[]")
		case "/airport/setMaster", "/airport/removeMaster", "/ifps/setCdmData", "/ifps/dpi":
			fmt.Fprint(w, "{}")
		case "/hoppie":
			mu.Lock()
			first := request.URL.Query().Get("type") == "poll" && readyToPoll && !requestDelivered
			if first {
				requestDelivered = true
			}
			if request.URL.Query().Get("type") == "poll" {
				polls++
			} else {
				sends++
			}
			mu.Unlock()
			if first {
				fmt.Fprintf(w, "ok {%s telex {REQUEST PREDEP CLEARANCE %s A320 TO ENGM AT EKCH STAND A18 ATIS A}}", cpdlc, cpdlc)
			} else {
				fmt.Fprint(w, "ok")
			}
		default:
			http.NotFound(w, request)
		}
	}))
	t.Cleanup(provider.Close)
	f := newRuntimeFixture(t, func(cfg *Config, deps *Dependencies) {
		root, err := filepath.Abs("../..")
		require.NoError(t, err)
		cfg.CDMConfigDir = filepath.Join(root, "config")
		cfg.EnableCDMConfigStore = true
		cfg.CDMConfigRefreshInterval = time.Second
		cfg.CDMKey = "fixture-key"
		cfg.EnableVATSIM = true
		cfg.EnableTransceivers = true
		cfg.EnableMetar = true
		cfg.EnableECFMP = true
		cfg.EnableStandAssignment = true
		cfg.StandAssignmentAircraftJSON = filepath.Join(root, "config/test/ICAO_Aircraft.json")
		cfg.HoppieLogon = "fixture-key"
		cfg.ECFMPBaseURL = provider.URL
		deps.VATSIMStatusURL = provider.URL + "/status"
		deps.VATSIMPollInterval = 5 * time.Second
		deps.TransceiversURL = provider.URL + "/transceivers"
		deps.TransceiversInterval = 5 * time.Second
		deps.NATS = NATSDependencies{HTTPClient: provider.Client(), CDMBaseURL: provider.URL, MetarBaseURL: provider.URL + "/metar", ATISURL: provider.URL + "/atis", HoppieBaseURL: provider.URL + "/hoppie"}
	})
	f.name = "LIVE"
	socket := f.socket(0, "111111", "EKCH_DEL", "119.905")
	f.await("accepted production CDM configuration", func() bool {
		page, _, err := f.apps[0].natsRuntime.cdm.Config.Read(f.ctx, "EKCH")
		return err == nil && page != nil
	})
	f.sync(socket, &es.Strip{Callsign: callsign, Origin: "EKCH", Destination: "ENGM", AircraftType: "A320", AssignedSquawk: "1001", Sid: "ODN1C", Runway: "22R", Route: "ODN DCT", HasFp: true, Eobt: clock, Stand: "A17", TrackingController: "EKCH_DEL"}, &es.Strip{Callsign: cpdlc, Origin: "EKCH", Destination: "ENGM", AircraftType: "A320", AssignedSquawk: "1002", Sid: "ODN1C", Runway: "22R", Route: "ODN DCT", HasFp: true, Eobt: clock, Stand: "A18", TrackingController: "EKCH_DEL"})
	mu.Lock()
	readyToPoll = true
	mu.Unlock()
	t.Cleanup(func() {
		if t.Failed() {
			state := f.state()
			t.Logf("PDC sequences: %v", state.Indexes[pb.EntityKind_PDC_SEQUENCE])
			mu.Lock()
			t.Logf("provider requests: %v", paths)
			mu.Unlock()
		}
	})
	f.await("owner workers accept provider and SAT generations", func() bool {
		state := f.state()
		return state.Indexes[pb.EntityKind_VATSIM_SESSION_CURSOR]["vatsim"] != nil && state.Indexes[pb.EntityKind_ATIS]["EKCH"] != nil && state.Indexes[pb.EntityKind_STAND_ASSIGNMENT][callsign] != nil
	})
	f.send(socket, &es.Envelope{Event: &es.Envelope_CdmTobtUpdate{CdmTobtUpdate: &es.CdmTobtUpdateEvent{Callsign: callsign, Tobt: clock}}})
	f.await("binary CDM action reaches actual policy", func() bool {
		return f.state().Indexes[pb.EntityKind_CDM_STATE][callsign].GetValue().GetCdmState().GetTobt() != nil
	})
	var reply *pb.CommandReply
	id := uuid.NewString()
	f.await("production PDC request accepted", func() bool {
		r := f.apps[1].natsRuntime
		state, err := r.projection.Read(sessionNATSRef(f.session))
		if err != nil {
			return false
		}
		revision := state.Indexes[pb.EntityKind_PDC_SEQUENCE][callsign].GetRevision()
		reply = r.Route(f.ctx, &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: state.Ref, Actor: &pb.Actor{Kind: pb.Actor_PILOT, Id: "111111", SessionId: &f.session}, ExpectedEntityRevision: &revision, Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Pdc{Pdc: &pb.PdcAction{Callsign: callsign, Change: &pb.PdcAction_Issue{Issue: &pb.IssuePdc{RequestChannel: "WEB", Atis: "A", Stand: "A17", AircraftType: "A320"}}}}}}})
		return natsReply(reply) == nil
	})
	require.Eventually(t, func() bool {
		sequence := f.state().Indexes[pb.EntityKind_PDC_SEQUENCE][cpdlc].GetValue().GetPdcSequence()
		mu.Lock()
		defer mu.Unlock()
		return polls > 0 && sends > 0 && sequence.GetState() == "CLEARED"
	}, 65*time.Second, 100*time.Millisecond, "Hoppie request passes production clearance and send policy")
	f.get(1, "/api/efb/flight?callsign="+callsign, http.StatusOK)
	f.get(1, "/api/pdc/status?callsign="+callsign, http.StatusOK)
	for _, path := range []string{"/status", "/network", "/transceivers", "/atis", "/metar/EKCH", "/flow-measure", "/airport", "/etfms/restrictions"} {
		mu.Lock()
		count := paths[path]
		mu.Unlock()
		require.Positive(t, count, path)
	}
	// The fresh quota gate is global and cannot grant the same call twice.
	reservation := uuid.NewString()
	r := f.apps[f.owner(globalNATSRef())].natsRuntime
	allowed, err := r.reserveQuota(context.Background(), reservation, "openmeteo", time.Now().UTC().Truncate(time.Minute), 500)
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = r.reserveQuota(context.Background(), reservation, "openmeteo", time.Now().UTC().Truncate(time.Minute), 500)
	require.NoError(t, err)
	require.False(t, allowed)
}
