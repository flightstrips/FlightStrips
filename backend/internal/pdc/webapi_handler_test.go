package pdc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"FlightStrips/internal/database"
	"FlightStrips/internal/pdc/mocks"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/shared"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type pdcAuthStub struct{}

func (pdcAuthStub) Validate(string) (shared.AuthenticatedUser, error) {
	return shared.NewAuthenticatedUser("1234567", 0, nil), nil
}

type handlerTestSetup struct {
	webapi        *WebAPI
	service       *Service
	queries       *database.Queries
	sessionID     int32
	mockStrip     *mocks.StripService
	mockFrontend  *mocks.FrontendHub
	mockEuroscope *mocks.EuroscopeHub
	mockHoppie    *mocks.HoppieClient
}

func setupHandlerTest(t *testing.T) *handlerTestSetup {
	t.Helper()

	dbPool, queries := testdata.SetupTestDB(t)

	sessionRepo := postgres.NewSessionRepository(dbPool)
	stripRepo := postgres.NewStripRepository(dbPool)
	sectorRepo := postgres.NewSectorOwnerRepository(dbPool)

	mockStrip := new(mocks.StripService)
	mockFrontend := new(mocks.FrontendHub)
	mockEuroscope := new(mocks.EuroscopeHub)
	mockHoppie := new(mocks.HoppieClient)

	adapter := &hoppieClientAdapter{HoppieClient: mockHoppie}
	service := &Service{
		client:        adapter,
		sessionRepo:   sessionRepo,
		stripRepo:     stripRepo,
		sectorRepo:    sectorRepo,
		frontendHub:   mockFrontend,
		stripService:  mockStrip,
		timeouts:      make(map[string]*timeoutTracker),
		timeoutConfig: 30 * time.Second,
	}

	sessionID := testdata.SeedTestSession(t, queries)
	webapi := NewWebAPI(pdcAuthStub{}, service, nil, false)

	t.Cleanup(func() {
		testdata.CleanupTestSession(t, queries, sessionID)
		mockStrip.AssertExpectations(t)
		mockFrontend.AssertExpectations(t)
		mockEuroscope.AssertExpectations(t)
		mockHoppie.AssertExpectations(t)
	})

	return &handlerTestSetup{
		webapi:        webapi,
		service:       service,
		queries:       queries,
		sessionID:     sessionID,
		mockStrip:     mockStrip,
		mockFrontend:  mockFrontend,
		mockEuroscope: mockEuroscope,
		mockHoppie:    mockHoppie,
	}
}

func unableRequest(callsign string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/pdc/unable",
		strings.NewReader(`{"callsign":"`+callsign+`"}`))
	req.Header.Set("Authorization", "Bearer token")
	return req
}

func acknowledgeRequest(callsign string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/pdc/acknowledge",
		strings.NewReader(`{"callsign":"`+callsign+`"}`))
	req.Header.Set("Authorization", "Bearer token")
	return req
}

// TestHandleUnableNoWebPDCRequest verifies that a strip with no web PDC request returns 404.
func TestHandleUnableNoWebPDCRequest(t *testing.T) {
	t.Parallel()

	s := setupHandlerTest(t)
	testdata.SeedTestStrip(t, s.queries, s.sessionID, "SAS123")

	rec := httptest.NewRecorder()
	s.webapi.handleUnable(rec, unableRequest("SAS123"))

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestHandleUnableStripNotCleared verifies that a strip in REQUESTED state returns 409.
func TestHandleUnableStripNotCleared(t *testing.T) {
	t.Parallel()

	s := setupHandlerTest(t)
	testdata.SeedTestStrip(t, s.queries, s.sessionID, "SAS123")

	s.mockFrontend.On("SendPdcStateChange", s.sessionID, "SAS123", string(StateRequested), "remarks").Return()

	err := s.service.SubmitWebPDCRequest(context.Background(), "SAS123", "B", "", "remarks", "A320")
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	s.webapi.handleUnable(rec, unableRequest("SAS123"))

	assert.Equal(t, http.StatusConflict, rec.Code)
}

// TestHandleUnableSuccessReturnsFailed verifies that a CLEARED strip transitions to FAILED on unable.
func TestHandleUnableSuccessReturnsFailed(t *testing.T) {
	t.Parallel()

	s := setupHandlerTest(t)
	testdata.SeedTestStrip(t, s.queries, s.sessionID, "SAS123")

	// Auto-issue clearance (no remarks, no Hoppie needed): strip goes to CLEARED
	s.mockStrip.On("MoveToBay", mock.Anything, s.sessionID, "SAS123", shared.BAY_CLEARED, true).Return(nil)
	s.mockFrontend.On("SendPdcStateChange", s.sessionID, "SAS123", string(StateCleared), "").Return()

	err := s.service.SubmitWebPDCRequest(context.Background(), "SAS123", "B", "", "", "A320")
	require.NoError(t, err)

	// HandleUnable expects the strip to be uncleaned and state set to FAILED
	// CID is "" because auto-issued clearances have no external CID
	s.mockStrip.On("UnclearStrip", mock.Anything, s.sessionID, "SAS123", "").Return(nil)
	s.mockFrontend.On("SendPdcStateChange", s.sessionID, "SAS123", string(StateFailed), "").Return()

	rec := httptest.NewRecorder()
	s.webapi.handleUnable(rec, unableRequest("SAS123"))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"state":"FAILED"`)
}

func TestHandleAcknowledgeSuccessConfirmsClearance(t *testing.T) {
	t.Parallel()

	s := setupHandlerTest(t)
	testdata.SeedTestStrip(t, s.queries, s.sessionID, "SAS123")

	s.mockStrip.On("MoveToBay", mock.Anything, s.sessionID, "SAS123", shared.BAY_CLEARED, true).Return(nil)
	s.mockFrontend.On("SendPdcStateChange", s.sessionID, "SAS123", string(StateCleared), "").Return()

	err := s.service.SubmitWebPDCRequest(context.Background(), "SAS123", "B", "", "", "A320")
	require.NoError(t, err)

	s.service.SetEuroscopeHub(s.mockEuroscope)
	mock.InOrder(
		s.mockStrip.On("ConfirmPdcClearance", mock.Anything, s.sessionID, "SAS123", shared.BAY_CLEARED, "").Return(nil),
		s.mockStrip.On("AutoAssumeForClearedStripByCid", mock.Anything, s.sessionID, "SAS123", "").Return(nil),
		s.mockEuroscope.On("SendPdcStateChange", s.sessionID, "SAS123", string(StateConfirmed), "").Return(),
		s.mockEuroscope.On("SendClearedFlag", s.sessionID, "", "SAS123", true).Return(),
		s.mockFrontend.On("SendPdcStateChange", s.sessionID, "SAS123", string(StateConfirmed), "").Return(),
	)

	rec := httptest.NewRecorder()
	s.webapi.handleAcknowledge(rec, acknowledgeRequest("SAS123"))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"state":"CONFIRMED"`)
}
