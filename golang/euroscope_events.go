package main

type EuroscopeEvent struct {
	Type EventType
}

type EuroscopeAuthenticationEvent struct {
	Type  EventType `json:"type"`
	Token string    `json:"token"`
}

func (e *EuroscopeAuthenticationEvent) ValidateToken() bool {
	return false
}

type EuroscopeLoginEvent struct {
	Type     EventType `json:"type"`
	Airport  string    `json:"airport"`
	Position string    `json:"position"`
	Callsign string    `json:"callsign"`
	Range    int       `json:"range"`
}
