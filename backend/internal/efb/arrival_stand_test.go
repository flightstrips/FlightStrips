package efb

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/testutil"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestArrivalStandRestrictionBoundary(t *testing.T) {
	now := time.Date(2026, 11, 6, 23, 45, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		offset time.Duration
		hasETA bool
		origin string
		want   string
	}{
		{name: "missing ETA", origin: "EDDF", want: "Stand requests require an arrival ETA"},
		{name: "too early", origin: "EDDF", hasETA: true, offset: 45*time.Minute + time.Nanosecond, want: "Stand requests are available 45 minutes before arrival"},
		{name: "exact boundary", origin: "EDDF", hasETA: true, offset: 45 * time.Minute},
		{name: "after arrival", origin: "EDDF", hasETA: true, offset: -time.Minute},
		{name: "departure unaffected", origin: "EKCH"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			strip := &models.Strip{Origin: tc.origin, Destination: "EKCH"}
			if tc.hasETA {
				strip.ArrivalETA = &models.ArrivalETA{Time: now.Add(tc.offset)}
			}
			if got := arrivalStandRestriction(strip, "EKCH", now); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNetworkArrivalStandEligibilityAndEnforcement(t *testing.T) {
	for _, tc := range []struct {
		name        string
		offset      time.Duration
		hasETA      bool
		wantAllowed bool
	}{
		{name: "unknown ETA"},
		{name: "too early", hasETA: true, offset: time.Hour},
		{name: "within window without existing assignment", hasETA: true, offset: 44 * time.Minute, wantAllowed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			strip := &models.Strip{Callsign: "SAS123", Origin: "EDDF", Destination: "EKCH"}
			if tc.hasETA {
				strip.ArrivalETA = &models.ArrivalETA{Time: time.Now().Add(tc.offset)}
			}
			stands := &trainingStandStub{}
			api := NewWebAPI(WebAPIConfig{
				Auth: authStub{}, Network: networkTestFlight(t, true, time.Now(), "SAS123", "EDDF", "EKCH"),
				Flights:     &finderStub{match: pdc.WebStripMatch{SessionID: 7, Strip: strip}},
				Assignments: assignmentLookupStub{err: pgx.ErrNoRows}, Stands: stands,
				Sessions: &testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*models.Session, error) {
					return &models.Session{ID: 7, Name: "LIVE", Airport: "EKCH"}, nil
				}},
			})
			req := httptest.NewRequest(http.MethodGet, "/efb/flight", nil)
			req.Header.Set("Authorization", "Bearer "+"token")
			rec := httptest.NewRecorder()
			api.handleFlight(rec, req)
			var result snapshot
			if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &result) != nil || result.Capabilities.Stand != tc.wantAllowed || (result.StandRequestUnavailableReason == "") != tc.wantAllowed {
				t.Fatalf("unexpected snapshot: %d %s", rec.Code, rec.Body.String())
			}
			req = httptest.NewRequest(http.MethodPut, "/efb/stand", strings.NewReader(`{"stand":"B4","version":0}`))
			req.Header.Set("Authorization", "Bearer "+"token")
			rec = httptest.NewRecorder()
			api.handleStand(rec, req)
			want := http.StatusConflict
			if tc.wantAllowed {
				want = http.StatusOK
			}
			if rec.Code != want || (stands.session != 0) != tc.wantAllowed {
				t.Fatalf("unexpected stand response: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestArrivalStandRequestsEnforceETAForTraining(t *testing.T) {
	for _, tc := range []struct {
		name   string
		offset time.Duration
		hasETA bool
		status int
		reason string
	}{
		{name: "missing", status: http.StatusConflict, reason: "require an arrival ETA"},
		{name: "too early", hasETA: true, offset: time.Hour, status: http.StatusConflict, reason: "45 minutes before arrival"},
		{name: "within window", hasETA: true, offset: 44 * time.Minute, status: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := trainingConfig(t)
			stands := &trainingStandStub{}
			cfg.Stands = stands
			cfg.TrainingStrips = &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
				strip := &models.Strip{Callsign: "SAS123", Origin: "EDDF", Destination: "EKCH"}
				if tc.hasETA {
					strip.ArrivalETA = &models.ArrivalETA{Time: time.Now().Add(tc.offset)}
				}
				return strip, nil
			}}
			api := NewWebAPI(cfg)
			flight := trainingRequest(api, http.MethodGet, "/efb/flight?training_session=8&callsign=SAS123", "")
			if flight.Code != http.StatusOK || (tc.reason != "" && !strings.Contains(flight.Body.String(), tc.reason)) {
				t.Fatalf("unexpected flight snapshot: %d %s", flight.Code, flight.Body.String())
			}
			rec := trainingRequest(api, http.MethodPut, "/efb/stand?training_session=8", `{"callsign":"SAS123","stand":"B4","version":3}`)
			if rec.Code != tc.status || (tc.reason != "" && !strings.Contains(rec.Body.String(), tc.reason)) {
				t.Fatalf("unexpected stand response: %d %s", rec.Code, rec.Body.String())
			}
			if (stands.session != 0) != (tc.status == http.StatusOK) {
				t.Fatalf("unexpected allocator call: %+v", stands)
			}
		})
	}
}
