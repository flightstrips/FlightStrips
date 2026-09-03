package cdm

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	pkgModels "FlightStrips/pkg/models"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type cdmAuthStub struct{}

func (cdmAuthStub) Validate(string) (shared.AuthenticatedUser, error) {
	return shared.NewAuthenticatedUser("1234567", 0, nil), nil
}

func TestWebAPIHandleSequenceRequiresAuthorization(t *testing.T) {
	t.Parallel()

	api := NewWebAPI(cdmAuthStub{}, &testutil.MockSessionRepository{
		ListFn: func(context.Context) ([]*models.Session, error) {
			return []*models.Session{}, nil
		},
	}, newTestSequenceService(&testutil.MockStripRepository{}, &testutil.MockSessionRepository{}, NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil), nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/cdm/sequence", nil)
	recorder := httptest.NewRecorder()

	api.handleSequence(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, recorder.Code)
	}
}

func TestWebAPIHandleSequenceReturnsSessionRows(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.May, 10, 10, 0, 0, 0, time.UTC)
	firstTobt := addMinutes(timeToClock(now), 10)
	confirmedBy := models.TobtConfirmedByATC
	firstTaxi := 10
	secondTaxi := 13
	firstPosition := 1
	secondPosition := 2
	first := &models.Strip{
		Callsign:         "SAS123",
		Origin:           "EKCH",
		Destination:      "ESSA",
		Runway:           testStringPtr("04L"),
		Sid:              testStringPtr("MIKLA1A"),
		AircraftCategory: testStringPtr("M"),
		CdmData: (&models.CdmData{
			Tobt:            testStringPtr(firstTobt),
			TobtConfirmedBy: &confirmedBy,
			Tsat:            testStringPtr("1010"),
			Ttot:            testStringPtr("1020"),
			Calculation: &models.CdmCalculation{
				BaseTime:         testStringPtr(firstTobt),
				BaseSource:       testStringPtr(models.CdmCalculationBaseTobt),
				TaxiMinutes:      &firstTaxi,
				TaxiRunway:       testStringPtr("04L"),
				SequencePosition: &firstPosition,
				ReasonMarkers: []models.CdmReasonMarker{
					{Kind: "runway_spacing", AgainstCallsign: testStringPtr("SK123"), FromTtot: testStringPtr("101800"), ToTtot: testStringPtr("102000")},
				},
			},
		}).Normalize(),
	}
	second := &models.Strip{
		Callsign:         "SAS456",
		Origin:           "EKCH",
		Destination:      "ESSA",
		Runway:           testStringPtr("04L"),
		Sid:              testStringPtr("MIKLA1A"),
		AircraftCategory: testStringPtr("M"),
		CdmData: (&models.CdmData{
			Tobt: testStringPtr(firstTobt),
			Tsat: testStringPtr("1013"),
			Ttot: testStringPtr("1023"),
			Calculation: &models.CdmCalculation{
				BaseTime:         testStringPtr(firstTobt),
				BaseSource:       testStringPtr(models.CdmCalculationBaseTobt),
				TaxiMinutes:      &secondTaxi,
				TaxiRunway:       testStringPtr("04L"),
				SequencePosition: &secondPosition,
				LeaderCallsign:   testStringPtr("SAS123"),
				LeaderTtot:       testStringPtr("1023"),
				ReasonMarkers: []models.CdmReasonMarker{
					{Kind: "wake_separation", AgainstCallsign: testStringPtr("SAS123"), AgainstRunway: testStringPtr("04L"), FromTtot: testStringPtr("102100"), ToTtot: testStringPtr("102300")},
				},
			},
		}).Normalize(),
	}

	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(ctx context.Context, session int32, origin string) ([]*models.Strip, error) {
			if session != 7 {
				return []*models.Strip{}, nil
			}
			return []*models.Strip{second, first}, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		ListFn: func(ctx context.Context) ([]*models.Session, error) {
			return []*models.Session{
				{
					ID:      7,
					Name:    "LIVE",
					Airport: "EKCH",
					ActiveRunways: pkgModels.ActiveRunways{
						DepartureRunways: []string{"04L"},
						ArrivalRunways:   []string{"22L"},
					},
				},
			}, nil
		},
		GetByIDFn: func(ctx context.Context, id int32) (*models.Session, error) {
			return &models.Session{
				ID:      id,
				Name:    "LIVE",
				Airport: "EKCH",
				ActiveRunways: pkgModels.ActiveRunways{
					DepartureRunways: []string{"04L"},
					ArrivalRunways:   []string{"22L"},
				},
			}, nil
		},
	}
	configStore := NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil)
	sequenceService := newTestSequenceService(stripRepo, sessionRepo, configStore, nil, nil)
	api := NewWebAPI(cdmAuthStub{}, sessionRepo, sequenceService)
	api.now = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodGet, "/cdm/sequence", nil)
	req.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()

	api.handleSequence(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}

	var payload sequenceResponse
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(payload.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(payload.Sessions))
	}

	session := payload.Sessions[0]
	if session.SessionID != 7 || session.Airport != "EKCH" || session.Name != "LIVE" {
		t.Fatalf("unexpected session metadata: %#v", session)
	}
	if len(session.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(session.Rows))
	}

	firstRow := session.Rows[0]
	secondRow := session.Rows[1]

	if firstRow.Callsign != "SAS123" || firstRow.Position == nil || *firstRow.Position != 1 {
		t.Fatalf("unexpected first row: %#v", firstRow)
	}
	if !firstRow.TobtConfirmed || firstRow.TobtConfirmedBy != models.TobtConfirmedByATC {
		t.Fatalf("expected TOBT confirmation on first row, got %#v", firstRow)
	}
	if firstRow.TaxiMinutes == nil || *firstRow.TaxiMinutes != 10 {
		t.Fatalf("expected taxi minutes on first row, got %#v", firstRow)
	}
	if !hasReason(firstRow.Reasons, "runway_spacing", "SK123") {
		t.Fatalf("expected movement reason on first row, got %#v", firstRow.Reasons)
	}
	if !strings.Contains(firstRow.Reasons[0].Message, "runway departure spacing") {
		t.Fatalf("expected compact runway-spacing message, got %#v", firstRow.Reasons)
	}

	if secondRow.Callsign != "SAS456" || secondRow.Position == nil || *secondRow.Position != 2 {
		t.Fatalf("unexpected second row: %#v", secondRow)
	}
	if !hasReason(secondRow.Reasons, "wake_separation", "SAS123") {
		t.Fatalf("expected wake-separation reason against SAS123, got %#v", secondRow.Reasons)
	}
	if firstRow.Ttot != "1020" || secondRow.Ttot != "1023" {
		t.Fatalf("expected endpoint to return stored TTOT values, got first=%q second=%q", firstRow.Ttot, secondRow.Ttot)
	}
}

