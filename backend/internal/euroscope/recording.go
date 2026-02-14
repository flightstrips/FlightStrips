package euroscope

import (
	"FlightStrips/internal/config"
	"encoding/json"
	"log/slog"
)

// RecordMessage records an incoming message if recording is enabled
func (hub *Hub) recordMessage(sessionID int32, rawMessage []byte) {
	if !config.IsRecordMode() {
		return
	}

	// Parse message to extract type
	var msg struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(rawMessage, &msg); err != nil {
		slog.Warn("Failed to parse message for recording", slog.Any("error", err))
		return
	}

	// Record the raw message payload
	var payload interface{}
	if err := json.Unmarshal(rawMessage, &payload); err != nil {
		slog.Warn("Failed to unmarshal message payload for recording", slog.Any("error", err))
		return
	}

	if err := hub.RecordEvent(sessionID, msg.Type, payload); err != nil {
		slog.Warn("Failed to record event", slog.Any("error", err))
	}
}
