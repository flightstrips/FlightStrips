package e2e

import (
	"context"
	"os"
	"testing"

	"FlightStrips/internal/config"
)

var testServer *TestServer

// TestMain sets up and tears down the test server
func TestMain(m *testing.M) {
	// Set TEST_MODE environment variable
	os.Setenv("TEST_MODE", "true")
	os.Setenv("RECORD_MODE", "false")

	// Change to backend directory for config files
	if err := os.Chdir("../../.."); err != nil {
		panic("Failed to change directory: " + err.Error())
	}

	// Initialize config
	config.InitConfig()

	// Start test server (with its own test container)
	var err error
	testServer, err = StartTestServer()
	if err != nil {
		panic("Failed to start test server: " + err.Error())
	}

	// Run tests
	code := m.Run()

	// Cleanup
	testServer.Stop()

	os.Exit(code)
}

// TestBasicSync tests that a sync event creates strips in the database
func TestBasicSync(t *testing.T) {
	// Clean database before test
	if err := testServer.CleanupDatabase(); err != nil {
		t.Fatalf("Failed to cleanup database: %v", err)
	}

	helper := NewTestHelper(t, testServer)

	// Ensure airport exists
	ctx := context.Background()
	if err := testServer.Queries.InsertAirport(ctx, "EKCH"); err != nil {
		t.Logf("Airport insert note (may already exist): %v", err)
	}

	// Run replay
	helper.FastReplay("recordings/sample_with_frontend_actions.json")

	// Wait for database to be updated
	helper.WaitForDatabaseUpdate()

	// Get session ID
	sessionID, err := helper.GetSessionID("EKCH", "LIVE")
	if err != nil {
		t.Fatalf("Failed to get session ID: %v", err)
	}

	// Verify strip was created
	helper.AssertStripExists(sessionID, "SAS123")
	helper.AssertStripCount(sessionID, 1)
	
	// Verify controller was created
	helper.AssertControllerOnline(sessionID, "EKCH_A_GND")

	// Verify EuroScope received messages from server
	helper.AssertEuroscopeReceivedMessage("session_info")
	
	if testing.Verbose() {
		helper.DumpDatabaseState()
		helper.DumpEuroscopeMessages()
	}

	t.Log("✓ Basic sync test completed successfully")
}

// TestFrontendActions tests that frontend actions are executed during replay
func TestFrontendActions(t *testing.T) {
	// Clean database before test
	if err := testServer.CleanupDatabase(); err != nil {
		t.Fatalf("Failed to cleanup database: %v", err)
	}

	helper := NewTestHelper(t, testServer)

	ctx := context.Background()
	if err := testServer.Queries.InsertAirport(ctx, "EKCH"); err != nil {
		t.Logf("Airport insert note: %v", err)
	}

	// Run replay with frontend actions
	helper.FastReplay("recordings/sample_with_frontend_actions.json")

	// Wait for all actions to complete
	helper.WaitForDatabaseUpdate()

	sessionID, err := helper.GetSessionID("EKCH", "LIVE")
	if err != nil {
		t.Fatalf("Failed to get session ID: %v", err)
	}

	// Verify strip exists
	helper.AssertStripExists(sessionID, "SAS123")

	// === VERIFY TWO-WAY COMMUNICATION ===
	
	// 1. Frontend receives initial data from server
	helper.AssertFrontendReceivedMessage("initial")
	t.Log("✓ Frontend received initial data")
	
	// 2. Frontend receives strip_update broadcasts (for database changes)
	helper.AssertFrontendReceivedMessage("strip_update")
	t.Log("✓ Frontend received strip_update broadcasts")
	
	// 3. EuroScope receives session_info (master/slave assignment)
	helper.AssertEuroscopeReceivedMessage("session_info")
	t.Log("✓ EuroScope received session_info")
	
	// 4. CRITICAL: When frontend updates strip fields, EuroScope receives individual field events
	//    The sample updates runway=22L and sid=VENER2A, then altitude=5000
	//    These are sent as "sid", "route", "altitude", "heading", "stand", etc.
	helper.AssertEuroscopeReceivedMessage("sid")
	t.Log("✓ EuroScope received SID update from frontend")

	if testing.Verbose() {
		helper.DumpDatabaseState()
		helper.DumpEuroscopeMessages()
		helper.DumpFrontendMessages()
	}

	t.Log("✓ Frontend actions test completed successfully")
	t.Log("✓ Verified frontend updates are sent to EuroScope as individual field events")
}

// TestComprehensiveFieldUpdates tests that all field update types are sent to EuroScope
func TestComprehensiveFieldUpdates(t *testing.T) {
	if err := testServer.CleanupDatabase(); err != nil {
		t.Fatalf("Failed to cleanup database: %v", err)
	}

	helper := NewTestHelper(t, testServer)

	ctx := context.Background()
	if err := testServer.Queries.InsertAirport(ctx, "EKCH"); err != nil {
		t.Logf("Airport insert note: %v", err)
	}

	// Run replay with comprehensive field updates
	helper.FastReplay("recordings/comprehensive_field_updates.json")

	// Wait for all actions to complete
	helper.WaitForDatabaseUpdate()

	sessionID, err := helper.GetSessionID("EKCH", "LIVE")
	if err != nil {
		t.Fatalf("Failed to get session ID: %v", err)
	}

	// Verify strip exists
	helper.AssertStripExists(sessionID, "SAS456")

	// === VERIFY ALL FIELD EVENT TYPES ===
	
	// Frontend sends: SID, Route, Altitude, Heading, Stand
	// EuroScope should receive each as individual field events
	
	helper.AssertEuroscopeReceivedMessage("sid")
	t.Log("✓ EuroScope received SID event")
	
	helper.AssertEuroscopeReceivedMessage("route")
	t.Log("✓ EuroScope received Route event")
	
	helper.AssertEuroscopeReceivedMessage("cleared_altitude")
	t.Log("✓ EuroScope received Cleared Altitude event")
	
	helper.AssertEuroscopeReceivedMessage("heading")
	t.Log("✓ EuroScope received Heading event")

	helper.AssertEuroscopeReceivedMessage("stand")
	t.Log("✓ EuroScope received Stand event")
	
	if testing.Verbose() {
		helper.DumpDatabaseState()
		helper.DumpEuroscopeMessages()
		helper.DumpFrontendMessages()
	}

	t.Log("✓ All field update events successfully sent to EuroScope")
}

// TestRealRecording tests with a real recorded session if available
func TestRealRecording(t *testing.T) {
	recordingFile := "recordings/EKCH_LIVE_20260214.json"

	// Check if file exists
	if _, err := os.Stat(recordingFile); os.IsNotExist(err) {
		t.Skip("Real recording file not found, skipping test")
	}

	if err := testServer.CleanupDatabase(); err != nil {
		t.Fatalf("Failed to cleanup database: %v", err)
	}

	helper := NewTestHelper(t, testServer)

	ctx := context.Background()
	if err := testServer.Queries.InsertAirport(ctx, "EKCH"); err != nil {
		t.Logf("Airport insert note: %v", err)
	}

	// Run replay at 100x speed
	helper.RunReplay(recordingFile, 100.0)

	helper.WaitForDatabaseUpdate()

	sessionID, err := helper.GetSessionID("EKCH", "LIVE")
	if err != nil {
		t.Fatalf("Failed to get session ID: %v", err)
	}

	// Verify controller was created
	helper.AssertControllerOnline(sessionID, "EKCH_A_GND")

	if testing.Verbose() {
		helper.DumpDatabaseState()
	}

	t.Log("✓ Real recording test completed successfully")
}
