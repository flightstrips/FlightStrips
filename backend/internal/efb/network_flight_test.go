package efb

import (
	"FlightStrips/internal/cdm"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/testutil"
	"FlightStrips/internal/vatsim"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func networkTestSource(t *testing.T, online bool, timestamp time.Time) *vatsim.Cache {
	return networkTestFlight(t, online, timestamp, "SAS99X", "EKCH", "EKYT")
}

func networkTestFlight(t *testing.T, online bool, timestamp time.Time, callsign, origin, destination string) *vatsim.Cache {
	return networkTestFlightWithRoute(t, online, timestamp, callsign, origin, destination, "DCT GOLGA")
}

func networkTestFlightWithRoute(t *testing.T, online bool, timestamp time.Time, callsign, origin, destination, route string) *vatsim.Cache {
	t.Helper()
	var feed *httptest.Server
	feed = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			fmt.Fprintf(w, `{"data":{"v3":[%q]}}`, feed.URL+"/data")
			return
		}
		group := "pilots"
		if !online {
			group = "prefiles"
		}
		fmt.Fprintf(w, `{"general":{"update_timestamp":%q},%q:[{"cid":1234567,"callsign":%q,"groundspeed":61,"flight_plan":{"departure":%q,"arrival":%q,"aircraft":"A320/M-SDE/LB1","aircraft_short":"A320","route":%q,"deptime":"1930"}}]}`,
			timestamp.UTC().Format(time.RFC3339), group, callsign, origin, destination, route)
	}))
	t.Cleanup(feed.Close)
	source := vatsim.NewCache(feed.URL+"/status", time.Hour, feed.Client())
	if _, _, err := source.GetCallsignByCID(context.Background(), "1234567"); err != nil {
		t.Fatal(err)
	}
	return source
}

type networkTimingStub struct {
	err      error
	callsign string
	tobt     string
	taxi     int
}

func (s *networkTimingStub) FlightByCallsign(_ context.Context, callsign string) (*cdm.IFPSData, error) {
	s.callsign = callsign
	return &cdm.IFPSData{Callsign: callsign, TOBT: "1935", CTOT: "1950", Taxi: 12, CDMData: cdm.CDMData{TSAT: "194000"}}, s.err
}

func (s *networkTimingStub) IFPSSetTobt(_ context.Context, callsign, tobt string, taxi int) error {
	s.callsign, s.tobt, s.taxi = callsign, tobt, taxi
	return s.err
}

func TestNetworkFlightDoesNotRequireFlightStrips(t *testing.T) {
	source := networkTestSource(t, true, time.Now())
	sensedStand := "A12"
	for _, tc := range []struct {
		name   string
		finder *finderStub
		viff   *networkTimingStub
		stand  string
		errors int
	}{
		{name: "no FlightStrips subsystem", viff: &networkTimingStub{}},
		{name: "no matching strip", finder: &finderStub{err: pdc.ErrWebStripNotFound}, viff: &networkTimingStub{}},
		{name: "stand database offline", finder: &finderStub{err: errors.New("database offline")}, viff: &networkTimingStub{}, errors: 1},
		{name: "vIFF offline", viff: &networkTimingStub{err: errors.New("vIFF offline")}, errors: 1},
		{name: "optional sensed stand", finder: &finderStub{match: pdc.WebStripMatch{SessionID: 7, Strip: &models.Strip{Stand: &sensedStand, Origin: "EKCH", Destination: "EKYT"}}}, viff: &networkTimingStub{}, stand: "A12"},
		{name: "stand from a previous flight", finder: &finderStub{match: pdc.WebStripMatch{SessionID: 7, Strip: &models.Strip{Stand: &sensedStand, Origin: "EDDF", Destination: "EKCH"}}}, viff: &networkTimingStub{}, errors: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := WebAPIConfig{Auth: authStub{}, Network: source, Viff: tc.viff, ATIS: &atisLookupStub{}}
			if tc.finder != nil {
				cfg.Flights = tc.finder
			}
			api := NewWebAPI(cfg)
			req := httptest.NewRequest(http.MethodGet, "/efb/flight?callsign=OTHER", nil)
			req.Header.Set("Authorization", "Bearer test")
			rec := httptest.NewRecorder()
			api.handleFlight(rec, req)
			var got snapshot
			if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
				t.Fatalf("expected standalone flight: %d %s", rec.Code, rec.Body.String())
			}
			if got.Callsign != "SAS99X" || got.Origin != "EKCH" || got.Destination != "EKYT" || got.Route == nil || *got.Route != "DCT GOLGA" || got.Phase != "DEPARTURE" {
				t.Fatalf("flight must come from VATSIM: %s", rec.Body.String())
			}
			if got.ATIS == nil || got.ATIS.Callsign != "EKCH_D_ATIS" || got.Capabilities.PDC || got.PDCAvailable || got.Capabilities.Stand {
				t.Fatalf("incorrect standalone capabilities/ATIS: %s", rec.Body.String())
			}
			if len(got.DataErrors) != tc.errors {
				t.Fatalf("missing integration error: %s", rec.Body.String())
			}
			if tc.viff.err == nil {
				if got.TOBT == nil || *got.TOBT != "1935" || got.TSAT == nil || *got.TSAT != "1940" || got.CTOT == nil || *got.CTOT != "1950" || got.Capabilities.TOBT {
					t.Fatalf("expected direct vIFF timings: %s", rec.Body.String())
				}
			} else if got.TOBT != nil || got.Capabilities.TOBT {
				t.Fatalf("must not fabricate timing data: %s", rec.Body.String())
			}
			if got.Stand != nil || got.StandState == nil || *got.StandState != standAssignmentUnavailable {
				t.Fatalf("without an allocator, sensed stands must not imply a reservation: %s", rec.Body.String())
			}
		})
	}
}

