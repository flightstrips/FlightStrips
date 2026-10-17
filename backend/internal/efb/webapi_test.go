package efb

import (
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/cdm"
	"FlightStrips/internal/metar"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/services"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type authStub struct{ err error }

type assignmentLookupStub struct {
	repository.StandAssignmentRepository
	assignment *models.StandAssignment
	err        error
}

func TestDepartureDoesNotSubstituteSensedStandForPilotReservation(t *testing.T) {
	for _, sensedStand := range []string{"A12", ""} {
		t.Run("stand="+sensedStand, func(t *testing.T) {
			tobt, tsat, ctot := "2010", "201500", "2030"
			api := NewWebAPI(WebAPIConfig{
				Network: networkTestFlight(t, true, time.Now(), "SAS123", "EKCH", "EDDF"),
				Auth:    authStub{}, CDMReady: true, ATIS: &atisLookupStub{},
				Assignments: assignmentLookupStub{assignment: &models.StandAssignment{Stand: "B4", Stage: "RESERVED", Version: 8}},
				Flights: &finderStub{match: pdc.WebStripMatch{SessionID: 7, Strip: &models.Strip{
					Callsign: "SAS123", Origin: "EKCH", Destination: "EDDF", Stand: &sensedStand,
					CdmData: &models.CdmData{Tobt: &tobt, Tsat: &tsat, Ctot: &ctot},
				}}},
				Sessions: &testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*models.Session, error) {
					return &models.Session{ID: 7, Airport: "EKCH"}, nil
				}},
				Viff: &networkTimingStub{},
			})
			req := httptest.NewRequest(http.MethodGet, "/efb/flight?callsign=SAS123", nil)
			req.Header.Set("Authorization", "Bearer test")
			rec := httptest.NewRecorder()
			api.handleFlight(rec, req)
			var flight snapshot
			if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &flight) != nil {
				t.Fatalf("departure lookup failed: %d %s", rec.Code, rec.Body.String())
			}
			if flight.Stand != nil || flight.StandState == nil || *flight.StandState != standAssignmentUnavailable {
				t.Fatalf("must not substitute a sensed/planned stand when the pilot allocator is unavailable: %s", rec.Body.String())
			}
			if flight.TOBT == nil || *flight.TOBT != "1935" || flight.TSAT == nil || *flight.TSAT != "1940" || flight.CTOT == nil || *flight.CTOT != "1950" || !flight.Capabilities.TOBT {
				t.Fatalf("departure must use vIFF, not local CDM timings: %s", rec.Body.String())
			}
			if flight.ATIS == nil || flight.ATIS.Callsign != "EKCH_D_ATIS" || flight.StandVersion != nil {
				t.Fatalf("incorrect departure data: %s", rec.Body.String())
			}
		})
	}
}

func (s assignmentLookupStub) GetAssignment(context.Context, int32, string) (*models.StandAssignment, error) {
	return s.assignment, s.err
}

