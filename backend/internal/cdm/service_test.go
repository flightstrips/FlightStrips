package cdm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func newFailingHTTPClient() *http.Client {
	return &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("unexpected outbound HTTP request during test: %s %s", r.Method, r.URL.String())
		}),
	}
}

func newTestClientWithAirportMasters(masters []AirportMaster) *Client {
	client := NewClient(
		WithAPIKey("test-key"),
		WithHTTPClient(newFailingHTTPClient()),
		WithAirportMasterCacheTTL(time.Minute),
	)
	client.storeAirportMasters(time.Now(), masters)
	return client
}

func TestHandleReadyRequest_UsesBackendRequestFlow(t *testing.T) {
	const sessionID = int32(11)
	const callsign = "EZY456"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ifps/dpi", r.URL.Path)
		assert.Equal(t, callsign, r.URL.Query().Get("callsign"))
		value := r.URL.Query().Get("value")
		assert.True(t, strings.HasPrefix(value, "REQTOBT/"), "expected REQTOBT value, got %s", value)
		assert.True(t, strings.HasSuffix(value, "/ATC"), "expected /ATC suffix, got %s", value)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var persisted *models.CdmData
	stripRepo := &testutil.MockStripRepository{
		GetCdmDataForCallsignFn: func(_ context.Context, _ int32, _ string) (*models.CdmData, error) {
			return (&models.CdmData{}).Normalize(), nil
		},
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
			persisted = data.Clone()
			return 1, nil
		},
	}

	frontendHub := &testutil.MockFrontendHub{}
	euroscopeHub := &testutil.MockEuroscopeHub{}

	client := NewClient(
		WithAPIKey("test-key"),
		WithBaseURL(server.URL),
	)

	service := NewCdmService(
		client,
		stripRepo,
		&testutil.MockSessionRepository{
			GetByIDFn: func(context.Context, int32) (*models.Session, error) {
				t.Fatalf("HandleReadyRequest should not resolve a target master position")
				return nil, nil
			},
		},
		&testutil.MockControllerRepository{
			GetByCallsignFn: func(context.Context, int32, string) (*models.Controller, error) {
				t.Fatalf("HandleReadyRequest should not target an individual controller")
				return nil, nil
			},
		},
	)
	service.SetFrontendHub(frontendHub)
	service.SetEuroscopeHub(euroscopeHub)

	err := service.HandleReadyRequest(context.Background(), sessionID, callsign)
	require.NoError(t, err)

	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Status)
	assert.True(t, strings.HasPrefix(*persisted.Status, "REQTOBT/"))
	assert.True(t, strings.HasSuffix(*persisted.Status, "/ATC"))

	require.Len(t, frontendHub.CdmWaits, 1)
	assert.Equal(t, callsign, frontendHub.CdmWaits[0].Callsign)
}

func TestHandleReadyRequest_WithoutValidClient_DoesNothing(t *testing.T) {
	frontendHub := &testutil.MockFrontendHub{}
	euroscopeHub := &testutil.MockEuroscopeHub{}

	service := NewCdmService(newTestClientWithAirportMasters(nil), &testutil.MockStripRepository{}, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	service.client.isValid = false
	service.SetFrontendHub(frontendHub)
	service.SetEuroscopeHub(euroscopeHub)

	err := service.HandleReadyRequest(context.Background(), 7, "SAS123")
	require.NoError(t, err)

	assert.Empty(t, frontendHub.CdmWaits)
}

func stringPtr(value string) *string {
	return &value
}

func TestHandleTobtUpdate_PersistsOverrideAndClearsRequestedTobt(t *testing.T) {
	const sessionID = int32(31)
	const callsign = "EIN123"

	existing := &models.CdmData{
		Eobt:    stringPtr("1000"),
		Tobt:    stringPtr("1005"),
		ReqTobt: stringPtr("1030"),
	}

	var persisted *models.CdmData
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, session int32, cs string) (*models.Strip, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return &models.Strip{Callsign: cs, Origin: "EKCH"}, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, session int32, cs string) (*models.CdmData, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return existing.Clone(), nil
		},
		SetCdmDataFn: func(_ context.Context, session int32, cs string, data *models.CdmData) (int64, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			persisted = data.Clone()
			return 1, nil
		},
	}

	frontendHub := &testutil.MockFrontendHub{}
	service := NewCdmService(newTestClientWithAirportMasters(nil), stripRepo, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	service.client.isValid = false
	service.SetFrontendHub(frontendHub)

	err := service.HandleTobtUpdate(context.Background(), sessionID, callsign, "1035", "EKCH_B_GND", "master")
	require.NoError(t, err)

	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Tobt)
	assert.Equal(t, "1035", *persisted.Tobt)
	require.NotNil(t, persisted.TobtSetBy)
	assert.Equal(t, "EKCH_B_GND", *persisted.TobtSetBy)
	require.NotNil(t, persisted.TobtConfirmedBy)
	assert.Equal(t, models.TobtConfirmedByATC, *persisted.TobtConfirmedBy)
	assert.Nil(t, persisted.ReqTobt)
	assert.True(t, persisted.Recalculate)

	require.Len(t, frontendHub.CdmUpdates, 1)
	assert.Equal(t, "1035", frontendHub.CdmUpdates[0].Tobt)
}