func TestNetworkProfileAndTobtUseAccountNotRequestedCallsign(t *testing.T) {
	source := networkTestSource(t, true, time.Now())
	timing := &networkTimingStub{}
	local := &cdmStub{}
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}, Network: source, Viff: timing, CDM: local, CDMReady: true,
		Flights: &finderStub{match: pdc.WebStripMatch{SessionID: 7, Strip: &models.Strip{Callsign: "SAS99X", Origin: "EKCH", Destination: "EKYT"}}},
		Sessions: &testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*models.Session, error) {
			return &models.Session{ID: 7, Name: "LIVE", Airport: "EKCH"}, nil
		}},
	})
	profile := httptest.NewRequest(http.MethodGet, "/efb/me?callsign=OTHER", nil)
	profile.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	api.handleMe(rec, profile)
	var got struct {
		Live     bool   `json:"live_mode"`
		Locked   bool   `json:"callsign_locked"`
		Callsign string `json:"online_callsign"`
	}
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || !got.Live || !got.Locked || got.Callsign != "SAS99X" {
		t.Fatalf("expected automatic CID lookup: %d %s", rec.Code, rec.Body.String())
	}
	update := httptest.NewRequest(http.MethodPut, "/efb/tobt", strings.NewReader(`{"callsign":"OTHER","tobt":"2000"}`))
	update.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	api.handleTobt(rec, update)
	if rec.Code != 200 || local.calls != 1 || local.session != 7 || local.callsign != "SAS99X" || local.tobt != "2000" || local.updatedBy != "EFB:1234567" || local.source != "PILOT" || timing.callsign != "" || timing.tobt != "" {
		t.Fatalf("expected authoritative CDM update for account flight without direct vIFF calls: %d %s local=%+v timing=%+v", rec.Code, rec.Body.String(), local, timing)
	}
}

func TestNetworkDepartureTobtDoesNotFallBackToRemoteWrites(t *testing.T) {
	source := networkTestSource(t, true, time.Now())
	for _, name := range []string{"missing CDM", "CDM disabled", "missing strip", "lookup failed", "different route", "missing session", "session failed", "training session", "different airport", "CDM failed"} {
		t.Run(name, func(t *testing.T) {
			timing, local := &networkTimingStub{}, &cdmStub{}
			finder := &finderStub{match: pdc.WebStripMatch{SessionID: 7, Strip: &models.Strip{Callsign: "SAS99X", Origin: "EKCH", Destination: "EKYT"}}}
			session := &models.Session{ID: 7, Name: "LIVE", Airport: "EKCH"}
			var sessionErr error
			cfg := WebAPIConfig{Auth: authStub{}, Network: source, Viff: timing, CDM: local, CDMReady: true, Flights: finder,
				Sessions: &testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*models.Session, error) { return session, sessionErr }},
			}
			wantStatus := http.StatusServiceUnavailable
			switch name {
			case "missing CDM":
				cfg.CDM = nil
			case "CDM disabled":
				cfg.CDMReady = false
			case "missing strip":
				finder.err = pdc.ErrWebStripNotFound
			case "lookup failed":
				finder.err = errors.New("database offline")
			case "different route":
				finder.match.Strip.Destination = "EDDF"
			case "missing session":
				session = nil
			case "session failed":
				sessionErr = errors.New("session database offline")
			case "training session":
				session.Name = "SWEATBOX"
			case "different airport":
				session.Airport = "EKYT"
			case "CDM failed":
				local.err = errors.New("CDM update failed")
				wantStatus = http.StatusConflict
			}
			api := NewWebAPI(cfg)
			req := httptest.NewRequest(http.MethodPut, "/efb/tobt", strings.NewReader(`{"tobt":"2000"}`))
			req.Header.Set("Authorization", "Bearer token")
			rec := httptest.NewRecorder()
			api.handleTobt(rec, req)
			if rec.Code != wantStatus || timing.callsign != "" || timing.tobt != "" || name != "CDM failed" && local.calls != 0 {
				t.Fatalf("invalid local updates must not reach vIFF: %d %s local=%+v remote=%+v", rec.Code, rec.Body.String(), local, timing)
			}
		})
	}
}

