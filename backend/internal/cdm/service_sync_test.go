package cdm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
)

type masterSyncTestTimes struct {
	Eobt        string
	Tobt        string
	ReqTobt     string
	Tsat        string
	Ttot        string
	Ctot        string
	TobtSeconds string
}

func futureMasterSyncTestTimes() masterSyncTestTimes {
	eobt := time.Now().UTC().Truncate(time.Minute).Add(15 * time.Minute)
	tobt := eobt.Add(10 * time.Minute)
	return masterSyncTestTimes{
		Eobt:        eobt.Format("1504"),
		Tobt:        tobt.Format("1504"),
		ReqTobt:     eobt.Add(5 * time.Minute).Format("1504"),
		Tsat:        eobt.Add(15 * time.Minute).Format("150405"),
		Ttot:        eobt.Add(25 * time.Minute).Format("150405"),
		Ctot:        eobt.Add(40 * time.Minute).Format("1504"),
		TobtSeconds: tobt.Format("150405"),
	}
}

func TestSyncCdmData_AcceptsAndClearsRequestedTobt(t *testing.T) {
	const sessionID = int32(77)
	const callsign = "SAS123"

	var (
		dpiValues []string
		calls     []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ifps/depAirport":
			_, _ = w.Write([]byte(`[{
			"callsign":"SAS123",
			"departure":"EKCH",
			"eobt":"1000",
			"tobt":"1010",
			"cdmSts":"REA",
			"cdmData":{
				"reqTobt":"1005",
				"reqTobtType":"PILOT",
				"reqAsrt":"100700"
			}
		}]`))
		case "/ifps/setCdmData":
			calls = append(calls, "push")
			w.WriteHeader(http.StatusOK)
		case "/ifps/dpi":
			value := r.URL.Query().Get("value")
			if strings.HasPrefix(value, "TOBT/") {
				calls = append(calls, "push")
			} else {
				calls = append(calls, "clear")
			}
			dpiValues = append(dpiValues, value)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	existing := (&models.CdmData{}).Normalize()
	var persisted *models.CdmData
	euroscopeHub := &testutil.MockEuroscopeHub{}
	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
				return &models.Strip{Callsign: callsign, Origin: "EKCH"}, nil
			},
			GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
				return []*models.CdmDataRow{{
					Callsign: callsign,
					Data:     existing.Clone(),
				}}, nil
			},
			SetCdmDataFn: func(_ context.Context, session int32, gotCallsign string, data *models.CdmData) (int64, error) {
				if session != sessionID || gotCallsign != callsign {
					t.Fatalf("unexpected persistence target %d %s", session, gotCallsign)
				}
				persisted = data.Clone()
				return 1, nil
			},
			GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
				return persisted.Clone(), nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)
	setTestCdmEuroscope(service, euroscopeHub)

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Name: "LIVE", Airport: "EKCH"})
	if err != nil {
		t.Fatalf("syncCdmData returned error: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected persisted CDM data")
	}
	if got := valueOrEmpty(persisted.Tobt); got != "1005" {
		t.Fatalf("expected requested TOBT to become authoritative, got %q", got)
	}
	if got := valueOrEmpty(persisted.TobtSetBy); got != "vIFF" {
		t.Fatalf("expected vIFF provenance, got %q", got)
	}
	if got := valueOrEmpty(persisted.TobtConfirmedBy); got != "PILOT" {
		t.Fatalf("expected exact request source, got %q", got)
	}
	if persisted.ViffRequestSyncPending {
		t.Fatal("expected request synchronization marker to clear after push and remote clear")
	}
	if len(dpiValues) != 2 || dpiValues[1] != "REQTOBT/NULL/NULL" {
		t.Fatalf("expected remote request to be cleared after export, got %v", dpiValues)
	}
	if len(calls) != 2 || calls[0] != "push" || calls[1] != "clear" {
		t.Fatalf("expected authoritative push before request clear, got %v", calls)
	}
	foundAccepted := false
	for _, message := range euroscopeHub.Broadcasts {
		event, ok := message.(euroscopeEvents.CdmUpdateEvent)
		if ok && event.Tobt == "1005" && event.TobtConfirmedBy == "PILOT" {
			foundAccepted = true
			break
		}
	}
	if !foundAccepted {
		t.Fatalf("expected accepted TOBT to be broadcast, got %#v", euroscopeHub.Broadcasts)
	}
}