func TestHandleManualCtot_BroadcastsEffectiveFrontendCtot(t *testing.T) {
	const sessionID = int32(43)
	const callsign = "SAS321"

	stored := (&models.CdmData{}).Normalize()
	frontendHub := &testutil.MockFrontendHub{}
	service := NewCdmService(
		newTestClientWithAirportMasters(nil),
		&testutil.MockStripRepository{
			GetByCallsignFn: func(_ context.Context, session int32, cs string) (*models.Strip, error) {
				assert.Equal(t, sessionID, session)
				assert.Equal(t, callsign, cs)
				return &models.Strip{Callsign: cs, Origin: "EKCH"}, nil
			},
			GetCdmDataForCallsignFn: func(_ context.Context, session int32, cs string) (*models.CdmData, error) {
				assert.Equal(t, sessionID, session)
				assert.Equal(t, callsign, cs)
				return stored.Clone(), nil
			},
			SetCdmDataFn: func(_ context.Context, session int32, cs string, data *models.CdmData) (int64, error) {
				assert.Equal(t, sessionID, session)
				assert.Equal(t, callsign, cs)
				stored = data.Clone()
				return 1, nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)
	service.SetFrontendHub(frontendHub)

	err := service.HandleManualCtot(context.Background(), sessionID, callsign, "1045")
	require.NoError(t, err)

	require.Len(t, frontendHub.CdmUpdates, 1)
	assert.Equal(t, "1045", frontendHub.CdmUpdates[0].Ctot)
	assert.True(t, stored.Recalculate)
	assert.NotNil(t, stored.Ctot)
	assert.Equal(t, "1045", *stored.Ctot)
	assert.NotNil(t, stored.CtotSource)
	assert.Equal(t, models.CtotSourceManual, *stored.CtotSource)
}

func TestSyncAsatForGroundState_SetsAndClearsCanonicalAsat(t *testing.T) {
	const sessionID = int32(42)
	const callsign = "SAS123"

	var stored = (&models.CdmData{}).Normalize()
	euroscopeHub := &testutil.MockEuroscopeHub{}
	service := NewCdmService(
		newTestClientWithAirportMasters(nil),
		&testutil.MockStripRepository{
			GetByCallsignFn: func(_ context.Context, session int32, cs string) (*models.Strip, error) {
				assert.Equal(t, sessionID, session)
				assert.Equal(t, callsign, cs)
				return &models.Strip{Callsign: cs, Origin: "EKCH"}, nil
			},
			GetCdmDataForCallsignFn: func(_ context.Context, session int32, cs string) (*models.CdmData, error) {
				assert.Equal(t, sessionID, session)
				assert.Equal(t, callsign, cs)
				return stored.Clone(), nil
			},
			SetCdmDataFn: func(_ context.Context, session int32, cs string, data *models.CdmData) (int64, error) {
				assert.Equal(t, sessionID, session)
				assert.Equal(t, callsign, cs)
				stored = data.Clone()
				return 1, nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)
	service.SetEuroscopeHub(euroscopeHub)

	err := service.SyncAsatForGroundState(context.Background(), sessionID, callsign, "PUSH")
	require.NoError(t, err)
	require.NotNil(t, stored.Asat)
	assert.Regexp(t, `^\d{4}$`, *stored.Asat)

	err = service.SyncAsatForGroundState(context.Background(), sessionID, callsign, "AIRB")
	require.NoError(t, err)
	assert.Nil(t, stored.Asat)

	require.Len(t, euroscopeHub.Broadcasts, 2)
	lastEvent, ok := euroscopeHub.Broadcasts[len(euroscopeHub.Broadcasts)-1].(euroscopeEvents.CdmUpdateEvent)
	require.True(t, ok)
	assert.Equal(t, "", lastEvent.Asat)
}

func TestPushTobt_UsesTaxiMinutesWithoutResolvingMasterPosition(t *testing.T) {
	const sessionID = int32(32)
	const callsign = "EIN456"

	var requestPath string
	var requestQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		requestQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("true"))
	}))
	defer server.Close()

	client := NewClient(
		WithAPIKey("test-key"),
		WithBaseURL(server.URL),
	)

	service := NewCdmService(
		client,
		&testutil.MockStripRepository{
			GetByCallsignFn: func(_ context.Context, session int32, cs string) (*models.Strip, error) {
				assert.Equal(t, sessionID, session)
				assert.Equal(t, callsign, cs)
				return &models.Strip{Callsign: cs, Origin: "EKCH", Runway: stringPtr("22R")}, nil
			},
		},
		&testutil.MockSessionRepository{
			GetByIDFn: func(context.Context, int32) (*models.Session, error) {
				t.Fatalf("PushTobt should not resolve an airport master position")
				return nil, nil
			},
		},
		&testutil.MockControllerRepository{},
	)

	service.SetConfigProvider(&stubConfigProvider{
		config: &CdmAirportConfig{
			Airport:            "EKCH",
			DefaultTaxiMinutes: 10,
			TaxiZones: []CdmTaxiZone{
				{Airport: "EKCH", Runway: "22R", Minutes: 12},
			},
		},
	})
	service.sessionMaster.Store(sessionID, true)

	err := service.PushTobt(context.Background(), sessionID, callsign, "1030")
	require.NoError(t, err)
	assert.Equal(t, "/ifps/dpi", requestPath)
	assert.Contains(t, requestQuery, "callsign=EIN456")
	assert.Contains(t, requestQuery, "value=TOBT%2F1030%2F12")
}

