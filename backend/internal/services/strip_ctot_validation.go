package services

import (
	"strings"
	"time"

	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
)

const (
	ctotValidationIssueType   = "CTOT"
	ctotValidationMessage     = "CTOT is more than 10 minutes in the future."
	ctotValidationActionKind  = "assign_holding_point"
	ctotValidationActionLabel = "ASSIGN HP"
	ctotValidationThreshold   = 10 * time.Minute
	ctotValidationRolloverGap = 12 * time.Hour
)

func isCtotValidation(status *internalModels.ValidationStatus) bool {
	return status != nil && status.IssueType == ctotValidationIssueType
}

func ctotValidationAction() *internalModels.ValidationAction {
	return &internalModels.ValidationAction{
		Label:      ctotValidationActionLabel,
		ActionKind: ctotValidationActionKind,
	}
}

func ctotValidationApplies(strip *internalModels.Strip) bool {
	if strip == nil || strip.Owner == nil || *strip.Owner == "" {
		return false
	}

	position, err := config.GetPositionBasedOnFrequency(*strip.Owner)
	if err != nil {
		position, err = config.GetPositionByName(*strip.Owner)
		if err != nil {
			return false
		}
	}
	if position.Section != "TWR" {
		return false
	}

	switch strip.Bay {
	case shared.BAY_TAXI_LWR, shared.BAY_DEPART:
		return true
	default:
		return false
	}
}

func parseValidationClockUTC(hhmm string, now time.Time) (time.Time, bool) {
	trimmed := strings.TrimSpace(hhmm)
	if len(trimmed) != 4 {
		return time.Time{}, false
	}
	for _, digit := range trimmed {
		if digit < '0' || digit > '9' {
			return time.Time{}, false
		}
	}

	hour := int(trimmed[0]-'0')*10 + int(trimmed[1]-'0')
	minute := int(trimmed[2]-'0')*10 + int(trimmed[3]-'0')
	if hour > 23 || minute > 59 {
		return time.Time{}, false
	}

	candidate := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
	if now.Sub(candidate) > ctotValidationRolloverGap {
		candidate = candidate.Add(24 * time.Hour)
	}

	return candidate, true
}

func ctotMoreThanThresholdAhead(ctot string, now time.Time) bool {
	ctotTime, ok := parseValidationClockUTC(ctot, now.UTC())
	if !ok {
		return false
	}

	return ctotTime.Sub(now.UTC()) > ctotValidationThreshold
}

// PlanCtotValidation is the same periodic policy without persistence or hub
// dependencies. The owner supplies a deterministic activation identity.
func PlanCtotValidation(strip *internalModels.Strip, now time.Time, activationID string, forceReactivate bool) *internalModels.ValidationStatus {
	if strip == nil {
		return nil
	}
	current := strip.ValidationStatus
	if validationCandidateIsInhibited(current, ctotValidationIssueType) {
		return current
	}
	ctot := ""
	if value := strip.EffectiveCtot(); value != nil {
		ctot = *value
	}
	if !ctotValidationApplies(strip) || !ctotMoreThanThresholdAhead(ctot, now) {
		if isCtotValidation(current) {
			return nil
		}
		return current
	}
	desired := &internalModels.ValidationStatus{IssueType: ctotValidationIssueType, Message: ctotValidationMessage, OwningPosition: *strip.Owner, Active: true, ActivationKey: activationID, CustomAction: ctotValidationAction()}
	if isCtotValidation(current) && current.OwningPosition == *strip.Owner && !forceReactivate {
		desired.Active, desired.ActivationKey = current.Active, current.ActivationKey
	}
	return desired
}
