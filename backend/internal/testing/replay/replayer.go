package replay

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"FlightStrips/internal/database"
	"FlightStrips/internal/testing/assertions"
	testFrontend "FlightStrips/internal/testing/frontend"
	"FlightStrips/internal/testing/recorder"
)

// Replayer orchestrates the replay of a recorded session
type Replayer struct {
	client         *Client
	frontendClient *testFrontend.Client
	session        *recorder.RecordedSession
	config         Config

	// Assertions
	assertionEngine *assertions.Engine
	sessionID       int32

	// Statistics
	stats ReplayStats
}

// ReplayStats tracks statistics during replay
type ReplayStats struct {
	StartTime           time.Time
	EndTime             time.Time
	EventsReplayed      int
	EventsFailed        int
	FrontendActionsRun  int
	TotalEvents         int
	Errors              []*ReplayError
}

// Duration returns the replay duration
func (s *ReplayStats) Duration() time.Duration {
	if s.EndTime.IsZero() {
		return time.Since(s.StartTime)
	}
	return s.EndTime.Sub(s.StartTime)
}

// NewReplayer creates a new replayer
func NewReplayer(config Config, queries *database.Queries, sessionID int32) (*Replayer, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// Load session
	session, err := LoadSession(config.SessionFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load session: %w", err)
	}

	// Validate session
	if err := ValidateSession(session); err != nil {
		return nil, fmt.Errorf("session validation failed: %w", err)
	}

	// Create client
	client, err := NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	// Create assertion engine if queries provided and there are assertions
	var assertionEngine *assertions.Engine
	if queries != nil && len(session.Assertions) > 0 {
		assertionEngine = assertions.NewEngine(queries, sessionID)
	}

	return &Replayer{
		client:          client,
		session:         session,
		config:          config,
		assertionEngine: assertionEngine,
		sessionID:       sessionID,
		stats: ReplayStats{
			TotalEvents: len(session.Events),
		},
	}, nil
}

// NewReplayerWithoutAssertions creates a replayer without assertion support
func NewReplayerWithoutAssertions(config Config) (*Replayer, error) {
	return NewReplayer(config, nil, 0)
}

// Replay executes the replay
func (r *Replayer) Replay(ctx context.Context) error {
	r.stats.StartTime = time.Now()

	slog.Info("Starting replay",
		slog.String("session", r.config.SessionFile),
		slog.String("mode", string(r.config.Mode)),
		slog.Int("events", r.stats.TotalEvents),
		slog.String("airport", r.session.Metadata.Airport))

	// Connect to server (this sends the token event)
	if err := r.client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer r.client.Close()

	// Start reading messages from server
	r.client.ReadMessages(ctx, r.handleServerMessage)

	// Connect frontend client if there are frontend clients with actions
	hasFrontendActions := false
	for _, feClient := range r.session.FrontendClients {
		if len(feClient.Actions) > 0 {
			hasFrontendActions = true
			break
		}
	}

	if hasFrontendActions {
		if err := r.connectFrontendClient(ctx); err != nil {
			slog.Warn("Failed to connect frontend client, frontend actions will be skipped", slog.Any("error", err))
		} else {
			defer r.frontendClient.Close()
		}
	}

	// Send synthesized login event before replaying recorded events
	if err := r.sendLoginEvent(); err != nil {
		return fmt.Errorf("failed to send login event: %w", err)
	}

	// Replay events
	if err := r.replayEvents(ctx); err != nil {
		return err
	}

	// If there are frontend actions, wait a bit longer for any async processing
	// to complete (e.g., strip updates being sent to EuroScope clients)
	if hasFrontendActions {
		if r.config.Verbose {
			slog.Info("Waiting for frontend actions to complete...")
		}
		time.Sleep(2 * time.Second)
	}

	r.stats.EndTime = time.Now()

	// Print summary
	r.printSummary()

	return nil
}

// connectFrontendClient connects a frontend client for simulating frontend actions
func (r *Replayer) connectFrontendClient(ctx context.Context) error {
	frontendURL := "ws://localhost:2994/frontEndEvents"
	if r.config.ServerURL != "" {
		// Try to derive frontend URL from euroscope URL
		frontendURL = strings.Replace(r.config.ServerURL, "/euroscopeEvents", "/frontEndEvents", 1)
	}

	r.frontendClient = testFrontend.NewClient(testFrontend.Config{
		ServerURL: frontendURL,
		Token:     "__TEST_TOKEN__", // Use same token as EuroScope to get same CID
		Verbose:   r.config.Verbose,
	})

	if err := r.frontendClient.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect frontend client: %w", err)
	}

	r.frontendClient.ReadMessages(ctx, func(messageType int, data []byte) {
		if r.config.Verbose {
			slog.Debug("Frontend received message", slog.String("data", string(data)))
		}
	})

	slog.Info("Frontend client connected for simulated actions")
	return nil
}

