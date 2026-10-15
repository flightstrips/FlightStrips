package events

const (
	Event = "token"
)

type AuthenticationEvent struct {
	FrequencyMessages bool   `json:"-"`
	Type              string `json:"type"`
	Token             string `json:"token"`
	Version           string `json:"version,omitempty"`
}
