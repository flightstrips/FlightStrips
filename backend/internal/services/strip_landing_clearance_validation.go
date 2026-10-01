package services

import (
	"time"
)

const (
	landingClearanceValidationIssueType   = "LANDING CLEARANCE"
	landingClearanceValidationMessage     = "Aircraft is not marked as cleared to land."
	landingClearanceValidationActionKind  = "runway_clearance"
	landingClearanceValidationActionLabel = "CLEAR TO LAND"
	landingClearanceValidationDelay       = 15 * time.Second
	// Temporary kill-switch while landing validation activation is known-bad.
	landingClearanceValidationCreationEnabled = false
)
