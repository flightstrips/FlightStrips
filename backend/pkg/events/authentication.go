package events

const (
	Event = "token"
)

type AuthenticationEvent struct {
	InstanceID        string `json:"-"`
	FrequencyMessages bool   `json:"-"`
	Type              string `json:"type"`
	Token             string `json:"token"`
	Version           string `json:"version,omitempty"`
}
