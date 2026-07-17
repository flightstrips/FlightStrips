package pdc

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/shared"
	pkgModels "FlightStrips/pkg/models"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// HoppieClientAdapter adapts the mock to the interface
type hoppieClientAdapter struct {
	HoppieClient *mockHoppieClient
}

func (a *hoppieClientAdapter) Poll(ctx context.Context, callsign string) ([]Message, error) {
	mockMessages, err := a.HoppieClient.Poll(ctx, callsign)
	if err != nil {
		return nil, err
	}
	// Convert mockHoppieMessage to pdc.Message
	messages := make([]Message, len(mockMessages))
	for i, m := range mockMessages {
		messages[i] = Message{
			From:   m.From,
			To:     m.To,
			Type:   m.Type,
			Packet: m.Packet,
			Raw:    m.Raw,
		}
	}
	return messages, nil
}

func (a *hoppieClientAdapter) SendCPDLC(ctx context.Context, from, to, packet string) error {
	return a.HoppieClient.SendCPDLC(ctx, from, to, packet)
}

func (a *hoppieClientAdapter) SendTelex(ctx context.Context, from, to, packet string) error {
	return a.HoppieClient.SendTelex(ctx, from, to, packet)
}

// Test suite setup helper
type PDCIntegrationTestSuite struct {
	service       *Service
	mockHoppie    *mockHoppieClient
	mockStrip     *mockPdcStripService
	mockFrontend  *mockPdcFrontendHub
	mockEuroscope *mockPdcEuroscopeHub
	queries       *database.Queries
}

func stringPtr(value string) *string {
	return &value
}

func readStripPdcState(t *testing.T, strip database.Strip) string {
	t.Helper()

	data := readStripPdcData(t, strip)
	return data.State
}

func readStripPdcData(t *testing.T, strip database.Strip) *models.PdcData {
	t.Helper()

	var data models.PdcData
	if len(strip.PdcData) > 0 {
		require.NoError(t, json.Unmarshal(strip.PdcData, &data))
	}

	return data.Normalize()
}

func (suite *PDCIntegrationTestSuite) SetupTest(t *testing.T) {
	// Create mocks
	suite.mockHoppie = new(mockHoppieClient)
	suite.mockStrip = new(mockPdcStripService)
	suite.mockFrontend = new(mockPdcFrontendHub)
	suite.mockEuroscope = new(mockPdcEuroscopeHub)

	// Setup database
	dbPool, queries := testdata.SetupTestDB(t)
	suite.queries = queries

	// Create repositories
	sessionRepo := postgres.NewSessionRepository(dbPool)
	stripRepo := postgres.NewStripRepository(dbPool)
	sectorRepo := postgres.NewSectorOwnerRepository(dbPool)
	controllerRepo := postgres.NewControllerRepository(dbPool)

	// Create service with mocks
	adapter := &hoppieClientAdapter{HoppieClient: suite.mockHoppie}
	suite.service = &Service{
		client:         adapter,
		sessionRepo:    sessionRepo,
		stripRepo:      stripRepo,
		sectorRepo:     sectorRepo,
		controllerRepo: controllerRepo,
		frontendHub:    suite.mockFrontend,
		euroscopeHub:   testPdcEuroscope{},
		stripService:   suite.mockStrip,
		timeouts:       make(map[string]*timeoutTracker),
		timeoutConfig:  30 * time.Second, // Long timeout to prevent firing during test
	}
	suite.mockStrip.On("ReevaluatePdcRequestValidations", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	suite.mockStrip.On("ClearMandatoryRouteCdm", mock.Anything, mock.Anything, mock.Anything).Maybe()

	// Seed test data
	sessionID := testdata.SeedTestSession(t, queries)
	testdata.SeedTestStrip(t, queries, sessionID, "SAS123")

	// Cleanup
	t.Cleanup(func() {
		testdata.CleanupTestSession(t, suite.queries, sessionID)
		suite.mockHoppie.AssertExpectations(t)
		suite.mockStrip.AssertExpectations(t)
		suite.mockFrontend.AssertExpectations(t)
		suite.mockEuroscope.AssertExpectations(t)
	})
}

// ===== INTEGRATION TESTS =====

func TestIssueClearanceFlow(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)

	callsign := "SAS123"
	cid := "CID123"
	sessionID := int32(1)
	remarks := "TEST CLEARANCE"
	ctx := context.Background()

	// Setup expectations
	suite.mockFrontend.On("GetAtisCodes", sessionID).Return("", "B").Once()
	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "CLRD TO") && strings.Contains(msg, "ESSA") &&
			strings.Contains(msg, "ATIS @B@") &&
			strings.Contains(msg, "NEXT FRQ: @118.105@") &&
			strings.Contains(msg, "Departure frequency: @124.980@") &&
			strings.Contains(msg, remarks)
	})).Return(nil)

	suite.mockStrip.On("MoveToBay", mock.Anything, sessionID, callsign, shared.BAY_CLEARED, true).Return(nil)

	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "CLEARED", "").Return()

	// Execute
	err := suite.service.IssueClearance(ctx, callsign, remarks, cid, sessionID)
	require.NoError(t, err)

	// Verify timeout was started with CID
	suite.service.timeoutsMutex.RLock()
	key := fmt.Sprintf("%s_%d", callsign, sessionID)
	tracker, exists := suite.service.timeouts[key]
	suite.service.timeoutsMutex.RUnlock()

	assert.True(t, exists, "Timeout should be started")
	assert.Equal(t, cid, tracker.cid, "CID should be stored in tracker")
	assert.Equal(t, callsign, tracker.callsign)
	assert.Equal(t, sessionID, tracker.sessionID)

	// Verify database state
	strip, err := suite.queries.GetStrip(context.Background(), database.GetStripParams{
		Session:  sessionID,
		Callsign: callsign,
	})
	require.NoError(t, err)
	assert.Equal(t, "CLEARED", readStripPdcState(t, strip))
}

