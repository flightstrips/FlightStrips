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
	PluginInstanceID  string `json:"plugin_instance_id,omitempty"`
	MembershipID      string `json:"membership_id,omitempty"`
	SessionID         int32  `json:"session_id,omitempty"`
}