func TestArrivalStandUsesFlightStripsAssignmentAndState(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	for _, tc := range []struct {
		name       string
		assignment *models.StandAssignment
		err        error
		status     int
		stand      string
		stage      string
	}{
		{name: "estimated", assignment: &models.StandAssignment{Stand: "B4", Stage: "ESTIMATED", Version: 3}, status: 200, stand: "B4", stage: "ESTIMATED"},
		{name: "assigned", assignment: &models.StandAssignment{Stand: "C27", Stage: "ASSIGNED", Version: 4}, status: 200, stand: "C27", stage: "ASSIGNED"},
		{name: "confirmed", assignment: &models.StandAssignment{Stand: "D1", Stage: "CONFIRMED", Version: 5}, status: 200, stand: "D1", stage: "CONFIRMED"},
		{name: "no assignment ignores old strip stand", err: pgx.ErrNoRows, status: 200},
		{name: "expired", assignment: &models.StandAssignment{Stand: "B4", Stage: "ASSIGNED", ExpiresAt: &past}, status: 200},
		{name: "released", assignment: &models.StandAssignment{Stand: "B4", Stage: "RELEASED"}, status: 200},
		{name: "lookup error", err: errors.New("database unavailable"), status: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stripStand := "A12"
			api := NewWebAPI(WebAPIConfig{
				Network:     networkTestFlight(t, true, time.Now(), "SAS123", "EDDF", "EKCH"),
				Auth:        authStub{},
				Assignments: assignmentLookupStub{assignment: tc.assignment, err: tc.err},
				Flights: &finderStub{match: pdc.WebStripMatch{SessionID: 7, Strip: &models.Strip{
					Callsign: "SAS123", Origin: "EDDF", Destination: "EKCH", Stand: &stripStand,
				}}},
				Sessions: &testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*models.Session, error) {
					return &models.Session{ID: 7, Airport: "EKCH"}, nil
				}},
			})
			req := httptest.NewRequest(http.MethodGet, "/efb/flight?callsign=SAS123", nil)
			req.Header.Set("Authorization", "Bearer test")
			rec := httptest.NewRecorder()
			api.handleFlight(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("unexpected response: %d %s", rec.Code, rec.Body.String())
			}
			if tc.status != 200 {
				return
			}
			var flight snapshot
			if err := json.Unmarshal(rec.Body.Bytes(), &flight); err != nil {
				t.Fatal(err)
			}
			if tc.stand == "" {
				if flight.Stand != nil || flight.StandState != nil {
					t.Fatalf("expected unallocated stand, got %s", rec.Body.String())
				}
				if tc.err != nil && !errors.Is(tc.err, pgx.ErrNoRows) && len(flight.DataErrors) == 0 {
					t.Fatal("stand lookup errors must be visible without blocking the flight")
				}
			} else if flight.Stand == nil || *flight.Stand != tc.stand || flight.StandState == nil || *flight.StandState != tc.stage || flight.StandVersion == nil || *flight.StandVersion != tc.assignment.Version {
				t.Fatalf("wrong stand assignment: %s", rec.Body.String())
			}
		})
	}
}

type atisLookupStub struct {
	airport   string
	departure bool
}

func (s *atisLookupStub) GetATIS(airport string, departure bool) *metar.ATIS {
	s.airport, s.departure = airport, departure
	if departure {
		return &metar.ATIS{Callsign: "EKCH_D_ATIS", Code: "D", Text: []string{"EKCH DEP ATIS D", "RWY IN USE 22R"}}
	}
	return &metar.ATIS{Callsign: "EKCH_A_ATIS", Code: "C", Text: []string{"EKCH ARR ATIS C", "RWY IN USE 22L"}}
}

func TestFlightSelectsEKCHArrivalAndDepartureATIS(t *testing.T) {
	for _, departure := range []bool{false, true} {
		t.Run(fmt.Sprintf("departure=%t", departure), func(t *testing.T) {
			origin, destination, station := "EDDF", "EKCH", "EKCH_A_ATIS"
			if departure {
				origin, destination, station = "EKCH", "EDDF", "EKCH_D_ATIS"
			}
			atis := &atisLookupStub{}
			api := NewWebAPI(WebAPIConfig{
				Network: networkTestFlight(t, true, time.Now(), "SAS123", origin, destination),
				Auth:    authStub{}, ATIS: atis,
				Flights: &finderStub{match: pdc.WebStripMatch{SessionID: 7, Strip: &models.Strip{
					Callsign: "SAS123", Origin: origin, Destination: destination,
				}}},
				Sessions: &testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*models.Session, error) {
					return &models.Session{ID: 7, Airport: "EKCH"}, nil
				}},
			})
			req := httptest.NewRequest(http.MethodGet, "/efb/flight?callsign=SAS123", nil)
			req.Header.Set("Authorization", "Bearer test")
			rec := httptest.NewRecorder()
			api.handleFlight(rec, req)
			var flight snapshot
			if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &flight) != nil {
				t.Fatalf("unexpected response: %d %s", rec.Code, rec.Body.String())
			}
			if atis.airport != "EKCH" || atis.departure != departure || flight.ATIS == nil || flight.ATIS.Callsign != station {
				t.Fatalf("wrong ATIS routing: airport=%s departure=%t flight=%+v", atis.airport, atis.departure, flight.ATIS)
			}
		})
	}
}

type snapshotReaderStub struct {
	snapshot navdata.ActiveGeometrySnapshot
}

func (s snapshotReaderStub) ActiveGeometrySnapshot(context.Context, navdata.AirportID) (navdata.ActiveGeometrySnapshot, error) {
	return s.snapshot, nil
}

func (s authStub) Validate(string) (shared.AuthenticatedUser, error) {
	return shared.NewAuthenticatedUser("1234567", 0, nil), s.err
}

