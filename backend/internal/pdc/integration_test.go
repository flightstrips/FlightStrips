package pdc

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/pdc/mocks"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"context"
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
	*mocks.HoppieClient
}

func (a *hoppieClientAdapter) Poll(ctx context.Context, callsign string) ([]Message, error) {
	mockMessages, err := a.HoppieClient.Poll(ctx, callsign)
	if err != nil {
		return nil, err
	}
	// Convert mocks.Message to pdc.Message
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

// Test suite setup helper
type PDCIntegrationTestSuite struct {
	service       *Service
	mockHoppie    *mocks.HoppieClient
	mockStrip     *mocks.StripService
	mockFrontend  *mocks.FrontendHub
	mockEuroscope *mocks.EuroscopeHub
	queries       *database.Queries
}

func (suite *PDCIntegrationTestSuite) SetupTest(t *testing.T) {
	// Create mocks
	suite.mockHoppie = new(mocks.HoppieClient)
	suite.mockStrip = new(mocks.StripService)
	suite.mockFrontend = new(mocks.FrontendHub)
	suite.mockEuroscope = new(mocks.EuroscopeHub)

	// Setup database
	dbPool, queries := testdata.SetupTestDB(t)
	suite.queries = queries

	// Create repositories
	sessionRepo := postgres.NewSessionRepository(dbPool)
	stripRepo := postgres.NewStripRepository(dbPool)
	sectorRepo := postgres.NewSectorOwnerRepository(dbPool)

	// Create service with mocks
	adapter := &hoppieClientAdapter{HoppieClient: suite.mockHoppie}
	suite.service = &Service{
		client:        adapter,
		sessionRepo:   sessionRepo,
		stripRepo:     stripRepo,
		sectorRepo:    sectorRepo,
		frontendHub:   suite.mockFrontend,
		stripService:  suite.mockStrip,
		timeouts:      make(map[string]*timeoutTracker),
		timeoutConfig: 30 * time.Second, // Long timeout to prevent firing during test
	}

	// Seed test data
	sessionID := testdata.SeedTestSession(t, queries)
	testdata.SeedTestStrip(t, queries, sessionID, "SAS123")

	// Cleanup
	t.Cleanup(func() {
		testdata.CleanupTestSession(t, suite.queries, sessionID)
		suite.mockHoppie.AssertExpectations(t)
		suite.mockStrip.AssertExpectations(t)
		suite.mockFrontend.AssertExpectations(t)
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
	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "CLRD TO") && strings.Contains(msg, "ESSA") && strings.Contains(msg, "118.105") && strings.Contains(msg, remarks)
	})).Return(nil)

	suite.mockStrip.On("ClearStrip", mock.Anything, sessionID, callsign, cid).Return(nil)

	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "CLEARED").Return()

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
	assert.Equal(t, "CLEARED", strip.PdcState)
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
	suite.mockStrip.On("ClearStrip", mock.Anything, sessionID, callsign, cid).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "CLEARED").Return()

	err := suite.service.IssueClearance(ctx, callsign, "", cid, sessionID)
	require.NoError(t, err)

	// Now handle WILCO
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "CONFIRMED").Return()

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
	assert.Equal(t, "CONFIRMED", strip.PdcState)
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
	suite.mockStrip.On("ClearStrip", mock.Anything, sessionID, callsign, cid).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "CLEARED").Return()

	err := suite.service.IssueClearance(ctx, callsign, "", cid, sessionID)
	require.NoError(t, err)

	// Now handle UNABLE - expect UnclearStrip with the CID
	suite.mockStrip.On("UnclearStrip", mock.Anything, sessionID, callsign, cid).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "FAILED").Return()

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
	assert.Equal(t, "FAILED", strip.PdcState)
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
	suite.mockStrip.On("ClearStrip", mock.Anything, sessionID, callsign, cid).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "CLEARED").Return()

	err := suite.service.IssueClearance(ctx, callsign, "", cid, sessionID)
	require.NoError(t, err)

	// Wait for timeout - expect UnclearStrip and no-response message
	suite.mockStrip.On("UnclearStrip", mock.Anything, sessionID, callsign, cid).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "NO_RESPONSE").Return()

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
	assert.Equal(t, "NO_RESPONSE", strip.PdcState)
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
	suite.mockStrip.On("ClearStrip", mock.Anything, sessionID, callsign, cid).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "CLEARED").Return()

	err := suite.service.IssueClearance(ctx, callsign, "", cid, sessionID)
	require.NoError(t, err)

	// Revert to voice - use newCid
	suite.mockStrip.On("UnclearStrip", mock.Anything, sessionID, callsign, newCid).Return(nil)
	suite.mockFrontend.On("SendPdcStateChange", sessionID, callsign, "REVERT_TO_VOICE").Return()

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
	assert.Equal(t, "REVERT_TO_VOICE", strip.PdcState)
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

func TestProcessPDCRequest_Success(t *testing.T) {
	t.Parallel()
	suite := &PDCIntegrationTestSuite{}
	suite.SetupTest(t)
	ctx := context.Background()

	callsign := "SAS123"

	// Expect ACK message to pilot
	suite.mockHoppie.On("SendCPDLC", mock.Anything, mock.Anything, callsign, mock.MatchedBy(func(msg string) bool {
		return strings.Contains(msg, "STANDBY")
	})).Return(nil)

	suite.mockFrontend.On("SendPdcStateChange", int32(1), callsign, "REQUESTED").Return()

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

	// Verify database state
	strip, err := suite.queries.GetStrip(context.Background(), database.GetStripParams{
		Session:  1,
		Callsign: callsign,
	})
	require.NoError(t, err)
	assert.Equal(t, "REQUESTED", strip.PdcState)
}