// sendLoginEvent synthesizes and sends a login event from metadata or client_connect events
func (r *Replayer) sendLoginEvent() error {
	// Try to find the first client_connect event to extract login info
	var callsign, position string
	var rang int32 = 200 // default

	for _, event := range r.session.Events {
		if event.Type == "client_connect" {
			var connectPayload recorder.ClientConnectPayload
			if err := json.Unmarshal(event.Payload, &connectPayload); err == nil {
				callsign = connectPayload.Callsign
				position = connectPayload.Position
				if connectPayload.Range > 0 {
					rang = connectPayload.Range
				}
				break
			}
		}
	}

	// Fallback to defaults if no client_connect found
	if callsign == "" {
		callsign = "REPLAY_CTR"
	}
	if position == "" {
		position = "REPLAY_POS"
	}

	loginEvent := map[string]interface{}{
		"type":       "login",
		"connection": r.session.Metadata.Connection,
		"airport":    r.session.Metadata.Airport,
		"position":   position,
		"callsign":   callsign,
		"range":      rang,
	}

	if r.config.Verbose {
		slog.Info("Sending synthesized login event", slog.Any("event", loginEvent))
	}
	
	return r.client.SendRawMessage(loginEvent)
}

// replayEvents replays all events from the session
func (r *Replayer) replayEvents(ctx context.Context) error {
	var lastTimestamp int64 = 0

	// Build assertion map for quick lookup
	assertionMap := make(map[int][]recorder.Assertion)
	for _, assertion := range r.session.Assertions {
		assertionMap[assertion.AfterEventIndex] = append(assertionMap[assertion.AfterEventIndex], assertion)
	}

	// Build frontend action map for quick lookup (flatten all client actions)
	frontendActionMap := make(map[int][]recorder.FrontendAction)
	for _, feClient := range r.session.FrontendClients {
		for _, action := range feClient.Actions {
			frontendActionMap[action.AfterEventIndex] = append(frontendActionMap[action.AfterEventIndex], action)
		}
	}

	for i, event := range r.session.Events {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Calculate delay based on mode
		delay := r.calculateDelay(event.TimestampMs, lastTimestamp)
		if delay > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		// Send event
		if err := r.client.SendEvent(event); err != nil {
			replayErr := &ReplayError{
				Message:    "failed to send event",
				EventIndex: i,
				EventType:  event.Type,
				Err:        err,
			}
			r.stats.EventsFailed++
			r.stats.Errors = append(r.stats.Errors, replayErr)

			if r.config.StopOnError {
				return replayErr
			}

			slog.Error("Event replay failed",
				slog.Int("index", i),
				slog.String("type", event.Type),
				slog.Any("error", err))
		} else {
			r.stats.EventsReplayed++
		}

		// Update progress (every 10 events or on last event)
		if !r.config.Verbose && (i%10 == 0 || i == len(r.session.Events)-1) {
			r.printProgress(i + 1)
		}

		// Execute assertions after this event
		if assertions, ok := assertionMap[i]; ok && r.assertionEngine != nil {
			// Small delay to allow backend to process the event
			time.Sleep(100 * time.Millisecond)

			for _, assertion := range assertions {
				result := r.assertionEngine.ExecuteAssertions(ctx, assertion)
				if !result.AllPassed {
					slog.Warn("Assertion failed",
						slog.Int("after_event", i),
						slog.String("description", assertion.Description))
				}
			}
		}

		// Execute frontend actions after this event
		if actions, ok := frontendActionMap[i]; ok && r.frontendClient != nil {
			// Give backend time to process the event before frontend actions
			time.Sleep(100 * time.Millisecond)
			
			for _, action := range actions {
				if err := r.executeFrontendAction(ctx, action); err != nil {
					slog.Error("Failed to execute frontend action",
						slog.Int("after_event", i),
						slog.String("action", action.Action),
						slog.Any("error", err))
				} else {
					r.stats.FrontendActionsRun++
				}
			}
		}

		lastTimestamp = event.TimestampMs
	}

	return nil
}

// calculateDelay calculates the delay before sending the next event
func (r *Replayer) calculateDelay(currentTimestamp, lastTimestamp int64) time.Duration {
	if r.config.Mode == ModeFast {
		return r.config.MinEventDelay
	}

	// Time-based mode
	if lastTimestamp == 0 {
		return 0 // First event, no delay
	}

	deltaMs := currentTimestamp - lastTimestamp
	if deltaMs <= 0 {
		return 0
	}

	// Apply speed multiplier
	adjustedMs := float64(deltaMs) / r.config.SpeedMultiplier

	return time.Duration(adjustedMs) * time.Millisecond
}

