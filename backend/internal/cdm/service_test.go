package cdm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestHandleReadyRequest_NoAirportMaster_DoesNothing(t *testing.T) {
	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Airport: "EKCH"}, nil
		},
	}

	controllerRepo := &testutil.MockControllerRepository{}
	stripRepo := &testutil.MockStripRepository{}
	frontendHub := &testutil.MockFrontendHub{}
	euroscopeHub := &testutil.MockEuroscopeHub{}

	service := NewCdmService(newTestClientWithAirportMasters(nil), stripRepo, sessionRepo, controllerRepo)
	service.SetFrontendHub(frontendHub)
	service.SetEuroscopeHub(euroscopeHub)

	err := service.HandleReadyRequest(context.Background(), 7, "SAS123")
	require.NoError(t, err)

	assert.Empty(t, frontendHub.CdmWaits)
	assert.Empty(t, euroscopeHub.CdmReadyRequests)
}

func TestHandleReadyRequest_FastPathTargetsConnectedMaster(t *testing.T) {
	t.Setenv("CDM_ES_FAST_PATH", "true")

	const sessionID = int32(7)
	const callsign = "SAS123"
	const targetPosition = "EKCH_B_GND"
	const cid = "1234567"

	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Airport: "EKCH"}, nil
		},
	}

	controllerRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, session int32, controllerCallsign string) (*models.Controller, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, targetPosition, controllerCallsign)
			return &models.Controller{Session: session, Callsign: controllerCallsign, Cid: stringPtr(cid)}, nil
		},
	}

	var persisted *models.CdmData
	stripRepo := &testutil.MockStripRepository{
		GetCdmDataForCallsignFn: func(_ context.Context, session int32, cs string) (*models.CdmData, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return (&models.CdmData{}).Normalize(), nil
		},
		SetCdmDataFn: func(_ context.Context, session int32, cs string, data *models.CdmData) (int64, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			persisted = data.Clone()
			return 1, nil
		},
	}

	frontendHub := &testutil.MockFrontendHub{}
	euroscopeHub := &testutil.MockEuroscopeHub{}

	service := NewCdmService(
		newTestClientWithAirportMasters([]AirportMaster{{ICAO: "EKCH", Position: targetPosition}}),
		stripRepo,
		sessionRepo,
		controllerRepo,
	)
	service.SetFrontendHub(frontendHub)
	service.SetEuroscopeHub(euroscopeHub)

	err := service.HandleReadyRequest(context.Background(), sessionID, callsign)
	require.NoError(t, err)

	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Pending)
	require.NotNil(t, persisted.Pending.RequestedAt)
	require.NotNil(t, persisted.Pending.TargetPosition)
	assert.Equal(t, "euroscope", persisted.Pending.Via)
	assert.Equal(t, targetPosition, *persisted.Pending.TargetPosition)
	assert.Nil(t, persisted.Canonical.Status)

	require.Len(t, frontendHub.CdmWaits, 1)
	assert.Equal(t, callsign, frontendHub.CdmWaits[0].Callsign)

	require.Len(t, euroscopeHub.CdmReadyRequests, 1)
	assert.Equal(t, sessionID, euroscopeHub.CdmReadyRequests[0].Session)
	assert.Equal(t, cid, euroscopeHub.CdmReadyRequests[0].Cid)
	assert.Equal(t, callsign, euroscopeHub.CdmReadyRequests[0].Callsign)
}

func TestHandleReadyRequest_FallbacksToAPIWhenMasterNotConnected(t *testing.T) {
	const sessionID = int32(11)
	const callsign = "EZY456"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ifps/setCdmStatus", r.URL.Path)
		assert.Equal(t, callsign, r.URL.Query().Get("callsign"))
		_, _ = w.Write([]byte("true"))
	}))
	defer server.Close()

	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
			return &models.Session{ID: id, Airport: "EKCH"}, nil
		},
	}

	controllerRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Controller, error) {
			return &models.Controller{Callsign: "EKCH_B_GND", Cid: nil}, nil
		},
	}

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
		WithAirportMasterCacheTTL(time.Minute),
	)
	client.storeAirportMasters(time.Now(), []AirportMaster{{ICAO: "EKCH", Position: "EKCH_B_GND"}})

	service := NewCdmService(client, stripRepo, sessionRepo, controllerRepo)
	service.SetFrontendHub(frontendHub)
	service.SetEuroscopeHub(euroscopeHub)

	err := service.HandleReadyRequest(context.Background(), sessionID, callsign)
	require.NoError(t, err)

	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Canonical.Status)
	assert.True(t, strings.HasPrefix(*persisted.Canonical.Status, "REQTOBT/"))
	assert.True(t, strings.HasSuffix(*persisted.Canonical.Status, "/ATC"))
	assert.Nil(t, persisted.Pending)

	require.Len(t, frontendHub.CdmWaits, 1)
	assert.Equal(t, callsign, frontendHub.CdmWaits[0].Callsign)
	assert.Empty(t, euroscopeHub.CdmReadyRequests)
}

