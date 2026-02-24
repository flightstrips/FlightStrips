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
	clients       map[string]*clientInfo // Track active clients by callsign
}

// clientInfo stores information about a connected client
type clientInfo struct {
	Callsign   string
	Frequency  string
	Position   string
	Range      int32
	ConnectTime time.Time
}

// NewRecorder creates a new recorder instance
func NewRecorder(airport, connection, description string) *Recorder {
	return &Recorder{
		session: &RecordedSession{
			Version: "2.0",
			Metadata: SessionMetadata{
				Airport:         airport,
				Connection:      connection,
				RecordedAt:      time.Now(),
				DurationSeconds: 0,
				Description:     description,
				Clients:         []string{},
				ClientCount:     0,
			},
			Events:          []RecordedEvent{},
			Assertions:      []Assertion{},
			FrontendClients: make(map[string]*FrontendClient),
		},
		startTime:  time.Now(),
		eventIndex: 0,
		autoSave:   true,
		clients:    make(map[string]*clientInfo),
	}
}

// ClientConnect records when a client connects
func (r *Recorder) ClientConnect(callsign, frequency, position string, rang int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if client already exists
	if _, exists := r.clients[callsign]; exists {
		slog.Warn("Client already connected", slog.String("callsign", callsign))
		return nil
	}

	// Store client info
	r.clients[callsign] = &clientInfo{
		Callsign:    callsign,
		Frequency:   frequency,
		Position:    position,
		Range:       rang,
		ConnectTime: time.Now(),
	}

	// Update metadata
	r.session.Metadata.Clients = append(r.session.Metadata.Clients, callsign)
	r.session.Metadata.ClientCount = len(r.clients)

	// Record connect event
	payload := ClientConnectPayload{
		Callsign:  callsign,
		Frequency: frequency,
		Position:  position,
		Range:     rang,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal connect payload: %w", err)
	}

	elapsed := time.Since(r.startTime)
	event := RecordedEvent{
		Index:       r.eventIndex,
		TimestampMs: elapsed.Milliseconds(),
		Type:        "client_connect",
		ClientID:    callsign,
		Payload:     payloadBytes,
	}

	r.session.Events = append(r.session.Events, event)
	r.eventIndex++

	slog.Info("Client connected",
		slog.String("callsign", callsign),
		slog.String("frequency", frequency))

	return nil
}

// ClientDisconnect records when a client disconnects
func (r *Recorder) ClientDisconnect(callsign, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if client exists
	client, exists := r.clients[callsign]
	if !exists {
		slog.Warn("Client not found for disconnect", slog.String("callsign", callsign))
		return nil
	}

	// Calculate duration
	duration := int(time.Since(client.ConnectTime).Seconds())

	// Record disconnect event
	payload := ClientDisconnectPayload{
		Reason:          reason,
		DurationSeconds: duration,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal disconnect payload: %w", err)
	}

	elapsed := time.Since(r.startTime)
	event := RecordedEvent{
		Index:       r.eventIndex,
		TimestampMs: elapsed.Milliseconds(),
		Type:        "client_disconnect",
		ClientID:    callsign,
		Payload:     payloadBytes,
	}

	r.session.Events = append(r.session.Events, event)
	r.eventIndex++

	// Remove client
	delete(r.clients, callsign)
	r.session.Metadata.ClientCount = len(r.clients)

	slog.Info("Client disconnected",
		slog.String("callsign", callsign),
		slog.String("reason", reason),
		slog.Int("duration_seconds", duration))

	return nil
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
func (r *Recorder) RecordEvent(clientID, eventType string, payload interface{}) error {
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
		ClientID:    clientID,
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

// AddFrontendClient adds a frontend client configuration
func (r *Recorder) AddFrontendClient(clientID, cid, description string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.session.FrontendClients == nil {
		r.session.FrontendClients = make(map[string]*FrontendClient)
	}

	r.session.FrontendClients[clientID] = &FrontendClient{
		ClientID:    clientID,
		CID:         cid,
		Description: description,
		Actions:     []FrontendAction{},
	}
}

// AddFrontendAction adds a frontend client action to be performed during replay
func (r *Recorder) AddFrontendAction(clientID string, afterEventIndex int, delayMs int, action string, callsign string, updates map[string]interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.session.FrontendClients == nil {
		r.session.FrontendClients = make(map[string]*FrontendClient)
	}

	// Create client if it doesn't exist
	if _, exists := r.session.FrontendClients[clientID]; !exists {
		r.session.FrontendClients[clientID] = &FrontendClient{
			ClientID: clientID,
			CID:      services.TestToken, // Default to test token
			Actions:  []FrontendAction{},
		}
	}

	frontendAction := FrontendAction{
		AfterEventIndex: afterEventIndex,
		DelayMs:         delayMs,
		Action:          action,
		Callsign:        callsign,
		Updates:         updates,
	}

	client := r.session.FrontendClients[clientID]
	client.Actions = append(client.Actions, frontendAction)
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

// GetActiveClients returns a list of currently connected clients
func (r *Recorder) GetActiveClients() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	clients := make([]string, 0, len(r.clients))
	for callsign := range r.clients {
		clients = append(clients, callsign)
	}
	return clients
}

// GetClientCount returns the number of currently connected clients
func (r *Recorder) GetClientCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.clients)
}
