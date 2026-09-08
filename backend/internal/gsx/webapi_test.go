package gsx

import (
	"FlightStrips/internal/models"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

type sessionFake struct {
	sessions []*models.Session
	err      error
}

func (f *sessionFake) GetByNameAndAirport(_ context.Context, name string, airport string) (*models.Session, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, session := range f.sessions {
		if strings.EqualFold(session.Name, name) && strings.EqualFold(session.Airport, airport) {
			return session, nil
		}
	}
	return nil, pgx.ErrNoRows
}

func (f *sessionFake) GetByNames(_ context.Context, name string) ([]*models.Session, error) {
	if f.err != nil {
		return nil, f.err
	}
	matches := make([]*models.Session, 0, len(f.sessions))
	for _, session := range f.sessions {
		if strings.EqualFold(session.Name, name) {
			matches = append(matches, session)
		}
	}
	return matches, nil
}

type stripFake struct {
	strips map[int32]map[string]*models.Strip
	err    error
}

func (f *stripFake) GetByCallsign(_ context.Context, session int32, callsign string) (*models.Strip, error) {
	if f.err != nil {
		return nil, f.err
	}
	if strip, ok := f.strips[session][callsign]; ok {
		return strip, nil
	}
	return nil, pgx.ErrNoRows
}

func ptr(value string) *string { return &value }

func newAPI(sessions *sessionFake, strips *stripFake) *WebAPI {
	return NewWebAPI(sessions, strips, nil, true)
}

func liveEKCH() *sessionFake {
	return &sessionFake{sessions: []*models.Session{{ID: 1, Name: "LIVE", Airport: "EKCH"}}}
}

func get(t *testing.T, api *WebAPI, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	return recorder
}

func decode(t *testing.T, recorder *httptest.ResponseRecorder) standResponse {
	t.Helper()
	var response standResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return response
}

func TestHandleStandReturnsAssignedStand(t *testing.T) {
	strips := &stripFake{strips: map[int32]map[string]*models.Strip{
		1: {"SAS1401": {Callsign: "SAS1401", Stand: ptr("A12")}},
	}}

	recorder := get(t, newAPI(liveEKCH(), strips), "/gsx/stand?callsign=SAS1401&icao=EKCH", nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	response := decode(t, recorder)
	if response.Stand == nil || *response.Stand != "A12" {
		t.Fatalf("expected stand A12, got %v", response.Stand)
	}
	if response.Revision != "A12" {
		t.Fatalf("expected revision A12, got %q", response.Revision)
	}
	if got := recorder.Header().Get("ETag"); got != `"A12"` {
		t.Fatalf("expected ETag \"A12\", got %q", got)
	}
}

func TestHandleStandLowercaseCallsignIsNormalized(t *testing.T) {
	strips := &stripFake{strips: map[int32]map[string]*models.Strip{
		1: {"SAS1401": {Callsign: "SAS1401", Stand: ptr("A12")}},
	}}

	recorder := get(t, newAPI(liveEKCH(), strips), "/gsx/stand?callsign=sas1401&icao=ekch", nil)

	response := decode(t, recorder)
	if response.Stand == nil || *response.Stand != "A12" {
		t.Fatalf("expected stand A12 for lowercase input, got %v", response.Stand)
	}
}

func TestHandleStandUnknownCallsignIsNotAnError(t *testing.T) {
	strips := &stripFake{strips: map[int32]map[string]*models.Strip{1: {}}}

	recorder := get(t, newAPI(liveEKCH(), strips), "/gsx/stand?callsign=NOPE123&icao=EKCH", nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 for an untracked callsign, got %d", recorder.Code)
	}
	response := decode(t, recorder)
	if response.Stand != nil {
		t.Fatalf("expected no stand, got %v", *response.Stand)
	}
	if response.Revision != noStandRevision {
		t.Fatalf("expected revision %q, got %q", noStandRevision, response.Revision)
	}
}

func TestHandleStandStripWithoutStandReturnsNull(t *testing.T) {
	strips := &stripFake{strips: map[int32]map[string]*models.Strip{
		1: {"SAS1401": {Callsign: "SAS1401", Stand: nil}},
	}}

	response := decode(t, get(t, newAPI(liveEKCH(), strips), "/gsx/stand?callsign=SAS1401&icao=EKCH", nil))
	if response.Stand != nil {
		t.Fatalf("expected no stand, got %v", *response.Stand)
	}
}

func TestHandleStandIgnoresNonLiveSessions(t *testing.T) {
	sessions := &sessionFake{sessions: []*models.Session{
		{ID: 7, Name: "Sweatbox", Airport: "EKCH"},
	}}
	strips := &stripFake{strips: map[int32]map[string]*models.Strip{
		7: {"SAS1401": {Callsign: "SAS1401", Stand: ptr("B4")}},
	}}

	response := decode(t, get(t, newAPI(sessions, strips), "/gsx/stand?callsign=SAS1401&icao=EKCH", nil))
	if response.Stand != nil {
		t.Fatalf("sweatbox stand must not reach a pilot, got %v", *response.Stand)
	}
}

func TestHandleStandIgnoresOtherAirports(t *testing.T) {
	sessions := &sessionFake{sessions: []*models.Session{
		{ID: 1, Name: "LIVE", Airport: "EKCH"},
		{ID: 2, Name: "LIVE", Airport: "EKBI"},
	}}
	strips := &stripFake{strips: map[int32]map[string]*models.Strip{
		2: {"SAS1401": {Callsign: "SAS1401", Stand: ptr("4")}},
	}}

	response := decode(t, get(t, newAPI(sessions, strips), "/gsx/stand?callsign=SAS1401&icao=EKCH", nil))
	if response.Stand != nil {
		t.Fatalf("expected no stand at EKCH, got %v", *response.Stand)
	}
}

func TestHandleStandMatchingETagReturns304(t *testing.T) {
	strips := &stripFake{strips: map[int32]map[string]*models.Strip{
		1: {"SAS1401": {Callsign: "SAS1401", Stand: ptr("A12")}},
	}}

	recorder := get(t, newAPI(liveEKCH(), strips), "/gsx/stand?callsign=SAS1401&icao=EKCH",
		map[string]string{"If-None-Match": `"A12"`})

	if recorder.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", recorder.Code)
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("expected empty body on 304, got %q", recorder.Body.String())
	}
}

func TestHandleStandChangedStandBreaksTheETag(t *testing.T) {
	strips := &stripFake{strips: map[int32]map[string]*models.Strip{
		1: {"SAS1401": {Callsign: "SAS1401", Stand: ptr("C34")}},
	}}

	recorder := get(t, newAPI(liveEKCH(), strips), "/gsx/stand?callsign=SAS1401&icao=EKCH",
		map[string]string{"If-None-Match": `"A12"`})

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200 after a reassignment, got %d", recorder.Code)
	}
	if response := decode(t, recorder); response.Revision != "C34" {
		t.Fatalf("expected revision C34, got %q", response.Revision)
	}
}

