package recorder

import (
	"encoding/json"
	"time"
)

// RecordedSession represents a complete recorded EuroScope WebSocket session
type RecordedSession struct {
	Version   string          `json:"version"`
	Metadata  SessionMetadata `json:"metadata"`
	Events    []RecordedEvent `json:"events"`
	Assertions []Assertion    `json:"assertions,omitempty"`
	FrontendActions []FrontendAction `json:"frontend_actions,omitempty"`
}

// SessionMetadata contains metadata about the recording session
type SessionMetadata struct {
	Airport         string    `json:"airport"`
	Connection      string    `json:"connection"`      // LIVE, SWEATBOX, PLAYBACK
	Position        string    `json:"position,omitempty"`
	Callsign        string    `json:"callsign,omitempty"`
	Range           int32     `json:"range,omitempty"`
	RecordedAt      time.Time `json:"recorded_at"`
	DurationSeconds int       `json:"duration_seconds"`
	Description     string    `json:"description,omitempty"`
}

// RecordedEvent represents a single WebSocket event with timing information
type RecordedEvent struct {
	Index       int             `json:"index"`
	TimestampMs int64           `json:"timestamp_ms"` // Milliseconds since session start
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload"`
}

// Assertion defines a validation check to be performed during replay
type Assertion struct {
	AfterEventIndex int              `json:"after_event_index"`
	Description     string           `json:"description"`
	Checks          []AssertionCheck `json:"checks"`
}

// AssertionCheck defines a single check to be performed
type AssertionCheck struct {
	Type     string                 `json:"type"` // strip_exists, strip_field_equals, controller_online, etc.
	Callsign string                 `json:"callsign,omitempty"`
	Field    string                 `json:"field,omitempty"`
	Expected interface{}            `json:"expected,omitempty"`
	Params   map[string]interface{} `json:"params,omitempty"`
}

// FrontendAction defines a frontend client action to be performed during replay
type FrontendAction struct {
	AfterEventIndex int                    `json:"after_event_index"`
	DelayMs         int                    `json:"delay_ms"`
	Action          string                 `json:"action"` // update_strip, query_strip, etc.
	Callsign        string                 `json:"callsign,omitempty"`
	Updates         map[string]interface{} `json:"updates,omitempty"`
	Params          map[string]interface{} `json:"params,omitempty"`
}