func TestNetworkArrivalTobtStillUsesOriginViffTiming(t *testing.T) {
	timing, local := &networkTimingStub{}, &cdmStub{}
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}, Network: networkTestFlight(t, true, time.Now(), "SAS99X", "EDDF", "EKCH"), Viff: timing, CDM: local, CDMReady: true})
	req := httptest.NewRequest(http.MethodPut, "/efb/tobt", strings.NewReader(`{"callsign":"OTHER","tobt":"2000"}`))
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	api.handleTobt(rec, req)
	if rec.Code != http.StatusOK || timing.callsign != "SAS99X" || timing.tobt != "2000" || timing.taxi != 12 || local.calls != 0 {
		t.Fatalf("arrival TOBT belongs to its origin, not EKCH CDM: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCallsignLookupDisablesEveryPDCAction(t *testing.T) {
	for _, state := range []string{"NONE", "CLEARED"} {
		t.Run(state, func(t *testing.T) {
			api := NewWebAPI(WebAPIConfig{Auth: authStub{}, CallsignLookup: true, PDCReady: true, Viff: &networkTimingStub{},
				Network: networkTestSource(t, true, time.Now()),
				Flights: &finderStub{match: pdc.WebStripMatch{SessionID: 7, Strip: &models.Strip{Origin: "EKCH", Destination: "EKYT", PdcState: state}}},
			})
			req := httptest.NewRequest(http.MethodGet, "/efb/flight?callsign=SAS99X", nil)
			req.Header.Set("Authorization", "Bearer token")
			rec := httptest.NewRecorder()
			api.handleFlight(rec, req)
			var got snapshot
			if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || !got.ReadOnly || got.Capabilities.PDC || got.PDCAvailable || got.PDCCanSubmit || got.PDCRequiresPilotAction {
				t.Fatalf("read-only flight must not expose PDC actions: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestNetworkTobtRejectsUnrelatedAirportBeforeAnyTimingAccess(t *testing.T) {
	timing, local := &networkTimingStub{}, &cdmStub{}
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}, Network: networkTestFlight(t, true, time.Now(), "SAS99X", "EDDF", "ESSA"), Viff: timing, CDM: local, CDMReady: true})
	req := httptest.NewRequest(http.MethodPut, "/efb/tobt", strings.NewReader(`{"tobt":"2000"}`))
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	api.handleTobt(rec, req)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "departing from or arriving at EKCH") || timing.callsign != "" || local.calls != 0 {
		t.Fatalf("out-of-scope flight must not access timing or perform updates: %d %s", rec.Code, rec.Body.String())
	}
}

func TestNetworkFlightRejectsUnavailableAndStaleFeed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		online bool
		at     time.Time
		status int
	}{
		{name: "VATSIM disabled", status: 503},
		{name: "stale data", online: true, at: time.Now().Add(-5 * time.Minute), status: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := WebAPIConfig{Auth: authStub{}}
			if !tc.at.IsZero() {
				cfg.Network = networkTestSource(t, tc.online, tc.at)
			}

			api := NewWebAPI(cfg)
			for _, method := range []string{http.MethodGet, http.MethodPut} {
				req := httptest.NewRequest(method, "/efb/flight", strings.NewReader(`{"tobt":"2000"}`))
				req.Header.Set("Authorization", "Bearer test")
				rec := httptest.NewRecorder()
				if method == http.MethodGet {
					api.handleFlight(rec, req)
				} else {
					api.handleTobt(rec, req)
				}
				if rec.Code != tc.status {
					t.Fatalf("expected %d, got %d %s", tc.status, rec.Code, rec.Body.String())
				}
			}
		})
	}
}

