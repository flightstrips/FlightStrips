package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"FlightStrips/internal/testing/replay"
)

// TestHelper provides helper functions for E2E tests
type TestHelper struct {
	Server   *TestServer
	T        *testing.T
	Replayer *replay.Replayer
}

// NewTestHelper creates a new test helper
func NewTestHelper(t *testing.T, server *TestServer) *TestHelper {
	return &TestHelper{
		Server: server,
		T:      t,
	}
}

// ReplaySession replays a recorded session and returns the replayer
func (h *TestHelper) ReplaySession(sessionFile string, speed float64) (*replay.Replayer, error) {
	config := replay.DefaultConfig()
	config.SessionFile = sessionFile
	config.ServerURL = h.Server.GetWebSocketURL()
	config.SpeedMultiplier = speed
	config.Mode = replay.ModeTimeBased
	config.StopOnError = true
	config.Verbose = testing.Verbose()

	replayer, err := replay.NewReplayerWithoutAssertions(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create replayer: %w", err)
	}

	return replayer, nil
}

// RunReplay runs a replay and fails the test if it encounters errors
func (h *TestHelper) RunReplay(sessionFile string, speed float64) {
	h.T.Helper()

	replayer, err := h.ReplaySession(sessionFile, speed)
	if err != nil {
		h.T.Fatalf("Failed to create replayer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := replayer.Replay(ctx); err != nil {
		h.T.Fatalf("Replay failed: %v", err)
	}

	stats := replayer.GetStats()
	if stats.EventsFailed > 0 {
		h.T.Errorf("Replay had %d failed events", stats.EventsFailed)
	}

	h.Replayer = replayer
	h.T.Logf("Replay completed: %d events replayed, %d frontend actions", 
		stats.EventsReplayed, stats.FrontendActionsRun)
}

// FastReplay runs a replay in fast mode
func (h *TestHelper) FastReplay(sessionFile string) {
	h.T.Helper()

	config := replay.DefaultConfig()
	config.SessionFile = sessionFile
	config.ServerURL = h.Server.GetWebSocketURL()
	config.Mode = replay.ModeFast
	config.StopOnError = true
	config.Verbose = testing.Verbose()

	replayer, err := replay.NewReplayerWithoutAssertions(config)
	if err != nil {
		h.T.Fatalf("Failed to create replayer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := replayer.Replay(ctx); err != nil {
		h.T.Fatalf("Replay failed: %v", err)
	}

	stats := replayer.GetStats()
	if stats.EventsFailed > 0 {
		h.T.Errorf("Replay had %d failed events", stats.EventsFailed)
	}

	h.Replayer = replayer
	h.T.Logf("Fast replay completed: %d events in %v", 
		stats.EventsReplayed, stats.Duration())
}

// AssertStripExists checks that a strip exists in the database
func (h *TestHelper) AssertStripExists(sessionID int32, callsign string) {
	h.T.Helper()
	
	ctx := context.Background()
	var exists bool
	err := h.Server.DBPool.QueryRow(ctx, 
		"SELECT EXISTS(SELECT 1 FROM strips WHERE callsign = $1 AND session = $2)", 
		callsign, sessionID).Scan(&exists)
	
	if err != nil {
		h.T.Fatalf("Failed to check if strip exists: %v", err)
	}

	if !exists {
		h.T.Errorf("Strip %s does not exist in session %d", callsign, sessionID)
	}
}

// AssertStripCount checks the number of strips in a session
func (h *TestHelper) AssertStripCount(sessionID int32, expected int) {
	h.T.Helper()
	
	ctx := context.Background()
	var count int
	err := h.Server.DBPool.QueryRow(ctx, 
		"SELECT COUNT(*) FROM strips WHERE session = $1", 
		sessionID).Scan(&count)
	
	if err != nil {
		h.T.Fatalf("Failed to count strips: %v", err)
	}

	if count != expected {
		h.T.Errorf("Expected %d strips in session %d, got %d", expected, sessionID, count)
	}
}

// AssertControllerOnline checks that a controller is online
func (h *TestHelper) AssertControllerOnline(sessionID int32, callsign string) {
	h.T.Helper()
	
	ctx := context.Background()
	var exists bool
	err := h.Server.DBPool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM controllers WHERE callsign = $1 AND session = $2)",
		callsign, sessionID).Scan(&exists)
	
	if err != nil {
		h.T.Fatalf("Failed to check if controller exists: %v", err)
	}

	if !exists {
		h.T.Errorf("Controller %s does not exist in session %d", callsign, sessionID)
	}
}

