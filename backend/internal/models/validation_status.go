package models

import "encoding/json"

// ValidationStatus represents an active validation issue on a strip.
// It is set by backend services and acknowledged by frontend controllers.
type ValidationStatus struct {
	IssueType      string            `json:"issue_type"`
	Message        string            `json:"message"`
	OwningPosition string            `json:"owning_position"`
	Active         bool              `json:"active"`
	ActivationKey  string            `json:"activation_key"`
	CustomAction   *ValidationAction `json:"custom_action,omitempty"`
}

// ValidationAction describes an optional corrective action shown in the dialog.
type ValidationAction struct {
	Label      string          `json:"label"`
	ActionKind string          `json:"action_kind"`
	Payload    json.RawMessage `json:"payload,omitempty"`
}
