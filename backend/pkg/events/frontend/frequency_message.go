package frontend

import "encoding/json"

// Actions are accepted by the backend before the message is dispatched. On
// failure, AppliedActions lets the composer retry only the remaining work.
type SendFrequencyMessageEvent struct {
	Type      EventType         `json:"type"`
	RequestID string            `json:"request_id"`
	Callsign  string            `json:"callsign"`
	Message   string            `json:"message"`
	Prepare   bool              `json:"prepare,omitempty"`
	Clearance bool              `json:"clearance,omitempty"`
	Actions   []json.RawMessage `json:"actions,omitempty"`
}

type FrequencyMessageResultEvent struct {
	Stand           string    `json:"stand,omitempty"`
	Prepared        bool      `json:"prepared,omitempty"`
	RunwayConfirmed bool      `json:"runway_confirmed,omitempty"`
	Type            EventType `json:"type"`
	RequestID       string    `json:"request_id"`
	Callsign        string    `json:"callsign"`
	Submitted       bool      `json:"submitted"`
	Error           string    `json:"error,omitempty"`
	AppliedActions  int32     `json:"applied_actions"`
}

func (e FrequencyMessageResultEvent) GetType() EventType { return FrequencyMessageResult }
func (e FrequencyMessageResultEvent) Marshal() ([]byte, error) {
	e.Type = e.GetType()
	return json.Marshal(e)
}
