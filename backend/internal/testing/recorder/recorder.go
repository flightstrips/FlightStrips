package recorder

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/services"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Recorder captures WebSocket events for replay
type Recorder struct {
	session       *RecordedSession
	startTime     time.Time
	eventIndex    int
	mu            sync.Mutex
	outputPath    string
	autoSave      bool
	autoSaveTimer *time.Timer
}

// NewRecorder creates a new recorder instance
func NewRecorder(airport, connection, description string) *Recorder {
	return &Recorder{
		session: &RecordedSession{
			Version: "1.0",
			Metadata: SessionMetadata{
				Airport:         airport,
				Connection:      connection,
				RecordedAt:      time.Now(),
				DurationSeconds: 0,
				Description:     description,
			},
			Events:          []RecordedEvent{},
			Assertions:      []Assertion{},
			FrontendActions: []FrontendAction{},
		},
		startTime:  time.Now(),
		eventIndex: 0,
		autoSave:   true,
	}
}

// SetLoginInfo sets the login information in metadata
func (r *Recorder) SetLoginInfo(position, callsign string, rang int32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.session.Metadata.Position = position
	r.session.Metadata.Callsign = callsign
	r.session.Metadata.Range = rang
}

// Start begins recording and returns the recorder
func (r *Recorder) Start() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.startTime = time.Now()
	slog.Info("Recording started", 
		slog.String("airport", r.session.Metadata.Airport),
		slog.String("connection", r.session.Metadata.Connection))

	// Setup auto-save every 30 seconds
	if r.autoSave {
		r.scheduleAutoSave()
	}
}

// RecordEvent captures a WebSocket event
func (r *Recorder) RecordEvent(eventType string, payload interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Sanitize token events
	if eventType == "token" {
		payload = map[string]string{
			"type":  "token",
			"token": services.TestToken,
		}
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	elapsed := time.Since(r.startTime)
	event := RecordedEvent{
		Index:       r.eventIndex,
		TimestampMs: elapsed.Milliseconds(),
		Type:        eventType,
		Payload:     payloadBytes,
	}

	r.session.Events = append(r.session.Events, event)
	r.eventIndex++

	return nil
}

// AddAssertion adds a validation check to be performed during replay
func (r *Recorder) AddAssertion(afterEventIndex int, description string, checks []AssertionCheck) {
	r.mu.Lock()
	defer r.mu.Unlock()

	assertion := Assertion{
		AfterEventIndex: afterEventIndex,
		Description:     description,
		Checks:          checks,
	}

	r.session.Assertions = append(r.session.Assertions, assertion)
}

// AddFrontendAction adds a frontend client action to be performed during replay
func (r *Recorder) AddFrontendAction(afterEventIndex int, delayMs int, action string, callsign string, updates map[string]interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()

	frontendAction := FrontendAction{
		AfterEventIndex: afterEventIndex,
		DelayMs:         delayMs,
		Action:          action,
		Callsign:        callsign,
		Updates:         updates,
	}

	r.session.FrontendActions = append(r.session.FrontendActions, frontendAction)
}

// Stop ends the recording and saves to disk
func (r *Recorder) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.autoSaveTimer != nil {
		r.autoSaveTimer.Stop()
	}

	// Update duration
	elapsed := time.Since(r.startTime)
	r.session.Metadata.DurationSeconds = int(elapsed.Seconds())

	slog.Info("Recording stopped",
		slog.Int("events", len(r.session.Events)),
		slog.Int("duration_seconds", r.session.Metadata.DurationSeconds))

	return r.saveToFile()
}

// saveToFile writes the recorded session to a JSON file
func (r *Recorder) saveToFile() error {
	recordingPath := config.GetRecordingPath()
	
	// Create recording directory if it doesn't exist
	if err := os.MkdirAll(recordingPath, 0755); err != nil {
		return fmt.Errorf("failed to create recording directory: %w", err)
	}

	// Generate filename based on timestamp and airport
	filename := fmt.Sprintf("%s_%s_%s.json",
		r.session.Metadata.Airport,
		r.session.Metadata.Connection,
		time.Now().Format("20060102_150405"))

	filepath := filepath.Join(recordingPath, filename)

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(r.session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return fmt.Errorf("failed to write recording file: %w", err)
	}

	r.outputPath = filepath
	slog.Info("Recording saved", slog.String("path", filepath))

	return nil
}

// scheduleAutoSave sets up periodic auto-saving
func (r *Recorder) scheduleAutoSave() {
	r.autoSaveTimer = time.AfterFunc(30*time.Second, func() {
		r.mu.Lock()
		elapsed := time.Since(r.startTime)
		r.session.Metadata.DurationSeconds = int(elapsed.Seconds())
		r.saveToFile()
		r.mu.Unlock()

		// Reschedule
		r.scheduleAutoSave()
	})
}

// GetOutputPath returns the path where the recording was saved
func (r *Recorder) GetOutputPath() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.outputPath
}

// GetEventCount returns the number of recorded events
func (r *Recorder) GetEventCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.session.Events)
}
