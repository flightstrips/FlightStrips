package pilot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"FlightStrips/internal/shared"
)

type authStub struct{}

func (authStub) Validate(string) (shared.AuthenticatedUser, error) {
	return shared.NewAuthenticatedUser("1234567", 0, nil), nil
}

type callsignLookupStub struct {
	callsign string
	found    bool
	err      error
}

func (s callsignLookupStub) GetCallsignByCID(context.Context, string) (string, bool, error) {
	return s.callsign, s.found, s.err
}

func TestHandleMeReturnsLockedCallsignWhenFound(t *testing.T) {
	api := NewWebAPI(authStub{}, callsignLookupStub{callsign: "SAS123", found: true}, nil, true)

	req := httptest.NewRequest(http.MethodGet, "/pilot/me", nil)
	req.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()

	api.handleMe(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "\"online_callsign\":\"SAS123\"") {
		t.Fatalf("expected online callsign in body, got %s", body)
	}
	if !strings.Contains(body, "\"callsign_locked\":true") {
		t.Fatalf("expected locked callsign flag in body, got %s", body)
	}
}

func TestHandleMeReturnsLiveModeTrue(t *testing.T) {
	api := NewWebAPI(authStub{}, callsignLookupStub{callsign: "SAS123", found: true}, nil, true)

	req := httptest.NewRequest(http.MethodGet, "/pilot/me", nil)
	req.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()

	api.handleMe(recorder, req)

	body := recorder.Body.String()
	if !strings.Contains(body, "\"live_mode\":true") {
		t.Fatalf("expected live_mode true in body, got %s", body)
	}
}

func TestHandleMeReturnsLiveModeFalseWhenVerificationDisabled(t *testing.T) {
	api := NewWebAPI(authStub{}, callsignLookupStub{}, nil, false)

	req := httptest.NewRequest(http.MethodGet, "/pilot/me", nil)
	req.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()

	api.handleMe(recorder, req)

	body := recorder.Body.String()
	if !strings.Contains(body, "\"live_mode\":false") {
		t.Fatalf("expected live_mode false in body, got %s", body)
	}
}

type flightLookupStub struct {
	info *FlightInfo
	err  error
}

func (s flightLookupStub) GetFlightInfo(_ context.Context, _ string) (*FlightInfo, error) {
	return s.info, s.err
}

func TestHandleFlightReturns503WhenLookupNilInLiveMode(t *testing.T) {
	api := NewWebAPI(authStub{}, nil, flightLookupStub{info: &FlightInfo{Callsign: "SAS123"}}, true)

	req := httptest.NewRequest(http.MethodGet, "/pilot/flight?callsign=SAS123", nil)
	req.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()

	api.handleFlight(recorder, req)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d when callsignLookup is nil in live mode, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

func TestHandleMeReturnsLiveModeWithNoOnlineCallsign(t *testing.T) {
	api := NewWebAPI(authStub{}, callsignLookupStub{found: false}, nil, true)

	req := httptest.NewRequest(http.MethodGet, "/pilot/me", nil)
	req.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()

	api.handleMe(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "\"live_mode\":true") {
		t.Fatalf("expected live_mode true in body, got %s", body)
	}
	if strings.Contains(body, "\"callsign_locked\":true") {
		t.Fatalf("expected callsign_locked false when no online callsign, got %s", body)
	}
}
