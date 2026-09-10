package euroscope

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/services"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	"log/slog"
)

// recordMessage records an incoming binary protobuf envelope when recording is enabled.
func (hub *Hub) recordMessage(sessionID int32, rawMessage []byte) {
	if !config.IsRecordMode() {
		return
	}

	eventType, _, err := euroscopeEvents.UnmarshalEnvelope(rawMessage)
	if err != nil {
		slog.Warn("Failed to decode protobuf message for recording", slog.Any("error", err))
		return
	}

	// Never persist a real access token in a recording.
	if eventType == euroscopeEvents.Authentication {
		rawMessage, err = euroscopeEvents.MarshalEnvelope(&euroscopeEvents.TokenEvent{Token: services.TestToken}, eventType)
		if err != nil {
			slog.Warn("Failed to sanitize protobuf token event", slog.Any("error", err))
			return
		}
	}

	if err := hub.RecordProtobufEvent(sessionID, euroscopeEvents.EventName(eventType), rawMessage); err != nil {
		slog.Warn("Failed to record protobuf event", slog.Any("error", err))
	}
}