type callsignStub struct {
	callsign string
	found    bool
}

func (s callsignStub) GetCallsignByCID(context.Context, string) (string, bool, error) {
	return s.callsign, s.found, nil
}

type finderStub struct {
	match     pdc.WebStripMatch
	err       error
	requested string
}

type departureFrequencyStub struct{ frequency string }

type cdmStub struct {
	calls     int
	session   int32
	callsign  string
	tobt      string
	updatedBy string
	source    string
}

type standActionsStub struct {
	availability []services.StandAvailability
	err          error
	session      int32
	airport      string
	callsign     string
}

func (s *standActionsStub) AssignForPilot(context.Context, int32, string, string, string, string, int32, bool) (*services.StandAllocationResult, error) {
	return nil, s.err
}

func (s *standActionsStub) AvailableForPilot(_ context.Context, session int32, airport, callsign string) ([]services.StandAvailability, error) {
	s.session, s.airport, s.callsign = session, airport, callsign
	return s.availability, s.err
}

func (s *cdmStub) HandleTobtUpdate(_ context.Context, session int32, callsign, tobt, updatedBy, source string) error {
	s.calls++
	s.session, s.callsign, s.tobt, s.updatedBy, s.source = session, callsign, tobt, updatedBy, source
	return nil
}

func (s departureFrequencyStub) ComputeDepartureFrequencyForStripContext(context.Context, *models.Strip, int32) (*string, error) {
	return &s.frequency, nil
}

func (s *finderStub) FindWebStripByCallsign(_ context.Context, callsign string) (pdc.WebStripMatch, error) {
	s.requested = callsign
	return s.match, s.err
}

