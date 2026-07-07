package services

import (
	"context"
	"strings"

	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/shared"

	"github.com/google/uuid"
)

const runwayTypeValidationIssueType = "RUNWAY TYPE"

func isRunwayTypeValidation(status *internalModels.ValidationStatus) bool {
	return status != nil && status.IssueType == runwayTypeValidationIssueType
}

func runwayTypeValidationApplies(strip *internalModels.Strip) bool {
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

	switch position.Section {
	case "GND":
		switch strip.Bay {
		case shared.BAY_CLEARED,
			shared.BAY_PUSH,
			shared.BAY_TAXI,
			shared.BAY_TAXI_LWR,
			shared.BAY_TAXI_TWR,
			shared.BAY_DEPART:
			return true
		}
	case "TWR":
		switch strip.Bay {
		case shared.BAY_TAXI_LWR,
			shared.BAY_TAXI_TWR,
			shared.BAY_DEPART:
			return true
		}
	}

	return false
}

func runwayTypeValidationMessage(fault *pdc.FlightPlanValidationFault) string {
	lines := []string{
		"Aircraft is assigned a runway it is not suitable to depart from:",
	}
	if fault != nil && strings.TrimSpace(fault.Message) != "" {
		lines = append(lines, "• "+fault.Message)
	}
	lines = append(lines, "Open DCL menu to assign a compatible runway or SID.")
	return strings.Join(lines, "\n")
}

func (s *StripService) applyRunwayTypeValidation(ctx context.Context, session int32, strip *internalModels.Strip, publish bool, forceReactivate bool) error {
	if strip == nil {
		return nil
	}

	current := strip.ValidationStatus
	if current != nil && !isRunwayTypeValidation(current) && !isCtotValidation(current) {
		return nil
	}

	fault := pdc.RunwayTypeValidationFault(strip)
	if !runwayTypeValidationApplies(strip) || fault == nil {
		if !isRunwayTypeValidation(current) {
			return nil
		}
		if err := s.validationStore.ClearValidationStatus(ctx, session, strip.Callsign); err != nil {
			return err
		}
		shared.AddDBOperations(ctx, 1)
		strip.ValidationStatus = nil
		s.queueOrSendStripUpdate(ctx, session, strip.Callsign, publish)
		return nil
	}

	owner := *strip.Owner
	desired := &internalModels.ValidationStatus{
		IssueType:      runwayTypeValidationIssueType,
		Message:        runwayTypeValidationMessage(fault),
		OwningPosition: owner,
		Active:         true,
		CustomAction:   pdcInvalidValidationAction(),
	}

	if isRunwayTypeValidation(current) && current.OwningPosition == owner && current.Message == desired.Message && !forceReactivate {
		desired.Active = current.Active
		desired.ActivationKey = current.ActivationKey
	} else {
		desired.ActivationKey = uuid.New().String()
	}

	if validationStatusEquals(current, desired) {
		return nil
	}

	if err := s.validationStore.SetValidationStatus(ctx, session, strip.Callsign, desired); err != nil {
		return err
	}
	shared.AddDBOperations(ctx, 1)
	strip.ValidationStatus = desired
	s.queueOrSendStripUpdate(ctx, session, strip.Callsign, publish)
	return nil
}

func (s *StripService) ReevaluateRunwayTypeValidation(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
	strip, available, err := s.getCachedStrip(ctx, session, callsign)
	if err != nil {
		return err
	}
	if !available {
		return nil
	}
	return s.applyRunwayTypeValidation(ctx, session, strip, publish, forceReactivate)
}
