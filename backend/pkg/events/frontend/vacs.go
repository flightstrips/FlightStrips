package frontend

import (
	"encoding/json"

	es "FlightStrips/pkg/events/euroscope"
)

const VacsRequest EventType = "vacs_request"
const VacsResult EventType = "vacs_result"

type VacsRequestEvent struct {
	RequestID string `json:"request_id"`
	Action    string `json:"action"`
	CallID    string `json:"call_id"`
	ClientID  string `json:"client_id"`
}

type VacsResultEvent struct {
	RequestID string        `json:"request_id"`
	State     *es.VacsState `json:"state,omitempty"`
	Error     string        `json:"error,omitempty"`
}

func (e VacsResultEvent) Marshal() ([]byte, error) {
	return json.Marshal(struct {
		Type EventType `json:"type"`
		VacsResultEvent
	}{VacsResult, e})
}