func TestSchedulePeriodicRecalculate_TriggersAllAirportSessions(t *testing.T) {
	const ekchSessionID = int32(7)
	const sweatboxSessionID = int32(8)

	sessions := map[int32]*models.Session{
		ekchSessionID:     {ID: ekchSessionID, Name: "LIVE", Airport: "EKCH"},
		sweatboxSessionID: {ID: sweatboxSessionID, Name: "SWEATBOX", Airport: "EKCH"},
	}

	var (
		mu           sync.Mutex
		recalculated = map[int32]int{}
	)

	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(_ context.Context, session int32, origin string) ([]*models.Strip, error) {
			mu.Lock()
			defer mu.Unlock()
			recalculated[session]++
			assert.Equal(t, "EKCH", origin)
			return nil, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		ListFn: func(context.Context) ([]*models.Session, error) {
			return []*models.Session{
				sessions[ekchSessionID],
				sessions[sweatboxSessionID],
				{ID: 9, Name: "PLAYBACK", Airport: ""},
			}, nil
		},
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			return sessions[id], nil
		},
	}

	configStore := NewCdmConfigStore("", "", "", 0, CdmConfigDefaults{}, nil)
	sequenceService := NewSequenceService(stripRepo, sessionRepo, configStore, &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})

	service := NewCdmService(newTestClientWithAirportMasters(nil), stripRepo, sessionRepo, &testutil.MockControllerRepository{})
	service.client.isValid = false
	service.debouncer = newRecalcDebouncer(time.Millisecond)
	service.SetSequenceService(sequenceService)

	// Mark both sessions as master so TriggerRecalculate proceeds.
	service.sessionMaster.Store(ekchSessionID, true)
	service.sessionMaster.Store(sweatboxSessionID, true)

	err := service.schedulePeriodicRecalculate(context.Background())
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return recalculated[ekchSessionID] == 1 && recalculated[sweatboxSessionID] == 1
	}, time.Second, 10*time.Millisecond)
}

func TestSetReady_SendsRea1ViaDpi(t *testing.T) {
	t.Parallel()
	const callsign = "BAW123"
	const sessionID = int32(5)

	var receivedValue string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ifps/dpi", r.URL.Path)
		receivedValue = r.URL.Query().Get("value")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	service := NewCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetCdmDataForCallsignFn: func(_ context.Context, _ int32, _ string) (*models.CdmData, error) {
				return (&models.CdmData{}).Normalize(), nil
			},
			SetCdmDataFn: func(_ context.Context, _ int32, _ string, _ *models.CdmData) (int64, error) {
				return 1, nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)

	service.SetFrontendHub(&testutil.MockFrontendHub{})
	service.sessionMaster.Store(sessionID, true)

	require.NoError(t, service.SetReady(context.Background(), sessionID, callsign))
	assert.Equal(t, "REA/1", receivedValue)
}