func TestSubmitWebPDCRequest_AutoIssuesClearanceWithoutHoppie(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)
	suite.service.client = nil

	ctx := context.Background()
	sessionID := int32(1)
	callsign := "SAS123"

	suite.mockFrontend.On("GetAtisCodes", sessionID).Return("X", "Y").Once()
	suite.mockStrip.On("MoveToBay", mock.Anything, sessionID, callsign, shared.BAY_CLEARED, true).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "CLEARED", "").Return()

	err := suite.service.SubmitWebPDCRequest(ctx, callsign, "B", "A12", "", "A320")
	require.NoError(t, err)

	strip, err := suite.queries.GetStrip(ctx, database.GetStripParams{
		Session:  sessionID,
		Callsign: callsign,
	})
	require.NoError(t, err)

	pdcData := readStripPdcData(t, strip)
	require.NotNil(t, pdcData.RequestChannel)
	require.NotNil(t, pdcData.Web)
	require.NotNil(t, pdcData.Web.Atis)
	require.NotNil(t, pdcData.Web.ClearanceText)
	assert.Equal(t, models.PdcChannelWeb, *pdcData.RequestChannel)
	assert.Equal(t, "B", *pdcData.Web.Atis)
	assert.Nil(t, pdcData.Web.Stand)
	assert.Contains(t, *pdcData.Web.ClearanceText, "CLRD TO: ESSA")
	assert.Contains(t, *pdcData.Web.ClearanceText, "RWY: 22L")
	assert.Contains(t, *pdcData.Web.ClearanceText, "SID: VEMBO2E")
	assert.Contains(t, *pdcData.Web.ClearanceText, "SQK: 2401")
	assert.Contains(t, *pdcData.Web.ClearanceText, "ATIS B")
	assert.Contains(t, *pdcData.Web.ClearanceText, "NEXT FRQ: 118.105")
	assert.Contains(t, *pdcData.Web.ClearanceText, "Departure frequency 124.980")
	assert.NotContains(t, *pdcData.Web.ClearanceText, "/data2/")
	assert.NotContains(t, *pdcData.Web.ClearanceText, "@")
	assert.NotContains(t, *pdcData.Web.ClearanceText, ". . .")
	assert.Equal(t, string(StateCleared), pdcData.State)

	suite.service.timeoutsMutex.RLock()
	_, exists := suite.service.timeouts[fmt.Sprintf("%s_%d", callsign, sessionID)]
	suite.service.timeoutsMutex.RUnlock()
	assert.True(t, exists, "web-issued clearances must start a confirmation timeout")
	suite.mockStrip.AssertNotCalled(t, "UpdateStand", mock.Anything, sessionID, callsign, "A12")
}

func TestIssueClearance_UsesDepartureAtisInsteadOfArrivalAtis(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)

	callsign := "SAS123"
	sessionID := int32(1)
	ctx := context.Background()

	suite.mockFrontend.On("GetAtisCodes", sessionID).Return("A", "D").Once()
	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "ATIS @D@") && !strings.Contains(msg, "ATIS @A@")
	})).Return(nil)
	suite.mockStrip.On("MoveToBay", mock.Anything, sessionID, callsign, shared.BAY_CLEARED, true).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "CLEARED", "").Return()

	err := suite.service.IssueClearance(ctx, callsign, "", "CID123", sessionID)
	require.NoError(t, err)
}

func TestIssueClearance_OmitsAtisWhenNoAtisIsAvailable(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)

	callsign := "SAS123"
	sessionID := int32(1)
	ctx := context.Background()

	suite.mockFrontend.On("GetAtisCodes", sessionID).Return("", "").Once()
	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return !strings.Contains(msg, "ATIS @") &&
			!strings.Contains(msg, "ATIS A") &&
			strings.Contains(msg, "NEXT FRQ: @118.105@")
	})).Return(nil)
	suite.mockStrip.On("MoveToBay", mock.Anything, sessionID, callsign, shared.BAY_CLEARED, true).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "CLEARED", "").Return()

	err := suite.service.IssueClearance(ctx, callsign, "", "CID123", sessionID)
	require.NoError(t, err)
}

func TestSubmitWebPDCRequest_TimesOutWithoutSendingHoppieNoResponse(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)

	suite.service.timeoutConfig = 100 * time.Millisecond

	ctx := context.Background()
	sessionID := int32(1)
	callsign := "SAS123"

	suite.mockStrip.On("MoveToBay", mock.Anything, sessionID, callsign, shared.BAY_CLEARED, true).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "CLEARED", "").Return()
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "NO_RESPONSE", "").Return()
	suite.mockStrip.On("UnclearStrip", mock.Anything, sessionID, callsign, "").Return(nil)

	err := suite.service.SubmitWebPDCRequest(ctx, callsign, "B", "", "", "A320")
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	strip, err := suite.queries.GetStrip(ctx, database.GetStripParams{
		Session:  sessionID,
		Callsign: callsign,
	})
	require.NoError(t, err)
	assert.Equal(t, "NO_RESPONSE", readStripPdcState(t, strip))

	suite.mockHoppie.AssertNotCalled(t, "SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "ACK NOT RECEIVED") && strings.Contains(msg, "CLEARANCE CANCELLED")
	}))
}

func TestSubmitWebPDCRequest_WithRemarksLeavesRequestPending(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)

	ctx := context.Background()
	sessionID := int32(1)
	callsign := "SAS123"
	remarks := "REQUEST PUSH APPROVAL FIRST"

	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "REQUESTED", remarks).Return()

	err := suite.service.SubmitWebPDCRequest(ctx, callsign, "C", "", remarks, "A320")
	require.NoError(t, err)

	strip, err := suite.queries.GetStrip(ctx, database.GetStripParams{
		Session:  sessionID,
		Callsign: callsign,
	})
	require.NoError(t, err)

	pdcData := readStripPdcData(t, strip)
	require.NotNil(t, pdcData.RequestChannel)
	require.NotNil(t, pdcData.Web)
	require.NotNil(t, pdcData.Web.Atis)
	assert.Equal(t, models.PdcChannelWeb, *pdcData.RequestChannel)
	assert.Equal(t, "C", *pdcData.Web.Atis)
	assert.Nil(t, pdcData.Web.ClearanceText)
	assert.Equal(t, remarks, valueOrEmpty(pdcData.RequestRemarks))
	assert.Equal(t, string(StateRequested), pdcData.State)
}

func TestSubmitWebPDCRequest_NoSIDOrVectorsRoutesToFaults(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)

	ctx := context.Background()
	sessionID := int32(1)
	callsign := "DAT55"
	testdata.SeedTestStripWithoutRouting(t, suite.queries, sessionID, callsign)

	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "REQUESTED_WITH_FAULTS", "").Return()

	err := suite.service.SubmitWebPDCRequest(ctx, callsign, "B", "", "", "A320")
	require.NoError(t, err)

	strip, err := suite.queries.GetStrip(ctx, database.GetStripParams{
		Session:  sessionID,
		Callsign: callsign,
	})
	require.NoError(t, err)

	pdcData := readStripPdcData(t, strip)
	require.NotNil(t, pdcData.Web)
	assert.Equal(t, string(StateRequestedWithFaults), pdcData.State)
	assert.Nil(t, pdcData.Web.ClearanceText)

	suite.mockStrip.AssertNotCalled(t, "MoveToBay", mock.Anything, sessionID, callsign, shared.BAY_CLEARED, true)
	suite.service.timeoutsMutex.RLock()
	_, exists := suite.service.timeouts[fmt.Sprintf("%s_%d", callsign, sessionID)]
	suite.service.timeoutsMutex.RUnlock()
	assert.False(t, exists, "a faulted request must not start a confirmation timeout")
}

