package services

import (
	"context"
	"errors"
	"strings"

	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	noStandValidationIssueType   = "NO STAND"
	noStandValidationMessage     = "This strip has no stand assigned."
	noStandValidationActionKind  = "assign_stand"
	noStandValidationActionLabel = "ASSIGN NEW"
)

func isNoStandValidation(status *internalModels.ValidationStatus) bool {
	return status != nil && status.IssueType == noStandValidationIssueType
}

func noStandValidationAction() *internalModels.ValidationAction {
	return &internalModels.ValidationAction{
		Label:      noStandValidationActionLabel,
		ActionKind: noStandValidationActionKind,
	}
}

func hasAssignedStand(stand *string) bool {
	if stand == nil {
		return false
	}
	return strings.TrimSpace(*stand) != ""
}

func noStandValidationApplies(strip *internalModels.Strip) bool {
	if strip == nil || strip.Owner == nil || *strip.Owner == "" || hasAssignedStand(strip.Stand) {
		return false
	}

	position, err := config.GetPositionBasedOnFrequency(*strip.Owner)
	if err != nil {
		position, err = config.GetPositionByName(*strip.Owner)
		if err != nil {
			return false
		}
	}
	if position.Section != "GND" && position.Section != "TWR" {
		return false
	}

	switch strip.Bay {
	case shared.BAY_NOT_CLEARED,
		shared.BAY_CLEARED,
		shared.BAY_PUSH,
		shared.BAY_TAXI,
		shared.BAY_TAXI_LWR,
		shared.BAY_TAXI_TWR,
		shared.BAY_TWY_ARR,
		shared.BAY_STAND:
		return true
	default:
		return false
	}
}

func (s *StripService) applyNoStandValidation(ctx context.Context, session int32, strip *internalModels.Strip, publish bool, forceReactivate bool) error {
	if strip == nil {
		return nil
	}

	current := strip.ValidationStatus
	if current != nil && !isNoStandValidation(current) {
		return nil
	}

	if !noStandValidationApplies(strip) {
		if !isNoStandValidation(current) {
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
		IssueType:      noStandValidationIssueType,
		Message:        noStandValidationMessage,
		OwningPosition: owner,
		Active:         true,
		CustomAction:   noStandValidationAction(),
	}

	if isNoStandValidation(current) && current.OwningPosition == owner && !forceReactivate {
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

func (s *StripService) ReevaluateNoStandValidation(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
	strip, available, err := s.getStripForNoStandValidation(ctx, session, callsign)
	if err != nil {
		return err
	}
	if !available {
		return nil
	}

	return s.applyNoStandValidation(ctx, session, strip, publish, forceReactivate)
}

func (s *StripService) ReevaluateNoStandValidationsForSession(ctx context.Context, session int32, publish bool) error {
	strips, available, err := s.listStripsForDuplicateSquawkValidation(ctx, session)
	if err != nil {
		return err
	}
	if !available {
		return nil
	}

	for _, strip := range strips {
		if err := s.applyNoStandValidation(ctx, session, strip, publish, false); err != nil {
			return err
		}
	}

	return nil
}

func (s *StripService) getStripForNoStandValidation(ctx context.Context, session int32, callsign string) (strip *internalModels.Strip, available bool, err error) {
	available = true
	defer func() {
		if recover() != nil {
			strip = nil
			err = nil
			available = false
		}
	}()

	strip, err = s.stripRepo.GetByCallsign(ctx, session, callsign)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	return strip, available, err
}
