package cdm

import (
	"context"
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

func TestSyncCdmData_PersistsFlowMessageAndReqTobtSource(t *testing.T) {
	const sessionID = int32(77)
	const callsign = "SAS123"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ifps/depAirport" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{
			"callsign":"SAS123",
			"departure":"EKCH",
			"eobt":"1000",
			"tobt":"1010",
			"ctot":"1040",
			"cdmSts":"REA",
			"cdmData":{
				"reqTobt":"1005",
				"reqTobtType":"PILOT",
				"reqAsrt":"100700",
				"tsat":"101500",
				"ttot":"102500",
				"reason":"REGUL"
			}
		}]`))
	}))
	defer server.Close()

	existing := (&models.CdmData{}).Normalize()
	var persisted *models.CdmData
	euroscopeHub := &testutil.MockEuroscopeHub{}
	service := NewCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
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
	service.SetEuroscopeHub(euroscopeHub)

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Airport: "EKCH"})
	if err != nil {
		t.Fatalf("syncCdmData returned error: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected persisted CDM data")
	}
	if got := valueOrEmpty(persisted.ReqTobt); got != "1005" {
		t.Fatalf("expected req_tobt to be persisted, got %q", got)
	}
	if got := valueOrEmpty(persisted.EcfmpID); got != "REGUL" {
		t.Fatalf("expected flow message to be persisted, got %q", got)
	}
	if len(euroscopeHub.Broadcasts) != 1 {
		t.Fatalf("expected one EuroScope broadcast, got %d", len(euroscopeHub.Broadcasts))
	}

	event, ok := euroscopeHub.Broadcasts[0].(euroscopeEvents.CdmUpdateEvent)
	if !ok {
		t.Fatalf("expected CdmUpdateEvent broadcast, got %T", euroscopeHub.Broadcasts[0])
	}
	if event.EcfmpID != "REGUL" {
		t.Fatalf("unexpected broadcast metadata: %#v", event)
	}
}

func TestSyncCdmData_PreservesExistingAsat(t *testing.T) {
	const sessionID = int32(78)
	const callsign = "SAS124"
	asat := "1031"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ifps/depAirport" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{
			"callsign":"SAS124",
			"departure":"EKCH",
			"eobt":"1000",
			"tobt":"1010",
			"ctot":"1040",
			"cdmSts":"REA",
			"cdmData":{"tsat":"101500","ttot":"102500"}
		}]`))
	}))
	defer server.Close()

	existing := &models.CdmData{Asat: &asat}
	var persisted *models.CdmData
	service := NewCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
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

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Airport: "EKCH"})
	if err != nil {
		t.Fatalf("syncCdmData returned error: %v", err)
	}
	if persisted == nil || valueOrEmpty(persisted.Asat) != asat {
		t.Fatalf("expected ASAT %q to be preserved, got %#v", asat, persisted)
	}
}