func TestPDCRequestOutcomes_MirrorCPDLCAndWeb(t *testing.T) {
	cases := []struct {
		name          string
		callsign      string
		remarks       string
		seedFaults    bool
		expectedState string
	}{
		{
			name:          "manual review remarks",
			callsign:      "SAS123",
			remarks:       "NO SID",
			expectedState: string(StateRequested),
		},
		{
			name:          "validation faults",
			callsign:      "DAT55",
			seedFaults:    true,
			expectedState: string(StateRequestedWithFaults),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runMirroredPDCRequestOutcome(t, models.PdcChannelCPDLC, tc.callsign, tc.remarks, tc.seedFaults, tc.expectedState)
			runMirroredPDCRequestOutcome(t, models.PdcChannelWeb, tc.callsign, tc.remarks, tc.seedFaults, tc.expectedState)
		})
	}
}

func runMirroredPDCRequestOutcome(t *testing.T, channel, callsign, remarks string, seedFaults bool, expectedState string) {
	t.Helper()

	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)

	ctx := context.Background()
	sessionID := int32(1)
	if seedFaults {
		testdata.SeedTestStripWithoutRouting(t, suite.queries, sessionID, callsign)
	}

	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, expectedState, remarks).Return()

	switch channel {
	case models.PdcChannelCPDLC:
		suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
			return strings.Contains(msg, "STANDBY")
		})).Return(nil).Once()

		payload := fmt.Sprintf("REQUEST PREDEP CLEARANCE %s A320 TO ESSA AT EKCH STAND A10 ATIS A", callsign)
		if remarks != "" {
			payload += "\n" + remarks
		}
		err := suite.service.ProcessPDCRequest(ctx, &IncomingMessage{
			Type:       MsgPDCRequest,
			From:       callsign,
			To:         "EKCH",
			Payload:    payload,
			RawMessage: callsign + " EKCH telex {" + payload + "}",
		}, sessionInformation{
			id:       sessionID,
			callsign: "EKCH",
		})
		require.NoError(t, err)
	case models.PdcChannelWeb:
		err := suite.service.SubmitWebPDCRequest(ctx, callsign, "B", "", remarks, "A320")
		require.NoError(t, err)
	default:
		t.Fatalf("unhandled PDC channel %q", channel)
	}

	strip, err := suite.queries.GetStrip(ctx, database.GetStripParams{
		Session:  sessionID,
		Callsign: callsign,
	})
	require.NoError(t, err)

	pdcData := readStripPdcData(t, strip)
	assert.Equal(t, expectedState, pdcData.State)
	assert.Equal(t, remarks, valueOrEmpty(pdcData.RequestRemarks))
}

func TestIssueClearance_ClearsManualReviewRequestRemarks(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)

	ctx := context.Background()
	sessionID := int32(1)
	callsign := "SAS123"
	remarks := "REQUEST PUSH APPROVAL FIRST"

	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "REQUESTED", remarks).Return()

	err := suite.service.SubmitWebPDCRequest(ctx, callsign, "C", "", remarks, "A320")
	require.NoError(t, err)

	suite.mockStrip.On("MoveToBay", mock.Anything, sessionID, callsign, shared.BAY_CLEARED, true).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "CLEARED", "").Return()

	err = suite.service.IssueClearance(ctx, callsign, "", "CID123", sessionID)
	require.NoError(t, err)

	strip, err := suite.queries.GetStrip(ctx, database.GetStripParams{
		Session:  sessionID,
		Callsign: callsign,
	})
	require.NoError(t, err)

	pdcData := readStripPdcData(t, strip)
	assert.Nil(t, pdcData.RequestRemarks)
	assert.Equal(t, string(StateCleared), pdcData.State)
	suite.mockStrip.AssertCalled(t, "ReevaluatePdcRequestValidations", mock.Anything, sessionID, callsign, true, false)
}

func TestSubmitWebPDCRequest_SearchesAllSessionsOutsideLiveMode(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)
	sessionRepo := postgres.NewSessionRepository(dbPool)
	stripRepo := postgres.NewStripRepository(dbPool)
	sectorRepo := postgres.NewSectorOwnerRepository(dbPool)

	service := &Service{
		client:        &hoppieClientAdapter{HoppieClient: new(mockHoppieClient)},
		sessionRepo:   sessionRepo,
		stripRepo:     stripRepo,
		sectorRepo:    sectorRepo,
		timeouts:      make(map[string]*timeoutTracker),
		timeoutConfig: 10 * time.Minute,
	}

	sessionID := testdata.SeedTestSessionNamedWithSectors(t, queries, "DEV", []database.InsertSectorOwnersParams{
		{
			Sector:     []string{"AA", "AD", "DEL", "GW", "SQ", "TE", "TW"},
			Position:   "118.105",
			Identifier: "EKCH_A_TWR",
		},
		{
			Sector:     []string{"DK"},
			Position:   "124.980",
			Identifier: "EKCH_K_DEP",
		},
	})
	testdata.SeedTestStrip(t, queries, sessionID, "SAS999")

	t.Cleanup(func() {
		testdata.CleanupTestSession(t, queries, sessionID)
	})

	match, err := service.FindWebStripByCallsign(context.Background(), "SAS999")
	require.NoError(t, err)
	assert.Equal(t, sessionID, match.SessionID)
	require.NotNil(t, match.Strip)
	assert.Equal(t, "SAS999", match.Strip.Callsign)
}

func TestIssueClearance_UsesAssignedSquawkInMessage(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)

	callsign := "SAS124"
	cid := "CID124"
	sessionID := int32(1)
	ctx := context.Background()

	testdata.SeedTestStripWithSquawks(t, suite.queries, sessionID, callsign, stringPtr("2000"), stringPtr("2401"))

	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "CLRD TO") && strings.Contains(msg, "SQK: @2401@") && !strings.Contains(msg, "SQK: @2000@")
	})).Return(nil)
	suite.mockStrip.On("MoveToBay", mock.Anything, sessionID, callsign, shared.BAY_CLEARED, true).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "CLEARED", "").Return()

	err := suite.service.IssueClearance(ctx, callsign, "", cid, sessionID)
	require.NoError(t, err)
}