func TestFlightRequiresBearerToken(t *testing.T) {
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}, Sessions: &testutil.MockSessionRepository{}, Live: true})
	rec := httptest.NewRecorder()
	api.handleFlight(rec, httptest.NewRequest(http.MethodGet, "/efb/flight", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestLiveFlightUsesAuthenticatedCIDCallsign(t *testing.T) {
	finder := &finderStub{match: pdc.WebStripMatch{SessionID: 7, Strip: &models.Strip{Callsign: "SAS123", Origin: "EKCH", Destination: "ESSA"}}}
	sessions := &testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*models.Session, error) {
		return &models.Session{ID: 7, Airport: "EKCH"}, nil
	}}
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}, Network: networkTestFlight(t, true, time.Now(), "SAS123", "EKCH", "ESSA"), Callsigns: callsignStub{callsign: "SAS123", found: true}, Flights: finder, Sessions: sessions, Live: true})
	req := httptest.NewRequest(http.MethodGet, "/efb/flight?callsign=OTHER", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	api.handleFlight(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if finder.requested != "SAS123" {
		t.Fatalf("expected authenticated callsign, got %q", finder.requested)
	}
	if !strings.Contains(rec.Body.String(), `"phase":"DEPARTURE"`) {
		t.Fatalf("missing departure snapshot: %s", rec.Body.String())
	}
}

func TestStandAvailabilityUsesAuthenticatedFlightAndReturnsTimeAwareEntries(t *testing.T) {
	stands := &standActionsStub{availability: []services.StandAvailability{
		{Stand: "A12", Available: true},
		{Stand: "A14", Reason: "reserved by SAS999"},
	}}
	finder := &finderStub{match: pdc.WebStripMatch{SessionID: 7, Strip: &models.Strip{Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH"}}}
	sessions := &testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*models.Session, error) {
		return &models.Session{ID: 7, Airport: "EKCH"}, nil
	}}
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}, Callsigns: callsignStub{callsign: "SAS123", found: true}, Flights: finder, Sessions: sessions, Stands: stands, Live: true})
	req := httptest.NewRequest(http.MethodGet, "/efb/stands?callsign=OTHER", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()

	api.handleStandAvailability(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if stands.session != 7 || stands.airport != "EKCH" || stands.callsign != "SAS123" {
		t.Fatalf("unexpected availability request: session=%d airport=%s callsign=%s", stands.session, stands.airport, stands.callsign)
	}
	var payload struct {
		Stands []services.StandAvailability `json:"stands"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Stands) != 2 || !payload.Stands[0].Available || payload.Stands[1].Reason != "reserved by SAS999" {
		t.Fatalf("unexpected availability response: %#v", payload.Stands)
	}
}

func TestLiveFlightAllowsOwnedPrefile(t *testing.T) {
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}, Network: networkTestSource(t, false, time.Now()), Callsigns: callsignStub{}, Flights: &finderStub{}, Sessions: &testutil.MockSessionRepository{}, Live: true})
	req := httptest.NewRequest(http.MethodGet, "/efb/flight", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	api.handleFlight(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"flight_state":"prefile"`) {
		t.Fatalf("expected owned prefile, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestInvalidTokenIsRejected(t *testing.T) {
	api := NewWebAPI(WebAPIConfig{Auth: authStub{err: errors.New("bad token")}, Sessions: &testutil.MockSessionRepository{}, Live: true})
	req := httptest.NewRequest(http.MethodGet, "/efb/me", nil)
	req.Header.Set("Authorization", "Bearer bad")
	rec := httptest.NewRecorder()
	api.handleMe(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestSnapshotUsesObservedStandAndNormalizesTSAT(t *testing.T) {
	stand := "A12"
	tsat := "124500"
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}})

	result := api.buildSnapshot(context.Background(), pdc.WebStripMatch{
		SessionID: 7,
		Strip: &models.Strip{
			Callsign: "SAS123", Origin: "EKCH", Destination: "ESSA", Stand: &stand,
			CdmData: &models.CdmData{Tsat: &tsat},
		},
	}, &models.Session{ID: 7, Airport: "EKCH"})

	if result.Stand == nil || *result.Stand != "A12" {
		t.Fatalf("expected observed stand A12 with SAT disabled, got %v", result.Stand)
	}
	if result.Capabilities.Stand {
		t.Fatal("expected stand reassignment to remain disabled")
	}
	if result.TSAT == nil || *result.TSAT != "1245" {
		t.Fatalf("expected four-digit TSAT, got %v", result.TSAT)
	}
}

func TestSnapshotOnlyIncludesComputedDepartureFrequency(t *testing.T) {
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}, Departures: departureFrequencyStub{frequency: "124.980"}})
	result := api.buildSnapshot(context.Background(), pdc.WebStripMatch{Strip: &models.Strip{
		Callsign: "SAS123", Origin: "EKCH", Destination: "ESSA",
	}}, &models.Session{Airport: "EKCH"})

	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"departure_frequency":"124.980"`) {
		t.Fatalf("missing computed departure frequency: %s", payload)
	}
}

func TestSnapshotIncludesSynchronizedArrivalSTAR(t *testing.T) {
	star := "LUXAL2A"
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}})
	result := api.buildSnapshot(context.Background(), pdc.WebStripMatch{Strip: &models.Strip{
		Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", Star: &star,
	}}, &models.Session{Airport: "EKCH"})

	if result.STAR == nil || *result.STAR != star {
		t.Fatalf("expected synchronized STAR %q, got %v", star, result.STAR)
	}
}

func TestSnapshotIncludesArrivalEAT(t *testing.T) {
	eta := time.Date(2026, time.July, 24, 14, 35, 0, 0, time.UTC)
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}})
	result := api.buildSnapshot(context.Background(), pdc.WebStripMatch{Strip: &models.Strip{
		Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", ArrivalETA: &models.ArrivalETA{Time: eta},
	}}, &models.Session{Airport: "EKCH"})

	if result.ArrivalETA == nil || *result.ArrivalETA != "1435" {
		t.Fatalf("expected arrival EAT 1435, got %v", result.ArrivalETA)
	}
}

func TestSnapshotUsesLastFixOfSTARPathAsTerminalFix(t *testing.T) {
	star, runway := "TUDLO4C", "22L"
	controllerHeading := int32(270)
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}, Terminal: terminal.Configuration{
		Airport:      "EKCH",
		RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-22", Runways: []navdata.RunwayID{"22L", "22R"}}},
		Feeders:      []terminal.Feeder{{ID: "TUDLO"}},
		Paths:        []terminal.Path{{Feeder: "TUDLO", RunwayGroup: "ARRIVAL-22", Fixes: []navdata.FixID{"TUDLO", "LUGAS", "ABEGI"}, SelectedHolding: "EKCH-LUGAS-PRIMARY-LOW"}},
	}, Navigation: snapshotReaderStub{snapshot: navdata.ActiveGeometrySnapshot{
		TerminalPaths: []navdata.TerminalPath{{Feeder: "TUDLO", RunwayGroup: "ARRIVAL-22", HoldingIDs: []navdata.HoldingID{"EKCH-LUGAS-PRIMARY-LOW"}, PublishedHeadingMagneticDeg: intPtr(37)}},
		Holdings:      []navdata.HoldingPattern{{ID: "EKCH-LUGAS-PRIMARY-LOW", Fix: "LUGAS", InboundCourseTrueDeg: 77, TurnDirection: navdata.TurnLeft}},
	}}})
	result := api.buildSnapshot(context.Background(), pdc.WebStripMatch{Strip: &models.Strip{
		Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", Star: &star, Runway: &runway, Heading: &controllerHeading,
	}}, &models.Session{Airport: "EKCH"})

	if result.TerminalFix == nil || *result.TerminalFix != "ABEGI" {
		t.Fatalf("expected terminal fix ABEGI, got %v", result.TerminalFix)
	}
	if result.PublishedHeading == nil || *result.PublishedHeading != 37 {
		t.Fatalf("expected published STAR heading 37, got %v", result.PublishedHeading)
	}
	if result.PublishedHoldingFix == nil || *result.PublishedHoldingFix != "LUGAS" {
		t.Fatalf("expected published holding fix LUGAS, got %v", result.PublishedHoldingFix)
	}
	if result.PublishedHoldingDetail == nil || *result.PublishedHoldingDetail != "077/LEFT" {
		t.Fatalf("expected published holding detail 077/LEFT, got %v", result.PublishedHoldingDetail)
	}
}

func intPtr(value int) *int { return &value }

func TestSnapshotDisablesTOBTWhenCDMIsNotReady(t *testing.T) {
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}, CDM: &cdmStub{}, CDMReady: false})
	result := api.buildSnapshot(context.Background(), pdc.WebStripMatch{Strip: &models.Strip{
		Callsign: "SAS123", Origin: "EKCH", Destination: "ESSA",
	}}, &models.Session{Airport: "EKCH"})

	if result.Capabilities.TOBT {
		t.Fatal("expected TOBT updates to be disabled while CDM is not ready")
	}
}

func TestTobtReturnsUnavailableWithoutVATSIMSource(t *testing.T) {
	cdm := &cdmStub{}
	finder := &finderStub{match: pdc.WebStripMatch{SessionID: 7, Strip: &models.Strip{
		Callsign: "SAS123", Origin: "EKCH", Destination: "ESSA",
	}}}
	sessions := &testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*models.Session, error) {
		return &models.Session{ID: 7, Airport: "EKCH"}, nil
	}}
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}, Flights: finder, Sessions: sessions, CDM: cdm, CDMReady: false})
	req := httptest.NewRequest(http.MethodPut, "/efb/tobt", strings.NewReader(`{"tobt":"1230","callsign":"SAS123"}`))
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()

	api.handleTobt(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}

	if cdm.calls != 0 {
		t.Fatalf("expected CDM update not to run, got %d calls", cdm.calls)
	}
}

func TestDepartureTobtDoesNotUseCDM(t *testing.T) {
	updater := &cdmStub{}
	finder := &finderStub{match: pdc.WebStripMatch{SessionID: 7, Strip: &models.Strip{
		Callsign: "SAS123", Origin: "EKCH", Destination: "EDDF",
	}}}
	api := NewWebAPI(WebAPIConfig{
		Network: networkTestFlight(t, true, time.Now(), "SAS123", "EKCH", "EDDF"),
		Auth:    authStub{}, Live: true, Callsigns: callsignStub{callsign: "SAS123", found: true},
		Flights: finder, Sessions: &testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*models.Session, error) {
			return &models.Session{ID: 7, Airport: "EKCH"}, nil
		}},
		CDM: updater, CDMReady: true, Viff: &networkTimingStub{},
	})
	req := httptest.NewRequest(http.MethodPut, "/efb/tobt", strings.NewReader(`{"callsign":"OTHER","tobt":"2040"}`))
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	api.handleTobt(rec, req)
	if rec.Code != http.StatusOK || finder.requested != "" {
		t.Fatalf("departure TOBT must not require FlightStrips: %d %s", rec.Code, rec.Body.String())
	}
	if updater.calls != 0 {
		t.Fatalf("departure TOBT must use vIFF, not CDM: %+v", updater)
	}
}

func TestArrivalViffTimingUsesCallsignWithoutAirportFilter(t *testing.T) {
	var updated bool
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("callsign") != "SAS123" {
			t.Errorf("unexpected callsign: %s", r.URL.RawQuery)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Error("missing server-side API key")
		}
		switch r.URL.Path {
		case "/ifps/callsign":
			if r.Method != http.MethodGet || r.URL.Query().Has("airport") {
				t.Error("lookup must be callsign-only")
			}
			_, _ = w.Write([]byte(`{"callsign":"SAS123","departure":"EDDF","arrival":"EKCH","taxi":12,"tobt":"1935","ctot":"1950","cdmData":{"tobt":"1935","tsat":"194000"}}`))
		case "/ifps/dpi":
			if r.Method != http.MethodPost || r.URL.Query().Get("value") != "OBT/2000/12" {
				t.Errorf("unexpected update: %s %s", r.Method, r.URL.RawQuery)
			}
			updated = true
			_, _ = w.Write([]byte("true"))
		default:
			t.Errorf("unexpected vIFF request: %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer remote.Close()
	finder := &finderStub{match: pdc.WebStripMatch{SessionID: 7, Strip: &models.Strip{
		Callsign: "SAS123", Origin: "EDDF", Destination: "EKCH",
	}}}
	api := NewWebAPI(WebAPIConfig{
		Network: networkTestFlight(t, true, time.Now(), "SAS123", "EDDF", "EKCH"),
		Auth:    authStub{}, Live: true, Callsigns: callsignStub{callsign: "SAS123", found: true},
		Flights: finder, Sessions: &testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*models.Session, error) {
			return &models.Session{ID: 7, Airport: "EKCH"}, nil
		}},
		Viff: cdm.NewClient(cdm.WithBaseURL(remote.URL), cdm.WithAPIKey("test-key")),
	})
	read := httptest.NewRequest(http.MethodGet, "/efb/flight?callsign=OTHER", nil)
	read.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	api.handleFlight(rec, read)
	var got snapshot
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("lookup failed: %d %s", rec.Code, rec.Body.String())
	}
	if got.TOBT == nil || *got.TOBT != "1935" || got.TSAT == nil || *got.TSAT != "1940" || got.CTOT == nil || *got.CTOT != "1950" || !got.Capabilities.TOBT {
		t.Fatalf("unexpected timing snapshot: %s", rec.Body.String())
	}
	update := httptest.NewRequest(http.MethodPut, "/efb/tobt", strings.NewReader(`{"callsign":"OTHER","tobt":"2000"}`))
	update.Header.Set("Authorization", "Bearer test")
	rec = httptest.NewRecorder()
	api.handleTobt(rec, update)
	if rec.Code != http.StatusOK || !updated {
		t.Fatalf("update failed: %d %s", rec.Code, rec.Body.String())
	}
}

func TestArrivalViffFailuresAreNotReportedAsSuccess(t *testing.T) {
	for _, response := range []string{"false", `{"callsign":"OTHER"}`} {
		t.Run(response, func(t *testing.T) {
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/ifps/callsign" && response == "false" {
					_, _ = w.Write([]byte(`{"callsign":"SAS123","taxi":12}`))
					return
				}
				_, _ = w.Write([]byte(response))
			}))
			defer remote.Close()
			api := NewWebAPI(WebAPIConfig{Auth: authStub{},
				Network: networkTestFlight(t, true, time.Now(), "SAS123", "EDDF", "EKCH"),
				Flights: &finderStub{match: pdc.WebStripMatch{SessionID: 7, Strip: &models.Strip{Callsign: "SAS123", Origin: "EDDF", Destination: "EKCH"}}},
				Sessions: &testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*models.Session, error) {
					return &models.Session{ID: 7, Airport: "EKCH"}, nil
				}},
				Viff: cdm.NewClient(cdm.WithBaseURL(remote.URL)),
			})
			req := httptest.NewRequest(http.MethodPut, "/efb/tobt", strings.NewReader(`{"callsign":"SAS123","tobt":"2000"}`))
			req.Header.Set("Authorization", "Bearer test")
			rec := httptest.NewRecorder()
			api.handleTobt(rec, req)
			if rec.Code != http.StatusBadGateway {
				t.Fatalf("expected explicit failure, got %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}