// WaitForDatabaseUpdate waits for the database to be updated
func (h *TestHelper) WaitForDatabaseUpdate() {
	time.Sleep(200 * time.Millisecond)
}

// AssertStripField checks a specific field value on a strip
func (h *TestHelper) AssertStripField(sessionID int32, callsign, field string, expectedValue interface{}) {
	h.T.Helper()

	ctx := context.Background()
	query := fmt.Sprintf("SELECT %s FROM strips WHERE callsign = $1 AND session = $2", field)
	var actualValue interface{}
	err := h.Server.DBPool.QueryRow(ctx, query, callsign, sessionID).Scan(&actualValue)
	
	if err != nil {
		h.T.Fatalf("Failed to query strip field %s: %v", field, err)
	}

	// Convert to string for comparison
	actualStr := fmt.Sprintf("%v", actualValue)
	expectedStr := fmt.Sprintf("%v", expectedValue)

	if actualStr != expectedStr {
		h.T.Errorf("Strip %s field %s: expected %v, got %v", callsign, field, expectedValue, actualValue)
	}
}

// GetSessionID retrieves the session ID for a given airport and name
func (h *TestHelper) GetSessionID(airport, name string) (int32, error) {
	ctx := context.Background()
	var sessionID int32
	err := h.Server.DBPool.QueryRow(ctx,
		"SELECT id FROM sessions WHERE airport = $1 AND name = $2",
		airport, name).Scan(&sessionID)
	
	if err != nil {
		return 0, fmt.Errorf("failed to get session ID: %w", err)
	}

	return sessionID, nil
}

// DumpDatabaseState dumps the current database state for debugging
func (h *TestHelper) DumpDatabaseState() {
	h.T.Helper()

	ctx := context.Background()
	h.T.Log("=== DATABASE STATE ===")
	
	// Dump sessions
	rows, err := h.Server.DBPool.Query(ctx, "SELECT id, airport, name FROM sessions")
	if err == nil {
		defer rows.Close()
		h.T.Log("Sessions:")
		for rows.Next() {
			var id int32
			var airport, name string
			rows.Scan(&id, &airport, &name)
			h.T.Logf("  - ID:%d Airport:%s Name:%s", id, airport, name)
		}
	}

	// Dump controllers
	rows, err = h.Server.DBPool.Query(ctx, "SELECT callsign, cid, session FROM controllers")
	if err == nil {
		defer rows.Close()
		h.T.Log("Controllers:")
		for rows.Next() {
			var callsign string
			var cid *string
			var sessionID int32
			rows.Scan(&callsign, &cid, &sessionID)
			cidStr := "<nil>"
			if cid != nil {
				cidStr = *cid
			}
			h.T.Logf("  - Callsign:%s CID:%s Session:%d", callsign, cidStr, sessionID)
		}
	}

	// Dump strips
	rows, err = h.Server.DBPool.Query(ctx, "SELECT callsign, origin, destination, runway, sid, squawk, session FROM strips")
	if err == nil {
		defer rows.Close()
		h.T.Log("Strips:")
		for rows.Next() {
			var callsign, origin, dest string
			var runway, sid, squawk *string
			var sessionID int32
			rows.Scan(&callsign, &origin, &dest, &runway, &sid, &squawk, &sessionID)
			h.T.Logf("  - %s: %s->%s RWY:%v SID:%v SQ:%v Session:%d", 
				callsign, origin, dest, ptrStr(runway), ptrStr(sid), ptrStr(squawk), sessionID)
		}
	}
}