func TestWebAPIHandleSequenceReturnsCurrentDepartureRatesWithoutAircraft(t *testing.T) {
	t.Parallel()

	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(context.Context, int32, string) ([]*models.Strip, error) {
			return []*models.Strip{}, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		ListFn: func(context.Context) ([]*models.Session, error) {
			return []*models.Session{{
				ID:      7,
				Name:    "LIVE",
				Airport: "EKCH",
				ActiveRunways: pkgModels.ActiveRunways{
					ArrivalRunways:   []string{"30"},
					DepartureRunways: []string{"22R"},
				},
			}}, nil
		},
	}
	configStore := NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil)
	configStore.configs["EKCH"] = &CdmAirportConfig{
		Airport:     "EKCH",
		DefaultRate: 40,
		Rates: []CdmRate{{
			Airport:   "EKCH",
			ArrRwyYes: []string{"30"},
			DepRwyYes: []string{"22R"},
			Rates:     []string{"30"},
		}},
	}
	api := NewWebAPI(cdmAuthStub{}, sessionRepo, newTestSequenceService(stripRepo, sessionRepo, configStore, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/cdm/sequence", nil)
	req.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()
	api.handleSequence(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	var payload sequenceResponse
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(payload.Sessions) != 1 || len(payload.Sessions[0].Rows) != 0 {
		t.Fatalf("expected one empty session, got %#v", payload)
	}
	rates := payload.Sessions[0].DepartureRates
	if len(rates) != 1 || rates[0].Runway != "22R" || rates[0].DeparturesHour != 30 {
		t.Fatalf("expected current 22R rate of 30 departures/hour, got %#v", rates)
	}
}

func TestWebAPIHandleSequenceAssignsMissingStoredPositionsIndependently(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.May, 10, 10, 0, 0, 0, time.UTC)
	firstTaxi := 10
	secondTaxi := 12
	first := &models.Strip{
		Callsign:         "SAS123",
		Origin:           "EKCH",
		Destination:      "ESSA",
		Runway:           testStringPtr("04L"),
		Sid:              testStringPtr("MIKLA1A"),
		AircraftCategory: testStringPtr("M"),
		CdmData: (&models.CdmData{
			Tobt: testStringPtr("1000"),
			Tsat: testStringPtr("1010"),
			Ttot: testStringPtr("1020"),
			Calculation: &models.CdmCalculation{
				BaseTime:    testStringPtr("1000"),
				BaseSource:  testStringPtr(models.CdmCalculationBaseTobt),
				TaxiMinutes: &firstTaxi,
				TaxiRunway:  testStringPtr("04L"),
			},
		}).Normalize(),
	}
	second := &models.Strip{
		Callsign:         "SAS456",
		Origin:           "EKCH",
		Destination:      "ESSA",
		Runway:           testStringPtr("04L"),
		Sid:              testStringPtr("MIKLA1A"),
		AircraftCategory: testStringPtr("M"),
		CdmData: (&models.CdmData{
			Tobt: testStringPtr("1005"),
			Tsat: testStringPtr("1015"),
			Ttot: testStringPtr("1025"),
			Calculation: &models.CdmCalculation{
				BaseTime:    testStringPtr("1005"),
				BaseSource:  testStringPtr(models.CdmCalculationBaseTobt),
				TaxiMinutes: &secondTaxi,
				TaxiRunway:  testStringPtr("04L"),
			},
		}).Normalize(),
	}

	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(ctx context.Context, session int32, origin string) ([]*models.Strip, error) {
			return []*models.Strip{second, first}, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		ListFn: func(ctx context.Context) ([]*models.Session, error) {
			return []*models.Session{{
				ID:      7,
				Name:    "LIVE",
				Airport: "EKCH",
				ActiveRunways: pkgModels.ActiveRunways{
					DepartureRunways: []string{"04L"},
					ArrivalRunways:   []string{"22L"},
				},
			}}, nil
		},
	}

	configStore := NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil)
	sequenceService := newTestSequenceService(stripRepo, sessionRepo, configStore, nil, nil)
	api := NewWebAPI(cdmAuthStub{}, sessionRepo, sequenceService)
	api.now = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodGet, "/cdm/sequence", nil)
	req.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()

	api.handleSequence(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}

	var payload sequenceResponse
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(payload.Sessions) != 1 || len(payload.Sessions[0].Rows) != 2 {
		t.Fatalf("unexpected payload: %#v", payload)
	}

	firstRow := payload.Sessions[0].Rows[0]
	secondRow := payload.Sessions[0].Rows[1]
	if firstRow.Position == nil || *firstRow.Position != 1 {
		t.Fatalf("expected first fallback position to be 1, got %#v", firstRow)
	}
	if secondRow.Position == nil || *secondRow.Position != 2 {
		t.Fatalf("expected second fallback position to be 2, got %#v", secondRow)
	}
}