func TestSyncCdmData_RetriesReadyWhenFlightIsMissingFromViffResponse(t *testing.T) {
	const sessionID = int32(78)
	const callsign = "SASREADY"

	now := time.Now().UTC().Truncate(time.Minute)
	tobt := now.Format("1504")
	tsat := now.Add(3 * time.Minute).Format("150405")
	ttot := now.Add(13 * time.Minute).Format("150405")
	stored := (&models.CdmData{
		Eobt:             &tobt,
		Tobt:             &tobt,
		Tsat:             &tsat,
		Ttot:             &ttot,
		ReadySyncPending: true,
	}).Normalize()

	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ifps/depAirport":
			_, _ = w.Write([]byte(`[]`))
		case "/ifps/dpi":
			calls = append(calls, r.URL.Query().Get("value"))
			w.WriteHeader(http.StatusOK)
		case "/ifps/setCdmData":
			calls = append(calls, "CDM")
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
				return []*models.CdmDataRow{{Callsign: callsign, Data: stored.Clone()}}, nil
			},
			GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
				return &models.Strip{Callsign: callsign, Origin: "EKCH", CdmData: stored.Clone()}, nil
			},
			GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
				return stored.Clone(), nil
			},
			SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
				stored = data.Clone()
				return 1, nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Name: "LIVE", Airport: "EKCH"})
	if err != nil {
		t.Fatalf("syncCdmData returned error: %v", err)
	}
	if stored.ReadySyncPending {
		t.Fatal("expected READY synchronization marker to clear")
	}
	if len(calls) != 2 || calls[0] != "REA/1" || calls[1] != "CDM" {
		t.Fatalf("expected READY and CDM export despite missing remote row, got %v", calls)
	}
}

func TestSyncCdmData_RetriesRecalculationBeforeClearingRequestedTobt(t *testing.T) {
	const sessionID = int32(79)
	const callsign = "SASRETRY"
	const runway = "22R"

	now := time.Now().UTC().Truncate(time.Minute)
	tobt := now.Add(15 * time.Minute).Format("1504")
	euroscopeSeenAt := now
	confirmedBy := "PILOT"
	setBy := "vIFF"
	stored := (&models.CdmData{
		Eobt:                   &tobt,
		Tobt:                   &tobt,
		TobtSetBy:              &setBy,
		TobtConfirmedBy:        &confirmedBy,
		TobtManuallyConfirmed:  true,
		ViffRequestSyncPending: true,
		Recalculate:            true,
		RecalculationMode:      models.CdmRecalculationRequired,
	}).Normalize()
	stripForState := func() *models.Strip {
		return &models.Strip{
			Callsign:        callsign,
			Session:         sessionID,
			Origin:          "EKCH",
			Runway:          testStringPtr(runway),
			EuroscopeSeenAt: &euroscopeSeenAt,
			CdmData:         stored.Clone(),
		}
	}

	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ifps/depAirport":
			_, _ = fmt.Fprintf(w, `[{"callsign":%q,"departure":"EKCH","cdmData":{"reqTobt":%q,"reqTobtType":"PILOT"}}]`, callsign, tobt)
		case "/ifps/setCdmData":
			calls = append(calls, "push")
			w.WriteHeader(http.StatusOK)
		case "/ifps/dpi":
			calls = append(calls, "clear")
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	getCdmDataCalls := 0
	stripRepo := &testutil.MockStripRepository{
		GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
			getCdmDataCalls++
			return []*models.CdmDataRow{{Callsign: callsign, Data: stored.Clone()}}, nil
		},
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
			return stripForState(), nil
		},
		ListByOriginFn: func(context.Context, int32, string) ([]*models.Strip, error) {
			return []*models.Strip{stripForState()}, nil
		},
		GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
			return stored.Clone(), nil
		},
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
			stored = data.Clone()
			return 1, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*models.Session, error) {
		return &models.Session{ID: sessionID, Name: "LIVE", Airport: "EKCH"}, nil
	}}
	configStore := NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil)
	service := newTestCdmService(NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)), stripRepo, sessionRepo, &testutil.MockControllerRepository{})
	service.SetConfigProvider(configStore)
	service.sequenceService = newTestSequenceService(stripRepo, sessionRepo, configStore, &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Name: "LIVE", Airport: "EKCH"})
	if err != nil {
		t.Fatalf("syncCdmData returned error: %v", err)
	}
	if stored.NeedsLocalRecalculation() || stored.ViffRequestSyncPending {
		t.Fatalf("expected recalculation and request synchronization to finish, got %#v", stored)
	}
	if valueOrEmpty(stored.Tsat) == "" || valueOrEmpty(stored.Ttot) == "" {
		t.Fatalf("expected authoritative TSAT/TTOT before request clear, got %#v", stored)
	}
	if len(calls) != 2 || calls[0] != "push" || calls[1] != "clear" {
		t.Fatalf("expected recalculated state push before clear, got %v", calls)
	}
	if getCdmDataCalls < 2 {
		t.Fatalf("expected airport CDM snapshot refresh after recalculation, got %d loads", getCdmDataCalls)
	}
}

