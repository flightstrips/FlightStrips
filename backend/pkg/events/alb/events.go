package alb

import "encoding/json"

type EventType = string

const (
	Login    EventType = "login"
	Query    EventType = "query"
	Response EventType = "response"
	A2A      EventType = "a2a"
)

type LoginEvent struct {
	Type     EventType `json:"type"`
	Callsign string    `json:"callsign"`
}

type QueryEvent struct {
	Type     EventType `json:"type"`
	Subtype  string    `json:"subtype"`
	Callsign string    `json:"callsign"`
	Dest     string    `json:"dest"`
	Elt      string    `json:"elt"`
}

type ResponseEvent struct {
	Type     EventType `json:"type"`
	Subtype  string    `json:"subtype"`
	Callsign string    `json:"callsign"`
	Dest     string    `json:"dest"`
	Accepted bool      `json:"accepted"`
	Plt      string    `json:"plt"`
}

func (e ResponseEvent) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

type A2AEvent struct {
	Type     EventType `json:"type"`
	Subtype  string    `json:"subtype"`
	Sender   string    `json:"sender"`
	Receiver string    `json:"receiver"`
	Callsign string    `json:"callsign"`
	Text     string    `json:"text"`
}

func (e A2AEvent) Marshal() ([]byte, error) {
	return json.Marshal(e)
}