func TestSyncCdmData_UsesNestedCtotForFrontendUpdate(t *testing.T) {
	const sessionID = int32(80)
	const callsign = "SAS126"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ifps/depAirport" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{
			"callsign":"SAS126",
			"departure":"EKCH",
			"eobt":"1000",
			"tobt":"1010",
			"ctot":"",
			"cdmSts":"REA",
			"cdmData":{"ctot":"104500","tsat":"101500","ttot":"102500"}
		}]`))
	}))
	defer server.Close()

	var persisted *models.CdmData
	frontendHub := &testutil.MockFrontendHub{}
	service := NewCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
				return []*models.CdmDataRow{{Callsign: callsign, Data: (&models.CdmData{}).Normalize()}}, nil
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
	service.SetFrontendHub(frontendHub)

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Airport: "EKCH"})
	if err != nil {
		t.Fatalf("syncCdmData returned error: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected persisted CDM data")
	}
	if got := valueOrEmpty(persisted.Ctot); got != "1045" {
		t.Fatalf("expected nested CTOT fallback to persist %q, got %q", "1045", got)
	}
	if len(frontendHub.CdmUpdates) != 1 {
		t.Fatalf("expected one frontend CTOT update, got %d", len(frontendHub.CdmUpdates))
	}
	if got := frontendHub.CdmUpdates[0].Ctot; got != "1045" {
		t.Fatalf("expected frontend CTOT update %q, got %q", "1045", got)
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

	service := NewCdmService(
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

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Airport: "EKCH"})
	if err == nil || !strings.Contains(err.Error(), "failed to persist CDM data") {
		t.Fatalf("expected persistence failure, got %v", err)
	}
}

func TestSyncCdmData_SlaveSession_StartupStatusInitializesAsat(t *testing.T) {
	const sessionID = int32(79)
	const callsign = "SAS125"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ifps/depAirport" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{
			"callsign":"SAS125",
			"departure":"EKCH",
			"eobt":"1000",
			"tobt":"1010",
			"ctot":"1040",
			"cdmSts":"STUP",
			"cdmData":{"reqAsrt":"100500","tsat":"101500","ttot":"102500"}
		}]`))
	}))
	defer server.Close()

	var persisted *models.CdmData
	euroscopeHub := &testutil.MockEuroscopeHub{}
	service := NewCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
				return []*models.CdmDataRow{{Callsign: callsign, Data: (&models.CdmData{}).Normalize()}}, nil
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
	service.SetEuroscopeHub(euroscopeHub)
	// Session is NOT master — full API sync applies.

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Airport: "EKCH"})
	if err != nil {
		t.Fatalf("syncCdmData returned error: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected persisted CDM data")
	}
	if got := valueOrEmpty(persisted.Asat); got == "" {
		t.Fatalf("expected ASAT to be initialized for slave session, got %#v", persisted)
	}
	if persisted.Calculation != nil {
		t.Fatalf("expected slave sync to clear local calculation snapshot, got %#v", persisted.Calculation)
	}
	if len(euroscopeHub.Broadcasts) != 1 {
		t.Fatalf("expected one EuroScope broadcast, got %d", len(euroscopeHub.Broadcasts))
	}
	event, ok := euroscopeHub.Broadcasts[0].(euroscopeEvents.CdmUpdateEvent)
	if !ok {
		t.Fatalf("expected CdmUpdateEvent broadcast, got %T", euroscopeHub.Broadcasts[0])
	}
	if event.Asat == "" {
		t.Fatalf("unexpected startup broadcast payload: %#v", event)
	}
}

func TestSyncCdmData_SlaveSession_ClearsPersistedCalculationSnapshot(t *testing.T) {
	const sessionID = int32(81)
	const callsign = "SAS131"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{
			"callsign":"SAS131",
			"departure":"EKCH",
			"eobt":"1000",
			"tobt":"1010",
			"ctot":"1040",
			"cdmSts":"REA",
			"cdmData":{"tsat":"101500","ttot":"102500"}
		}]`))
	}))
	defer server.Close()

	minutes := 14
	runway := "22R"
	existing := (&models.CdmData{
		Calculation: &models.CdmCalculation{
			TaxiMinutes: &minutes,
			TaxiRunway:  &runway,
		},
	}).Normalize()

	var persisted *models.CdmData
	service := NewCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
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

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Airport: "EKCH"})
	if err != nil {
		t.Fatalf("syncCdmData returned error: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected persisted CDM data")
	}
	if persisted.Calculation != nil {
		t.Fatalf("expected slave sync to drop local calculation snapshot, got %#v", persisted.Calculation)
	}
}

func TestSyncCdmData_MasterSession_DoesNotSyncTsatFromAPI(t *testing.T) {
	const sessionID = int32(80)
	const callsign = "SAS130"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{
			"callsign":"SAS130",
			"departure":"EKCH",
			"eobt":"1000",
			"tobt":"1010",
			"ctot":"",
			"cdmSts":"STUP",
			"cdmData":{"tsat":"101500","ttot":"102500"}
		}]`))
	}))
	defer server.Close()

	var persisted *models.CdmData
	service := NewCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
				return []*models.CdmDataRow{{Callsign: callsign, Data: (&models.CdmData{}).Normalize()}}, nil
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
	service.sessionMaster.Store(sessionID, true)

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Airport: "EKCH"})
	if err != nil {
		t.Fatalf("syncCdmData returned error: %v", err)
	}
	// No changes (no CTOT/REQTOBT in the API response), so nothing should be persisted.
	if persisted != nil {
		t.Fatalf("master session must not sync TSAT from API, but got persisted: %#v", persisted)
	}
}