func TestSyncCdmData_RecoversPendingRequestAfterRemoteClear(t *testing.T) {
	const sessionID = int32(91)
	const callsign = "SASRECOVER"
	times := futureMasterSyncTestTimes()
	confirmedBy := "PILOT"
	setBy := "vIFF"
	stored := (&models.CdmData{
		Eobt:                   testStringPtr(times.Eobt),
		Tobt:                   testStringPtr(times.Tobt),
		TobtSetBy:              &setBy,
		TobtConfirmedBy:        &confirmedBy,
		TobtManuallyConfirmed:  true,
		Tsat:                   testStringPtr(times.Tsat),
		Ttot:                   testStringPtr(times.Ttot),
		ViffRequestSyncPending: true,
	}).Normalize()

	pushes := 0
	remoteClears := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ifps/depAirport":
			_, _ = fmt.Fprintf(w, `[{"callsign":%q,"departure":"EKCH","eobt":%q,"tobt":%q,"cdmData":{}}]`, callsign, times.Eobt, times.Tobt)
		case "/ifps/setCdmData":
			pushes++
			w.WriteHeader(http.StatusOK)
		case "/ifps/dpi":
			remoteClears++
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	persistAttempts := 0
	stripRepo := &testutil.MockStripRepository{
		GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
			return []*models.CdmDataRow{{Callsign: callsign, Data: stored.Clone()}}, nil
		},
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
			return &models.Strip{Callsign: callsign, Origin: "EKCH", Runway: testStringPtr("22R"), CdmData: stored.Clone()}, nil
		},
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
			persistAttempts++
			if persistAttempts == 1 {
				return 0, nil
			}
			stored = data.Clone()
			return 1, nil
		},
	}
	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		stripRepo,
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)
	session := &models.Session{ID: sessionID, Name: "LIVE", Airport: "EKCH"}

	if err := service.syncCdmData(context.Background(), session); err == nil {
		t.Fatal("expected the first local marker clear to fail")
	}
	if !stored.ViffRequestSyncPending {
		t.Fatal("expected the marker to remain after the failed local persistence")
	}
	if err := service.syncCdmData(context.Background(), session); err != nil {
		t.Fatalf("expected the next poll to recover the local marker: %v", err)
	}
	if stored.ViffRequestSyncPending {
		t.Fatal("expected the recovered local marker to clear")
	}
	if pushes != 2 {
		t.Fatalf("expected idempotent authoritative export on each attempt, got %d", pushes)
	}
	if remoteClears != 0 {
		t.Fatalf("expected no repeated remote clear after REQTOBT disappeared, got %d", remoteClears)
	}
}

func TestSyncCdmData_PreservesExistingAsat(t *testing.T) {
	const sessionID = int32(78)
	const callsign = "SAS124"
	asat := "1031"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ifps/depAirport":
			_, _ = w.Write([]byte(`[{
				"callsign":"SAS124",
				"departure":"EKCH",
				"eobt":"1000",
				"tobt":"1010",
				"ctot":"1040",
				"cdmSts":"REA",
				"cdmData":{"tsat":"101500","ttot":"102500"}
			}]`))
		case "/ifps/dpi", "/ifps/setCdmData":
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	existing := &models.CdmData{Asat: &asat}
	var persisted *models.CdmData
	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
				return &models.Strip{Callsign: callsign, Origin: "EKCH"}, nil
			},
			GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
				return []*models.CdmDataRow{{Callsign: callsign, Data: existing.Clone()}}, nil
			},
			SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
				persisted = data.Clone()
				return 1, nil
			},
			GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
				return persisted.Clone(), nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Name: "LIVE", Airport: "EKCH"})
	if err != nil {
		t.Fatalf("syncCdmData returned error: %v", err)
	}
	if persisted == nil || valueOrEmpty(persisted.Asat) != asat {
		t.Fatalf("expected ASAT %q to be preserved, got %#v", asat, persisted)
	}
}