func ptrStr(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

// AssertEuroscopeReceivedMessage checks that EuroScope client received a specific message type
func (h *TestHelper) AssertEuroscopeReceivedMessage(eventType string) {
	h.T.Helper()

	if h.Replayer == nil {
		h.T.Fatal("No replayer available - run a replay first")
	}

	messages := h.Replayer.GetClient().GetReceivedMessages()
	
	for _, msg := range messages {
		if msg.EventType == eventType {
			h.T.Logf("✓ EuroScope received %s message", eventType)
			return
		}
	}

	h.T.Errorf("EuroScope did not receive %s message", eventType)
}

// AssertFrontendReceivedMessage checks that frontend client received a specific message type
func (h *TestHelper) AssertFrontendReceivedMessage(eventType string) {
	h.T.Helper()

	if h.Replayer == nil {
		h.T.Fatal("No replayer available - run a replay first")
	}

	frontendClient := h.Replayer.GetFrontendClient()
	if frontendClient == nil {
		h.T.Skip("No frontend client in this replay")
	}

	messages := frontendClient.GetReceivedMessages()
	
	for _, msg := range messages {
		if msg.EventType == eventType {
			h.T.Logf("✓ Frontend received %s message", eventType)
			return
		}
	}

	h.T.Errorf("Frontend did not receive %s message", eventType)
}

// DumpEuroscopeMessages dumps all messages received by EuroScope client
func (h *TestHelper) DumpEuroscopeMessages() {
	h.T.Helper()

	if h.Replayer == nil {
		h.T.Log("No replayer available")
		return
	}

	messages := h.Replayer.GetClient().GetReceivedMessages()
	h.T.Logf("=== EUROSCOPE RECEIVED %d MESSAGES ===", len(messages))
	
	messageTypes := make(map[string]int)
	for _, msg := range messages {
		messageTypes[msg.EventType]++
	}

	for eventType, count := range messageTypes {
		h.T.Logf("  - %s: %d", eventType, count)
	}
}

// DumpFrontendMessages dumps all messages received by frontend client
func (h *TestHelper) DumpFrontendMessages() {
	h.T.Helper()

	if h.Replayer == nil {
		h.T.Log("No replayer available")
		return
	}

	frontendClient := h.Replayer.GetFrontendClient()
	if frontendClient == nil {
		h.T.Log("No frontend client in this replay")
		return
	}

	messages := frontendClient.GetReceivedMessages()
	h.T.Logf("=== FRONTEND RECEIVED %d MESSAGES ===", len(messages))
	
	messageTypes := make(map[string]int)
	for _, msg := range messages {
		messageTypes[msg.EventType]++
	}

	for eventType, count := range messageTypes {
		h.T.Logf("  - %s: %d", eventType, count)
	}
}

// AssertFrontendUpdateSentToEuroscope verifies that when frontend sends an update,
// EuroScope receives a corresponding broadcast
func (h *TestHelper) AssertFrontendUpdateSentToEuroscope(callsign string) {
	h.T.Helper()

	if h.Replayer == nil {
		h.T.Fatal("No replayer available - run a replay first")
	}

	// Check EuroScope received strip_update for this callsign
	messages := h.Replayer.GetClient().GetReceivedMessages()
	
	for _, msg := range messages {
		if msg.EventType == "strip_update" {
			// Parse the message to check callsign
			var stripUpdate map[string]interface{}
			if err := json.Unmarshal(msg.Data, &stripUpdate); err == nil {
				if cs, ok := stripUpdate["callsign"].(string); ok && cs == callsign {
					h.T.Logf("✓ EuroScope received strip_update for %s after frontend update", callsign)
					return
				}
			}
		}
	}

	h.T.Errorf("EuroScope did not receive strip_update for %s after frontend update", callsign)
}
