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
	pkgModels "FlightStrips/pkg/models"

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

func markSessionLive(service *Service, sessionID int32) {
	service.sessionUsesViff.Store(sessionID, true)
}

func markSessionNonLive(service *Service, sessionID int32) {
	service.sessionUsesViff.Store(sessionID, false)
}

func TestHandleReadyRequest_UsesReadyFlow(t *testing.T) {
	const sessionID = int32(11)
	const callsign = "EZY456"
	var (
		requestMu     sync.Mutex
		requestValues []string
		readCount     int
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ifps/dpi":
			assert.Equal(t, callsign, r.URL.Query().Get("callsign"))
			requestMu.Lock()
			requestValues = append(requestValues, r.URL.Query().Get("value"))
			requestMu.Unlock()
			w.WriteHeader(http.StatusOK)
		case "/ifps/callsign":
			assert.Equal(t, callsign, r.URL.Query().Get("callsign"))
			requestMu.Lock()
			readCount++
			requestMu.Unlock()
			_, _ = w.Write([]byte(`{"callsign":"EZY456","departure":"EKCH","ctot":"1040","cdmData":{"reason":"REGUL"}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	var persisted *models.CdmData
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{Callsign: callsign, Origin: "EKCH"}, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, _ int32, _ string) (*models.CdmData, error) {
			if persisted != nil {
				return persisted.Clone(), nil
			}
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

	service := newTestCdmService(
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
	setTestCdmFrontend(service, frontendHub)
	setTestCdmEuroscope(service, euroscopeHub)
	service.sessionMaster.Store(sessionID, true)
	markSessionLive(service, sessionID)
	err := service.HandleReadyRequest(context.Background(), sessionID, callsign, "EKCH_DEL", "ATC")
	require.NoError(t, err)

	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Status)
	assert.Equal(t, "REA", *persisted.Status)
	require.NotNil(t, persisted.Eobt)
	assert.Equal(t, *persisted.Tobt, *persisted.Eobt)
	require.NotNil(t, persisted.Tobt)
	require.NotNil(t, persisted.Ctot)
	assert.Equal(t, "1040", *persisted.Ctot)
	require.Eventually(t, func() bool {
		requestMu.Lock()
		defer requestMu.Unlock()
		return len(requestValues) == 2 && readCount == 1
	}, time.Second, 10*time.Millisecond)
	requestMu.Lock()
	assert.Contains(t, requestValues, "REA/1")
	assert.True(t, strings.HasPrefix(requestValues[0], "TOBT/") || strings.HasPrefix(requestValues[1], "TOBT/"))
	requestMu.Unlock()

	require.Len(t, frontendHub.CdmUpdates, 3)
	assert.Equal(t, callsign, frontendHub.CdmUpdates[len(frontendHub.CdmUpdates)-1].Callsign)
	assert.Equal(t, "REA", frontendHub.CdmUpdates[len(frontendHub.CdmUpdates)-1].Event.Status)
	assert.Equal(t, "1040", frontendHub.CdmUpdates[len(frontendHub.CdmUpdates)-1].Ctot)
	require.Len(t, frontendHub.CdmWaits, 1)
	assert.Equal(t, callsign, frontendHub.CdmWaits[0].Callsign)
	require.NotEmpty(t, euroscopeHub.Broadcasts)
	foundEobtBroadcast := false
	for _, message := range euroscopeHub.Broadcasts {
		event, ok := message.(euroscopeEvents.EobtEvent)
		if !ok {
			continue
		}
		if event.Callsign == callsign && event.Eobt == *persisted.Eobt {
			foundEobtBroadcast = true
			break
		}
	}
	assert.True(t, foundEobtBroadcast, "expected EOBT event to be broadcast to EuroScope")
}

func TestHandleReadyRequest_SendsEobtToResolvedEuroscopeMaster(t *testing.T) {
	const sessionID = int32(12)
	const callsign = "EZY457"
	const masterCallsign = "EKCH_M_TWR"
	const masterCid = "10000005"
	currentTobt := time.Now().UTC().Add(45 * time.Minute).Format("1504")
	futureEobt := time.Now().UTC().Add(60 * time.Minute).Format("1504")

	var persisted *models.CdmData
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{Callsign: callsign, Origin: "EKCH"}, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, _ int32, _ string) (*models.CdmData, error) {
			if persisted != nil {
				return persisted.Clone(), nil
			}
			return (&models.CdmData{Eobt: stringPtr(futureEobt), Tobt: stringPtr(currentTobt)}).Normalize(), nil
		},
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
			persisted = data.Clone()
			return 1, nil
		},
	}

	frontendHub := &testutil.MockFrontendHub{}
	euroscopeHub := &testutil.MockEuroscopeHub{
		GetMasterCallsignFn: func(session int32) string {
			assert.Equal(t, sessionID, session)
			return masterCallsign
		},
	}
	controllerRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, session int32, callsign string) (*models.Controller, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, masterCallsign, callsign)
			return &models.Controller{Cid: stringPtr(masterCid)}, nil
		},
	}

	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, &testutil.MockSessionRepository{}, controllerRepo)
	setTestCdmFrontend(service, frontendHub)
	setTestCdmEuroscope(service, euroscopeHub)
	service.sessionMaster.Store(sessionID, true)
	markSessionNonLive(service, sessionID)

	err := service.HandleReadyRequest(context.Background(), sessionID, callsign, "EKCH_DEL", "ATC")
	require.NoError(t, err)

	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Eobt)
	require.Len(t, euroscopeHub.Eobts, 1)
	assert.Equal(t, sessionID, euroscopeHub.Eobts[0].Session)
	assert.Equal(t, masterCid, euroscopeHub.Eobts[0].Cid)
	assert.Equal(t, callsign, euroscopeHub.Eobts[0].Callsign)
	assert.Equal(t, *persisted.Eobt, euroscopeHub.Eobts[0].Eobt)
}

func TestHandleReadyRequest_WithoutValidClient_StillRecalculatesLocally(t *testing.T) {
	const callsign = "SAS123"
	const sessionID = int32(7)
	runway := "22R"
	nowTobt := time.Now().UTC().Format("1504")
	phase := "I"

	stored := (&models.CdmData{
		Tobt:  stringPtr(nowTobt),
		Phase: stringPtr(phase),
	}).Normalize()

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: callsign,
				Session:  sessionID,
				Origin:   "EKCH",
				Runway:   &runway,
				CdmData:  stored.Clone(),
			}, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, _ int32, _ string) (*models.CdmData, error) {
			return stored.Clone(), nil
		},
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
			stored = data.Clone()
			return 1, nil
		},
		ListByOriginFn: func(_ context.Context, _ int32, origin string) ([]*models.Strip, error) {
			assert.Equal(t, "EKCH", origin)
			return []*models.Strip{{
				Callsign: callsign,
				Session:  sessionID,
				Origin:   "EKCH",
				Runway:   &runway,
				CdmData:  stored.Clone(),
			}}, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			assert.Equal(t, sessionID, id)
			return &models.Session{
				ID: id,
				ActiveRunways: pkgModels.ActiveRunways{
					DepartureRunways: []string{runway},
				},
			}, nil
		},
	}

	frontendHub := &testutil.MockFrontendHub{}
	euroscopeHub := &testutil.MockEuroscopeHub{}
	configStore := NewCdmConfigStore("", "", "", time.Minute, CdmConfigDefaults{}, newFailingHTTPClient())
	sequenceService := newTestSequenceService(stripRepo, sessionRepo, configStore, frontendHub, euroscopeHub)

	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, sessionRepo, &testutil.MockControllerRepository{})
	service.client.isValid = false
	setTestCdmFrontend(service, frontendHub)
	setTestCdmEuroscope(service, euroscopeHub)
	service.SetConfigProvider(configStore)
	service.SetSequenceService(sequenceService)
	service.sessionMaster.Store(sessionID, true)

	err := service.HandleReadyRequest(context.Background(), sessionID, callsign, "EKCH_DEL", "ATC")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return stored.Tsat != nil &&
			*stored.Tsat != "" &&
			stored.Status != nil &&
			*stored.Status == "REA" &&
			(stored.Phase == nil || *stored.Phase == "") &&
			!stored.NeedsLocalRecalculation()
	}, time.Second, 10*time.Millisecond)
	require.Len(t, frontendHub.CdmWaits, 1)
	assert.Equal(t, callsign, frontendHub.CdmWaits[0].Callsign)
}

func stringPtr(value string) *string {
	return &value
}

func TestHandleTobtUpdate_PersistsOverrideAndClearsRequestedTobt(t *testing.T) {
	const sessionID = int32(31)
	const callsign = "EIN123"
	updatedTobt := time.Now().UTC().Add(20 * time.Minute).Format("1504")

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
	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	service.client.isValid = false
	setTestCdmFrontend(service, frontendHub)

	err := service.HandleTobtUpdate(context.Background(), sessionID, callsign, updatedTobt, "EKCH_B_GND", "master")
	require.NoError(t, err)

	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Tobt)
	assert.Equal(t, updatedTobt, *persisted.Tobt)
	require.NotNil(t, persisted.TobtSetBy)
	assert.Equal(t, "EKCH_B_GND", *persisted.TobtSetBy)
	require.NotNil(t, persisted.TobtConfirmedBy)
	assert.Equal(t, models.TobtConfirmedByATC, *persisted.TobtConfirmedBy)
	assert.True(t, persisted.TobtManuallyConfirmed)
	assert.False(t, persisted.TobtAutoSynced)
	assert.Nil(t, persisted.ReqTobt)
	assert.True(t, persisted.Recalculate)

	require.Len(t, frontendHub.CdmUpdates, 1)
	assert.Equal(t, updatedTobt, frontendHub.CdmUpdates[0].Tobt)
}

func TestHandleTobtUpdate_PastValueDoesNotMarkRecalculation(t *testing.T) {
	const sessionID = int32(142)
	const callsign = "SAS124"

	pastTobt := time.Now().UTC().Add(-20 * time.Minute).Format("1504")
	currentTobt := "1030"
	tsat := time.Now().UTC().Add(20 * time.Minute).Format("1504")

	var persisted *models.CdmData
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, session int32, cs string) (*models.Strip, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return &models.Strip{Callsign: cs, Session: sessionID, Origin: "EKCH"}, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, session int32, cs string) (*models.CdmData, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return (&models.CdmData{
				Tobt: &currentTobt,
				Tsat: &tsat,
			}).Normalize(), nil
		},
		SetCdmDataFn: func(_ context.Context, session int32, cs string, data *models.CdmData) (int64, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			persisted = data.Clone()
			return 1, nil
		},
	}

	frontendHub := &testutil.MockFrontendHub{}
	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	service.client.isValid = false
	setTestCdmFrontend(service, frontendHub)

	err := service.HandleTobtUpdate(context.Background(), sessionID, callsign, pastTobt, "EKCH_B_GND", "master")
	require.NoError(t, err)

	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Tobt)
	assert.Equal(t, pastTobt, *persisted.Tobt)
	assert.True(t, persisted.TobtManuallyConfirmed)
	assert.False(t, persisted.TobtAutoSynced)
	assert.False(t, persisted.Recalculate)
	require.NotNil(t, persisted.Tsat)
	assert.Equal(t, tsat, *persisted.Tsat)
	require.Len(t, frontendHub.CdmUpdates, 1)
	assert.Equal(t, pastTobt, frontendHub.CdmUpdates[0].Tobt)
	assert.Equal(t, tsat, frontendHub.CdmUpdates[0].Tsat)
}

func TestHandleEobtUpdate_SyncsLaterFutureTobtAndMarksRecalculation(t *testing.T) {
	const sessionID = int32(143)
	const callsign = "SAS125"

	currentEobt := "1000"
	currentTobt := time.Now().UTC().Add(10 * time.Minute).Format("1504")
	futureEobt := time.Now().UTC().Add(20 * time.Minute).Format("1504")

	var persisted *models.CdmData
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, session int32, cs string) (*models.Strip, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return &models.Strip{Callsign: cs, Session: sessionID, Origin: "EKCH"}, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, session int32, cs string) (*models.CdmData, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return (&models.CdmData{
				Eobt: &currentEobt,
				Tobt: &currentTobt,
			}).Normalize(), nil
		},
		SetCdmDataFn: func(_ context.Context, session int32, cs string, data *models.CdmData) (int64, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			persisted = data.Clone()
			return 1, nil
		},
	}

	frontendHub := &testutil.MockFrontendHub{}
	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	service.client.isValid = false
	setTestCdmFrontend(service, frontendHub)

	err := service.HandleEobtUpdate(context.Background(), sessionID, callsign, futureEobt, "EKCH_DEL", "ATC")
	require.NoError(t, err)

	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Eobt)
	require.NotNil(t, persisted.Tobt)
	assert.Equal(t, futureEobt, *persisted.Eobt)
	assert.Equal(t, futureEobt, *persisted.Tobt)
	assert.Nil(t, persisted.TobtSetBy)
	assert.Nil(t, persisted.TobtConfirmedBy)
	assert.True(t, persisted.TobtAutoSynced)
	assert.False(t, persisted.TobtManuallyConfirmed)
	assert.True(t, persisted.Recalculate)
	require.Len(t, frontendHub.CdmUpdates, 1)
	assert.Equal(t, futureEobt, frontendHub.CdmUpdates[0].Eobt)
	assert.Equal(t, futureEobt, frontendHub.CdmUpdates[0].Tobt)
}

func TestHandleEobtUpdate_EarlierFutureValueStillSyncsTobtAndMarksRecalculation(t *testing.T) {
	const sessionID = int32(144)
	const callsign = "SAS126"

	currentEobt := "1000"
	currentTobt := time.Now().UTC().Add(30 * time.Minute).Format("1504")
	earlierFutureEobt := time.Now().UTC().Add(20 * time.Minute).Format("1504")

	var persisted *models.CdmData
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, session int32, cs string) (*models.Strip, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return &models.Strip{Callsign: cs, Session: sessionID, Origin: "EKCH"}, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, session int32, cs string) (*models.CdmData, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return (&models.CdmData{
				Eobt: &currentEobt,
				Tobt: &currentTobt,
			}).Normalize(), nil
		},
		SetCdmDataFn: func(_ context.Context, session int32, cs string, data *models.CdmData) (int64, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			persisted = data.Clone()
			return 1, nil
		},
	}

	frontendHub := &testutil.MockFrontendHub{}
	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	service.client.isValid = false
	setTestCdmFrontend(service, frontendHub)

	err := service.HandleEobtUpdate(context.Background(), sessionID, callsign, earlierFutureEobt, "EKCH_DEL", "ATC")
	require.NoError(t, err)

	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Eobt)
	require.NotNil(t, persisted.Tobt)
	assert.Equal(t, earlierFutureEobt, *persisted.Eobt)
	assert.Equal(t, earlierFutureEobt, *persisted.Tobt)
	assert.True(t, persisted.TobtAutoSynced)
	assert.False(t, persisted.TobtManuallyConfirmed)
	assert.True(t, persisted.Recalculate)
	require.Len(t, frontendHub.CdmUpdates, 1)
	assert.Equal(t, earlierFutureEobt, frontendHub.CdmUpdates[0].Eobt)
	assert.Equal(t, earlierFutureEobt, frontendHub.CdmUpdates[0].Tobt)
}

func TestHandleEobtUpdate_PastValueDoesNotSyncTobtOrMarkRecalculation(t *testing.T) {
	const sessionID = int32(144)
	const callsign = "SAS126"

	currentEobt := "1000"
	currentTobt := "1015"
	pastEobt := time.Now().UTC().Add(-20 * time.Minute).Format("1504")

	var persisted *models.CdmData
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, session int32, cs string) (*models.Strip, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return &models.Strip{Callsign: cs, Session: sessionID, Origin: "EKCH"}, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, session int32, cs string) (*models.CdmData, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return (&models.CdmData{
				Eobt: &currentEobt,
				Tobt: &currentTobt,
			}).Normalize(), nil
		},
		SetCdmDataFn: func(_ context.Context, session int32, cs string, data *models.CdmData) (int64, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			persisted = data.Clone()
			return 1, nil
		},
	}

	frontendHub := &testutil.MockFrontendHub{}
	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	service.client.isValid = false
	setTestCdmFrontend(service, frontendHub)

	err := service.HandleEobtUpdate(context.Background(), sessionID, callsign, pastEobt, "EKCH_DEL", "ATC")
	require.NoError(t, err)

	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Eobt)
	require.NotNil(t, persisted.Tobt)
	assert.Equal(t, pastEobt, *persisted.Eobt)
	assert.Equal(t, currentTobt, *persisted.Tobt)
	assert.False(t, persisted.Recalculate)
	require.Len(t, frontendHub.CdmUpdates, 1)
	assert.Equal(t, pastEobt, frontendHub.CdmUpdates[0].Eobt)
	assert.Equal(t, currentTobt, frontendHub.CdmUpdates[0].Tobt)
}

func TestHandleEobtUpdate_ExpiredTsatMarksRecalculationAndStillSyncsTobt(t *testing.T) {
	const sessionID = int32(145)
	const callsign = "SAS127"

	currentEobt := "1000"
	currentTobt := time.Now().UTC().Add(20 * time.Minute).Format("1504")
	earlierFutureEobt := time.Now().UTC().Add(10 * time.Minute).Format("1504")
	expiredTsat := time.Now().UTC().Add(-10 * time.Minute).Format("1504")

	var persisted *models.CdmData
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, session int32, cs string) (*models.Strip, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return &models.Strip{Callsign: cs, Session: sessionID, Origin: "EKCH"}, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, session int32, cs string) (*models.CdmData, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return (&models.CdmData{
				Eobt: &currentEobt,
				Tobt: &currentTobt,
				Tsat: &expiredTsat,
			}).Normalize(), nil
		},
		SetCdmDataFn: func(_ context.Context, session int32, cs string, data *models.CdmData) (int64, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			persisted = data.Clone()
			return 1, nil
		},
	}

	frontendHub := &testutil.MockFrontendHub{}
	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	service.client.isValid = false
	setTestCdmFrontend(service, frontendHub)

	err := service.HandleEobtUpdate(context.Background(), sessionID, callsign, earlierFutureEobt, "EKCH_DEL", "ATC")
	require.NoError(t, err)

	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Eobt)
	require.NotNil(t, persisted.Tobt)
	assert.Equal(t, earlierFutureEobt, *persisted.Eobt)
	assert.Equal(t, earlierFutureEobt, *persisted.Tobt)
	assert.True(t, persisted.TobtAutoSynced)
	assert.False(t, persisted.TobtManuallyConfirmed)
	assert.True(t, persisted.Recalculate)
	require.Len(t, frontendHub.CdmUpdates, 1)
	assert.Equal(t, earlierFutureEobt, frontendHub.CdmUpdates[0].Eobt)
	assert.Equal(t, earlierFutureEobt, frontendHub.CdmUpdates[0].Tobt)
}

func TestHandleEobtUpdate_MasterSession_ClampsFarFutureValueSyncsBackAndMarksReason(t *testing.T) {
	const sessionID = int32(146)
	const callsign = "SAS128"
	const masterCid = "123456"
	now := time.Now().UTC()
	currentEobt := "1000"
	currentTobt := addMinutes(timeToClock(now), 20)
	rawFutureEobt := truncateCDMClockValue(addMinutes(timeToClock(now), 60))
	expectedClamped := truncateCDMClockValue(addMinutes(timeToClock(now), masterEobtClampTarget))

	initial := (&models.CdmData{
		Eobt: &currentEobt,
		Tobt: testStringPtr(currentTobt),
	}).Normalize()
	var persisted *models.CdmData
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, session int32, cs string) (*models.Strip, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return &models.Strip{Callsign: cs, Session: sessionID, Origin: "EKCH"}, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, session int32, cs string) (*models.CdmData, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			if persisted != nil {
				return persisted.Clone(), nil
			}
			return initial.Clone(), nil
		},
		SetCdmDataFn: func(_ context.Context, session int32, cs string, data *models.CdmData) (int64, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			persisted = data.Clone()
			return 1, nil
		},
	}

	controllerRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, session int32, callsign string) (*models.Controller, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, "EKCH_A_TWR", callsign)
			return &models.Controller{Cid: testStringPtr(masterCid)}, nil
		},
	}
	euroscopeHub := &testutil.MockEuroscopeHub{
		GetMasterCallsignFn: func(int32) string {
			return "EKCH_A_TWR"
		},
	}

	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, &testutil.MockSessionRepository{}, controllerRepo)
	service.client.isValid = false
	setTestCdmEuroscope(service, euroscopeHub)
	service.sessionMaster.Store(sessionID, true)

	err := service.HandleEobtUpdate(context.Background(), sessionID, callsign, rawFutureEobt, "EKCH_DEL", "ATC")
	require.NoError(t, err)

	require.NotNil(t, persisted)
	assert.Equal(t, expectedClamped, valueOrEmpty(persisted.Eobt))
	assert.Equal(t, expectedClamped, valueOrEmpty(persisted.Tobt))
	assert.True(t, persisted.TobtAutoSynced)
	assert.False(t, persisted.TobtManuallyConfirmed)
	assert.Empty(t, valueOrEmpty(persisted.TobtConfirmedBy))
	assert.True(t, persisted.Recalculate)
	require.NotNil(t, persisted.Calculation)
	require.Len(t, persisted.Calculation.ReasonMarkers, 1)
	assert.Equal(t, eobtCappedReasonKind, persisted.Calculation.ReasonMarkers[0].Kind)
	assert.Equal(t, eobtCappedReasonMessage, persisted.Calculation.ReasonMarkers[0].Message)
	require.Len(t, euroscopeHub.Eobts, 1)
	assert.Equal(t, masterCid, euroscopeHub.Eobts[0].Cid)
	assert.Equal(t, expectedClamped, euroscopeHub.Eobts[0].Eobt)
}

func TestPrepareEuroscopeEobtSync_MasterSessionClampsBeforeInitialSequenceAndLeavesTobtUnconfirmed(t *testing.T) {
	const sessionID = int32(146)
	now := time.Date(2026, time.July, 13, 10, 0, 0, 0, time.UTC)
	rawFutureEobt := truncateCDMClockValue(addMinutes(timeToClock(now), 60))
	expectedClamped := truncateCDMClockValue(addMinutes(timeToClock(now), masterEobtClampTarget))

	service := newTestCdmService(newTestClientWithAirportMasters(nil), &testutil.MockStripRepository{}, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	service.sessionMaster.Store(sessionID, true)

	updated, corrected, clamped := service.PrepareEuroscopeEobtSync(sessionID, &models.CdmData{}, rawFutureEobt, now)

	require.True(t, clamped)
	assert.Equal(t, expectedClamped, corrected)
	assert.Equal(t, expectedClamped, valueOrEmpty(updated.Eobt))
	assert.Equal(t, expectedClamped, valueOrEmpty(updated.Tobt))
	assert.True(t, updated.TobtAutoSynced)
	assert.False(t, updated.TobtManuallyConfirmed)
	assert.Empty(t, valueOrEmpty(updated.TobtConfirmedBy))
	assert.Empty(t, valueOrEmpty(updated.TobtSetBy))
	assert.True(t, updated.Recalculate)
	require.NotNil(t, updated.Calculation)
	require.Len(t, updated.Calculation.ReasonMarkers, 1)
	assert.Equal(t, eobtCappedReasonKind, updated.Calculation.ReasonMarkers[0].Kind)
}

func TestPrepareEuroscopeEobtSync_ClearsLegacyAutoConfirmationButPreservesManualConfirmation(t *testing.T) {
	const sessionID = int32(149)
	now := time.Date(2026, time.July, 13, 10, 0, 0, 0, time.UTC)
	previousEobt := truncateCDMClockValue(addMinutes(timeToClock(now), 15))
	rawFutureEobt := truncateCDMClockValue(addMinutes(timeToClock(now), 60))
	expectedClamped := truncateCDMClockValue(addMinutes(timeToClock(now), masterEobtClampTarget))
	atc := models.TobtConfirmedByATC
	pilot := models.TobtConfirmedByPilot
	setBy := "EKCH_DEL"

	service := newTestCdmService(newTestClientWithAirportMasters(nil), &testutil.MockStripRepository{}, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	service.sessionMaster.Store(sessionID, true)

	t.Run("legacy auto-follow metadata is cleared", func(t *testing.T) {
		updated, corrected, clamped := service.PrepareEuroscopeEobtSync(sessionID, &models.CdmData{
			Eobt:            &previousEobt,
			Tobt:            &previousEobt,
			TobtSetBy:       &setBy,
			TobtConfirmedBy: &atc,
		}, rawFutureEobt, now)

		require.True(t, clamped)
		assert.Equal(t, expectedClamped, corrected)
		assert.Equal(t, expectedClamped, valueOrEmpty(updated.Tobt))
		assert.True(t, updated.TobtAutoSynced)
		assert.False(t, updated.TobtManuallyConfirmed)
		assert.Empty(t, valueOrEmpty(updated.TobtConfirmedBy))
		assert.Empty(t, valueOrEmpty(updated.TobtSetBy))
	})

	t.Run("manual confirmation remains protected", func(t *testing.T) {
		updated, corrected, clamped := service.PrepareEuroscopeEobtSync(sessionID, &models.CdmData{
			Eobt:                  &previousEobt,
			Tobt:                  &previousEobt,
			TobtSetBy:             &setBy,
			TobtConfirmedBy:       &pilot,
			TobtManuallyConfirmed: true,
		}, rawFutureEobt, now)

		require.True(t, clamped)
		assert.Equal(t, expectedClamped, corrected)
		assert.Equal(t, expectedClamped, valueOrEmpty(updated.Eobt))
		assert.Equal(t, previousEobt, valueOrEmpty(updated.Tobt))
		assert.False(t, updated.TobtAutoSynced)
		assert.True(t, updated.TobtManuallyConfirmed)
		assert.Equal(t, models.TobtConfirmedByPilot, valueOrEmpty(updated.TobtConfirmedBy))
		assert.Equal(t, setBy, valueOrEmpty(updated.TobtSetBy))
	})
}

func TestHandleEobtUpdate_MasterSession_ClampsEmptyValueToNowPlus30(t *testing.T) {
	const sessionID = int32(149)
	const callsign = "SAS131"
	const masterCid = "123457"
	now := time.Now().UTC()
	currentEobt := "1000"
	currentTobt := addMinutes(timeToClock(now), 20)
	expectedClamped := truncateCDMClockValue(addMinutes(timeToClock(now), masterEobtClampTarget))

	initial := (&models.CdmData{
		Eobt: &currentEobt,
		Tobt: testStringPtr(currentTobt),
	}).Normalize()
	var persisted *models.CdmData
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, session int32, cs string) (*models.Strip, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return &models.Strip{Callsign: cs, Session: sessionID, Origin: "EKCH"}, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, session int32, cs string) (*models.CdmData, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			if persisted != nil {
				return persisted.Clone(), nil
			}
			return initial.Clone(), nil
		},
		SetCdmDataFn: func(_ context.Context, session int32, cs string, data *models.CdmData) (int64, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			persisted = data.Clone()
			return 1, nil
		},
	}

	controllerRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, session int32, callsign string) (*models.Controller, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, "EKCH_A_TWR", callsign)
			return &models.Controller{Cid: testStringPtr(masterCid)}, nil
		},
	}
	euroscopeHub := &testutil.MockEuroscopeHub{
		GetMasterCallsignFn: func(int32) string {
			return "EKCH_A_TWR"
		},
	}

	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, &testutil.MockSessionRepository{}, controllerRepo)
	service.client.isValid = false
	setTestCdmEuroscope(service, euroscopeHub)
	service.sessionMaster.Store(sessionID, true)

	err := service.HandleEobtUpdate(context.Background(), sessionID, callsign, "", "EKCH_DEL", "ATC")
	require.NoError(t, err)

	require.NotNil(t, persisted)
	assert.Equal(t, expectedClamped, valueOrEmpty(persisted.Eobt))
	assert.Equal(t, expectedClamped, valueOrEmpty(persisted.Tobt))
	assert.True(t, persisted.TobtAutoSynced)
	assert.False(t, persisted.TobtManuallyConfirmed)
	assert.Empty(t, valueOrEmpty(persisted.TobtConfirmedBy))
	assert.True(t, persisted.Recalculate)
	require.NotNil(t, persisted.Calculation)
	require.Len(t, persisted.Calculation.ReasonMarkers, 1)
	assert.Equal(t, eobtCappedReasonKind, persisted.Calculation.ReasonMarkers[0].Kind)
	assert.Equal(t, eobtCappedReasonMessage, persisted.Calculation.ReasonMarkers[0].Message)
	require.Len(t, euroscopeHub.Eobts, 1)
	assert.Equal(t, masterCid, euroscopeHub.Eobts[0].Cid)
	assert.Equal(t, expectedClamped, euroscopeHub.Eobts[0].Eobt)
}

func TestHandleEobtUpdate_DoesNotOverwriteConfirmedTobt(t *testing.T) {
	testCases := []struct {
		name              string
		confirmedBy       string
		currentTobt       string
		expectRecalculate bool
	}{
		{
			name:              "pilot confirmed",
			confirmedBy:       models.TobtConfirmedByPilot,
			currentTobt:       time.Now().UTC().Add(10 * time.Minute).Format("1504"),
			expectRecalculate: true,
		},
		{
			name:              "atc confirmed",
			confirmedBy:       models.TobtConfirmedByATC,
			currentTobt:       time.Now().UTC().Add(10 * time.Minute).Format("1504"),
			expectRecalculate: true,
		},
		{
			name:              "pilot confirmed midnight",
			confirmedBy:       models.TobtConfirmedByPilot,
			currentTobt:       "0000",
			expectRecalculate: true,
		},
		{
			name:              "atc confirmed midnight",
			confirmedBy:       models.TobtConfirmedByATC,
			currentTobt:       "0000",
			expectRecalculate: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			const sessionID = int32(148)
			const callsign = "SAS130"

			currentEobt := "1000"
			currentTobt := tc.currentTobt
			futureEobt := time.Now().UTC().Add(20 * time.Minute).Format("1504")
			confirmedBy := tc.confirmedBy

			var persisted *models.CdmData
			stripRepo := &testutil.MockStripRepository{
				GetByCallsignFn: func(_ context.Context, session int32, cs string) (*models.Strip, error) {
					assert.Equal(t, sessionID, session)
					assert.Equal(t, callsign, cs)
					return &models.Strip{Callsign: cs, Session: sessionID, Origin: "EKCH"}, nil
				},
				GetCdmDataForCallsignFn: func(_ context.Context, session int32, cs string) (*models.CdmData, error) {
					assert.Equal(t, sessionID, session)
					assert.Equal(t, callsign, cs)
					return (&models.CdmData{
						Eobt:                  &currentEobt,
						Tobt:                  &currentTobt,
						TobtConfirmedBy:       &confirmedBy,
						TobtManuallyConfirmed: true,
					}).Normalize(), nil
				},
				SetCdmDataFn: func(_ context.Context, session int32, cs string, data *models.CdmData) (int64, error) {
					assert.Equal(t, sessionID, session)
					assert.Equal(t, callsign, cs)
					persisted = data.Clone()
					return 1, nil
				},
			}

			frontendHub := &testutil.MockFrontendHub{}
			service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
			service.client.isValid = false
			setTestCdmFrontend(service, frontendHub)

			err := service.HandleEobtUpdate(context.Background(), sessionID, callsign, futureEobt, "EKCH_DEL", "ATC")
			require.NoError(t, err)

			require.NotNil(t, persisted)
			assert.Equal(t, futureEobt, valueOrEmpty(persisted.Eobt))
			assert.Equal(t, currentTobt, valueOrEmpty(persisted.Tobt))
			assert.Equal(t, tc.confirmedBy, valueOrEmpty(persisted.TobtConfirmedBy))
			assert.True(t, persisted.TobtManuallyConfirmed)
			assert.False(t, persisted.TobtAutoSynced)
			assert.Equal(t, tc.expectRecalculate, persisted.Recalculate)
			require.Len(t, frontendHub.CdmUpdates, 1)
			assert.Equal(t, futureEobt, frontendHub.CdmUpdates[0].Eobt)
			assert.Equal(t, currentTobt, frontendHub.CdmUpdates[0].Tobt)
		})
	}
}

func TestHandleEobtUpdate_LegacyAutoSyncedTobtContinuesFollowingEobt(t *testing.T) {
	const sessionID = int32(151)
	const callsign = "SAS133"

	currentEobt := time.Now().UTC().Add(25 * time.Minute).Format("1504")
	currentTobt := currentEobt
	nextEobt := time.Now().UTC().Add(15 * time.Minute).Format("1504")
	confirmedBy := models.TobtConfirmedByATC
	setBy := "EKCH_DEL"

	var persisted *models.CdmData
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, session int32, cs string) (*models.Strip, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return &models.Strip{Callsign: cs, Session: sessionID, Origin: "EKCH"}, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, session int32, cs string) (*models.CdmData, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return (&models.CdmData{
				Eobt:            &currentEobt,
				Tobt:            &currentTobt,
				TobtSetBy:       &setBy,
				TobtConfirmedBy: &confirmedBy,
			}).Normalize(), nil
		},
		SetCdmDataFn: func(_ context.Context, session int32, cs string, data *models.CdmData) (int64, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			persisted = data.Clone()
			return 1, nil
		},
	}

	frontendHub := &testutil.MockFrontendHub{}
	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	service.client.isValid = false
	setTestCdmFrontend(service, frontendHub)

	err := service.HandleEobtUpdate(context.Background(), sessionID, callsign, nextEobt, "EKCH_DEL", "ATC")
	require.NoError(t, err)

	require.NotNil(t, persisted)
	assert.Equal(t, nextEobt, valueOrEmpty(persisted.Eobt))
	assert.Equal(t, nextEobt, valueOrEmpty(persisted.Tobt))
	assert.True(t, persisted.TobtAutoSynced)
	assert.False(t, persisted.TobtManuallyConfirmed)
	assert.Empty(t, valueOrEmpty(persisted.TobtConfirmedBy))
	assert.True(t, persisted.Recalculate)
	require.Len(t, frontendHub.CdmUpdates, 1)
	assert.Equal(t, nextEobt, frontendHub.CdmUpdates[0].Tobt)
}

func TestHandleEobtUpdate_NonMasterSession_DoesNotClampFarFutureValue(t *testing.T) {
	const sessionID = int32(147)
	const callsign = "SAS129"
	now := time.Now().UTC()
	currentEobt := "1000"
	currentTobt := addMinutes(timeToClock(now), 20)
	rawFutureEobt := truncateCDMClockValue(addMinutes(timeToClock(now), 60))

	initial := (&models.CdmData{
		Eobt: &currentEobt,
		Tobt: testStringPtr(currentTobt),
	}).Normalize()
	var persisted *models.CdmData
	listByOriginCalled := false
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, session int32, cs string) (*models.Strip, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return &models.Strip{Callsign: cs, Session: sessionID, Origin: "EKCH"}, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, session int32, cs string) (*models.CdmData, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			if persisted != nil {
				return persisted.Clone(), nil
			}
			return initial.Clone(), nil
		},
		SetCdmDataFn: func(_ context.Context, session int32, cs string, data *models.CdmData) (int64, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			persisted = data.Clone()
			return 1, nil
		},
		ListByOriginFn: func(_ context.Context, _ int32, _ string) ([]*models.Strip, error) {
			listByOriginCalled = true
			return nil, nil
		},
	}
	euroscopeHub := &testutil.MockEuroscopeHub{}

	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	service.client.isValid = false
	service.debouncer = newRecalcDebouncer(time.Millisecond)
	setTestCdmEuroscope(service, euroscopeHub)
	service.SetSequenceService(newTestSequenceService(
		stripRepo,
		&testutil.MockSessionRepository{},
		&stubConfigProvider{},
		&testutil.MockFrontendHub{},
		&testutil.MockEuroscopeHub{},
	))

	err := service.HandleEobtUpdate(context.Background(), sessionID, callsign, rawFutureEobt, "EKCH_DEL", "ATC")
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	require.NotNil(t, persisted)
	assert.Equal(t, rawFutureEobt, valueOrEmpty(persisted.Eobt))
	assert.Equal(t, rawFutureEobt, valueOrEmpty(persisted.Tobt))
	assert.Nil(t, persisted.Calculation)
	assert.Empty(t, euroscopeHub.Eobts)
	assert.False(t, listByOriginCalled, "non-master EOBT update should not trigger local recalculation")
}

func TestHandleTobtUpdate_SameValueConvertsAutoSyncedTobtToManualConfirmation(t *testing.T) {
	const sessionID = int32(152)
	const callsign = "SAS134"

	currentTobt := time.Now().UTC().Add(-20 * time.Minute).Format("1504")
	currentEobt := currentTobt

	var persisted *models.CdmData
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, session int32, cs string) (*models.Strip, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return &models.Strip{Callsign: cs, Session: sessionID, Origin: "EKCH"}, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, session int32, cs string) (*models.CdmData, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return (&models.CdmData{
				Eobt:           &currentEobt,
				Tobt:           &currentTobt,
				TobtAutoSynced: true,
			}).Normalize(), nil
		},
		SetCdmDataFn: func(_ context.Context, session int32, cs string, data *models.CdmData) (int64, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			persisted = data.Clone()
			return 1, nil
		},
	}

	frontendHub := &testutil.MockFrontendHub{}
	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	service.client.isValid = false
	setTestCdmFrontend(service, frontendHub)

	err := service.HandleTobtUpdate(context.Background(), sessionID, callsign, currentTobt, "EKCH_B_GND", "master")
	require.NoError(t, err)

	require.NotNil(t, persisted)
	assert.Equal(t, currentTobt, valueOrEmpty(persisted.Tobt))
	assert.Equal(t, "EKCH_B_GND", valueOrEmpty(persisted.TobtSetBy))
	assert.Equal(t, models.TobtConfirmedByATC, valueOrEmpty(persisted.TobtConfirmedBy))
	assert.True(t, persisted.TobtManuallyConfirmed)
	assert.False(t, persisted.TobtAutoSynced)
	assert.False(t, persisted.Recalculate)
	require.Len(t, frontendHub.CdmUpdates, 1)
	assert.Equal(t, currentTobt, frontendHub.CdmUpdates[0].Tobt)
}

func TestHandleEobtUpdate_NonMasterSession_IgnoresEmptyValue(t *testing.T) {
	const sessionID = int32(150)
	const callsign = "SAS132"

	loadCalled := false
	listByOriginCalled := false
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			loadCalled = true
			return &models.Strip{Callsign: callsign, Session: sessionID, Origin: "EKCH"}, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, _ int32, _ string) (*models.CdmData, error) {
			loadCalled = true
			return (&models.CdmData{}).Normalize(), nil
		},
		ListByOriginFn: func(_ context.Context, _ int32, _ string) ([]*models.Strip, error) {
			listByOriginCalled = true
			return nil, nil
		},
	}

	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	service.client.isValid = false
	service.debouncer = newRecalcDebouncer(time.Millisecond)
	service.SetSequenceService(newTestSequenceService(
		stripRepo,
		&testutil.MockSessionRepository{},
		&stubConfigProvider{},
		&testutil.MockFrontendHub{},
		&testutil.MockEuroscopeHub{},
	))

	err := service.HandleEobtUpdate(context.Background(), sessionID, callsign, "", "EKCH_DEL", "ATC")
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)
	assert.False(t, loadCalled, "empty non-master EOBT should be ignored before loading state")
	assert.False(t, listByOriginCalled, "empty non-master EOBT should not trigger recalculation")
}

func TestHandleClxTobtUpdate_BroadcastsFinalCalculatedCdmData(t *testing.T) {
	const sessionID = int32(44)
	const callsign = "SAS654"
	const airport = "EKCH"
	runway := "22L"
	tobt := time.Now().UTC().Add(20 * time.Minute).Format("1504")

	eobt := "1000"
	stored := (&models.CdmData{Eobt: &eobt}).Normalize()
	stripForState := func() *models.Strip {
		return &models.Strip{
			Callsign: callsign,
			Session:  sessionID,
			Origin:   airport,
			Runway:   &runway,
			CdmData:  stored.Clone(),
		}
	}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, session int32, cs string) (*models.Strip, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return stripForState(), nil
		},
		ListByOriginFn: func(_ context.Context, session int32, origin string) ([]*models.Strip, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, airport, origin)
			return []*models.Strip{stripForState()}, nil
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
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			assert.Equal(t, sessionID, id)
			return &models.Session{
				ID: id,
				ActiveRunways: pkgModels.ActiveRunways{
					DepartureRunways: []string{runway},
				},
			}, nil
		},
	}
	frontendHub := &testutil.MockFrontendHub{}
	configStore := NewCdmConfigStore("", "", "", time.Minute, CdmConfigDefaults{}, newFailingHTTPClient())
	sequenceService := newTestSequenceService(stripRepo, sessionRepo, configStore, frontendHub, nil)

	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, sessionRepo, &testutil.MockControllerRepository{})
	service.client.isValid = false
	setTestCdmFrontend(service, frontendHub)
	service.SetConfigProvider(configStore)
	service.SetSequenceService(sequenceService)
	service.sessionMaster.Store(sessionID, true)

	err := service.HandleClxTobtUpdate(context.Background(), sessionID, callsign, tobt, "EKCH_B_GND", "ATC")
	require.NoError(t, err)

	assert.Equal(t, tobt, *stored.Tobt)
	assert.False(t, stored.NeedsLocalRecalculation())
	require.NotNil(t, stored.Tsat)
	require.NotNil(t, stored.Ttot)

	require.Len(t, frontendHub.CdmUpdates, 1)
	assert.Equal(t, callsign, frontendHub.CdmUpdates[0].Callsign)
	assert.Equal(t, tobt, frontendHub.CdmUpdates[0].Tobt)
	assert.Equal(t, truncateCDMClockValue(*stored.Tsat), frontendHub.CdmUpdates[0].Tsat)
	assert.Equal(t, truncateCDMClockValue(*stored.Ttot), frontendHub.CdmUpdates[0].Event.Ttot)
}

func TestHandleManualCtot_BroadcastsEffectiveFrontendCtot(t *testing.T) {
	const sessionID = int32(43)
	const callsign = "SAS321"

	stored := (&models.CdmData{}).Normalize()
	frontendHub := &testutil.MockFrontendHub{}
	service := newTestCdmService(
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
	setTestCdmFrontend(service, frontendHub)

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
	service := newTestCdmService(
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
	setTestCdmEuroscope(service, euroscopeHub)

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

	service := newTestCdmService(
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
	markSessionLive(service, sessionID)

	err := service.PushTobt(context.Background(), sessionID, callsign, "1030")
	require.NoError(t, err)
	assert.Equal(t, "/ifps/dpi", requestPath)
	assert.Contains(t, requestQuery, "callsign=EIN456")
	assert.Contains(t, requestQuery, "value=TOBT%2F1030%2F12")
}

func TestPushTobt_UsesPersistedCalculationTaxiMinutes(t *testing.T) {
	const sessionID = int32(33)
	const callsign = "SAS789"
	taxiMinutes := 14
	zero := 0.0

	var requestQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("true"))
	}))
	defer server.Close()

	client := NewClient(
		WithAPIKey("test-key"),
		WithBaseURL(server.URL),
	)

	service := newTestCdmService(
		client,
		&testutil.MockStripRepository{
			GetByCallsignFn: func(_ context.Context, session int32, cs string) (*models.Strip, error) {
				assert.Equal(t, sessionID, session)
				assert.Equal(t, callsign, cs)
				return &models.Strip{
					Callsign: callsign,
					Origin:   "EKCH",
					Runway:   stringPtr("22R"),
					CdmData: (&models.CdmData{
						Calculation: &models.CdmCalculation{
							TaxiMinutes: &taxiMinutes,
							TaxiRunway:  stringPtr("22R"),
						},
					}).Normalize(),
					PositionLatitude:  &zero,
					PositionLongitude: &zero,
				}, nil
			},
		},
		&testutil.MockSessionRepository{},
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
	markSessionLive(service, sessionID)

	err := service.PushTobt(context.Background(), sessionID, callsign, "1030")
	require.NoError(t, err)
	assert.Contains(t, requestQuery, "value=TOBT%2F1030%2F14")
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
	sequenceService := newTestSequenceService(stripRepo, sessionRepo, configStore, &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})

	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, sessionRepo, &testutil.MockControllerRepository{})
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

	service := newTestCdmService(
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

	frontendHub := &testutil.MockFrontendHub{}
	setTestCdmFrontend(service, frontendHub)
	service.sessionMaster.Store(sessionID, true)
	markSessionLive(service, sessionID)

	require.NoError(t, service.SetReady(context.Background(), sessionID, callsign))
	assert.Equal(t, "REA/1", receivedValue)
	require.Len(t, frontendHub.CdmUpdates, 1)
	assert.Equal(t, "REA", frontendHub.CdmUpdates[len(frontendHub.CdmUpdates)-1].Event.Status)
}

func TestSetReady_MasterPersistsAndExportsAsrt(t *testing.T) {
	t.Parallel()
	const callsign = "SAS123"
	const sessionID = int32(123)

	dpiCh := make(chan string, 1)
	setCdmCh := make(chan url.Values, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ifps/dpi":
			dpiCh <- r.URL.Query().Get("value")
		case "/ifps/setCdmData":
			setCdmCh <- r.URL.Query()
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	runway := "22R"
	stored := (&models.CdmData{
		Tobt: stringPtr("1010"),
		Tsat: stringPtr("1015"),
		Ttot: stringPtr("1025"),
	}).Normalize()
	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetCdmDataForCallsignFn: func(_ context.Context, _ int32, _ string) (*models.CdmData, error) {
				return stored.Clone(), nil
			},
			SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
				stored = data.Clone()
				return 1, nil
			},
			GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
				return &models.Strip{Callsign: callsign, Runway: &runway}, nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)
	setTestCdmFrontend(service, &testutil.MockFrontendHub{})
	service.sessionMaster.Store(sessionID, true)
	markSessionLive(service, sessionID)

	beforeNow := time.Now().UTC().Format("1504")
	require.NoError(t, service.SetReady(context.Background(), sessionID, callsign))
	afterNow := time.Now().UTC().Format("1504")

	require.NotNil(t, stored.Asrt)
	assert.Contains(t, []string{beforeNow, afterNow}, *stored.Asrt)
	assert.Equal(t, "REA", valueOrEmpty(stored.Status))
	select {
	case dpi := <-dpiCh:
		assert.Equal(t, "REA/1", dpi)
	case <-time.After(time.Second):
		t.Fatal("expected ready DPI to be sent to vIFF")
	}
	select {
	case setCdm := <-setCdmCh:
		assert.Equal(t, *stored.Asrt+"00", setCdm.Get("asrt"))
		assert.Equal(t, "22R", setCdm.Get("depInfo"))
	case <-time.After(time.Second):
		t.Fatal("expected ASRT to be exported to vIFF")
	}
}

func TestSetReady_WithoutValidClient_PersistsLocalReadyStatus(t *testing.T) {
	t.Parallel()
	const callsign = "SAS938"
	const sessionID = int32(938)

	var persisted *models.CdmData
	service := newTestCdmService(
		newTestClientWithAirportMasters(nil),
		&testutil.MockStripRepository{
			GetCdmDataForCallsignFn: func(_ context.Context, _ int32, _ string) (*models.CdmData, error) {
				if persisted != nil {
					return persisted.Clone(), nil
				}
				return (&models.CdmData{}).Normalize(), nil
			},
			SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
				persisted = data.Clone()
				return 1, nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)
	service.client.isValid = false
	frontendHub := &testutil.MockFrontendHub{}
	setTestCdmFrontend(service, frontendHub)

	require.NoError(t, service.SetReady(context.Background(), sessionID, callsign))

	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Status)
	assert.Equal(t, "REA", *persisted.Status)
	require.Len(t, frontendHub.CdmUpdates, 1)
	assert.Equal(t, "REA", frontendHub.CdmUpdates[len(frontendHub.CdmUpdates)-1].Event.Status)
	require.Len(t, frontendHub.CdmWaits, 1)
	assert.Equal(t, callsign, frontendHub.CdmWaits[0].Callsign)
}

func TestSetReady_WithValidClient_NonMasterSendsReaWithoutPersistingLocally(t *testing.T) {
	t.Parallel()
	const callsign = "SAS777"
	const sessionID = int32(777)

	var receivedValue string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ifps/dpi", r.URL.Path)
		receivedValue = r.URL.Query().Get("value")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var persisted *models.CdmData
	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetCdmDataForCallsignFn: func(_ context.Context, _ int32, _ string) (*models.CdmData, error) {
				return (&models.CdmData{}).Normalize(), nil
			},
			SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
				persisted = data.Clone()
				return 1, nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)
	frontendHub := &testutil.MockFrontendHub{}
	setTestCdmFrontend(service, frontendHub)
	markSessionLive(service, sessionID)

	require.NoError(t, service.SetReady(context.Background(), sessionID, callsign))
	assert.Equal(t, "REA/1", receivedValue)
	assert.Nil(t, persisted)
	assert.Empty(t, frontendHub.CdmUpdates)
	assert.Empty(t, frontendHub.CdmWaits)
}

func TestHandleReadyRequest_WithValidClient_NonMasterOnlySendsRea(t *testing.T) {
	t.Parallel()
	const callsign = "SAS778"
	const sessionID = int32(778)

	var receivedValue string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ifps/dpi", r.URL.Path)
		receivedValue = r.URL.Query().Get("value")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var persisted *models.CdmData
	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
				return &models.Strip{Callsign: callsign, Origin: "EKCH"}, nil
			},
			GetCdmDataForCallsignFn: func(_ context.Context, _ int32, _ string) (*models.CdmData, error) {
				return (&models.CdmData{Tobt: stringPtr("1700")}).Normalize(), nil
			},
			SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
				persisted = data.Clone()
				return 1, nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)
	frontendHub := &testutil.MockFrontendHub{}
	setTestCdmFrontend(service, frontendHub)
	markSessionLive(service, sessionID)

	require.NoError(t, service.HandleReadyRequest(context.Background(), sessionID, callsign, "EKCH_DEL", "ATC"))
	assert.Equal(t, "REA/1", receivedValue)
	assert.Nil(t, persisted)
	assert.Empty(t, frontendHub.CdmUpdates)
	assert.Empty(t, frontendHub.CdmWaits)
}

func TestHandleReadyRequest_WithoutValidClient_PersistsReadyWhenNoTobtChangeNeeded(t *testing.T) {
	const callsign = "NOZ938"
	const sessionID = int32(44)

	pastTobt := time.Now().UTC().Add(-5 * time.Minute).Format("1504")
	var persisted *models.CdmData

	service := newTestCdmService(
		newTestClientWithAirportMasters(nil),
		&testutil.MockStripRepository{
			GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
				return &models.Strip{Callsign: callsign, Origin: "EKCH"}, nil
			},
			GetCdmDataForCallsignFn: func(_ context.Context, _ int32, _ string) (*models.CdmData, error) {
				if persisted != nil {
					return persisted.Clone(), nil
				}
				return (&models.CdmData{Tobt: stringPtr(pastTobt)}).Normalize(), nil
			},
			SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
				persisted = data.Clone()
				return 1, nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)
	service.client.isValid = false
	frontendHub := &testutil.MockFrontendHub{}
	setTestCdmFrontend(service, frontendHub)

	beforeNow := time.Now().UTC().Format("1504")
	require.NoError(t, service.HandleReadyRequest(context.Background(), sessionID, callsign, "EKCH_DEL", "ATC"))
	afterNow := time.Now().UTC().Format("1504")

	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Status)
	assert.Equal(t, "REA", *persisted.Status)
	require.NotNil(t, persisted.Tobt)
	assert.Contains(t, []string{beforeNow, afterNow}, *persisted.Tobt)
	require.Len(t, frontendHub.CdmUpdates, 2)
	assert.Equal(t, "REA", frontendHub.CdmUpdates[len(frontendHub.CdmUpdates)-1].Event.Status)
	require.Len(t, frontendHub.CdmWaits, 1)
	assert.Equal(t, callsign, frontendHub.CdmWaits[0].Callsign)
}

func TestHandleReadyRequest_UpdatesFutureTobtToNow(t *testing.T) {
	const callsign = "SAS211"
	const sessionID = int32(211)

	futureTobt := time.Now().UTC().Add(15 * time.Minute).Format("1504")
	var persisted *models.CdmData

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
				return &models.Strip{Callsign: callsign, Origin: "EKCH"}, nil
			},
			GetCdmDataForCallsignFn: func(_ context.Context, _ int32, _ string) (*models.CdmData, error) {
				if persisted != nil {
					return persisted.Clone(), nil
				}
				return (&models.CdmData{Tobt: stringPtr(futureTobt)}).Normalize(), nil
			},
			SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
				persisted = data.Clone()
				return 1, nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)
	setTestCdmFrontend(service, &testutil.MockFrontendHub{})
	service.sessionMaster.Store(sessionID, true)
	markSessionLive(service, sessionID)

	beforeNow := time.Now().UTC().Format("1504")
	err := service.HandleReadyRequest(context.Background(), sessionID, callsign, "EKCH_DEL", "ATC")
	afterNow := time.Now().UTC().Format("1504")
	require.NoError(t, err)

	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Tobt)
	assert.Contains(t, []string{beforeNow, afterNow}, *persisted.Tobt)
	require.NotNil(t, persisted.Status)
	assert.Equal(t, "REA", *persisted.Status)
	require.NotNil(t, persisted.TobtSetBy)
	assert.Equal(t, "EKCH_DEL", *persisted.TobtSetBy)
	require.NotNil(t, persisted.TobtConfirmedBy)
	assert.Equal(t, models.TobtConfirmedByATC, *persisted.TobtConfirmedBy)
	assert.True(t, persisted.TobtManuallyConfirmed)
	assert.False(t, persisted.TobtAutoSynced)
}

func TestHandleReadyRequest_PreservesTobtWithinTsatWindow(t *testing.T) {
	const callsign = "SAS210"
	const sessionID = int32(210)

	previousTobt := time.Now().UTC().Add(15 * time.Minute).Format("1504")
	activeTsat := time.Now().UTC().Add(5 * time.Minute).Format("1504")
	var persisted *models.CdmData

	service := newTestCdmService(
		newTestClientWithAirportMasters(nil),
		&testutil.MockStripRepository{
			GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
				return &models.Strip{Callsign: callsign, Origin: "EKCH"}, nil
			},
			GetCdmDataForCallsignFn: func(_ context.Context, _ int32, _ string) (*models.CdmData, error) {
				if persisted != nil {
					return persisted.Clone(), nil
				}
				return (&models.CdmData{Tobt: stringPtr(previousTobt), Tsat: stringPtr(activeTsat)}).Normalize(), nil
			},
			SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
				persisted = data.Clone()
				return 1, nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)
	service.client.isValid = false
	setTestCdmFrontend(service, &testutil.MockFrontendHub{})

	require.NoError(t, service.HandleReadyRequest(context.Background(), sessionID, callsign, "EKCH_DEL", "ATC"))
	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Tobt)
	assert.Equal(t, previousTobt, *persisted.Tobt)
	require.NotNil(t, persisted.Status)
	assert.Equal(t, "REA", *persisted.Status)
}

func TestHandleReadyRequest_UpdatesPastTobtToNowWithActiveTsat(t *testing.T) {
	const callsign = "SAS212"
	const sessionID = int32(212)

	pastTobt := time.Now().UTC().Add(-5 * time.Minute).Format("1504")
	activeTsat := time.Now().UTC().Add(10 * time.Minute).Format("1504")
	previousAsrt := time.Now().UTC().Add(-5 * time.Minute).Format("1504")
	readyStatus := "REA"
	var persisted *models.CdmData

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		&testutil.MockStripRepository{
			GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
				return &models.Strip{Callsign: callsign, Origin: "EKCH"}, nil
			},
			GetCdmDataForCallsignFn: func(_ context.Context, _ int32, _ string) (*models.CdmData, error) {
				if persisted != nil {
					return persisted.Clone(), nil
				}
				return (&models.CdmData{
					Tobt:   stringPtr(pastTobt),
					Tsat:   stringPtr(activeTsat),
					Asrt:   stringPtr(previousAsrt),
					Status: stringPtr(readyStatus),
				}).Normalize(), nil
			},
			SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
				persisted = data.Clone()
				return 1, nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)
	setTestCdmFrontend(service, &testutil.MockFrontendHub{})
	service.sessionMaster.Store(sessionID, true)
	markSessionLive(service, sessionID)

	beforeNow := time.Now().UTC().Format("1504")
	err := service.HandleReadyRequest(context.Background(), sessionID, callsign, "EKCH_DEL", "ATC")
	afterNow := time.Now().UTC().Format("1504")
	require.NoError(t, err)

	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Tobt)
	assert.Contains(t, []string{beforeNow, afterNow}, *persisted.Tobt)
	require.NotNil(t, persisted.Asrt)
	assert.Contains(t, []string{beforeNow, afterNow}, *persisted.Asrt)
	require.NotNil(t, persisted.Status)
	assert.Equal(t, "REA", *persisted.Status)
}

func TestHandleReadyRequest_MasterSessionRecalculatesTsatAndTtot(t *testing.T) {
	const callsign = "SAS215"
	const sessionID = int32(215)
	runway := "22R"
	previousTobt := time.Now().UTC().Add(15 * time.Minute).Format("1504")
	previousTsat := previousTobt
	previousTtot := time.Now().UTC().Add(25 * time.Minute).Format("1504")

	stored := (&models.CdmData{
		Tobt: stringPtr(previousTobt),
		Tsat: stringPtr(previousTsat),
		Ttot: stringPtr(previousTtot),
	}).Normalize()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ifps/dpi", "/ifps/setCdmData":
			w.WriteHeader(http.StatusOK)
		case "/ifps/callsign":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("true"))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: callsign,
				Session:  sessionID,
				Origin:   "EKCH",
				Runway:   &runway,
				CdmData:  stored.Clone(),
			}, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, _ int32, _ string) (*models.CdmData, error) {
			return stored.Clone(), nil
		},
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
			stored = data.Clone()
			return 1, nil
		},
		ListByOriginFn: func(_ context.Context, _ int32, origin string) ([]*models.Strip, error) {
			assert.Equal(t, "EKCH", origin)
			return []*models.Strip{{
				Callsign: callsign,
				Session:  sessionID,
				Origin:   "EKCH",
				Runway:   &runway,
				CdmData:  stored.Clone(),
			}}, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			assert.Equal(t, sessionID, id)
			return &models.Session{
				ID: id,
				ActiveRunways: pkgModels.ActiveRunways{
					DepartureRunways: []string{runway},
				},
			}, nil
		},
	}

	client := NewClient(
		WithAPIKey("test-key"),
		WithBaseURL(server.URL),
	)
	frontendHub := &testutil.MockFrontendHub{}
	configStore := NewCdmConfigStore("", "", "", time.Minute, CdmConfigDefaults{}, newFailingHTTPClient())
	sequenceService := newTestSequenceService(stripRepo, sessionRepo, configStore, frontendHub, &testutil.MockEuroscopeHub{})

	service := newTestCdmService(client, stripRepo, sessionRepo, &testutil.MockControllerRepository{})
	setTestCdmFrontend(service, frontendHub)
	service.SetConfigProvider(configStore)
	service.SetSequenceService(sequenceService)
	service.sessionMaster.Store(sessionID, true)
	markSessionLive(service, sessionID)

	beforeNow := time.Now().UTC().Format("1504")
	require.NoError(t, service.HandleReadyRequest(context.Background(), sessionID, callsign, "EKCH_DEL", "ATC"))
	afterNow := time.Now().UTC().Format("1504")

	require.NotNil(t, stored.Tobt)
	assert.Contains(t, []string{beforeNow, afterNow}, *stored.Tobt)
	require.NotNil(t, stored.Eobt)
	assert.Equal(t, *stored.Tobt, *stored.Eobt)
	require.NotNil(t, stored.Tsat)
	assert.Contains(t, []string{beforeNow, afterNow}, truncateCDMClockValue(*stored.Tsat))
	assert.NotEqual(t, previousTsat, *stored.Tsat)
	require.NotNil(t, stored.Ttot)
	assert.NotEqual(t, previousTtot, *stored.Ttot)
	require.NotNil(t, stored.Status)
	assert.Equal(t, "REA", *stored.Status)
	assert.False(t, stored.NeedsLocalRecalculation())
	require.NotEmpty(t, frontendHub.CdmUpdates)
	lastUpdate := frontendHub.CdmUpdates[len(frontendHub.CdmUpdates)-1]
	assert.Contains(t, []string{beforeNow, afterNow}, lastUpdate.Tsat)
	assert.NotEmpty(t, lastUpdate.Event.Ttot)
	assert.Equal(t, "REA", lastUpdate.Event.Status)
}

func TestHandleReadyRequest_MasterSessionUsesNextFreeGapWithoutMovingExistingFlights(t *testing.T) {
	const callsign = "SAS500"
	const sessionID = int32(500)
	runway := "22R"
	beforeNow := time.Now().UTC().Format("1504")
	afterNow := beforeNow

	firstTsat := beforeNow
	secondTsat := addMinutes(firstTsat, 2)
	thirdTsat := addMinutes(firstTsat, 4)

	stored := map[string]*models.CdmData{
		"SAS123": (&models.CdmData{
			Tobt:                  stringPtr(firstTsat),
			Tsat:                  stringPtr(firstTsat),
			Ttot:                  stringPtr(addMinutes(firstTsat, 10)),
			TobtManuallyConfirmed: true,
		}).Normalize(),
		"SAS456": (&models.CdmData{
			Tobt:                  stringPtr(secondTsat),
			Tsat:                  stringPtr(secondTsat),
			Ttot:                  stringPtr(addMinutes(secondTsat, 10)),
			TobtManuallyConfirmed: true,
		}).Normalize(),
		"SAS400": (&models.CdmData{
			Tobt:                  stringPtr(thirdTsat),
			Tsat:                  stringPtr(thirdTsat),
			Ttot:                  stringPtr(addMinutes(thirdTsat, 10)),
			TobtManuallyConfirmed: true,
		}).Normalize(),
		callsign: (&models.CdmData{
			Tobt: stringPtr(addMinutes(firstTsat, 20)),
			Tsat: stringPtr(addMinutes(firstTsat, 20)),
			Ttot: stringPtr(addMinutes(firstTsat, 30)),
		}).Normalize(),
	}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, cs string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: cs,
				Session:  sessionID,
				Origin:   "EKCH",
				Runway:   &runway,
				CdmData:  stored[cs].Clone(),
			}, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, _ int32, cs string) (*models.CdmData, error) {
			return stored[cs].Clone(), nil
		},
		SetCdmDataFn: func(_ context.Context, _ int32, cs string, data *models.CdmData) (int64, error) {
			stored[cs] = data.Clone()
			return 1, nil
		},
		ListByOriginFn: func(_ context.Context, _ int32, origin string) ([]*models.Strip, error) {
			assert.Equal(t, "EKCH", origin)
			return []*models.Strip{
				{Callsign: "SAS123", Session: sessionID, Origin: "EKCH", Runway: &runway, CdmData: stored["SAS123"].Clone()},
				{Callsign: "SAS456", Session: sessionID, Origin: "EKCH", Runway: &runway, CdmData: stored["SAS456"].Clone()},
				{Callsign: "SAS400", Session: sessionID, Origin: "EKCH", Runway: &runway, CdmData: stored["SAS400"].Clone()},
				{Callsign: callsign, Session: sessionID, Origin: "EKCH", Runway: &runway, CdmData: stored[callsign].Clone()},
			}, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			assert.Equal(t, sessionID, id)
			return &models.Session{
				ID: id,
				ActiveRunways: pkgModels.ActiveRunways{
					DepartureRunways: []string{runway},
				},
			}, nil
		},
	}

	frontendHub := &testutil.MockFrontendHub{}
	configStore := NewCdmConfigStore("", "", "", time.Minute, CdmConfigDefaults{}, newFailingHTTPClient())
	configStore.configs["EKCH"] = &CdmAirportConfig{
		Airport:            "EKCH",
		DefaultRate:        30,
		DefaultTaxiMinutes: 10,
	}
	sequenceService := newTestSequenceService(stripRepo, sessionRepo, configStore, frontendHub, &testutil.MockEuroscopeHub{})

	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, sessionRepo, &testutil.MockControllerRepository{})
	service.client.isValid = false
	setTestCdmFrontend(service, frontendHub)
	service.SetConfigProvider(configStore)
	service.SetSequenceService(sequenceService)
	service.sessionMaster.Store(sessionID, true)

	require.NoError(t, service.HandleReadyRequest(context.Background(), sessionID, callsign, "EKCH_DEL", "ATC"))
	afterNow = time.Now().UTC().Format("150405")

	expectedTsats := []string{addMinutes(beforeNow, 6), addMinutes(afterNow, 6)}
	expectedTtots := []string{addMinutes(beforeNow, 16), addMinutes(afterNow, 16)}

	assert.Equal(t, firstTsat, *stored["SAS123"].Tsat)
	assert.Equal(t, addMinutes(firstTsat, 10), *stored["SAS123"].Ttot)
	assert.Equal(t, secondTsat, *stored["SAS456"].Tsat)
	assert.Equal(t, addMinutes(secondTsat, 10), *stored["SAS456"].Ttot)
	assert.Equal(t, thirdTsat, *stored["SAS400"].Tsat)
	assert.Equal(t, addMinutes(thirdTsat, 10), *stored["SAS400"].Ttot)
	assert.Contains(t, expectedTsats, *stored[callsign].Tsat)
	assert.Contains(t, expectedTtots, *stored[callsign].Ttot)
	assert.Equal(t, "REA", *stored[callsign].Status)
	assert.False(t, stored[callsign].NeedsLocalRecalculation())
}

func TestHandleReadyRequest_NonMasterWithoutValidClient_DoesNotRecalculate(t *testing.T) {
	const callsign = "SAS310"
	const sessionID = int32(310)
	runway := "22R"
	currentTobt := time.Now().UTC().Format("1504")

	var recalculated bool
	stored := (&models.CdmData{
		Tobt: stringPtr(currentTobt),
		Tsat: stringPtr(time.Now().UTC().Add(10 * time.Minute).Format("1504")),
	}).Normalize()

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: callsign,
				Session:  sessionID,
				Origin:   "EKCH",
				Runway:   &runway,
				CdmData:  stored.Clone(),
			}, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, _ int32, _ string) (*models.CdmData, error) {
			return stored.Clone(), nil
		},
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
			stored = data.Clone()
			return 1, nil
		},
		ListByOriginFn: func(_ context.Context, _ int32, origin string) ([]*models.Strip, error) {
			recalculated = true
			assert.Equal(t, "EKCH", origin)
			return []*models.Strip{{
				Callsign: callsign,
				Session:  sessionID,
				Origin:   "EKCH",
				Runway:   &runway,
				CdmData:  stored.Clone(),
			}}, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			assert.Equal(t, sessionID, id)
			return &models.Session{
				ID: id,
				ActiveRunways: pkgModels.ActiveRunways{
					DepartureRunways: []string{runway},
				},
			}, nil
		},
	}

	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, sessionRepo, &testutil.MockControllerRepository{})
	service.client.isValid = false
	setTestCdmFrontend(service, &testutil.MockFrontendHub{})
	setTestCdmEuroscope(service, &testutil.MockEuroscopeHub{})
	service.SetConfigProvider(NewCdmConfigStore("", "", "", time.Minute, CdmConfigDefaults{}, newFailingHTTPClient()))
	service.SetSequenceService(newTestSequenceService(stripRepo, sessionRepo, NewCdmConfigStore("", "", "", time.Minute, CdmConfigDefaults{}, newFailingHTTPClient()), &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{}))

	require.NoError(t, service.HandleReadyRequest(context.Background(), sessionID, callsign, "EKCH_DEL", "ATC"))
	assert.False(t, recalculated)
}

func TestHandleReadyRequest_UpdatesTobtToNowWhenTsatExpiredOrPhaseInvalid(t *testing.T) {
	tests := []struct {
		name string
		data *models.CdmData
	}{
		{
			name: "expired tsat",
			data: (&models.CdmData{
				Tobt: stringPtr(time.Now().UTC().Add(-15 * time.Minute).Format("1504")),
				Tsat: stringPtr(time.Now().UTC().Add(-10 * time.Minute).Format("1504")),
			}).Normalize(),
		},
		{
			name: "invalid phase",
			data: (&models.CdmData{
				Tobt:  stringPtr(time.Now().UTC().Add(-5 * time.Minute).Format("1504")),
				Phase: stringPtr("I"),
			}).Normalize(),
		},
		{
			name: "expired tsat with current tobt",
			data: (&models.CdmData{
				Tobt: stringPtr(time.Now().UTC().Format("1504")),
				Tsat: stringPtr(time.Now().UTC().Add(-10 * time.Minute).Format("1504")),
			}).Normalize(),
		},
		{
			name: "invalid phase with current tobt",
			data: (&models.CdmData{
				Tobt:  stringPtr(time.Now().UTC().Format("1504")),
				Phase: stringPtr("I"),
			}).Normalize(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const callsign = "SAS213"
			const sessionID = int32(213)
			var persisted *models.CdmData

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			service := newTestCdmService(
				NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
				&testutil.MockStripRepository{
					GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
						return &models.Strip{Callsign: callsign, Origin: "EKCH"}, nil
					},
					GetCdmDataForCallsignFn: func(_ context.Context, _ int32, _ string) (*models.CdmData, error) {
						if persisted != nil {
							return persisted.Clone(), nil
						}
						return tt.data.Clone(), nil
					},
					SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
						persisted = data.Clone()
						return 1, nil
					},
				},
				&testutil.MockSessionRepository{},
				&testutil.MockControllerRepository{},
			)
			setTestCdmFrontend(service, &testutil.MockFrontendHub{})
			service.sessionMaster.Store(sessionID, true)
			markSessionLive(service, sessionID)

			beforeNow := time.Now().UTC().Format("1504")
			err := service.HandleReadyRequest(context.Background(), sessionID, callsign, "EKCH_DEL", "ATC")
			afterNow := time.Now().UTC().Format("1504")
			require.NoError(t, err)

			require.NotNil(t, persisted)
			require.NotNil(t, persisted.Tobt)
			assert.Contains(t, []string{beforeNow, afterNow}, *persisted.Tobt)
			require.NotNil(t, persisted.Status)
			assert.Equal(t, "REA", *persisted.Status)
			assert.True(t, persisted.Recalculate)
		})
	}
}

func TestHandleReadyRequest_PhaseInvalidEventuallyProducesNewTsat(t *testing.T) {
	const callsign = "SAS214"
	const sessionID = int32(214)
	runway := "22R"
	nowTobt := time.Now().UTC().Format("1504")
	phase := "I"

	stored := (&models.CdmData{
		Tobt:  stringPtr(nowTobt),
		Phase: stringPtr(phase),
	}).Normalize()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("true"))
	}))
	defer server.Close()

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				Callsign: callsign,
				Session:  sessionID,
				Origin:   "EKCH",
				Runway:   &runway,
				CdmData:  stored.Clone(),
			}, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, _ int32, _ string) (*models.CdmData, error) {
			return stored.Clone(), nil
		},
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
			stored = data.Clone()
			return 1, nil
		},
		ListByOriginFn: func(_ context.Context, _ int32, origin string) ([]*models.Strip, error) {
			assert.Equal(t, "EKCH", origin)
			return []*models.Strip{{
				Callsign: callsign,
				Session:  sessionID,
				Origin:   "EKCH",
				Runway:   &runway,
				CdmData:  stored.Clone(),
			}}, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			assert.Equal(t, sessionID, id)
			return &models.Session{
				ID: id,
				ActiveRunways: pkgModels.ActiveRunways{
					DepartureRunways: []string{runway},
				},
			}, nil
		},
	}

	client := NewClient(
		WithAPIKey("test-key"),
		WithBaseURL(server.URL),
	)
	frontendHub := &testutil.MockFrontendHub{}
	configStore := NewCdmConfigStore("", "", "", time.Minute, CdmConfigDefaults{}, newFailingHTTPClient())
	sequenceService := newTestSequenceService(stripRepo, sessionRepo, configStore, frontendHub, &testutil.MockEuroscopeHub{})

	service := newTestCdmService(client, stripRepo, sessionRepo, &testutil.MockControllerRepository{})
	setTestCdmFrontend(service, frontendHub)
	service.SetConfigProvider(configStore)
	service.SetSequenceService(sequenceService)
	service.sessionMaster.Store(sessionID, true)
	markSessionLive(service, sessionID)
	service.debouncer = newRecalcDebouncer(time.Millisecond)

	err := service.HandleReadyRequest(context.Background(), sessionID, callsign, "EKCH_DEL", "ATC")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return stored.Tsat != nil &&
			*stored.Tsat != "" &&
			(stored.Phase == nil || *stored.Phase == "") &&
			!stored.NeedsLocalRecalculation()
	}, time.Second, 10*time.Millisecond)
}

func TestPrepareTobtUpdate_SameValueMarksRecalculationWhenTsatExpired(t *testing.T) {
	const sessionID = int32(214)
	const callsign = "SAS214"

	now := time.Date(2026, 5, 10, 18, 0, 0, 0, time.UTC)
	currentTobt := "1800"
	expiredTsat := "1749"

	service := newTestCdmService(newTestClientWithAirportMasters(nil), &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, session int32, cs string) (*models.Strip, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return &models.Strip{Callsign: cs, Session: sessionID, Origin: "EKCH"}, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, session int32, cs string) (*models.CdmData, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return (&models.CdmData{
				Tobt: &currentTobt,
				Tsat: &expiredTsat,
			}).Normalize(), nil
		},
	}, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})

	strip, _, updated, previousTobt, changed, shouldTriggerRecalculate, err := service.prepareTobtUpdate(context.Background(), sessionID, callsign, currentTobt, "EKCH_DEL", now)
	require.NoError(t, err)
	require.NotNil(t, strip)
	require.NotNil(t, updated)
	assert.Equal(t, currentTobt, previousTobt)
	assert.True(t, changed)
	assert.True(t, shouldTriggerRecalculate)
	assert.True(t, updated.Recalculate)
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
	service := newTestCdmService(
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

	setTestCdmFrontend(service, &testutil.MockFrontendHub{})
	service.sessionMaster.Store(sessionID, true)
	markSessionLive(service, sessionID)

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

	service := newTestCdmService(
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

	setTestCdmEuroscope(service, &testutil.MockEuroscopeHub{})
	service.sessionMaster.Store(sessionID, true)
	markSessionLive(service, sessionID)

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
	tsat := "153000"
	ttot := "154000"
	ecfmp := "REGUL"
	data := &models.CdmData{
		Tobt:    stringPtr(tobt),
		Tsat:    stringPtr(tsat),
		Ttot:    stringPtr(ttot),
		EcfmpID: stringPtr(ecfmp),
	}

	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		nil, nil, nil,
	)
	markSessionLive(service, 1)

	service.pushViffAfterRecalcAsync(1, "BAW999", nil, data)

	require.Eventually(t, func() bool {
		select {
		case q := <-setCdmCh:
			return q.Get("tobt") == "152500" && q.Get("tsat") == tsat && q.Get("ttot") == ttot && q.Get("reason") == ecfmp
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

	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithBaseURL(server.URL)),
		nil, nil, nil,
	)
	markSessionLive(service, 1)

	service.pushViffAfterRecalcAsync(1, "EIN001", nil, data)

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

type trackingConfigProvider struct {
	stubConfigProvider
	airport string
	active  bool
	called  bool
}

func (s *trackingConfigProvider) SetLvo(airport string, active bool) {
	s.airport = airport
	s.active = active
	s.called = true
}

// ---- SetSessionCdmMaster ----

func TestSetSessionCdmMaster_True_UpdatesDBAndCachesAndRegistersMaster(t *testing.T) {
	const sessionID = int32(77)
	const airport = "EKCH"

	var dbUpdatedID int32
	var dbUpdatedMaster bool
	type masterCall struct {
		path     string
		position string
	}
	var gotMasterCall *masterCall

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMasterCall = &masterCall{
			path:     r.URL.Path,
			position: r.URL.Query().Get("position"),
		}
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
	service := newTestCdmService(client, &testutil.MockStripRepository{
		ListByOriginFn: func(context.Context, int32, string) ([]*models.Strip, error) {
			return nil, nil
		},
	}, sessionRepo, &testutil.MockControllerRepository{})
	setTestCdmEuroscope(service, &testutil.MockEuroscopeHub{
		GetMasterCallsignFn: func(int32) string {
			return "EKCH_A_TWR"
		},
	})

	err := service.SetSessionCdmMaster(context.Background(), sessionID, true)
	require.NoError(t, err)

	assert.Equal(t, sessionID, dbUpdatedID, "UpdateCdmMaster called with wrong session ID")
	assert.True(t, dbUpdatedMaster, "UpdateCdmMaster called with master=false")

	v, ok := service.sessionMaster.Load(sessionID)
	assert.True(t, ok && v.(bool), "sessionMaster map should have true for session")

	// Allow the async goroutine to call the HTTP endpoint.
	require.Eventually(t, func() bool {
		return gotMasterCall != nil
	}, time.Second, 10*time.Millisecond, "expected master registration HTTP call")
	assert.Equal(t, "/airport/setMaster", gotMasterCall.path)
	assert.Equal(t, DefaultMasterPosition, gotMasterCall.position)
}

func TestSetSessionCdmMaster_True_NormalizesExistingFarFutureEobtAndSyncsBack(t *testing.T) {
	const sessionID = int32(78)
	const airport = "EKCH"
	const callsign = "SAS130"
	const masterCid = "654321"
	now := time.Now().UTC()
	rawFutureEobt := truncateCDMClockValue(addMinutes(timeToClock(now), 60))
	currentTobt := addMinutes(timeToClock(now), 20)
	expectedClamped := truncateCDMClockValue(addMinutes(timeToClock(now), masterEobtClampTarget))

	var persisted *models.CdmData
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(_ context.Context, gotSession int32, gotAirport string) ([]*models.Strip, error) {
			assert.Equal(t, sessionID, gotSession)
			assert.Equal(t, airport, gotAirport)
			return []*models.Strip{{
				Callsign: callsign,
				Session:  sessionID,
				Origin:   airport,
				CdmData: (&models.CdmData{
					Eobt: testStringPtr(rawFutureEobt),
					Tobt: testStringPtr(currentTobt),
				}).Normalize(),
			}}, nil
		},
		SetCdmDataFn: func(_ context.Context, session int32, cs string, data *models.CdmData) (int64, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			persisted = data.Clone()
			return 1, nil
		},
		GetCdmDataForCallsignFn: func(_ context.Context, session int32, cs string) (*models.CdmData, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return persisted.Clone(), nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		UpdateCdmMasterFn: func(_ context.Context, id int32, master bool) error {
			assert.Equal(t, sessionID, id)
			assert.True(t, master)
			return nil
		},
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Name: "LIVE", Airport: airport}, nil
		},
	}
	controllerRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, session int32, callsign string) (*models.Controller, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, "EKCH_A_TWR", callsign)
			return &models.Controller{Cid: testStringPtr(masterCid)}, nil
		},
	}
	euroscopeHub := &testutil.MockEuroscopeHub{
		GetMasterCallsignFn: func(int32) string {
			return "EKCH_A_TWR"
		},
	}

	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, sessionRepo, controllerRepo)
	service.client.isValid = false
	setTestCdmEuroscope(service, euroscopeHub)

	err := service.SetSessionCdmMaster(context.Background(), sessionID, true)
	require.NoError(t, err)

	require.NotNil(t, persisted)
	assert.Equal(t, expectedClamped, valueOrEmpty(persisted.Eobt))
	assert.Equal(t, expectedClamped, valueOrEmpty(persisted.Tobt))
	require.NotNil(t, persisted.Calculation)
	require.Len(t, persisted.Calculation.ReasonMarkers, 1)
	assert.Equal(t, eobtCappedReasonKind, persisted.Calculation.ReasonMarkers[0].Kind)
	require.Len(t, euroscopeHub.Eobts, 1)
	assert.Equal(t, masterCid, euroscopeHub.Eobts[0].Cid)
	assert.Equal(t, expectedClamped, euroscopeHub.Eobts[0].Eobt)
}

func TestSetSessionCdmMaster_True_TriggersImmediateRecalculate(t *testing.T) {
	const sessionID = int32(79)
	const airport = "EKCH"

	recalcTriggered := make(chan struct{}, 1)
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(_ context.Context, gotSession int32, gotAirport string) ([]*models.Strip, error) {
			assert.Equal(t, sessionID, gotSession)
			assert.Equal(t, airport, gotAirport)
			select {
			case recalcTriggered <- struct{}{}:
			default:
			}
			return nil, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		UpdateCdmMasterFn: func(_ context.Context, id int32, master bool) error {
			assert.Equal(t, sessionID, id)
			assert.True(t, master)
			return nil
		},
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Name: "LIVE", Airport: airport}, nil
		},
	}

	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, sessionRepo, &testutil.MockControllerRepository{})
	service.client.isValid = false
	service.debouncer = newRecalcDebouncer(time.Millisecond)
	service.SetSequenceService(newTestSequenceService(
		stripRepo,
		sessionRepo,
		&stubConfigProvider{},
		&testutil.MockFrontendHub{},
		&testutil.MockEuroscopeHub{},
	))

	err := service.SetSessionCdmMaster(context.Background(), sessionID, true)
	require.NoError(t, err)

	select {
	case <-recalcTriggered:
	case <-time.After(time.Second):
		t.Fatal("expected SetSessionCdmMaster(true) to trigger an immediate recalculation")
	}
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
	service := newTestCdmService(client, &testutil.MockStripRepository{}, sessionRepo, &testutil.MockControllerRepository{})
	setTestCdmEuroscope(service, &testutil.MockEuroscopeHub{
		GetMasterCallsignFn: func(int32) string {
			return "EKCH_A_TWR"
		},
	})
	// Pre-populate the cache as if the session was previously master.
	service.sessionMaster.Store(sessionID, true)

	err := service.SetSessionCdmMaster(context.Background(), sessionID, false)
	require.NoError(t, err)

	require.NotNil(t, dbUpdatedMaster)
	assert.False(t, *dbUpdatedMaster, "UpdateCdmMaster should be called with master=false")

	_, ok := service.sessionMaster.Load(sessionID)
	assert.False(t, ok, "sessionMaster map entry should have been removed")

	// The async clear call should always use the shared FlightStrips master position.
	require.Eventually(t, func() bool {
		return gotClearCall != nil
	}, time.Second, 10*time.Millisecond, "expected ClearMasterAirport HTTP call")
	assert.Equal(t, "/airport/removeMaster", gotClearCall.path)
	assert.Equal(t, DefaultMasterPosition, gotClearCall.position)
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

	seqSvc := newTestSequenceService(stripRepo, &testutil.MockSessionRepository{}, &stubConfigProvider{}, &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})
	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})

	service.debouncer = newRecalcDebouncer(time.Millisecond)
	service.SetSequenceService(seqSvc)

	// Session 99 is NOT in the sessionMaster map.
	service.TriggerRecalculate(context.Background(), 99, "EKCH")

	// Give debouncer time to fire (if it were going to).
	time.Sleep(50 * time.Millisecond)
	assert.False(t, listByOriginCalled, "RecalculateAirport should not be called for non-master session")
}

func TestTriggerRecalculate_SkipsNonMasterSessionWhenClientUnavailable(t *testing.T) {
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

	seqSvc := newTestSequenceService(stripRepo, sessionRepo, &stubConfigProvider{}, &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})
	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, sessionRepo, &testutil.MockControllerRepository{})
	service.client.isValid = false
	service.debouncer = newRecalcDebouncer(time.Millisecond)
	service.SetSequenceService(seqSvc)

	service.TriggerRecalculate(context.Background(), 99, "EKCH")

	select {
	case <-listByOriginCalled:
		t.Fatal("expected non-master session to skip recalculation even when CDM client is unavailable")
	case <-time.After(50 * time.Millisecond):
	}
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

	seqSvc := newTestSequenceService(stripRepo, sessionRepo, &stubConfigProvider{}, &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})
	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, sessionRepo, &testutil.MockControllerRepository{})

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

func TestTriggerRecalculate_DetachesFromCanceledCallerContext(t *testing.T) {
	listByOriginCalled := make(chan struct{}, 1)
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(ctx context.Context, _ int32, _ string) ([]*models.Strip, error) {
			require.NoError(t, ctx.Err())
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

	seqSvc := newTestSequenceService(stripRepo, sessionRepo, &stubConfigProvider{}, &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})
	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, sessionRepo, &testutil.MockControllerRepository{})
	service.debouncer = newRecalcDebouncer(time.Millisecond)
	service.SetSequenceService(seqSvc)

	const sessionID = int32(56)
	service.sessionMaster.Store(sessionID, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	service.TriggerRecalculate(ctx, sessionID, "EKCH")

	select {
	case <-listByOriginCalled:
	case <-time.After(time.Second):
		t.Fatal("expected RecalculateAirport to run even after caller context cancellation")
	}
}

func TestTriggerRecalculateForAirport_RunsMatchingMasterSessions(t *testing.T) {
	listByOriginCalled := make(chan int32, 2)
	stripRepo := &testutil.MockStripRepository{
		ListByOriginFn: func(_ context.Context, session int32, _ string) ([]*models.Strip, error) {
			select {
			case listByOriginCalled <- session:
			default:
			}
			return nil, nil
		},
	}

	sessions := map[int32]*models.Session{
		55: {ID: 55, Airport: "EKCH"},
		56: {ID: 56, Airport: "ESSA"},
		57: {ID: 57, Airport: "EKCH"},
	}
	sessionRepo := &testutil.MockSessionRepository{
		ListFn: func(_ context.Context) ([]*models.Session, error) {
			return []*models.Session{sessions[55], sessions[56], sessions[57]}, nil
		},
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			return sessions[id], nil
		},
	}

	seqSvc := newTestSequenceService(stripRepo, sessionRepo, &stubConfigProvider{}, &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})
	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, sessionRepo, &testutil.MockControllerRepository{})

	service.debouncer = newRecalcDebouncer(time.Millisecond)
	service.SetSequenceService(seqSvc)
	service.sessionMaster.Store(int32(55), true)

	require.NoError(t, service.TriggerRecalculateForAirport(context.Background(), "EKCH"))

	select {
	case sessionID := <-listByOriginCalled:
		assert.Equal(t, int32(55), sessionID)
	case <-time.After(time.Second):
		t.Fatal("expected airport recalculation to be scheduled for matching master session")
	}

	select {
	case sessionID := <-listByOriginCalled:
		t.Fatalf("unexpected additional recalculation for session %d", sessionID)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestPushViffAfterRecalcAsync_NonLiveSessionSkipsViff(t *testing.T) {
	t.Parallel()

	const sessionID = int32(222)
	data := &models.CdmData{Tsat: stringPtr("153000")}
	service := newTestCdmService(
		NewClient(WithAPIKey("test-key"), WithHTTPClient(newFailingHTTPClient())),
		nil, nil, nil,
	)
	markSessionNonLive(service, sessionID)

	service.pushViffAfterRecalcAsync(sessionID, "SAS222", nil, data)
}

// ---- syncSessions ----

func TestSyncSessions_RegistersMasterForCdmMasterSessions(t *testing.T) {
	type masterCall struct {
		path     string
		position string
	}
	var gotMasterCall *masterCall

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/airport/setMaster" {
			gotMasterCall = &masterCall{
				path:     r.URL.Path,
				position: r.URL.Query().Get("position"),
			}
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
	service := newTestCdmService(client, stripRepo, sessionRepo, &testutil.MockControllerRepository{})
	setTestCdmFrontend(service, &testutil.MockFrontendHub{})
	setTestCdmEuroscope(service, &testutil.MockEuroscopeHub{
		GetMasterCallsignFn: func(int32) string {
			return "EKCH_A_TWR"
		},
	})

	err := service.syncSessions(context.Background())
	require.NoError(t, err)

	v, ok := service.sessionMaster.Load(sessionID)
	assert.True(t, ok && v.(bool), "sessionMaster should be populated for CdmMaster session")

	require.Eventually(t, func() bool {
		return gotMasterCall != nil
	}, time.Second, 10*time.Millisecond, "expected master registration HTTP call")
	assert.Equal(t, "/airport/setMaster", gotMasterCall.path)
	assert.Equal(t, DefaultMasterPosition, gotMasterCall.position)
}

func TestSyncSessions_TriggersImmediateRecalculateForPersistedMasterSession(t *testing.T) {
	const sessionID = int32(13)
	const airport = "EKCH"

	recalcTriggered := make(chan struct{}, 1)
	stripRepo := &testutil.MockStripRepository{
		GetCdmDataFn: func(_ context.Context, _ int32) ([]*models.CdmDataRow, error) {
			return nil, nil
		},
		ListByOriginFn: func(_ context.Context, gotSession int32, gotAirport string) ([]*models.Strip, error) {
			assert.Equal(t, sessionID, gotSession)
			assert.Equal(t, airport, gotAirport)
			select {
			case recalcTriggered <- struct{}{}:
			default:
			}
			return nil, nil
		},
	}
	sessionRepo := &testutil.MockSessionRepository{
		ListFn: func(_ context.Context) ([]*models.Session, error) {
			return []*models.Session{
				{ID: sessionID, Name: "SWEATBOX", Airport: airport, CdmMaster: true},
			}, nil
		},
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Name: "SWEATBOX", Airport: airport}, nil
		},
	}

	service := newTestCdmService(newTestClientWithAirportMasters(nil), stripRepo, sessionRepo, &testutil.MockControllerRepository{})
	service.client.isValid = false
	service.debouncer = newRecalcDebouncer(time.Millisecond)
	service.SetSequenceService(newTestSequenceService(
		stripRepo,
		sessionRepo,
		&stubConfigProvider{},
		&testutil.MockFrontendHub{},
		&testutil.MockEuroscopeHub{},
	))

	err := service.syncSessions(context.Background())
	require.NoError(t, err)

	select {
	case <-recalcTriggered:
	case <-time.After(time.Second):
		t.Fatal("expected persisted master session to trigger recalculation during startup sync")
	}
}

func TestSyncSessions_DoesNotRegisterMasterForSlaveSession(t *testing.T) {
	const sessionID = int32(11)

	sessionRepo := &testutil.MockSessionRepository{
		ListFn: func(_ context.Context) ([]*models.Session, error) {
			return []*models.Session{
				{ID: sessionID, Name: "LIVE", Airport: "EKCH", CdmMaster: false},
			}, nil
		},
	}

	client := NewClient(WithAPIKey("test-key"), WithHTTPClient(newFailingHTTPClient()))
	service := newTestCdmService(client, &testutil.MockStripRepository{}, sessionRepo, &testutil.MockControllerRepository{})
	// Disable HTTP calls so syncCdmData exits early (client must have isValid=false to skip the vIFF sync).
	service.client.isValid = false
	setTestCdmFrontend(service, &testutil.MockFrontendHub{})
	setTestCdmEuroscope(service, &testutil.MockEuroscopeHub{})

	err := service.syncSessions(context.Background())
	require.NoError(t, err)

	_, ok := service.sessionMaster.Load(sessionID)
	assert.False(t, ok, "sessionMaster should not be populated for slave session")
}

func TestSyncSessions_DoesNotUseViffForNonLiveMasterSession(t *testing.T) {
	const sessionID = int32(12)

	sessionRepo := &testutil.MockSessionRepository{
		ListFn: func(_ context.Context) ([]*models.Session, error) {
			return []*models.Session{
				{ID: sessionID, Name: "SWEATBOX", Airport: "EKCH", CdmMaster: true},
			}, nil
		},
	}

	client := NewClient(WithAPIKey("test-key"), WithHTTPClient(newFailingHTTPClient()))
	service := newTestCdmService(client, &testutil.MockStripRepository{}, sessionRepo, &testutil.MockControllerRepository{})

	err := service.syncSessions(context.Background())
	require.NoError(t, err)

	v, ok := service.sessionMaster.Load(sessionID)
	assert.True(t, ok && v.(bool), "sessionMaster should be populated for non-LIVE master session")
	live, ok := service.sessionUsesViff.Load(sessionID)
	assert.True(t, ok)
	assert.False(t, live.(bool), "non-LIVE session must not be marked as vIFF-enabled")
}

func TestSyncSessions_SynchronizesLvoFromRunwayStatus(t *testing.T) {
	const sessionID = int32(12)

	sessionRepo := &testutil.MockSessionRepository{
		ListFn: func(_ context.Context) ([]*models.Session, error) {
			return []*models.Session{
				{
					ID:      sessionID,
					Name:    "LIVE",
					Airport: "EKCH",
					ActiveRunways: pkgModels.ActiveRunways{
						RunwayStatus: map[string]string{"04L/22L": "LOW_VIS"},
					},
				},
			}, nil
		},
	}

	client := NewClient(WithAPIKey("test-key"), WithHTTPClient(newFailingHTTPClient()))
	service := newTestCdmService(client, &testutil.MockStripRepository{}, sessionRepo, &testutil.MockControllerRepository{})
	service.client.isValid = false
	provider := &trackingConfigProvider{}
	service.SetConfigProvider(provider)

	err := service.syncSessions(context.Background())
	require.NoError(t, err)
	assert.True(t, provider.called)
	assert.Equal(t, "EKCH", provider.airport)
	assert.True(t, provider.active)
}