func TestSyncCdmData_MasterSession_NormalizesExistingFarFutureEobt(t *testing.T) {
	const sessionID = int32(90)
	const callsign = "SAS190"
	const airport = "EKCH"
	const masterCid = "777888"
	now := time.Now().UTC()
	rawFutureEobt := truncateCDMClockValue(addMinutes(timeToClock(now), 60))
	expectedClamped := truncateCDMClockValue(addMinutes(timeToClock(now), masterEobtClampTarget))
	currentTobt := expectedClamped

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ifps/depAirport":
			_, _ = w.Write([]byte(`[]`))
		case "/ifps/dpi", "/ifps/setCdmData":
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	initial := (&models.CdmData{
		Eobt: testStringPtr(rawFutureEobt),
		Tobt: testStringPtr(currentTobt),
	}).Normalize()
	var persisted *models.CdmData
	euroscopeHub := &testutil.MockEuroscopeHub{
		GetMasterCallsignFn: func(int32) string {
			return "EKCH_A_TWR"
		},
	}
	controllerRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, session int32, callsign string) (*models.Controller, error) {
			if session != sessionID {
				t.Fatalf("unexpected session %d", session)
			}
			if callsign != "EKCH_A_TWR" {
				t.Fatalf("unexpected callsign %s", callsign)
			}
			return &models.Controller{Cid: testStringPtr(masterCid)}, nil
		},
	}
	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
				return []*models.CdmDataRow{{Callsign: callsign, Data: initial.Clone()}}, nil
			},
			GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Strip, error) {
				if gotSession != sessionID || gotCallsign != callsign {
					t.Fatalf("unexpected GetByCallsign target %d %s", gotSession, gotCallsign)
				}
				return &models.Strip{
					Callsign: callsign,
					Session:  sessionID,
					Origin:   airport,
				}, nil
			},
			ListByOriginFn: func(_ context.Context, gotSession int32, gotAirport string) ([]*models.Strip, error) {
				if gotSession != sessionID || gotAirport != airport {
					t.Fatalf("unexpected ListByOrigin target %d %s", gotSession, gotAirport)
				}
				return []*models.Strip{{
					Callsign: callsign,
					Session:  sessionID,
					Origin:   airport,
					CdmData:  initial.Clone(),
				}}, nil
			},
			SetCdmDataFn: func(_ context.Context, session int32, gotCallsign string, data *models.CdmData) (int64, error) {
				if session != sessionID || gotCallsign != callsign {
					t.Fatalf("unexpected persistence target %d %s", session, gotCallsign)
				}
				persisted = data.Clone()
				return 1, nil
			},
			GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
				return persisted.Clone(), nil
			},
		},
		&testutil.MockSessionRepository{},
		controllerRepo,
	)
	setTestCdmEuroscope(service, euroscopeHub)

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Name: "LIVE", Airport: airport})
	if err != nil {
		t.Fatalf("syncCdmData returned error: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected persisted CDM data")
	}
	if got := valueOrEmpty(persisted.Eobt); got != expectedClamped {
		t.Fatalf("expected persisted EOBT %q, got %q", expectedClamped, got)
	}
	if got := valueOrEmpty(persisted.Tobt); got != expectedClamped {
		t.Fatalf("expected persisted TOBT %q, got %q", expectedClamped, got)
	}
	if !persisted.TobtAutoSynced {
		t.Fatalf("expected normalized TOBT to stay auto-synced, got %#v", persisted)
	}
	if persisted.TobtManuallyConfirmed {
		t.Fatalf("expected normalized TOBT to remain non-manual, got %#v", persisted)
	}
	if persisted.Calculation == nil || len(persisted.Calculation.ReasonMarkers) == 0 {
		t.Fatalf("expected stored reason markers, got %#v", persisted.Calculation)
	}
	if persisted.Calculation.ReasonMarkers[0].Kind != eobtCappedReasonKind {
		t.Fatalf("expected first reason marker %q, got %#v", eobtCappedReasonKind, persisted.Calculation.ReasonMarkers)
	}
	if len(euroscopeHub.Eobts) != 1 {
		t.Fatalf("expected one EuroScope EOBT sync-back, got %d", len(euroscopeHub.Eobts))
	}
	if euroscopeHub.Eobts[0].Cid != masterCid || euroscopeHub.Eobts[0].Eobt != expectedClamped {
		t.Fatalf("unexpected EuroScope EOBT sync-back: %#v", euroscopeHub.Eobts[0])
	}
}

