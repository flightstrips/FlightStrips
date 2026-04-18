package services

import (
	"context"
	"fmt"
	"strings"

	internalModels "FlightStrips/internal/models"

	"github.com/google/uuid"
)

const wrongSquawkValidationIssueType = "WRONG SQUAWK"

func isWrongSquawkValidation(status *internalModels.ValidationStatus) bool {
	return status != nil && status.IssueType == wrongSquawkValidationIssueType
}

func normalizeObservedSquawkCode(code *string) string {
	if code == nil {
		return ""
	}

	normalized := strings.TrimSpace(*code)
	if len(normalized) != 4 {
		return ""
	}
	for _, digit := range normalized {
		if digit < '0' || digit > '7' {
			return ""
		}
	}

	return normalized
}

func wrongSquawkPresentForStrip(strip *internalModels.Strip) bool {
	assigned := normalizeObservedSquawkCode(strip.AssignedSquawk)
	observed := normalizeObservedSquawkCode(strip.Squawk)
	return assigned != "" && observed != "" && assigned != observed
}

func wrongSquawkValidationMessage(strip *internalModels.Strip) string {
	assigned := normalizeObservedSquawkCode(strip.AssignedSquawk)
	observed := normalizeObservedSquawkCode(strip.Squawk)

	switch {
	case observed != "" && assigned != "":
		return fmt.Sprintf("Pilot is transmitting squawk %s but assigned squawk is %s.", observed, assigned)
	case observed != "":
		return fmt.Sprintf("Pilot is transmitting squawk %s.", observed)
	case assigned != "":
		return fmt.Sprintf("Assigned squawk is %s.", assigned)
	default:
		return "Pilot squawk does not match the assigned squawk."
	}
}

func (s *StripService) applyWrongSquawkValidation(ctx context.Context, session int32, strip *internalModels.Strip, publish bool, forceReactivate bool) error {
	if strip == nil {
		return nil
	}

	current := strip.ValidationStatus
	if current != nil && !isWrongSquawkValidation(current) && !isCtotValidation(current) && !isNoStandValidation(current) {
		return nil
	}

	if !wrongSquawkPresentForStrip(strip) || !squawkValidationApplies(strip) {
		if !isWrongSquawkValidation(current) {
			return nil
		}
		if err := s.stripRepo.ClearValidationStatus(ctx, session, strip.Callsign); err != nil {
			return err
		}
		strip.ValidationStatus = nil
		if publish && s.publisher != nil {
			s.publisher.SendStripUpdate(session, strip.Callsign)
		}
		return nil
	}

	owner := *strip.Owner
	desired := &internalModels.ValidationStatus{
		IssueType:      wrongSquawkValidationIssueType,
		Message:        wrongSquawkValidationMessage(strip),
		OwningPosition: owner,
		Active:         true,
	}

	if isWrongSquawkValidation(current) && current.OwningPosition == owner && current.Message == desired.Message && !forceReactivate {
		desired.Active = current.Active
		desired.ActivationKey = current.ActivationKey
	} else {
		desired.ActivationKey = uuid.New().String()
	}

	if validationStatusEquals(current, desired) {
		return nil
	}

	if err := s.stripRepo.SetValidationStatus(ctx, session, strip.Callsign, desired); err != nil {
		return err
	}
	strip.ValidationStatus = desired
	if publish && s.publisher != nil {
		s.publisher.SendStripUpdate(session, strip.Callsign)
	}
	return nil
}