func stringPtr(value string) *string {
	return &value
}

func TestHandleLocalObservation_PersistsOverridesAndBroadcastsEffectiveValues(t *testing.T) {
	const sessionID = int32(22)
	const callsign = "SAS999"
	const sourcePosition = "EKCH_B_GND"

	existing := &models.CdmData{
		Canonical: models.CdmCanonical{
			Eobt: stringPtr("1200"),
			Tobt: stringPtr("1205"),
			Tsat: stringPtr("1210"),
			Ctot: stringPtr("1220"),
		},
		Pending: &models.CdmPendingRequest{
			Via: "euroscope",
		},
	}

	var persisted *models.CdmData
	stripRepo := &testutil.MockStripRepository{
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
	service.SetFrontendHub(frontendHub)

	err := service.HandleLocalObservation(context.Background(), sessionID, euroscopeEvents.CdmLocalDataEvent{
		Callsign:       callsign,
		SourcePosition: sourcePosition,
		SourceRole:     "slave",
		Tobt:           "1207",
		Tsat:           "1213",
		Ttot:           "1218",
		Asrt:           "1201",
		Tsac:           "1202",
		ManualCtot:     "1225",
	})
	require.NoError(t, err)

	require.NotNil(t, persisted)
	require.Nil(t, persisted.Pending)
	require.Contains(t, persisted.LocalOverrides, "tobt")
	require.Contains(t, persisted.LocalOverrides, "tsat")
	require.Contains(t, persisted.LocalOverrides, "ttot")
	assert.Equal(t, "1207", persisted.LocalOverrides["tobt"].Value)
	assert.Equal(t, sourcePosition, persisted.LocalOverrides["tobt"].SourcePosition)
	assert.Equal(t, "slave", persisted.LocalOverrides["tobt"].SourceRole)
	require.NotNil(t, persisted.LocalOverrides["tobt"].ExpiresAt)
	assert.Equal(t, "1201", *persisted.Plugin.Asrt)
	assert.Equal(t, "1202", *persisted.Plugin.Tsac)
	assert.Equal(t, "1225", *persisted.Plugin.ManualCtot)

	require.Len(t, frontendHub.CdmUpdates, 1)
	assert.Equal(t, testutil.CdmUpdateCall{
		Session:  sessionID,
		Callsign: callsign,
		Eobt:     "1200",
		Tobt:     "1207",
		Tsat:     "1213",
		Ctot:     "1220",
	}, frontendHub.CdmUpdates[0])
}

func TestHandleLocalObservation_SuppressesDuplicateNoOp(t *testing.T) {
	const sessionID = int32(23)
	const callsign = "SAS111"

	now := time.Now().UTC()
	expiresAt := now.Add(10 * time.Minute)
	stripRepo := &testutil.MockStripRepository{
		GetCdmDataForCallsignFn: func(_ context.Context, session int32, cs string) (*models.CdmData, error) {
			assert.Equal(t, sessionID, session)
			assert.Equal(t, callsign, cs)
			return (&models.CdmData{
				Canonical: models.CdmCanonical{
					Eobt: stringPtr("1200"),
				},
				LocalOverrides: map[string]models.CdmFieldOverride{
					"tobt": {
						Value:          "1207",
						ObservedAt:     now,
						SourcePosition: "EKCH_B_GND",
						SourceRole:     "master",
						ExpiresAt:      &expiresAt,
					},
				},
			}).Normalize(), nil
		},
		SetCdmDataFn: func(_ context.Context, _ int32, _ string, _ *models.CdmData) (int64, error) {
			t.Fatalf("SetCdmData should not be called for duplicate local observation")
			return 0, nil
		},
	}

	frontendHub := &testutil.MockFrontendHub{}
	service := NewCdmService(newTestClientWithAirportMasters(nil), stripRepo, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	service.SetFrontendHub(frontendHub)

	err := service.HandleLocalObservation(context.Background(), sessionID, euroscopeEvents.CdmLocalDataEvent{
		Callsign:       callsign,
		SourcePosition: "EKCH_B_GND",
		SourceRole:     "master",
		Tobt:           "1207",
	})
	require.NoError(t, err)
	assert.Empty(t, frontendHub.CdmUpdates)
}