func TestSyncCdmData_MasterSession_NormalizesEmptyEobt(t *testing.T) {
	const sessionID = int32(93)
	const callsign = "SAS193"
	const airport = "EKCH"
	const masterCid = "777891"
	now := time.Now().UTC()
	expectedClamped := truncateCDMClockValue(addMinutes(timeToClock(now), masterEobtClampTarget))
	currentTobt := expectedClamped

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ifps/depAirport":
			_, _ = w.Write([]byte(`[]`))
		case "/ifps/dpi", "/ifps/setCdmData":
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	initial := (&models.CdmData{
		Tobt: testStringPtr(currentTobt),
	}).Normalize()
	var persisted *models.CdmData
	euroscopeHub := &testutil.MockEuroscopeHub{
		GetMasterCallsignFn: func(int32) string {
			return "EKCH_A_TWR"
		},
	}
	controllerRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, session int32, callsign string) (*models.Controller, error) {
			if session != sessionID {
				t.Fatalf("unexpected session %d", session)
			}
			if callsign != "EKCH_A_TWR" {
				t.Fatalf("unexpected callsign %s", callsign)
			}
			return &models.Controller{Cid: testStringPtr(masterCid)}, nil
		},
	}
	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
				return []*models.CdmDataRow{{Callsign: callsign, Data: initial.Clone()}}, nil
			},
			GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Strip, error) {
				if gotSession != sessionID || gotCallsign != callsign {
					t.Fatalf("unexpected GetByCallsign target %d %s", gotSession, gotCallsign)
				}
				return &models.Strip{
					Callsign: callsign,
					Session:  sessionID,
					Origin:   airport,
				}, nil
			},
			ListByOriginFn: func(_ context.Context, gotSession int32, gotAirport string) ([]*models.Strip, error) {
				if gotSession != sessionID || gotAirport != airport {
					t.Fatalf("unexpected ListByOrigin target %d %s", gotSession, gotAirport)
				}
				return []*models.Strip{{
					Callsign: callsign,
					Session:  sessionID,
					Origin:   airport,
					CdmData:  initial.Clone(),
				}}, nil
			},
			SetCdmDataFn: func(_ context.Context, session int32, gotCallsign string, data *models.CdmData) (int64, error) {
				if session != sessionID || gotCallsign != callsign {
					t.Fatalf("unexpected persistence target %d %s", session, gotCallsign)
				}
				persisted = data.Clone()
				return 1, nil
			},
			GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
				return persisted.Clone(), nil
			},
		},
		&testutil.MockSessionRepository{},
		controllerRepo,
	)
	setTestCdmEuroscope(service, euroscopeHub)

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Name: "LIVE", Airport: airport})
	if err != nil {
		t.Fatalf("syncCdmData returned error: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected persisted CDM data")
	}
	if got := valueOrEmpty(persisted.Eobt); got != expectedClamped {
		t.Fatalf("expected persisted EOBT %q, got %q", expectedClamped, got)
	}
	if got := valueOrEmpty(persisted.Tobt); got != expectedClamped {
		t.Fatalf("expected persisted TOBT %q, got %q", expectedClamped, got)
	}
	if !persisted.TobtAutoSynced {
		t.Fatalf("expected normalized TOBT to stay auto-synced, got %#v", persisted)
	}
	if persisted.TobtManuallyConfirmed {
		t.Fatalf("expected normalized TOBT to remain non-manual, got %#v", persisted)
	}
	if persisted.Calculation == nil || len(persisted.Calculation.ReasonMarkers) == 0 {
		t.Fatalf("expected stored reason markers, got %#v", persisted.Calculation)
	}
	if persisted.Calculation.ReasonMarkers[0].Kind != eobtCappedReasonKind {
		t.Fatalf("expected first reason marker %q, got %#v", eobtCappedReasonKind, persisted.Calculation.ReasonMarkers)
	}
	if len(euroscopeHub.Eobts) != 1 {
		t.Fatalf("expected one EuroScope EOBT sync-back, got %d", len(euroscopeHub.Eobts))
	}
	if euroscopeHub.Eobts[0].Cid != masterCid || euroscopeHub.Eobts[0].Eobt != expectedClamped {
		t.Fatalf("unexpected EuroScope EOBT sync-back: %#v", euroscopeHub.Eobts[0])
	}
}

func TestSyncCdmData_MasterSession_KeepsFreshCtotWhenEobtNormalizationAlsoRuns(t *testing.T) {
	const sessionID = int32(91)
	const callsign = "SAS191"
	const airport = "EKCH"
	const masterCid = "777889"
	now := time.Now().UTC()
	rawFutureEobt := truncateCDMClockValue(addMinutes(timeToClock(now), 60))
	currentTobt := truncateCDMClockValue(addMinutes(timeToClock(now), 35))
	expectedClamped := truncateCDMClockValue(addMinutes(timeToClock(now), masterEobtClampTarget))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ifps/depAirport":
			_, _ = w.Write([]byte(`[{
				"callsign":"SAS191",
				"departure":"EKCH",
				"eobt":"1000",
				"tobt":"1010",
				"ctot":"1040",
				"cdmSts":"REA",
				"cdmData":{
					"reqTobt":"1005",
					"reqTobtType":"PILOT",
					"reason":"REGUL"
				}
			}]`))
		case "/ifps/dpi", "/ifps/setCdmData":
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	initial := (&models.CdmData{
		Eobt: &rawFutureEobt,
		Tobt: &currentTobt,
	}).Normalize()
	var persisted *models.CdmData
	var persistCount int
	euroscopeHub := &testutil.MockEuroscopeHub{
		GetMasterCallsignFn: func(int32) string {
			return "EKCH_A_TWR"
		},
	}
	controllerRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, session int32, callsign string) (*models.Controller, error) {
			if session != sessionID {
				t.Fatalf("unexpected session %d", session)
			}
			if callsign != "EKCH_A_TWR" {
				t.Fatalf("unexpected callsign %s", callsign)
			}
			return &models.Controller{Cid: testStringPtr(masterCid)}, nil
		},
	}
	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
				return []*models.CdmDataRow{{Callsign: callsign, Data: initial.Clone()}}, nil
			},
			GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Strip, error) {
				if gotSession != sessionID || gotCallsign != callsign {
					t.Fatalf("unexpected GetByCallsign target %d %s", gotSession, gotCallsign)
				}
				return &models.Strip{
					Callsign: callsign,
					Session:  sessionID,
					Origin:   airport,
					Runway:   testStringPtr("22R"),
				}, nil
			},
			SetCdmDataFn: func(_ context.Context, session int32, gotCallsign string, data *models.CdmData) (int64, error) {
				if session != sessionID || gotCallsign != callsign {
					t.Fatalf("unexpected persistence target %d %s", session, gotCallsign)
				}
				persisted = data.Clone()
				persistCount++
				return 1, nil
			},
			GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
				return persisted.Clone(), nil
			},
		},
		&testutil.MockSessionRepository{},
		controllerRepo,
	)
	setTestCdmEuroscope(service, euroscopeHub)

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Name: "LIVE", Airport: airport})
	if err != nil {
		t.Fatalf("syncCdmData returned error: %v", err)
	}
	if persistCount != 3 {
		t.Fatalf("expected request merge, sync-marker clear, and EOBT normalization persists, got %d", persistCount)
	}
	if persisted == nil {
		t.Fatal("expected persisted CDM data")
	}
	if got := valueOrEmpty(persisted.Eobt); got != expectedClamped {
		t.Fatalf("expected persisted EOBT %q, got %q", expectedClamped, got)
	}
	if got := valueOrEmpty(persisted.Tobt); got != "1005" {
		t.Fatalf("expected requested TOBT to take precedence, got %q", got)
	}
	if got := valueOrEmpty(persisted.Ctot); got != "" {
		t.Fatalf("expected obsolete automatic CTOT to be cleared, got %q", got)
	}
	if persisted.TobtAutoSynced {
		t.Fatalf("expected accepted vIFF TOBT to be authoritative, got %#v", persisted)
	}
	if !persisted.TobtManuallyConfirmed || valueOrEmpty(persisted.TobtSetBy) != "vIFF" {
		t.Fatalf("expected accepted vIFF TOBT provenance, got %#v", persisted)
	}
	if got := valueOrEmpty(persisted.EcfmpID); got != "" {
		t.Fatalf("expected obsolete automatic reason to be cleared, got %q", got)
	}
	if len(euroscopeHub.Eobts) != 1 {
		t.Fatalf("expected one EuroScope EOBT sync-back, got %d", len(euroscopeHub.Eobts))
	}
}