func TestHandleWilcoFlow(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)

	callsign := "SAS123"
	sessionID := int32(1)
	cid := "CID123"
	ctx := context.Background()

	// First issue a clearance to set up state
	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.Anything).Return(nil)
	suite.mockStrip.On("MoveToBay", mock.Anything, sessionID, callsign, shared.BAY_CLEARED, true).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "CLEARED", "").Return()

	err := suite.service.IssueClearance(ctx, callsign, "", cid, sessionID)
	require.NoError(t, err)

	// Now handle WILCO
	suite.service.euroscopeHub = suite.mockEuroscope
	mock.InOrder(
		suite.mockStrip.On("ConfirmPdcClearance", mock.Anything, sessionID, callsign, shared.BAY_CLEARED, cid).Return(nil),
		suite.mockStrip.On("AutoAssumeForClearedStripByCid", mock.Anything, sessionID, callsign, cid).Return(nil),
		suite.mockFrontend.On("SendStripUpdate", sessionID, callsign).Return(),
		suite.mockEuroscope.On("SendPdcStateChange", sessionID, callsign, "CONFIRMED", "").Return(),
		suite.mockEuroscope.On("SendClearedFlag", sessionID, cid, callsign, true).Return(),
		suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "CONFIRMED", "").Return(),
	)

	incomingMsg := &IncomingMessage{
		Type:       MsgWilco,
		From:       callsign,
		To:         "EKCH",
		Payload:    "/data2/1/1/N/WILCO",
		RawMessage: callsign + " EKCH cpdlc {/data2/1/1/N/WILCO}",
	}

	session := sessionInformation{
		id:       sessionID,
		callsign: "EKCH",
	}

	err = suite.service.HandleWilco(ctx, incomingMsg, session)
	require.NoError(t, err)

	// Verify timeout was cancelled
	suite.service.timeoutsMutex.RLock()
	key := fmt.Sprintf("%s_%d", callsign, sessionID)
	_, exists := suite.service.timeouts[key]
	suite.service.timeoutsMutex.RUnlock()

	assert.False(t, exists, "Timeout should be cancelled")

	// Verify NO UnclearStrip was called (strip stays cleared)
	suite.mockStrip.AssertNotCalled(t, "UnclearStrip")

	// Verify database state
	strip, err := suite.queries.GetStrip(context.Background(), database.GetStripParams{
		Session:  sessionID,
		Callsign: callsign,
	})
	require.NoError(t, err)
	assert.Equal(t, "CONFIRMED", readStripPdcState(t, strip))
}

func TestHandleWilcoConfirmClearanceFailureLeavesPdcCleared(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)

	callsign := "SAS123"
	sessionID := int32(1)
	cid := "CID123"
	ctx := context.Background()

	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.Anything).Return(nil)
	suite.mockStrip.On("MoveToBay", mock.Anything, sessionID, callsign, shared.BAY_CLEARED, true).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "CLEARED", "").Return()

	err := suite.service.IssueClearance(ctx, callsign, "", cid, sessionID)
	require.NoError(t, err)

	suite.service.euroscopeHub = suite.mockEuroscope
	suite.mockStrip.On("ConfirmPdcClearance", mock.Anything, sessionID, callsign, shared.BAY_CLEARED, cid).Return(errors.New("boom"))

	incomingMsg := &IncomingMessage{
		Type:       MsgWilco,
		From:       callsign,
		To:         "EKCH",
		Payload:    "/data2/1/1/N/WILCO",
		RawMessage: callsign + " EKCH cpdlc {/data2/1/1/N/WILCO}",
	}

	session := sessionInformation{
		id:       sessionID,
		callsign: "EKCH",
	}

	err = suite.service.HandleWilco(ctx, incomingMsg, session)
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to confirm strip clearance")

	suite.service.timeoutsMutex.RLock()
	key := fmt.Sprintf("%s_%d", callsign, sessionID)
	_, exists := suite.service.timeouts[key]
	suite.service.timeoutsMutex.RUnlock()
	assert.True(t, exists, "Timeout should remain active when confirmation fails")

	strip, err := suite.queries.GetStrip(context.Background(), database.GetStripParams{
		Session:  sessionID,
		Callsign: callsign,
	})
	require.NoError(t, err)
	assert.Equal(t, "CLEARED", readStripPdcState(t, strip))

	suite.mockEuroscope.AssertNotCalled(t, "SendPdcStateChange", sessionID, callsign, "CONFIRMED", "")
	suite.mockEuroscope.AssertNotCalled(t, "SendClearedFlag", sessionID, cid, callsign, true)
	suite.mockFrontend.AssertNotCalled(t, "SendPdcStateChange", sessionID, callsign, "CONFIRMED", "")
	suite.mockStrip.AssertNotCalled(t, "AutoAssumeForClearedStripByCid", mock.Anything, sessionID, callsign, cid)
}

func TestHandleUnableFlow(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)

	callsign := "SAS123"
	sessionID := int32(1)
	cid := "CID123"
	ctx := context.Background()

	// First issue a clearance
	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.Anything).Return(nil)
	suite.mockStrip.On("MoveToBay", mock.Anything, sessionID, callsign, shared.BAY_CLEARED, true).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "CLEARED", "").Return()

	err := suite.service.IssueClearance(ctx, callsign, "", cid, sessionID)
	require.NoError(t, err)

	// Now handle UNABLE - expect UnclearStrip with the CID
	suite.mockStrip.On("UnclearStrip", mock.Anything, sessionID, callsign, cid).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "FAILED", "").Return()

	session := sessionInformation{
		id:       sessionID,
		callsign: "EKCH",
	}

	err = suite.service.HandleUnable(ctx, callsign, session)
	require.NoError(t, err)

	// Verify CID was retrieved from tracker and used
	suite.mockStrip.AssertCalled(t, "UnclearStrip", mock.Anything, sessionID, callsign, cid)

	// Verify timeout was cancelled
	suite.service.timeoutsMutex.RLock()
	key := fmt.Sprintf("%s_%d", callsign, sessionID)
	_, exists := suite.service.timeouts[key]
	suite.service.timeoutsMutex.RUnlock()

	assert.False(t, exists, "Timeout should be cancelled")

	// Verify database state
	strip, err := suite.queries.GetStrip(context.Background(), database.GetStripParams{
		Session:  sessionID,
		Callsign: callsign,
	})
	require.NoError(t, err)
	assert.Equal(t, "FAILED", readStripPdcState(t, strip))
}

