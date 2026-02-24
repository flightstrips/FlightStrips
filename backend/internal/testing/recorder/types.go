package recorder

import (
	"encoding/json"
	"time"
)

// RecordedSession represents a complete recorded EuroScope WebSocket session
type RecordedSession struct {
	Version         string                     `json:"version"`
	Metadata        SessionMetadata            `json:"metadata"`
	Events          []RecordedEvent            `json:"events"`
	Assertions      []Assertion                `json:"assertions,omitempty"`
	FrontendClients map[string]*FrontendClient `json:"frontend_clients,omitempty"`
}

// SessionMetadata contains metadata about the recording session
type SessionMetadata struct {
	Airport         string    `json:"airport"`
	Connection      string    `json:"connection"` // LIVE, SWEATBOX, PLAYBACK
	Clients         []string  `json:"clients,omitempty"`
	ClientCount     int       `json:"client_count"`
	RecordedAt      time.Time `json:"recorded_at"`
	DurationSeconds int       `json:"duration_seconds"`
	Description     string    `json:"description,omitempty"`
}

// RecordedEvent represents a single WebSocket event with timing information
type RecordedEvent struct {
	Index       int             `json:"index"`
	TimestampMs int64           `json:"timestamp_ms"` // Milliseconds since session start
	Type        string          `json:"type"`
	ClientID    string          `json:"client_id,omitempty"` // Callsign of the client (empty for synthetic events)
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

// FrontendClient defines a frontend client with its actions
type FrontendClient struct {
	ClientID    string           `json:"client_id"`
	CID         string           `json:"cid"`
	Description string           `json:"description,omitempty"`
	Actions     []FrontendAction `json:"actions"`
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

// ClientConnectPayload represents the payload of a client_connect event
type ClientConnectPayload struct {
	Callsign  string `json:"callsign"`
	Frequency string `json:"frequency"`
	Position  string `json:"position,omitempty"`
	Range     int32  `json:"range,omitempty"`
}

// ClientDisconnectPayload represents the payload of a client_disconnect event
type ClientDisconnectPayload struct {
	Reason          string `json:"reason"`
	DurationSeconds int    `json:"duration_seconds"`
}