// executeFrontendAction executes a single frontend action
func (r *Replayer) executeFrontendAction(ctx context.Context, action recorder.FrontendAction) error {
	// Apply delay if specified
	if action.DelayMs > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(action.DelayMs) * time.Millisecond):
		}
	}

	if r.config.Verbose {
		slog.Info("Executing frontend action",
			slog.String("action", action.Action),
			slog.String("callsign", action.Callsign))
	}

	switch action.Action {
	case "update_strip":
		return r.updateStripFromAction(action)
	case "update_field":
		return r.updateStripFieldFromAction(action)
	default:
		return fmt.Errorf("unknown frontend action: %s", action.Action)
	}
}

// updateStripFromAction updates a strip using the updates map
func (r *Replayer) updateStripFromAction(action recorder.FrontendAction) error {
	version := int32(0)
	if v, ok := action.Updates["version"].(float64); ok {
		version = int32(v)
	}

	return r.frontendClient.UpdateStrip(action.Callsign, version, action.Updates)
}

// updateStripFieldFromAction updates a single field on a strip
func (r *Replayer) updateStripFieldFromAction(action recorder.FrontendAction) error {
	version := int32(0)
	if v, ok := action.Params["version"].(float64); ok {
		version = int32(v)
	}

	field := ""
	if f, ok := action.Params["field"].(string); ok {
		field = f
	}

	value := action.Params["value"]

	return r.frontendClient.UpdateStripField(action.Callsign, version, field, value)
}

// handleServerMessage processes messages received from the server
func (r *Replayer) handleServerMessage(messageType int, data []byte) {
	if r.config.Verbose {
		slog.Debug("Received message from server",
			slog.Int("type", messageType),
			slog.Int("bytes", len(data)),
			slog.String("data", string(data)))
	}
}

// printSummary prints replay statistics
func (r *Replayer) printSummary() {
	duration := r.stats.Duration()
	successRate := float64(r.stats.EventsReplayed) / float64(r.stats.TotalEvents) * 100

	// Clear progress line and move to new line
	fmt.Print("\r" + strings.Repeat(" ", 80) + "\r")

	slog.Info("Replay completed",
		slog.Duration("duration", duration),
		slog.Int("events_replayed", r.stats.EventsReplayed),
		slog.Int("events_failed", r.stats.EventsFailed),
		slog.Int("frontend_actions", r.stats.FrontendActionsRun),
		slog.Int("total_events", r.stats.TotalEvents),
		slog.Float64("success_rate", successRate))

	if len(r.stats.Errors) > 0 {
		slog.Warn("Replay had errors", slog.Int("count", len(r.stats.Errors)))
		for _, err := range r.stats.Errors {
			slog.Error("Error during replay", slog.Any("error", err))
		}
	}

	// Print assertion results
	if r.assertionEngine != nil {
		fmt.Println() // Add spacing
		r.assertionEngine.PrintResults()
	}
}

// printProgress prints a progress indicator on a single line
func (r *Replayer) printProgress(current int) {
	percent := float64(current) / float64(r.stats.TotalEvents) * 100
	elapsed := time.Since(r.stats.StartTime)
	eventsPerSec := float64(current) / elapsed.Seconds()
	
	// Calculate ETA
	remaining := r.stats.TotalEvents - current
	eta := time.Duration(0)
	if eventsPerSec > 0 {
		eta = time.Duration(float64(remaining)/eventsPerSec) * time.Second
	}
	
	// Create progress bar
	barWidth := 30
	filled := int(percent / 100 * float64(barWidth))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	
	// Print on same line using \r
	fmt.Printf("\rReplaying: [%s] %d/%d (%.1f%%) | %.1f ev/s | ETA: %s",
		bar, current, r.stats.TotalEvents, percent, eventsPerSec, eta.Round(time.Second))
}

// GetAssertionEngine returns the assertion engine (can be nil)
func (r *Replayer) GetAssertionEngine() *assertions.Engine {
	return r.assertionEngine
}

// GetStats returns the replay statistics
func (r *Replayer) GetStats() ReplayStats {
	return r.stats
}

// GetClient returns the EuroScope replay client
func (r *Replayer) GetClient() *Client {
	return r.client
}

// GetFrontendClient returns the frontend client if available
func (r *Replayer) GetFrontendClient() *testFrontend.Client {
	return r.frontendClient
}