func TestHandleApproveReqTobt_ClearsReqTobtOnViff(t *testing.T) {
	t.Parallel()
	const callsign = "SAS456"
	const sessionID = int32(6)

	reqTobt := "1530"
	existing := &models.CdmData{
		Eobt:    stringPtr("1520"),
		ReqTobt: stringPtr(reqTobt),
	}

	clearedCh := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := r.URL.Query().Get("value")
		if value == "REQTOBT/NULL/NULL" {
			clearedCh <- value
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var persisted *models.CdmData
	service := NewCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
				return &models.Strip{Callsign: callsign, Origin: "EKCH"}, nil
			},
			GetCdmDataForCallsignFn: func(_ context.Context, _ int32, _ string) (*models.CdmData, error) {
				if persisted != nil {
					return persisted.Clone(), nil
				}
				return existing.Clone(), nil
			},
			SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
				persisted = data.Clone()
				return 1, nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)

	service.SetFrontendHub(&testutil.MockFrontendHub{})
	service.sessionMaster.Store(sessionID, true)

	require.NoError(t, service.HandleApproveReqTobt(context.Background(), sessionID, callsign, "GND", "master"))
	require.NotNil(t, persisted)
	assert.Equal(t, reqTobt, *persisted.Tobt)

	require.Eventually(t, func() bool {
		select {
		case v := <-clearedCh:
			return v == "REQTOBT/NULL/NULL"
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond, "expected REQTOBT/NULL/NULL to be sent to vIFF")
}

func TestSyncAsatForGroundState_SetsAobtLocallyAndPushesToViff(t *testing.T) {
	t.Parallel()
	const callsign = "EZY789"
	const sessionID = int32(8)

	var stored = (&models.CdmData{}).Normalize()
	aobtCh := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := r.URL.Query().Get("value")
		if strings.HasPrefix(value, "AOBT/") {
			aobtCh <- value
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	service := NewCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
				return &models.Strip{Callsign: callsign, Origin: "EKCH"}, nil
			},
			GetCdmDataForCallsignFn: func(_ context.Context, _ int32, _ string) (*models.CdmData, error) {
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

	service.SetEuroscopeHub(&testutil.MockEuroscopeHub{})
	service.sessionMaster.Store(sessionID, true)

	// Ground state → PUSH: ASAT and AOBT should be set
	require.NoError(t, service.SyncAsatForGroundState(context.Background(), sessionID, callsign, "PUSH"))
	require.NotNil(t, stored.Asat)
	require.NotNil(t, stored.Aobt)
	assert.Regexp(t, `^\d{4}$`, *stored.Aobt)

	require.Eventually(t, func() bool {
		select {
		case v := <-aobtCh:
			return strings.HasPrefix(v, "AOBT/") && v != "AOBT/NULL"
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond, "expected AOBT/<time> to be sent")

	// Ground state → airborne: AOBT should be cleared
	require.NoError(t, service.SyncAsatForGroundState(context.Background(), sessionID, callsign, "AIRB"))
	assert.Nil(t, stored.Aobt)

	require.Eventually(t, func() bool {
		select {
		case v := <-aobtCh:
			return v == "AOBT/NULL"
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond, "expected AOBT/NULL to be sent")
}

func TestPushViffAfterRecalcAsync_SendsSetCdmDataWhenTsatPresent(t *testing.T) {
	t.Parallel()

	setCdmCh := make(chan url.Values, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ifps/setCdmData" {
			setCdmCh <- r.URL.Query()
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tobt := "1525"
	tsat := "1530"
	ttot := "1540"
	ecfmp := "REGUL"
	data := &models.CdmData{
		Tobt:    stringPtr(tobt),
		Tsat:    stringPtr(tsat),
		Ttot:    stringPtr(ttot),
		EcfmpID: stringPtr(ecfmp),
	}

	service := NewCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		nil, nil, nil,
	)


	service.pushViffAfterRecalcAsync("BAW999", nil, data)

	require.Eventually(t, func() bool {
		select {
		case q := <-setCdmCh:
			return q.Get("tobt") == tobt && q.Get("tsat") == tsat && q.Get("ttot") == ttot && q.Get("reason") == ecfmp
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
}

func TestPushViffAfterRecalcAsync_SendsSuspWhenPhaseInvalid(t *testing.T) {
	t.Parallel()

	suspCh := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ifps/dpi" && r.URL.Query().Get("value") == "SUSP" {
			suspCh <- "SUSP"
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	phase := "I"
	data := &models.CdmData{Phase: stringPtr(phase)}

	service := NewCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		nil, nil, nil,
	)


	service.pushViffAfterRecalcAsync("EIN001", nil, data)

	require.Eventually(t, func() bool {
		select {
		case v := <-suspCh:
			return v == "SUSP"
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
}

type stubConfigProvider struct {
	config *CdmAirportConfig
}

func (s *stubConfigProvider) ConfigForAirport(string) *CdmAirportConfig {
	return s.config
}

func (s *stubConfigProvider) SetLvo(string, bool) {}

func (s *stubConfigProvider) SetDelay(CdmDelay) {}

func (s *stubConfigProvider) ClearDelay(string, string) {}

// ---- SetSessionCdmMaster ----

func TestSetSessionCdmMaster_True_UpdatesDBAndCachesAndRegistersMaster(t *testing.T) {
	const sessionID = int32(77)
	const airport = "EKCH"

	var dbUpdatedID int32
	var dbUpdatedMaster bool
	var masterCallPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		masterCallPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sessionRepo := &testutil.MockSessionRepository{
		UpdateCdmMasterFn: func(_ context.Context, id int32, master bool) error {
			dbUpdatedID = id
			dbUpdatedMaster = master
			return nil
		},
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Name: "LIVE", Airport: airport}, nil
		},
	}

	client := NewClient(WithAPIKey("test-key"), WithBaseURL(srv.URL))
	service := NewCdmService(client, &testutil.MockStripRepository{}, sessionRepo, &testutil.MockControllerRepository{})


	err := service.SetSessionCdmMaster(context.Background(), sessionID, true)
	require.NoError(t, err)

	assert.Equal(t, sessionID, dbUpdatedID, "UpdateCdmMaster called with wrong session ID")
	assert.True(t, dbUpdatedMaster, "UpdateCdmMaster called with master=false")

	v, ok := service.sessionMaster.Load(sessionID)
	assert.True(t, ok && v.(bool), "sessionMaster map should have true for session")

	// Allow the async goroutine to call the HTTP endpoint.
	require.Eventually(t, func() bool {
		return masterCallPath != ""
	}, time.Second, 10*time.Millisecond, "expected master registration HTTP call")
}

func TestSetSessionCdmMaster_False_UpdatesDBAndRemovesFromCache(t *testing.T) {
	const sessionID = int32(78)
	const airport = "EKCH"

	var dbUpdatedMaster *bool
	type clearCall struct {
		path     string
		position string
	}
	var gotClearCall *clearCall

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClearCall = &clearCall{
			path:     r.URL.Path,
			position: r.URL.Query().Get("position"),
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sessionRepo := &testutil.MockSessionRepository{
		UpdateCdmMasterFn: func(_ context.Context, _ int32, master bool) error {
			dbUpdatedMaster = &master
			return nil
		},
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Name: "LIVE", Airport: airport}, nil
		},
	}

	client := NewClient(WithAPIKey("test-key"), WithBaseURL(srv.URL))
	service := NewCdmService(client, &testutil.MockStripRepository{}, sessionRepo, &testutil.MockControllerRepository{})
	// Pre-populate the cache as if the session was previously master.
	service.sessionMaster.Store(sessionID, true)

	err := service.SetSessionCdmMaster(context.Background(), sessionID, false)
	require.NoError(t, err)

	require.NotNil(t, dbUpdatedMaster)
	assert.False(t, *dbUpdatedMaster, "UpdateCdmMaster should be called with master=false")

	_, ok := service.sessionMaster.Load(sessionID)
	assert.False(t, ok, "sessionMaster map entry should have been removed")

	// The async clear call should reach the server with the FlightStrips position.
	require.Eventually(t, func() bool {
		return gotClearCall != nil
	}, time.Second, 10*time.Millisecond, "expected ClearMasterAirport HTTP call")
	assert.Equal(t, "/airport/clearMaster", gotClearCall.path)
	assert.Equal(t, DefaultMasterPosition, gotClearCall.position, "clearMaster must pass the FlightStrips position")
}

// ---- TriggerRecalculate per-session master ----

func TestTriggerRecalculate_SkipsNonMasterSession(t *testing.T) {
	listByOriginCalled := false
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(_ context.Context, _ int32, _ string) ([]*models.Strip, error) {
			listByOriginCalled = true
			return nil, nil
		},
	}

	seqSvc := NewSequenceService(stripRepo, &testutil.MockSessionRepository{}, &stubConfigProvider{}, &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})
	service := NewCdmService(newTestClientWithAirportMasters(nil), stripRepo, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})

	service.debouncer = newRecalcDebouncer(time.Millisecond)
	service.SetSequenceService(seqSvc)

	// Session 99 is NOT in the sessionMaster map.
	service.TriggerRecalculate(context.Background(), 99, "EKCH")

	// Give debouncer time to fire (if it were going to).
	time.Sleep(50 * time.Millisecond)
	assert.False(t, listByOriginCalled, "RecalculateAirport should not be called for non-master session")
}

func TestTriggerRecalculate_RunsForMasterSession(t *testing.T) {
	listByOriginCalled := make(chan struct{}, 1)
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(_ context.Context, _ int32, _ string) ([]*models.Strip, error) {
			select {
			case listByOriginCalled <- struct{}{}:
			default:
			}
			return nil, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Airport: "EKCH"}, nil
		},
	}

	seqSvc := NewSequenceService(stripRepo, sessionRepo, &stubConfigProvider{}, &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})
	service := NewCdmService(newTestClientWithAirportMasters(nil), stripRepo, sessionRepo, &testutil.MockControllerRepository{})

	service.debouncer = newRecalcDebouncer(time.Millisecond)
	service.SetSequenceService(seqSvc)

	const sessionID = int32(55)
	service.sessionMaster.Store(sessionID, true)

	service.TriggerRecalculate(context.Background(), sessionID, "EKCH")

	select {
	case <-listByOriginCalled:
		// success — RecalculateAirport was invoked
	case <-time.After(time.Second):
		t.Fatal("expected RecalculateAirport to be called for master session")
	}
}

// ---- syncLiveSessions ----

func TestSyncLiveSessions_RegistersMasterForCdmMasterSessions(t *testing.T) {
	var masterCallPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/airport/setMaster" {
			masterCallPath = r.URL.Path
		}
		// Return empty JSON array for IFPS endpoint.
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer srv.Close()

	const sessionID = int32(10)

	sessionRepo := &testutil.MockSessionRepository{
		ListFn: func(_ context.Context) ([]*models.Session, error) {
			return []*models.Session{
				{ID: sessionID, Name: "LIVE", Airport: "EKCH", CdmMaster: true},
			}, nil
		},
	}

	stripRepo := &testutil.MockStripRepository{
		GetCdmDataFn: func(_ context.Context, _ int32) ([]*models.CdmDataRow, error) {
			return nil, nil
		},
	}
	client := NewClient(WithAPIKey("test-key"), WithBaseURL(srv.URL))
	service := NewCdmService(client, stripRepo, sessionRepo, &testutil.MockControllerRepository{})
	service.SetFrontendHub(&testutil.MockFrontendHub{})
	service.SetEuroscopeHub(&testutil.MockEuroscopeHub{})

	err := service.syncLiveSessions(context.Background())
	require.NoError(t, err)

	v, ok := service.sessionMaster.Load(sessionID)
	assert.True(t, ok && v.(bool), "sessionMaster should be populated for CdmMaster session")

	require.Eventually(t, func() bool {
		return masterCallPath != ""
	}, time.Second, 10*time.Millisecond, "expected master registration HTTP call")
}

func TestSyncLiveSessions_DoesNotRegisterMasterForSlaveSession(t *testing.T) {
	const sessionID = int32(11)

	sessionRepo := &testutil.MockSessionRepository{
		ListFn: func(_ context.Context) ([]*models.Session, error) {
			return []*models.Session{
				{ID: sessionID, Name: "LIVE", Airport: "EKCH", CdmMaster: false},
			}, nil
		},
	}

	client := NewClient(WithAPIKey("test-key"), WithHTTPClient(newFailingHTTPClient()))
	service := NewCdmService(client, &testutil.MockStripRepository{}, sessionRepo, &testutil.MockControllerRepository{})
	// Disable HTTP calls so syncCdmData exits early (client must have isValid=false to skip the vIFF sync).
	service.client.isValid = false
	service.SetFrontendHub(&testutil.MockFrontendHub{})
	service.SetEuroscopeHub(&testutil.MockEuroscopeHub{})

	err := service.syncLiveSessions(context.Background())
	require.NoError(t, err)

	_, ok := service.sessionMaster.Load(sessionID)
	assert.False(t, ok, "sessionMaster should not be populated for slave session")
}
