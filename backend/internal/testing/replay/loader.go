package replay

import (
	"encoding/json"
	"fmt"
	"os"

	"FlightStrips/internal/testing/recorder"
)

// LoadSession loads a recorded session from a JSON file
func LoadSession(filepath string) (*recorder.RecordedSession, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	var session recorder.RecordedSession
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to parse session JSON: %w", err)
	}

	// Validate session format
	if session.Version != "1.0" {
		return nil, fmt.Errorf("unsupported session version: %s", session.Version)
	}

	if len(session.Events) == 0 {
		return nil, fmt.Errorf("session contains no events")
	}

	return &session, nil
}

// ValidateSession performs basic validation on a loaded session
func ValidateSession(session *recorder.RecordedSession) error {
	if session.Metadata.Airport == "" {
		return fmt.Errorf("session metadata missing airport")
	}

	// Check events are in order
	for i := 1; i < len(session.Events); i++ {
		if session.Events[i].Index != session.Events[i-1].Index+1 {
			return fmt.Errorf("event sequence broken at index %d", i)
		}
		if session.Events[i].TimestampMs < session.Events[i-1].TimestampMs {
			return fmt.Errorf("event timestamps not monotonic at index %d", i)
		}
	}

	return nil
}
