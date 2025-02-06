package main

type EuroscopeEvent struct {
	Type EventType
}

type EuroscopeAuthenticationEvent struct {
	Type  EventType
	Token string
}

type EuroscopeLoginEvent struct {
	Type     EventType `json:"type"`
	Airport  string    `json:"airport"`
	Position string    `json:"position"`
	Callsign string    `json:"callsign"`
	Range    int       `json:"range"`
}

type EuroscopeControllerOnlineEvent struct {
	Type     EventType `json:"type"`
	Position string    `json:"position"`
	Callsign string    `json:"callsign"`
}