func TestTimeoutExpiryFlow(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)

	// Override timeout configuration for this test
	suite.service.timeoutConfig = 100 * time.Millisecond

	callsign := "SAS123"
	sessionID := int32(1)
	cid := "CID123"
	ctx := context.Background()

	// Issue clearance with short timeout
	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.Anything).Return(nil).Times(2) // Initial + timeout message
	suite.mockStrip.On("MoveToBay", mock.Anything, sessionID, callsign, shared.BAY_CLEARED, true).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "CLEARED", "").Return()

	err := suite.service.IssueClearance(ctx, callsign, "", cid, sessionID)
	require.NoError(t, err)

	// Wait for timeout - expect UnclearStrip and no-response message
	suite.mockStrip.On("UnclearStrip", mock.Anything, sessionID, callsign, cid).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "NO_RESPONSE", "").Return()

	// Wait for timeout to fire (100ms + buffer)
	time.Sleep(200 * time.Millisecond)

	// Verify UnclearStrip was called with correct CID
	suite.mockStrip.AssertCalled(t, "UnclearStrip", mock.Anything, sessionID, callsign, cid)

	// Verify no-response message was sent
	suite.mockHoppie.AssertCalled(t, "SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "ACK NOT RECEIVED") && strings.Contains(msg, "CLEARANCE CANCELLED")
	}))

	// Verify database state
	strip, err := suite.queries.GetStrip(context.Background(), database.GetStripParams{
		Session:  sessionID,
		Callsign: callsign,
	})
	require.NoError(t, err)
	assert.Equal(t, "NO_RESPONSE", readStripPdcState(t, strip))
}

func TestRevertToVoiceFlow(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)

	callsign := "SAS123"
	sessionID := int32(1)
	cid := "CID123"
	newCid := "CID456" // Different controller reverts
	ctx := context.Background()

	// Issue clearance
	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.Anything).Return(nil).Times(2)
	suite.mockStrip.On("MoveToBay", mock.Anything, sessionID, callsign, shared.BAY_CLEARED, true).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "CLEARED", "").Return()

	err := suite.service.IssueClearance(ctx, callsign, "", cid, sessionID)
	require.NoError(t, err)

	// Revert to voice - use newCid
	suite.mockStrip.On("UnclearStrip", mock.Anything, sessionID, callsign, newCid).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "REVERT_TO_VOICE", "").Return()

	err = suite.service.RevertToVoice(ctx, callsign, sessionID, newCid)
	require.NoError(t, err)

	// Verify correct CID was used (newCid, not cid from IssueClearance)
	suite.mockStrip.AssertCalled(t, "UnclearStrip", mock.Anything, sessionID, callsign, newCid)

	// Verify revert message sent to pilot
	suite.mockHoppie.AssertCalled(t, "SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "REVERT TO VOICE")
	}))

	// Verify database state
	strip, err := suite.queries.GetStrip(context.Background(), database.GetStripParams{
		Session:  sessionID,
		Callsign: callsign,
	})
	require.NoError(t, err)
	assert.Equal(t, "REVERT_TO_VOICE", readStripPdcState(t, strip))
}

func TestRestorePendingTimeouts_ExpiresClearedPdcAfterRestart(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)

	const callsign = "SAS321"
	const sessionID = int32(1)

	testdata.SeedClearedTestStrip(t, suite.queries, sessionID, callsign)

	requestChannel := models.PdcChannelWeb
	messageSequence := int32(42)
	messageSent := time.Now().UTC().Add(-2 * time.Second)
	issuedByCid := "CID123"

	require.NoError(t, suite.service.stripRepo.SetPdcData(context.Background(), sessionID, callsign, &models.PdcData{
		State:           string(StateCleared),
		RequestChannel:  &requestChannel,
		MessageSequence: &messageSequence,
		MessageSent:     &messageSent,
		IssuedByCid:     &issuedByCid,
		Web:             &models.PdcWebData{},
	}))

	suite.mockStrip.On("UnclearStrip", mock.Anything, sessionID, callsign, issuedByCid).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "NO_RESPONSE", "").Return()

	restartedService := &Service{
		client:        &hoppieClientAdapter{HoppieClient: suite.mockHoppie},
		sessionRepo:   suite.service.sessionRepo,
		stripRepo:     suite.service.stripRepo,
		frontendHub:   suite.mockFrontend,
		euroscopeHub:  testPdcEuroscope{},
		stripService:  suite.mockStrip,
		timeouts:      make(map[string]*timeoutTracker),
		timeoutConfig: 100 * time.Millisecond,
	}
	suite.mockStrip.On("ReevaluatePdcRequestValidations", mock.Anything, sessionID, callsign, true, false).Return(nil).Maybe()

	restored, err := restartedService.restorePendingTimeouts(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, restored)

	require.Eventually(t, func() bool {
		strip, err := suite.queries.GetStrip(context.Background(), database.GetStripParams{
			Session:  sessionID,
			Callsign: callsign,
		})
		if err != nil {
			return false
		}
		return readStripPdcState(t, strip) == "NO_RESPONSE"
	}, time.Second, 25*time.Millisecond)
}

func TestConfirmVoiceClearance_ClearsPdcStateAndPublishesPdcMessage(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)

	callsign := "SAS123"
	sessionID := int32(1)
	ctx := context.Background()

	requestedAt := time.Now().UTC()
	requestChannel := models.PdcChannelWeb
	webAtis := "B"
	webClearance := "CLRD TO: ESSA"

	require.NoError(t, suite.service.stripRepo.SetPdcData(ctx, sessionID, callsign, &models.PdcData{
		State:          string(StateRequested),
		RequestChannel: &requestChannel,
		RequestedAt:    &requestedAt,
		Web: &models.PdcWebData{
			Atis:          &webAtis,
			ClearanceText: &webClearance,
		},
	}))

	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "NONE", "").Return()
	suite.mockFrontend.On("SendMessage", sessionID, "SYSTEM", "SAS123: CLEARANCE given / confirmed over voice.", []string{"CLR-DEL"}).Return()

	err := suite.service.ConfirmVoiceClearance(ctx, callsign, sessionID)
	require.NoError(t, err)

	strip, err := suite.queries.GetStrip(context.Background(), database.GetStripParams{
		Session:  sessionID,
		Callsign: callsign,
	})
	require.NoError(t, err)

	pdcData := readStripPdcData(t, strip)
	assert.Equal(t, models.PdcStateNone, pdcData.State)
	assert.Nil(t, pdcData.RequestChannel)
	assert.Nil(t, pdcData.RequestedAt)
	assert.Nil(t, pdcData.Web)
}

// ===== ERROR SCENARIO TESTS =====