func TestSyncCdmData_MasterSession_DoesNotOverwriteConfirmedTobtDuringEobtNormalization(t *testing.T) {
	const sessionID = int32(92)
	const callsign = "SAS192"
	const airport = "EKCH"
	const masterCid = "777890"
	now := time.Now().UTC()
	rawFutureEobt := truncateCDMClockValue(addMinutes(timeToClock(now), 60))
	currentTobt := "0000"
	confirmedBy := models.TobtConfirmedByPilot
	expectedClamped := truncateCDMClockValue(addMinutes(timeToClock(now), masterEobtClampTarget))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ifps/depAirport":
			_, _ = w.Write([]byte(`[]`))
		case "/ifps/dpi", "/ifps/setCdmData":
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	initial := (&models.CdmData{
		Eobt:                  &rawFutureEobt,
		Tobt:                  &currentTobt,
		TobtConfirmedBy:       &confirmedBy,
		TobtManuallyConfirmed: true,
	}).Normalize()
	var persisted *models.CdmData
	euroscopeHub := &testutil.MockEuroscopeHub{
		GetMasterCallsignFn: func(int32) string {
			return "EKCH_A_TWR"
		},
	}
	controllerRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, session int32, callsign string) (*models.Controller, error) {
			if session != sessionID {
				t.Fatalf("unexpected session %d", session)
			}
			if callsign != "EKCH_A_TWR" {
				t.Fatalf("unexpected callsign %s", callsign)
			}
			return &models.Controller{Cid: testStringPtr(masterCid)}, nil
		},
	}
	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
				return []*models.CdmDataRow{{Callsign: callsign, Data: initial.Clone()}}, nil
			},
			GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Strip, error) {
				if gotSession != sessionID || gotCallsign != callsign {
					t.Fatalf("unexpected GetByCallsign target %d %s", gotSession, gotCallsign)
				}
				return &models.Strip{
					Callsign: callsign,
					Session:  sessionID,
					Origin:   airport,
				}, nil
			},
			ListByOriginFn: func(_ context.Context, gotSession int32, gotAirport string) ([]*models.Strip, error) {
				if gotSession != sessionID || gotAirport != airport {
					t.Fatalf("unexpected ListByOrigin target %d %s", gotSession, gotAirport)
				}
				return []*models.Strip{{
					Callsign: callsign,
					Session:  sessionID,
					Origin:   airport,
					CdmData:  initial.Clone(),
				}}, nil
			},
			SetCdmDataFn: func(_ context.Context, session int32, gotCallsign string, data *models.CdmData) (int64, error) {
				if session != sessionID || gotCallsign != callsign {
					t.Fatalf("unexpected persistence target %d %s", session, gotCallsign)
				}
				persisted = data.Clone()
				return 1, nil
			},
			GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
				return persisted.Clone(), nil
			},
		},
		&testutil.MockSessionRepository{},
		controllerRepo,
	)
	setTestCdmEuroscope(service, euroscopeHub)

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Name: "LIVE", Airport: airport})
	if err != nil {
		t.Fatalf("syncCdmData returned error: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected persisted CDM data")
	}
	if got := valueOrEmpty(persisted.Eobt); got != expectedClamped {
		t.Fatalf("expected persisted EOBT %q, got %q", expectedClamped, got)
	}
	if got := valueOrEmpty(persisted.Tobt); got != currentTobt {
		t.Fatalf("expected confirmed TOBT %q to be preserved, got %q", currentTobt, got)
	}
	if got := valueOrEmpty(persisted.TobtConfirmedBy); got != confirmedBy {
		t.Fatalf("expected TOBT confirmation %q to be preserved, got %q", confirmedBy, got)
	}
	if !persisted.TobtManuallyConfirmed {
		t.Fatalf("expected TOBT manual confirmation to be preserved, got %#v", persisted)
	}
	if len(euroscopeHub.Eobts) != 1 {
		t.Fatalf("expected one EuroScope EOBT sync-back, got %d", len(euroscopeHub.Eobts))
	}
}