func TestHandleStandAcceptsWeakETag(t *testing.T) {
	strips := &stripFake{strips: map[int32]map[string]*models.Strip{
		1: {"SAS1401": {Callsign: "SAS1401", Stand: ptr("A12")}},
	}}

	recorder := get(t, newAPI(liveEKCH(), strips), "/gsx/stand?callsign=SAS1401&icao=EKCH",
		map[string]string{"If-None-Match": `W/"A12"`})

	if recorder.Code != http.StatusNotModified {
		t.Fatalf("expected 304 for a weak tag, got %d", recorder.Code)
	}
}

func TestHandleStandRejectsMissingAndMalformedInput(t *testing.T) {
	strips := &stripFake{strips: map[int32]map[string]*models.Strip{1: {}}}
	api := newAPI(liveEKCH(), strips)

	for _, target := range []string{
		"/gsx/stand",
		"/gsx/stand?callsign=",
		"/gsx/stand?callsign=SAS%201401",
		"/gsx/stand?callsign=SAS1401&icao=TOOLONG",
	} {
		if code := get(t, api, target, nil).Code; code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", target, code)
		}
	}
}

func TestHandleStandRejectsNonGET(t *testing.T) {
	mux := http.NewServeMux()
	newAPI(liveEKCH(), &stripFake{}).RegisterRoutes(mux)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/gsx/stand?callsign=SAS1401", nil))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", recorder.Code)
	}
}

func TestHandleStandRepositoryFailureIsRetryable(t *testing.T) {
	strips := &stripFake{err: pgx.ErrTxClosed}

	recorder := get(t, newAPI(liveEKCH(), strips), "/gsx/stand?callsign=SAS1401&icao=EKCH", nil)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 so the script retries, got %d", recorder.Code)
	}
}

func TestHandleStandDisclosesOnlyTheAssignment(t *testing.T) {
	strips := &stripFake{strips: map[int32]map[string]*models.Strip{
		1: {"SAS1401": {
			Callsign:    "SAS1401",
			Stand:       ptr("A12"),
			Origin:      "EKCH",
			Destination: "EGLL",
			Route:       ptr("SECRET ROUTE"),
			VatsimCID:   ptr("1234567"),
		}},
	}}

	recorder := get(t, newAPI(liveEKCH(), strips), "/gsx/stand?callsign=SAS1401&icao=EKCH", nil)

	var payload map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload) != 3 {
		t.Fatalf("expected exactly stand, pushback and revision, got %v", payload)
	}
	for _, key := range []string{"stand", "pushback", "revision"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("missing %q in %v", key, payload)
		}
	}
}