func TestSyncCdmData_MasterSession_MarksRecalculationForReqTobtAndCtotChanges(t *testing.T) {
	const sessionID = int32(82)
	const callsign = "SAS132"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{
			"callsign":"SAS132",
			"departure":"EKCH",
			"eobt":"1000",
			"tobt":"1010",
			"ctot":"1040",
			"cdmSts":"REA",
			"cdmData":{"reqTobt":"1005","reason":"REGUL"}
		}]`))
	}))
	defer server.Close()

	var persisted *models.CdmData
	service := NewCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
				return []*models.CdmDataRow{{
					Callsign: callsign,
					Data: (&models.CdmData{
						Tobt: testStringPtr("1010"),
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
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)
	service.sessionMaster.Store(sessionID, true)

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Airport: "EKCH"})
	if err != nil {
		t.Fatalf("syncCdmData returned error: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected persisted CDM data")
	}
	if got := valueOrEmpty(persisted.Ctot); got != "1040" {
		t.Fatalf("expected CTOT %q, got %q", "1040", got)
	}
	if got := valueOrEmpty(persisted.ReqTobt); got != "1005" {
		t.Fatalf("expected req_tobt %q, got %q", "1005", got)
	}
	if !persisted.Recalculate {
		t.Fatalf("expected master sync to mark recalculation pending, got %#v", persisted)
	}
}

func TestSyncCdmData_MasterSession_DoesNotExportStaleLocalTimesWhileRecalcPending(t *testing.T) {
	const sessionID = int32(83)
	const callsign = "SAS133"

	setCdmCh := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ifps/depAirport":
			_, _ = w.Write([]byte(`[{
				"callsign":"SAS133",
				"departure":"EKCH",
				"eobt":"1000",
				"tobt":"1010",
				"ctot":"1040",
				"cdmSts":"REA",
				"cdmData":{"reqTobt":"1005","reason":"REGUL"}
			}]`))
		case "/ifps/setCdmData":
			setCdmCh <- struct{}{}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	var persisted *models.CdmData
	service := NewCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetCdmDataFn: func(context.Context, int32) ([]*models.CdmDataRow, error) {
				return []*models.CdmDataRow{{
					Callsign: callsign,
					Data: (&models.CdmData{
						Tobt: testStringPtr("1010"),
						Tsat: testStringPtr("1015"),
						Ttot: testStringPtr("1025"),
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
	service.sessionMaster.Store(sessionID, true)

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Airport: "EKCH"})
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

	setCdmCh := make(chan url.Values, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ifps/depAirport":
			_, _ = w.Write([]byte(`[{
				"callsign":"SAS131",
				"departure":"EKCH",
				"eobt":"1000",
				"tobt":"1010",
				"ctot":"",
				"cdmSts":"STUP",
				"cdmData":{"tsat":"","ttot":"","reason":""}
			}]`))
		case "/ifps/setCdmData":
			setCdmCh <- r.URL.Query()
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	local := (&models.CdmData{
		Tobt: testStringPtr("1010"),
		Tsat: testStringPtr("101500"),
		Ttot: testStringPtr("102500"),
	}).Normalize()

	service := NewCdmService(
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
	service.sessionMaster.Store(sessionID, true)

	err := service.syncCdmData(context.Background(), &models.Session{ID: sessionID, Airport: "EKCH"})
	if err != nil {
		t.Fatalf("syncCdmData returned error: %v", err)
	}

	select {
	case q := <-setCdmCh:
		if q.Get("callsign") != callsign || q.Get("tobt") != "101000" || q.Get("tsat") != "101500" || q.Get("ttot") != "102500" || q.Get("depInfo") != "22R" {
			t.Fatalf("unexpected setCdmData payload: %v", q)
		}
	case <-time.After(time.Second):
		t.Fatal("expected master sync to push local CDM data to vIFF")
	}
}
