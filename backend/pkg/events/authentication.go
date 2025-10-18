package events

const (
	Event = "token"
)

type AuthenticationEvent struct {
	Type  string `json:"type"`
	Token string `json:"token"`
}