// --- scenery translation -------------------------------------------------

func sceneriesFixture() Sceneries {
	return Sceneries{"EKCH": &SceneryConfig{
		ICAO: "EKCH",
		Gates: map[string]map[string]GateScenery{
			"A31": {
				"Simnord-Sonnich": {Stand: "Gate A31", Points: map[string]string{"Z/L": "Z2 Face E"}},
				"FlyTampa":        {Points: map[string]string{"Z/L": "Z2 EAST"}},
			},
		},
	}}
}

func standAt(stand, releasePoint string) *stripFake {
	strip := &models.Strip{Callsign: "SAS1401", Stand: ptr(stand)}
	if releasePoint != "" {
		strip.ReleasePoint = ptr(releasePoint)
	}
	return &stripFake{strips: map[int32]map[string]*models.Strip{1: {"SAS1401": strip}}}
}

func withSceneries(strips *stripFake) *WebAPI {
	return NewWebAPI(liveEKCH(), strips, sceneriesFixture(), true)
}

func TestHandleStandTranslatesForTheScenery(t *testing.T) {
	api := withSceneries(standAt("A31", "Z/L"))

	recorder := get(t, api, "/gsx/stand?callsign=SAS1401&icao=EKCH&scenery=Simnord-Sonnich", nil)
	response := decode(t, recorder)

	if response.Stand == nil || *response.Stand != "Gate A31" {
		t.Fatalf("stand: got %v want Gate A31", response.Stand)
	}
	if response.Pushback == nil || *response.Pushback != "Z2 Face E" {
		t.Fatalf("pushback: got %v want Z2 Face E", response.Pushback)
	}
	if response.Revision != "Gate A31|Z2 Face E" {
		t.Fatalf("revision: got %q", response.Revision)
	}
}

func TestHandleStandSameGateDifferentScenery(t *testing.T) {
	api := withSceneries(standAt("A31", "Z/L"))

	response := decode(t, get(t, api, "/gsx/stand?callsign=SAS1401&icao=EKCH&scenery=FlyTampa", nil))
	if response.Pushback == nil || *response.Pushback != "Z2 EAST" {
		t.Fatalf("pushback: got %v want Z2 EAST", response.Pushback)
	}
	if response.Stand == nil || *response.Stand != "A31" {
		t.Fatalf("stand should fall back to the controller's value, got %v", response.Stand)
	}
}

func TestHandleStandWithoutSceneryPublishesNoPushback(t *testing.T) {
	api := withSceneries(standAt("A31", "Z/L"))

	response := decode(t, get(t, api, "/gsx/stand?callsign=SAS1401&icao=EKCH", nil))
	if response.Pushback != nil {
		t.Fatalf("expected no pushback without a scenery, got %v", *response.Pushback)
	}
	if response.Stand == nil || *response.Stand != "A31" {
		t.Fatalf("stand: got %v", response.Stand)
	}
}

func TestHandleStandNoReleasePointMeansNoPushback(t *testing.T) {
	api := withSceneries(standAt("A31", ""))

	response := decode(t, get(t, api, "/gsx/stand?callsign=SAS1401&icao=EKCH&scenery=Simnord-Sonnich", nil))
	if response.Pushback != nil {
		t.Fatalf("expected no pushback, got %v", *response.Pushback)
	}
	if response.Revision != "Gate A31" {
		t.Fatalf("revision: got %q", response.Revision)
	}
}

func TestHandleStandPushbackChangeBreaksTheETag(t *testing.T) {
	api := withSceneries(standAt("A31", "Z/L"))
	target := "/gsx/stand?callsign=SAS1401&icao=EKCH&scenery=Simnord-Sonnich"

	first := get(t, api, target, nil)
	etag := first.Header().Get("ETag")
	if code := get(t, api, target, map[string]string{"If-None-Match": etag}).Code; code != http.StatusNotModified {
		t.Fatalf("unchanged assignment should be 304, got %d", code)
	}

	// Controller clears the release point: same stand, different revision.
	moved := withSceneries(standAt("A31", ""))
	if code := get(t, moved, target, map[string]string{"If-None-Match": etag}).Code; code != http.StatusOK {
		t.Fatalf("a pushback change must break the ETag, got %d", code)
	}
}