func TestCallsignLookupIncludesPrefilesAndNeverAllowsWrites(t *testing.T) {
	for _, online := range []bool{true, false} {
		t.Run(fmt.Sprintf("online=%t", online), func(t *testing.T) {
			timing := &networkTimingStub{}
			api := NewWebAPI(WebAPIConfig{
				Auth: authStub{}, CallsignLookup: true, Viff: timing,
				Network: networkTestFlight(t, online, time.Now(), "OTHER99", "EKCH", "EKYT"),
			})
			profile := httptest.NewRequest(http.MethodGet, "/efb/me", nil)
			profile.Header.Set("Authorization", "Bearer test")
			rec := httptest.NewRecorder()
			api.handleMe(rec, profile)
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"callsign_lookup":true`) || !strings.Contains(rec.Body.String(), `"live_mode":false`) {
				t.Fatalf("manual profile unavailable: %d %s", rec.Code, rec.Body.String())
			}
			req := httptest.NewRequest(http.MethodGet, "/efb/flight?callsign=%20other99%20", nil)
			req.Header.Set("Authorization", "Bearer test")
			rec = httptest.NewRecorder()
			api.handleFlight(rec, req)
			var got snapshot
			if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.Callsign != "OTHER99" || !got.ReadOnly || got.Capabilities != (capabilities{}) {
				t.Fatalf("expected read-only callsign result: %d %s", rec.Code, rec.Body.String())
			}
			if (got.FlightState == vatsim.FlightStateOnline) != online || got.TOBT == nil || *got.TOBT != "1935" {
				t.Fatalf("missing flight state or available vIFF data: %s", rec.Body.String())
			}
			if online && (got.Groundspeed == nil || *got.Groundspeed != 61) || !online && got.Groundspeed != nil {
				t.Fatalf("expected sensed speed only for online flights: %s", rec.Body.String())
			}
			for _, path := range []string{"/efb/tobt", "/efb/stand"} {
				update := httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{"callsign":"OTHER99","tobt":"2000","stand":"A12"}`))
				update.Header.Set("Authorization", "Bearer test")
				rec = httptest.NewRecorder()
				if path == "/efb/tobt" {
					api.handleTobt(rec, update)
				} else {
					api.handleStand(rec, update)
				}
				if rec.Code != http.StatusForbidden || timing.tobt != "" {
					t.Fatalf("manual lookup must reject writes: %d %s", rec.Code, rec.Body.String())
				}
			}
		})
	}
}

func TestCallsignLookupCannotOverrideProductionAccountBinding(t *testing.T) {
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}, Live: true, CallsignLookup: true, Network: networkTestSource(t, true, time.Now())})
	req := httptest.NewRequest(http.MethodGet, "/efb/flight?callsign=OTHER", nil)
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	api.handleFlight(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"callsign":"SAS99X"`) || api.callsignLookup {
		t.Fatalf("production must remain CID-bound: %d %s", rec.Code, rec.Body.String())
	}

}

func TestNetworkDepartureExposesPDCFromMatchingStrip(t *testing.T) {
	source := networkTestSource(t, true, time.Now())
	clearance := "CLEARED TO EKYT"
	for _, tc := range []struct {
		state                                  string
		ready, cleared, canSubmit, pilotAction bool
	}{
		{state: "NONE", ready: true, canSubmit: true},
		{state: "REQUESTED", ready: true},
		{state: "CLEARED", ready: true, pilotAction: true},
		{state: "CONFIRMED", ready: true, cleared: true},
		{state: "NONE"},
	} {
		t.Run(fmt.Sprintf("%s/ready=%t", tc.state, tc.ready), func(t *testing.T) {
			strip := &models.Strip{Origin: "EKCH", Destination: "EKYT", PdcState: tc.state, Cleared: tc.cleared,
				PdcData: &models.PdcData{Web: &models.PdcWebData{ClearanceText: &clearance}}}
			api := NewWebAPI(WebAPIConfig{Auth: authStub{}, Network: source, Viff: &networkTimingStub{}, PDCReady: tc.ready,
				Flights: &finderStub{match: pdc.WebStripMatch{SessionID: 7, Strip: strip}}})
			req := httptest.NewRequest(http.MethodGet, "/efb/flight", nil)
			req.Header.Set("Authorization", "Bearer token")
			rec := httptest.NewRecorder()
			api.handleFlight(rec, req)
			var got snapshot
			if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
			if got.PDCState != tc.state || got.PDCCanSubmit != tc.canSubmit || got.PDCRequiresPilotAction != tc.pilotAction || got.Capabilities.PDC != tc.ready || got.PDCClearanceText == nil || *got.PDCClearanceText != clearance {
				t.Fatalf("unexpected PDC snapshot: %s", rec.Body.String())
			}
		})
	}
}

func TestNetworkFlightRejectsUnrelatedAirport(t *testing.T) {
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}, Network: networkTestFlight(t, true, time.Now(), "SAS99X", "EDDF", "ESSA")})
	req := httptest.NewRequest(http.MethodGet, "/efb/flight", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	api.handleFlight(rec, req)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "departing from or arriving at EKCH") {
		t.Fatalf("expected explicit airport scope: %d %s", rec.Code, rec.Body.String())
	}
}
