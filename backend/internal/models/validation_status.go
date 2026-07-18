package models

import "encoding/json"

const ValidationIssueTypeStandAssignment = "STAND ASSIGNMENT"

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

// IsBlocking reports whether the validation should stop normal strip handling.
// Stand-assignment conflicts are advisory: controllers must still be able to
// move, coordinate, and otherwise operate the strip while resolving the stand.
func (s *ValidationStatus) IsBlocking() bool {
	return s != nil && s.Active && s.IssueType != ValidationIssueTypeStandAssignment
}

// ValidationAction describes an optional corrective action shown in the dialog.
type ValidationAction struct {
	Label      string          `json:"label"`
	ActionKind string          `json:"action_kind"`
	Payload    json.RawMessage `json:"payload,omitempty"`
}