func TestProcessPDCRequest_FlightPlanNotHeld(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)
	ctx := context.Background()

	callsign := "NONEXISTENT"

	// Expect error message to pilot
	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "NOT HELD") || strings.Contains(msg, "RCD REJECTED")
	})).Return(nil)

	incomingMsg := &IncomingMessage{
		Type:       MsgPDCRequest,
		From:       callsign,
		To:         "EKCH",
		Payload:    "REQUEST PREDEP CLEARANCE " + callsign + " A320 TO ESSA AT EKCH STAND A10 ATIS A",
		RawMessage: callsign + " EKCH telex {REQUEST PREDEP CLEARANCE " + callsign + " A320 TO ESSA AT EKCH STAND A10 ATIS A}",
	}

	session := sessionInformation{
		id:       1,
		callsign: "EKCH",
	}

	err := suite.service.ProcessPDCRequest(ctx, incomingMsg, session)

	// Should return error but still send message to pilot
	assert.Error(t, err)
	suite.mockHoppie.AssertCalled(t, "SendCPDLC", mock.Anything, mock.Anything, callsign, mock.Anything)
}

func TestProcessPDCRequest_InvalidAircraftType(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)
	ctx := context.Background()

	callsign := "SAS123"

	// Expect invalid aircraft error message
	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "TYPE MISMATCH") || strings.Contains(msg, "INVALID AIRCRAFT")
	})).Return(nil)

	// Request with B738 but strip has A320
	incomingMsg := &IncomingMessage{
		Type:       MsgPDCRequest,
		From:       callsign,
		To:         "EKCH",
		Payload:    "REQUEST PREDEP CLEARANCE " + callsign + " B738 TO ESSA AT EKCH STAND A10 ATIS A",
		RawMessage: callsign + " EKCH telex {REQUEST PREDEP CLEARANCE " + callsign + " B738 TO ESSA AT EKCH STAND A10 ATIS A}",
	}

	session := sessionInformation{
		id:       1,
		callsign: "EKCH",
	}

	err := suite.service.ProcessPDCRequest(ctx, incomingMsg, session)
	assert.Error(t, err)
	suite.mockHoppie.AssertCalled(t, "SendCPDLC", mock.Anything, mock.Anything, callsign, mock.Anything)
}

func TestProcessPDCRequest_AircraftTypeWithEquipmentSuffix(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)
	ctx := context.Background()

	callsign := "DLH6LK"

	// Seed strip with full ICAO aircraft type including equipment suffix
	testdata.SeedTestStripWithAircraftType(t, suite.queries, 1, callsign, "A321/M-SDE3FGHIRWY/LB1")

	// Auto-approved requests should send the clearance directly without a processing ACK.
	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "CLRD TO")
	})).Return(nil).Once()
	suite.mockStrip.On("MoveToBay", mock.Anything, int32(1), callsign, shared.BAY_CLEARED, true).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", int32(1), callsign, "CLEARED", "").Return()

	incomingMsg := &IncomingMessage{
		Type:       MsgPDCRequest,
		From:       callsign,
		To:         "EKCH",
		Payload:    fmt.Sprintf("REQUEST PREDEP CLEARANCE %s A321 TO ESSA AT EKCH STAND A10 ATIS A", callsign),
		RawMessage: fmt.Sprintf("%s EKCH telex {REQUEST PREDEP CLEARANCE %s A321 TO ESSA AT EKCH STAND A10 ATIS A}", callsign, callsign),
	}

	session := sessionInformation{
		id:       1,
		callsign: "EKCH",
	}

	err := suite.service.ProcessPDCRequest(ctx, incomingMsg, session)
	require.NoError(t, err)

	strip, err := suite.queries.GetStrip(ctx, database.GetStripParams{
		Session:  1,
		Callsign: callsign,
	})
	require.NoError(t, err)
	assert.Equal(t, "CLEARED", readStripPdcState(t, strip))
	suite.mockHoppie.AssertNotCalled(t, "SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "STANDBY")
	}))
}

func TestProcessPDCRequest_Success(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)
	ctx := context.Background()

	callsign := "SAS123"

	// Since strip has clearance data set, IssueClearance will auto-issue directly.
	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "CLRD TO")
	})).Return(nil).Once()

	// IssueClearance should only move the strip into the cleared bay
	suite.mockStrip.On("MoveToBay", mock.Anything, int32(1), callsign, shared.BAY_CLEARED, true).Return(nil)

	// State goes to CLEARED after auto-issue
	suite.mockFrontend.On("SendPdcStateChange", int32(1), callsign, "CLEARED", "").Return()

	incomingMsg := &IncomingMessage{
		Type:       MsgPDCRequest,
		From:       callsign,
		To:         "EKCH",
		Payload:    testdata.ValidPDCRequest(),
		RawMessage: callsign + " EKCH telex {" + testdata.ValidPDCRequest() + "}",
	}

	session := sessionInformation{
		id:       1,
		callsign: "EKCH",
	}

	err := suite.service.ProcessPDCRequest(ctx, incomingMsg, session)
	require.NoError(t, err)

	// Verify database state — strip should be CLEARED after auto-issue
	strip, err := suite.queries.GetStrip(context.Background(), database.GetStripParams{
		Session:  1,
		Callsign: callsign,
	})
	require.NoError(t, err)
	assert.Equal(t, "CLEARED", readStripPdcState(t, strip))
	suite.mockHoppie.AssertNotCalled(t, "SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "STANDBY")
	}))
}

func TestProcessPDCRequest_EobtOutsideFormerWindowStillAutoIssues(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)
	ctx := context.Background()

	callsign := "SAS130"
	testdata.SeedTestStrip(t, suite.queries, 1, callsign)
	_, err := suite.queries.SetCdmData(ctx, database.SetCdmDataParams{
		Session:  1,
		Callsign: callsign,
		CdmData:  []byte(`{"eobt":"2359"}`),
	})
	require.NoError(t, err)

	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "CLRD TO")
	})).Return(nil).Once()
	suite.mockStrip.On("MoveToBay", mock.Anything, int32(1), callsign, shared.BAY_CLEARED, true).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", int32(1), callsign, "CLEARED", "").Return()

	incomingMsg := &IncomingMessage{
		Type:       MsgPDCRequest,
		From:       callsign,
		To:         "EKCH",
		Payload:    "REQUEST PREDEP CLEARANCE " + callsign + " A320 TO ESSA AT EKCH STAND A10 ATIS A",
		RawMessage: callsign + " EKCH telex {REQUEST PREDEP CLEARANCE " + callsign + " A320 TO ESSA AT EKCH STAND A10 ATIS A}",
	}

	session := sessionInformation{
		id:       1,
		callsign: "EKCH",
	}

	err = suite.service.ProcessPDCRequest(ctx, incomingMsg, session)
	require.NoError(t, err)

	strip, err := suite.queries.GetStrip(ctx, database.GetStripParams{
		Session:  1,
		Callsign: callsign,
	})
	require.NoError(t, err)
	assert.Equal(t, "CLEARED", readStripPdcState(t, strip))
	suite.mockStrip.AssertNotCalled(t, "ReevaluatePdcRequestValidations", mock.Anything, int32(1), callsign, true, true)
}

