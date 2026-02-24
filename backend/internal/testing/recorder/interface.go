package recorder

// RecordingHub defines the interface for hubs that support recording
type RecordingHub interface {
	// RecordEvent records an event if recording is enabled
	RecordEvent(sessionID int32, clientID, eventType string, payload interface{}) error
	
	// ClientConnect records when a client connects
	ClientConnect(sessionID int32, callsign, frequency, position string, rang int32) error
	
	// ClientDisconnect records when a client disconnects
	ClientDisconnect(sessionID int32, callsign, reason string) error
	
	// StartRecording starts recording for a session
	StartRecording(sessionID int32, airport, connection, description string) error
	
	// StopRecording stops recording for a session
	StopRecording(sessionID int32) error
	
	// IsRecording returns true if the session is being recorded
	IsRecording(sessionID int32) bool
}
