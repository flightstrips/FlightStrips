package cdm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClientWithAirportMasters(masters []AirportMaster) *Client {
	client := NewClient(
		WithAPIKey("test-key"),
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