func TestBuildPersistedSequenceRows_IncludesEobtCappedReasonMarker(t *testing.T) {
	t.Parallel()

	eobt := "1030"
	euroscopeSeenAt := time.Now().UTC()
	rows := buildPersistedSequenceRows([]*models.Strip{{
		Callsign:        "SAS131",
		Origin:          "EKCH",
		Destination:     "ESSA",
		EuroscopeSeenAt: &euroscopeSeenAt,
		CdmData: (&models.CdmData{
			Eobt: &eobt,
			Calculation: &models.CdmCalculation{
				ReasonMarkers: []models.CdmReasonMarker{{
					Kind:    eobtCappedReasonKind,
					Message: eobtCappedReasonMessage,
				}},
			},
		}).Normalize(),
	}}, true, time.Now().UTC())

	if len(rows) != 1 {
		t.Fatalf("expected one row, got %#v", rows)
	}
	if !hasReason(rows[0].response.Reasons, eobtCappedReasonKind, "") {
		t.Fatalf("expected EOBT cap reason, got %#v", rows[0].response.Reasons)
	}
	if rows[0].response.Reasons[0].Message != eobtCappedReasonMessage {
		t.Fatalf("expected reason message %q, got %#v", eobtCappedReasonMessage, rows[0].response.Reasons)
	}
}

func hasReason(reasons []sequenceReasonResponse, kind string, against string) bool {
	for _, reason := range reasons {
		if reason.Kind != kind {
			continue
		}
		if against != "" && !strings.EqualFold(reason.AgainstCallsign, against) {
			continue
		}
		return true
	}
	return false
}