func TestSyncCdmData_UsesNestedCtotForFrontendUpdate(t *testing.T) {
	const sessionID = int32(80)
	const callsign = "SAS126"
	times := futureMasterSyncTestTimes()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ifps/depAirport" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = fmt.Fprintf(w, `[{
			"callsign":"SAS126",
			"departure":"EKCH",
			"eobt":"%s",
			"tobt":"%s",
			"ctot":"",
			"cdmSts":"REA",
			"cdmData":{"ctot":"%s00","tsat":"%s","ttot":"%s"}
		}]`, times.Eobt, times.Tobt, times.Ctot, times.Tsat, times.Ttot)
	}))
	defer server.Close()

	var persisted *models.CdmData
	frontendHub := &testutil.MockFrontendHub{}
	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
				return []*models.CdmDataRow{{
					Callsign: callsign,
					Data: (&models.CdmData{
						Eobt: testStringPtr(times.Eobt),
					}).Normalize(),
				}}, nil
			},
			SetCdmDataFn: func(_ context.Context, session int32, gotCallsign string, data *models.CdmData) (int64, error) {
				if session != sessionID || gotCallsign != callsign {
					t.Fatalf("unexpected persistence target %d %s", session, gotCallsign)
				}
				persisted = data.Clone()
				return 1, nil
			},
			GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
				return persisted.Clone(), nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)
	setTestCdmFrontend(service, frontendHub)

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Name: "LIVE", Airport: "EKCH"})
	if err != nil {
		t.Fatalf("syncCdmData returned error: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected persisted CDM data")
	}
	if got := valueOrEmpty(persisted.Ctot); got != times.Ctot {
		t.Fatalf("expected nested CTOT fallback to persist %q, got %q", times.Ctot, got)
	}
	if len(frontendHub.CdmUpdates) != 1 {
		t.Fatalf("expected one frontend CTOT update, got %d", len(frontendHub.CdmUpdates))
	}
	if got := frontendHub.CdmUpdates[0].Ctot; got != times.Ctot {
		t.Fatalf("expected frontend CTOT update %q, got %q", times.Ctot, got)
	}
}

func TestSyncCdmData_ReturnsErrorWhenPersistSkipsRow(t *testing.T) {
	const sessionID = int32(81)
	const callsign = "SAS127"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ifps/depAirport" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{
			"callsign":"SAS127",
			"departure":"EKCH",
			"eobt":"1000",
			"tobt":"1010",
			"ctot":"1040",
			"cdmSts":"REA",
			"cdmData":{"tsat":"101500","ttot":"102500"}
		}]`))
	}))
	defer server.Close()

	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
				return []*models.CdmDataRow{{Callsign: callsign, Data: (&models.CdmData{}).Normalize()}}, nil
			},
			SetCdmDataFn: func(_ context.Context, _ int32, _ string, _ *models.CdmData) (int64, error) {
				return 0, nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Name: "LIVE", Airport: "EKCH"})
	if err == nil || !strings.Contains(err.Error(), "failed to persist CDM data") {
		t.Fatalf("expected persistence failure, got %v", err)
	}
}

func TestSyncCdmData_MasterSession_DoesNotSyncTsatFromAPI(t *testing.T) {
	const sessionID = int32(80)
	const callsign = "SAS130"
	times := futureMasterSyncTestTimes()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `[{
			"callsign":"SAS130",
			"departure":"EKCH",
			"eobt":%q,
			"tobt":%q,
			"ctot":"",
			"cdmSts":"STUP",
			"cdmData":{"tsat":%q,"ttot":%q}
		}]`, times.Eobt, times.Tobt, times.Tsat, times.Ttot)
	}))
	defer server.Close()

	var persisted *models.CdmData
	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
				return []*models.CdmDataRow{{
					Callsign: callsign,
					Data: (&models.CdmData{
						Eobt: testStringPtr(times.Eobt),
					}).Normalize(),
				}}, nil
			},
			GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
				return &models.Strip{Callsign: callsign, Origin: "EKCH", Runway: testStringPtr("22R")}, nil
			},
			SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
				persisted = data.Clone()
				return 1, nil
			},
			GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
				return persisted.Clone(), nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)
	// Mark session as CDM master — TSAT/TOBT/Status from API must be ignored.

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Name: "LIVE", Airport: "EKCH"})
	if err != nil {
		t.Fatalf("syncCdmData returned error: %v", err)
	}
	// No changes (no CTOT/REQTOBT in the API response), so nothing should be persisted.
	if persisted != nil {
		t.Fatalf("master session must not sync TSAT from API, but got persisted: %#v", persisted)
	}
}