func TestProcessPDCRequest_WithRemarksDoesNotAutoIssue(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)
	ctx := context.Background()

	callsign := "SAS128"
	testdata.SeedTestStrip(t, suite.queries, 1, callsign)

	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "STANDBY")
	})).Return(nil).Once()
	suite.mockFrontend.On("SendPdcStateChange", int32(1), callsign, "REQUESTED", "NO SID").Return()

	incomingMsg := &IncomingMessage{
		Type:       MsgPDCRequest,
		From:       callsign,
		To:         "EKCH",
		Payload:    "REQUEST PREDEP CLEARANCE " + callsign + " A320 TO ESSA AT EKCH STAND A10 ATIS A\nNO SID",
		RawMessage: callsign + " EKCH telex {REQUEST PREDEP CLEARANCE " + callsign + " A320 TO ESSA AT EKCH STAND A10 ATIS A\nNO SID}",
	}

	session := sessionInformation{
		id:       1,
		callsign: "EKCH",
	}

	err := suite.service.ProcessPDCRequest(ctx, incomingMsg, session)
	require.NoError(t, err)

	suite.mockStrip.AssertNotCalled(t, "MoveToBay", mock.Anything, int32(1), callsign, shared.BAY_CLEARED, true)

	strip, err := suite.queries.GetStrip(context.Background(), database.GetStripParams{
		Session:  1,
		Callsign: callsign,
	})
	require.NoError(t, err)
	stripPdc := readStripPdcData(t, strip)
	assert.Equal(t, "REQUESTED", stripPdc.State)
	require.NotNil(t, stripPdc.RequestRemarks)
	assert.Equal(t, "NO SID", *stripPdc.RequestRemarks)
}

func TestProcessPDCRequest_ARINCWithRemarksDoesNotAutoIssue(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)
	ctx := context.Background()

	callsign := "SAS129"
	testdata.SeedTestStrip(t, suite.queries, 1, callsign)

	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "STANDBY")
	})).Return(nil).Once()
	suite.mockFrontend.On("SendPdcStateChange", int32(1), callsign, "REQUESTED", "NO SID").Return()

	incomingMsg := &IncomingMessage{
		Type:       MsgPDCRequest,
		From:       callsign,
		To:         "EKCH",
		Payload:    "RCD\n" + callsign + "-EKCH-GATE A10-ESSA\nATIS A\n-TYP/A320\n-RMK/NO SID",
		RawMessage: callsign + " EKCH cpdlc {RCD\n" + callsign + "-EKCH-GATE A10-ESSA\nATIS A\n-TYP/A320\n-RMK/NO SID}",
	}

	session := sessionInformation{
		id:       1,
		callsign: "EKCH",
	}

	err := suite.service.ProcessPDCRequest(ctx, incomingMsg, session)
	require.NoError(t, err)

	suite.mockStrip.AssertNotCalled(t, "MoveToBay", mock.Anything, int32(1), callsign, shared.BAY_CLEARED, true)

	strip, err := suite.queries.GetStrip(context.Background(), database.GetStripParams{
		Session:  1,
		Callsign: callsign,
	})
	require.NoError(t, err)
	stripPdc := readStripPdcData(t, strip)
	assert.Equal(t, "REQUESTED", stripPdc.State)
	require.NotNil(t, stripPdc.RequestRemarks)
	assert.Equal(t, "NO SID", *stripPdc.RequestRemarks)
}

func TestProcessPDCRequest_InvalidAssignedSquawkDoesNotAutoIssue(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)
	ctx := context.Background()

	callsign := "SAS125"
	testdata.SeedTestStripWithSquawks(t, suite.queries, 1, callsign, stringPtr("2000"), stringPtr(""))

	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "STANDBY")
	})).Return(nil).Once()
	suite.mockFrontend.On("SendPdcStateChange", int32(1), callsign, "REQUESTED", "").Return()

	incomingMsg := &IncomingMessage{
		Type:       MsgPDCRequest,
		From:       callsign,
		To:         "EKCH",
		Payload:    "REQUEST PREDEP CLEARANCE " + callsign + " A320 TO ESSA AT EKCH STAND A10 ATIS A",
		RawMessage: callsign + " EKCH telex {REQUEST PREDEP CLEARANCE " + callsign + " A320 TO ESSA AT EKCH STAND A10 ATIS A}",
	}

	session := sessionInformation{
		id:       1,
		callsign: "EKCH",
	}

	err := suite.service.ProcessPDCRequest(ctx, incomingMsg, session)
	require.NoError(t, err)

	strip, err := suite.queries.GetStrip(context.Background(), database.GetStripParams{
		Session:  1,
		Callsign: callsign,
	})
	require.NoError(t, err)
	assert.Equal(t, "REQUESTED", readStripPdcState(t, strip))
}

func TestProcessPDCRequest_InactiveDepartureRunwayCreatesFault(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)
	ctx := context.Background()

	callsign := "SAS127"
	testdata.SeedTestStrip(t, suite.queries, 1, callsign)
	require.NoError(t, suite.queries.UpdateActiveRunways(ctx, database.UpdateActiveRunwaysParams{
		ID: 1,
		ActiveRunways: pkgModels.ActiveRunways{
			DepartureRunways: []string{"22R"},
		},
	}))

	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "STANDBY")
	})).Return(nil).Once()
	suite.mockFrontend.On("SendPdcStateChange", int32(1), callsign, "REQUESTED_WITH_FAULTS", "").Return()

	incomingMsg := &IncomingMessage{
		Type:       MsgPDCRequest,
		From:       callsign,
		To:         "EKCH",
		Payload:    "REQUEST PREDEP CLEARANCE " + callsign + " A320 TO ESSA AT EKCH STAND A10 ATIS A",
		RawMessage: callsign + " EKCH telex {REQUEST PREDEP CLEARANCE " + callsign + " A320 TO ESSA AT EKCH STAND A10 ATIS A}",
	}

	session := sessionInformation{
		id:       1,
		callsign: "EKCH",
	}

	err := suite.service.ProcessPDCRequest(ctx, incomingMsg, session)
	require.NoError(t, err)

	strip, err := suite.queries.GetStrip(context.Background(), database.GetStripParams{
		Session:  1,
		Callsign: callsign,
	})
	require.NoError(t, err)
	assert.Equal(t, "REQUESTED_WITH_FAULTS", readStripPdcState(t, strip))
	suite.mockStrip.AssertCalled(t, "ReevaluatePdcRequestValidations", mock.Anything, int32(1), callsign, true, true)
}

