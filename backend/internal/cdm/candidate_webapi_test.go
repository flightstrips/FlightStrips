package cdm

import (
	"FlightStrips/internal/models"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCandidateSequenceHTTPUsesPersistedPolicyAndJSON(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	position, taxi := 2, 11
	input := CandidateSequenceInput{Session: &models.Session{ID: 7, Name: "LIVE", Airport: "EKCH"}, Config: NewDefaultAirportConfig("EKCH"), Strips: []*models.Strip{{Callsign: "SAS1", Origin: "EKCH", CdmData: &models.CdmData{Tobt: testStringPtr("1205"), Tsat: testStringPtr("120500"), Ttot: testStringPtr("121600"), Calculation: &models.CdmCalculation{SequencePosition: &position, TaxiMinutes: &taxi, BaseSource: testStringPtr("TOBT")}}}}}
	input.Session.ActiveRunways.DepartureRunways = []string{"22R"}
	input.Strips[0].EuroscopeSeenAt = &now
	fail := false
	api, err := NewCandidateWebAPI(cdmAuthStub{}, func(context.Context) ([]CandidateSequenceInput, error) {
		if fail {
			return nil, errors.New("projection unready")
		}
		return []CandidateSequenceInput{input}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	api.Now = func() time.Time { return now }
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/cdm/sequence", nil)
	unauthorized := httptest.NewRecorder()
	mux.ServeHTTP(unauthorized, req)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatal(unauthorized.Code)
	}
	req.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, req)
	var result sequenceResponse
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" || json.Unmarshal(response.Body.Bytes(), &result) != nil {
		t.Fatal(response.Body.String())
	}
	row := result.Sessions[0].Rows[0]
	if row.Callsign != "SAS1" || row.Ttot != "1216" || row.Position == nil || *row.Position != position || row.TaxiMinutes == nil || *row.TaxiMinutes != taxi {
		t.Fatalf("persisted presentation changed: %+v", row)
	}
	fail = true
	unavailable := httptest.NewRecorder()
	mux.ServeHTTP(unavailable, req)
	if unavailable.Code != http.StatusServiceUnavailable {
		t.Fatal(unavailable.Code)
	}
}