func TestSyncCdmData_MasterSession_DoesNotExportStaleLocalTimesWhileRecalcPending(t *testing.T) {
	const sessionID = int32(83)
	const callsign = "SAS133"
	times := futureMasterSyncTestTimes()

	setCdmCh := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ifps/depAirport":
			_, _ = fmt.Fprintf(w, `[{
				"callsign":"SAS133",
				"departure":"EKCH",
				"eobt":%q,
				"tobt":%q,
				"ctot":%q,
				"cdmSts":"REA",
				"cdmData":{"reason":"REGUL"}
			}]`, times.Eobt, times.Tobt, times.Ctot)
		case "/ifps/setCdmData":
			setCdmCh <- struct{}{}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	var persisted *models.CdmData
	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
				return []*models.CdmDataRow{{
					Callsign: callsign,
					Data: (&models.CdmData{
						Eobt: testStringPtr(times.Eobt),
						Tobt: testStringPtr(times.Tobt),
						Tsat: testStringPtr(times.Tsat),
						Ttot: testStringPtr(times.Ttot),
					}).Normalize(),
				}}, nil
			},
			SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
				persisted = data.Clone()
				return 1, nil
			},
			GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
				return persisted.Clone(), nil
			},
			GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
				return &models.Strip{Callsign: callsign, Origin: "EKCH", Runway: testStringPtr("22R")}, nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Name: "LIVE", Airport: "EKCH"})
	if err != nil {
		t.Fatalf("syncCdmData returned error: %v", err)
	}
	if persisted == nil || !persisted.Recalculate {
		t.Fatalf("expected recalculation-pending state, got %#v", persisted)
	}

	select {
	case <-setCdmCh:
		t.Fatal("expected master sync to defer vIFF export until recalculation completes")
	case <-time.After(150 * time.Millisecond):
	}
}

func TestSyncCdmData_MasterSession_PushesLocalTimesToViffWhenApiDiffers(t *testing.T) {
	const sessionID = int32(81)
	const callsign = "SAS131"
	times := futureMasterSyncTestTimes()

	setCdmCh := make(chan url.Values, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ifps/depAirport":
			_, _ = fmt.Fprintf(w, `[{
				"callsign":"SAS131",
				"departure":"EKCH",
				"eobt":%q,
				"tobt":%q,
				"ctot":"",
				"cdmSts":"STUP",
				"cdmData":{"tsat":"","ttot":"","reason":""}
			}]`, times.Eobt, times.Tobt)
		case "/ifps/setCdmData":
			setCdmCh <- r.URL.Query()
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	local := (&models.CdmData{
		Eobt: testStringPtr(times.Eobt),
		Tobt: testStringPtr(times.Tobt),
		Tsat: testStringPtr(times.Tsat),
		Ttot: testStringPtr(times.Ttot),
	}).Normalize()

	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
				return []*models.CdmDataRow{{Callsign: callsign, Data: local.Clone()}}, nil
			},
			GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
				return &models.Strip{Callsign: callsign, Runway: testStringPtr("22R")}, nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Name: "LIVE", Airport: "EKCH"})
	if err != nil {
		t.Fatalf("syncCdmData returned error: %v", err)
	}

	select {
	case q := <-setCdmCh:
		if q.Get("callsign") != callsign || q.Get("tobt") != times.TobtSeconds || q.Get("tsat") != times.Tsat || q.Get("ttot") != times.Ttot || q.Get("depInfo") != "22R" {
			t.Fatalf("unexpected setCdmData payload: %v", q)
		}
	case <-time.After(time.Second):
		t.Fatal("expected master sync to push local CDM data to vIFF")
	}
}

func TestMarkViffPushPending_DeduplicatesAndAllowsRetryAfterClear(t *testing.T) {
	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithHTTPClient(newFailingHTTPClient())),
		&testutil.MockStripRepository{},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)
	state := viffPushState{
		Params: SetCdmDataParams{
			Callsign: "SAS251",
			Tsat:     "101500",
		},
	}

	if !service.markViffPushPending(1, "SAS251", state) {
		t.Fatal("expected first vIFF push state to be marked pending")
	}
	if service.markViffPushPending(1, "SAS251", state) {
		t.Fatal("expected duplicate vIFF push state to be deduplicated")
	}

	service.clearPendingViffPush(1, "SAS251", state)
	if !service.markViffPushPending(1, "SAS251", state) {
		t.Fatal("expected cleared vIFF push state to be eligible for retry")
	}
}