func TestProcessPDCRequest_MandatoryRouteRequiresManualApprovalWithoutChangingStrip(t *testing.T) {

	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)
	ctx := context.Background()

	callsign := "SAS128"
	testdata.SeedTestStrip(t, suite.queries, 1, callsign)

	strip, err := suite.service.stripRepo.GetByCallsign(ctx, 1, callsign)
	require.NoError(t, err)
	runway := "22R"
	sid := "GOLGA2C"
	route := "GOLGA DCT"
	strip.Runway = &runway
	strip.Sid = &sid
	strip.Route = &route
	strip.CdmData = (&models.CdmData{
		EcfmpRestrictions: []models.EcfmpRestriction{
			{Type: "mandatory_route", Routes: []string{"VEDAR DCT"}},
		},
	}).Normalize()
	_, err = suite.service.stripRepo.Update(ctx, strip)
	require.NoError(t, err)

	require.NoError(t, suite.queries.UpdateSessionSids(ctx, database.UpdateSessionSidsParams{
		ID: 1,
		AvailableSids: pkgModels.AvailableSids{
			{Name: "VEDAR2C", Runway: "22R"},
		},
	}))

	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "STANDBY")
	})).Return(nil).Once()
	suite.mockFrontend.On("SendPdcStateChange", int32(1), callsign, "REQUESTED_WITH_FAULTS", "").Return()

	incomingMsg := &IncomingMessage{
		Type:       MsgPDCRequest,
		From:       callsign,
		To:         "EKCH",
		Payload:    "REQUEST PREDEP CLEARANCE " + callsign + " A320 TO ESSA AT EKCH STAND A10 ATIS A",
		RawMessage: callsign + " EKCH telex {REQUEST PREDEP CLEARANCE " + callsign + " A320 TO ESSA AT EKCH STAND A10 ATIS A}",
	}

	session := sessionInformation{id: 1, callsign: "EKCH"}
	err = suite.service.ProcessPDCRequest(ctx, incomingMsg, session)
	require.NoError(t, err)

	updatedStrip, err := suite.service.stripRepo.GetByCallsign(ctx, 1, callsign)
	require.NoError(t, err)
	require.NotNil(t, updatedStrip.Route)
	require.NotNil(t, updatedStrip.Sid)
	assert.Equal(t, "GOLGA DCT", *updatedStrip.Route)
	assert.Equal(t, "GOLGA2C", *updatedStrip.Sid)

	dbStrip, err := suite.queries.GetStrip(ctx, database.GetStripParams{Session: 1, Callsign: callsign})
	require.NoError(t, err)
	assert.Equal(t, "REQUESTED_WITH_FAULTS", readStripPdcState(t, dbStrip))
}

func TestProcessPDCRequest_AlreadyCleared(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)
	ctx := context.Background()

	callsign := "SAS126"
	sessionID := int32(1)

	// Seed a strip that is already cleared
	testdata.SeedClearedTestStrip(t, suite.queries, sessionID, callsign)

	// Expect the "already cleared" rejection to be sent back to the pilot
	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "CLEARANCE ALREADY ISSUED")
	})).Return(nil)

	incomingMsg := &IncomingMessage{
		Type:       MsgPDCRequest,
		From:       callsign,
		To:         "EKCH",
		Payload:    "REQUEST PREDEP CLEARANCE " + callsign + " A320 TO ESSA AT EKCH STAND A10 ATIS A",
		RawMessage: callsign + " EKCH telex {REQUEST PREDEP CLEARANCE " + callsign + " A320 TO ESSA AT EKCH STAND A10 ATIS A}",
	}

	session := sessionInformation{
		id:       sessionID,
		callsign: "EKCH",
	}

	err := suite.service.ProcessPDCRequest(ctx, incomingMsg, session)
	assert.Error(t, err, "should return error for already-cleared aircraft")
	suite.mockHoppie.AssertCalled(t, "SendCPDLC", mock.Anything, mock.Anything, callsign, mock.Anything)
}

func TestIssueClearance_MandatoryRouteIncludesFullRouteAndCorrectedSid(t *testing.T) {

	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)

	ctx := context.Background()
	callsign := "SAS129"
	sessionID := int32(1)
	cid := "CID123"

	testdata.SeedTestStrip(t, suite.queries, sessionID, callsign)
	strip, err := suite.service.stripRepo.GetByCallsign(ctx, sessionID, callsign)
	require.NoError(t, err)
	runway := "22R"
	sid := "GOLGA2C"
	route := "GOLGA DCT"
	strip.Runway = &runway
	strip.Sid = &sid
	strip.Route = &route
	strip.CdmData = (&models.CdmData{
		EcfmpRestrictions: []models.EcfmpRestriction{
			{Type: "mandatory_route", Routes: []string{"VEDAR DCT"}},
		},
	}).Normalize()
	_, err = suite.service.stripRepo.Update(ctx, strip)
	require.NoError(t, err)

	require.NoError(t, suite.queries.UpdateSessionSids(ctx, database.UpdateSessionSidsParams{
		ID: sessionID,
		AvailableSids: pkgModels.AvailableSids{
			{Name: "VEDAR2C", Runway: "22R"},
		},
	}))

	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "SID: @VEDAR2C@") && strings.Contains(msg, "MANDATORY ROUTE: @VEDAR DCT@")
	})).Return(nil).Once()
	suite.mockStrip.On("MoveToBay", mock.Anything, sessionID, callsign, shared.BAY_CLEARED, true).Return(nil)
	suite.mockFrontend.On("SendStripUpdate", sessionID, callsign).Return().Once()
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "CLEARED", "").Return()

	err = suite.service.IssueClearance(ctx, callsign, "", cid, sessionID)
	require.NoError(t, err)

	updatedStrip, err := suite.service.stripRepo.GetByCallsign(ctx, sessionID, callsign)
	require.NoError(t, err)
	require.NotNil(t, updatedStrip.Route)
	require.NotNil(t, updatedStrip.Sid)
	assert.Equal(t, "VEDAR DCT", *updatedStrip.Route)
	assert.Equal(t, "VEDAR2C", *updatedStrip.Sid)
}
