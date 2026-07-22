package efb

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type authStub struct{ err error }

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

type cdmStub struct{ calls int }

func (s *cdmStub) HandleTobtUpdate(context.Context, int32, string, string, string, string) error {
	s.calls++
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
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}, Callsigns: callsignStub{callsign: "SAS123", found: true}, Flights: finder, Sessions: sessions, Live: true})
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

func TestLiveFlightReturnsNotFoundWhenPilotOffline(t *testing.T) {
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}, Callsigns: callsignStub{}, Flights: &finderStub{}, Sessions: &testutil.MockSessionRepository{}, Live: true})
	req := httptest.NewRequest(http.MethodGet, "/efb/flight", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	api.handleFlight(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
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

func TestSnapshotDisablesTOBTWhenCDMIsNotReady(t *testing.T) {
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}, CDM: &cdmStub{}, CDMReady: false})
	result := api.buildSnapshot(context.Background(), pdc.WebStripMatch{Strip: &models.Strip{
		Callsign: "SAS123", Origin: "EKCH", Destination: "ESSA",
	}}, &models.Session{Airport: "EKCH"})

	if result.Capabilities.TOBT {
		t.Fatal("expected TOBT updates to be disabled while CDM is not ready")
	}
}

func TestTobtReturnsUnavailableWhenCDMIsNotReady(t *testing.T) {
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
