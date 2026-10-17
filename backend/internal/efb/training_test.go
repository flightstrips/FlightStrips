package efb

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/services"
	"FlightStrips/internal/testutil"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func trainingConfig(t *testing.T) WebAPIConfig {
	t.Helper()
	tobt, tsat, sid, runway := "2100", "210500", "GOLGA1A", "22R"
	return WebAPIConfig{
		Auth: authStub{}, Training: true, CallsignLookup: true, CDMReady: true,
		CDM: &cdmStub{}, ATIS: &atisLookupStub{},
		Sessions: &testutil.MockSessionRepository{
			ListFn: func(context.Context) ([]*models.Session, error) {
				return []*models.Session{{ID: 7, Name: "LIVE", Airport: "EKCH"}, {ID: 8, Name: "Sweatbox", Airport: "EKCH"}}, nil
			},
			GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
				name := "LIVE"
				if id == 8 {
					name = "Sweatbox"
				}
				return &models.Session{ID: id, Name: name, Airport: "EKCH"}, nil
			},
		},
		TrainingStrips: &testutil.MockStripRepository{GetByCallsignFn: func(_ context.Context, id int32, callsign string) (*models.Strip, error) {
			if id != 8 || callsign != "SAS123" {
				t.Fatalf("training strip lookup escaped selected session: %d %s", id, callsign)
			}
			return &models.Strip{Callsign: callsign, Origin: "EKCH", Destination: "EKYT", Sid: &sid, Runway: &runway, CdmData: &models.CdmData{Tobt: &tobt, Tsat: &tsat}}, nil
		}},
	}
}

func trainingRequest(api *WebAPI, method, path, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestSweatboxReadsStripsWithoutVATSIMOrViff(t *testing.T) {
	api := NewWebAPI(trainingConfig(t))
	rec := trainingRequest(api, http.MethodGet, "/efb/flight?training_session=8&callsign=sas123", "")
	var got snapshot
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("training lookup failed: %d %s", rec.Code, rec.Body.String())
	}
	if got.Callsign != "SAS123" || got.TOBT == nil || *got.TOBT != "2100" || got.TSAT == nil || *got.TSAT != "2105" || got.InitialClimb == nil || *got.InitialClimb != "FL70" || got.DepartureFrequency == nil || *got.DepartureFrequency != "120.255" || got.ReadOnly || !got.Capabilities.TOBT || got.Capabilities.PDC || got.PDCAvailable {
		t.Fatalf("incorrect training snapshot: %s", rec.Body.String())
	}
}

func TestSweatboxListsOnlySweatboxSessions(t *testing.T) {
	rec := trainingRequest(NewWebAPI(trainingConfig(t)), http.MethodGet, "/efb/training-sessions", "")
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "LIVE") || !strings.Contains(rec.Body.String(), `"id":8`) {
		t.Fatalf("wrong training session list: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSweatboxTobtUpdatesSelectedSessionOnly(t *testing.T) {
	cfg := trainingConfig(t)
	cdm := cfg.CDM.(*cdmStub)
	rec := trainingRequest(NewWebAPI(cfg), http.MethodPut, "/efb/tobt?training_session=8", `{"callsign":"SAS123","tobt":"2130"}`)
	if rec.Code != 200 || cdm.calls != 1 || cdm.session != 8 || cdm.callsign != "SAS123" || cdm.tobt != "2130" {
		t.Fatalf("training TOBT did not reach local CDM: %d %s %+v", rec.Code, rec.Body.String(), cdm)
	}
}

type trainingStandStub struct {
	standActionsStub
	session         int32
	version         int32
	allowUnsuitable bool
}

func (s *trainingStandStub) AssignForPilot(_ context.Context, session int32, airport, cid, callsign, stand string, version int32, allowUnsuitable bool) (*services.StandAllocationResult, error) {
	s.session, s.version = session, version
	s.allowUnsuitable = allowUnsuitable
	return &services.StandAllocationResult{Assignment: models.StandAssignment{Callsign: callsign, Stand: stand, Stage: "ASSIGNED", Version: version + 1}}, nil
}

func TestSweatboxStandWritesAndAvailabilityAreSessionScoped(t *testing.T) {
	cfg := trainingConfig(t)
	stands := &trainingStandStub{}
	cfg.Stands = stands
	api := NewWebAPI(cfg)
	rec := trainingRequest(api, http.MethodPut, "/efb/stand?training_session=8", `{"callsign":"SAS123","stand":"B4","version":3}`)
	if rec.Code != 200 || stands.session != 8 || stands.version != 3 || !strings.Contains(rec.Body.String(), `"stand_state":"ASSIGNED"`) {
		t.Fatalf("training stand update failed: %d %s", rec.Code, rec.Body.String())
	}
	rec = trainingRequest(api, http.MethodGet, "/efb/stands?training_session=8&callsign=SAS123", "")
	if rec.Code != 200 || stands.standActionsStub.session != 8 {
		t.Fatalf("training stand availability failed: %d %s", rec.Code, rec.Body.String())
	}
	rec = trainingRequest(api, http.MethodPut, "/efb/stand?training_session=8", `{"callsign":"SAS123","stand":"B4","version":4,"allow_unsuitable":true}`)
	if rec.Code != 200 || !stands.allowUnsuitable {
		t.Fatalf("confirmed suitability exception not forwarded: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSweatboxRejectsLiveSessionsInvalidSelectionAndDisabledMode(t *testing.T) {
	for _, tc := range []struct {
		selection     string
		enabled, live bool
		status        int
	}{
		{"7", true, false, 403}, {"8", false, false, 403}, {"8", true, true, 403},
		{"", true, false, 400}, {"oops", true, false, 400}, {"-1", true, false, 400},
	} {
		cfg := trainingConfig(t)
		cfg.Training, cfg.Live = tc.enabled, tc.live
		for _, request := range []struct{ method, path, body string }{
			{http.MethodGet, "/efb/flight", ""},
			{http.MethodGet, "/efb/stands", ""},
			{http.MethodPut, "/efb/tobt", `{"callsign":"SAS123","tobt":"2130"}`},
			{http.MethodPut, "/efb/stand", `{"callsign":"SAS123","stand":"B4"}`},
		} {
			rec := trainingRequest(NewWebAPI(cfg), request.method, request.path+"?training_session="+tc.selection+"&callsign=SAS123", request.body)
			if rec.Code != tc.status {
				t.Fatalf("%s %s selection %q: expected %d, got %d %s", request.method, request.path, tc.selection, tc.status, rec.Code, rec.Body.String())
			}
		}
	}
}
