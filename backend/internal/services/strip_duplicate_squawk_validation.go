package services

import (
	"bytes"
	"context"
	"strings"

	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/helpers"

	"github.com/google/uuid"
)

const (
	duplicateSquawkValidationIssueType   = "DUPLICATE SQUAWK"
	duplicateSquawkValidationMessage     = "This strip has a duplicate squawk."
	duplicateSquawkValidationActionKind  = "generate_squawk"
	duplicateSquawkValidationActionLabel = "ASSIGN NEW"
)

func isDuplicateSquawkValidation(status *internalModels.ValidationStatus) bool {
	return status != nil && status.IssueType == duplicateSquawkValidationIssueType
}

func isSquawkValidation(status *internalModels.ValidationStatus) bool {
	return isDuplicateSquawkValidation(status) || isWrongSquawkValidation(status)
}

func squawkValidationApplies(strip *internalModels.Strip) bool {
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
		shared.BAY_DEPART:
		return true
	default:
		return false
	}
}

func duplicateSquawkValidationAction() *internalModels.ValidationAction {
	return &internalModels.ValidationAction{
		Label:      duplicateSquawkValidationActionLabel,
		ActionKind: duplicateSquawkValidationActionKind,
	}
}

func validationStatusEquals(current *internalModels.ValidationStatus, desired *internalModels.ValidationStatus) bool {
	if current == nil || desired == nil {
		return current == desired
	}

	if current.IssueType != desired.IssueType ||
		current.Message != desired.Message ||
		current.OwningPosition != desired.OwningPosition ||
		current.Active != desired.Active ||
		current.ActivationKey != desired.ActivationKey {
		return false
	}

	if current.CustomAction == nil || desired.CustomAction == nil {
		return current.CustomAction == desired.CustomAction
	}

	return current.CustomAction.Label == desired.CustomAction.Label &&
		current.CustomAction.ActionKind == desired.CustomAction.ActionKind &&
		bytes.Equal(current.CustomAction.Payload, desired.CustomAction.Payload)
}

func normalizeValidationSquawkCode(code *string) string {
	if code == nil {
		return ""
	}

	normalized := strings.TrimSpace(*code)
	if !helpers.IsValidAssignedSquawk(normalized) {
		return ""
	}

	return normalized
}

func duplicateSquawkCodesForStrip(strip *internalModels.Strip) map[string]struct{} {
	codes := make(map[string]struct{}, 2)
	if strip == nil {
		return codes
	}

	if code := normalizeValidationSquawkCode(strip.AssignedSquawk); code != "" {
		codes[code] = struct{}{}
	}
	if code := normalizeValidationSquawkCode(strip.Squawk); code != "" {
		codes[code] = struct{}{}
	}

	return codes
}

func duplicateSquawkCodeMembership(strips []*internalModels.Strip) map[string]int {
	membership := make(map[string]int)
	for _, strip := range strips {
		for code := range duplicateSquawkCodesForStrip(strip) {
			membership[code]++
		}
	}

	return membership
}

func duplicateSquawkPresentForStrip(strip *internalModels.Strip, membership map[string]int) bool {
	code := normalizeValidationSquawkCode(strip.AssignedSquawk)
	return code != "" && membership[code] > 1
}

func (s *StripService) applyDuplicateSquawkValidationState(ctx context.Context, session int32, strip *internalModels.Strip, duplicatePresent bool, publish bool, forceReactivate bool) error {
	if strip == nil {
		return nil
	}

	current := strip.ValidationStatus
	if current != nil && !isDuplicateSquawkValidation(current) && !isWrongSquawkValidation(current) && !isCtotValidation(current) && !isNoStandValidation(current) {
		return nil
	}

	if !duplicatePresent || !squawkValidationApplies(strip) {
		if !isDuplicateSquawkValidation(current) {
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
		IssueType:      duplicateSquawkValidationIssueType,
		Message:        duplicateSquawkValidationMessage,
		OwningPosition: owner,
		Active:         true,
		CustomAction:   duplicateSquawkValidationAction(),
	}

	if isDuplicateSquawkValidation(current) && current.OwningPosition == owner && !forceReactivate {
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

func (s *StripService) applyDuplicateSquawkValidation(ctx context.Context, session int32, strip *internalModels.Strip, membership map[string]int, publish bool, forceReactivate bool) error {
	return s.applyDuplicateSquawkValidationState(ctx, session, strip, duplicateSquawkPresentForStrip(strip, membership), publish, forceReactivate)
}

func (s *StripService) listStripsForDuplicateSquawkValidation(ctx context.Context, session int32) (strips []*internalModels.Strip, available bool, err error) {
	available = true
	defer func() {
		if recover() != nil {
			strips = nil
			err = nil
			available = false
		}
	}()

	strips, err = s.stripRepo.List(ctx, session)
	return strips, available, err
}

func (s *StripService) getStripForDuplicateSquawkValidation(ctx context.Context, session int32, callsign string) (strip *internalModels.Strip, available bool, err error) {
	available = true
	defer func() {
		if recover() != nil {
			strip = nil
			err = nil
			available = false
		}
	}()

	strip, err = s.stripRepo.GetByCallsign(ctx, session, callsign)
	return strip, available, err
}

func (s *StripService) reevaluateDuplicateSquawkValidation(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
	strips, available, err := s.listStripsForDuplicateSquawkValidation(ctx, session)
	if err != nil {
		return err
	}
	if !available {
		return s.reevaluateStoredDuplicateSquawkValidation(ctx, session, callsign, publish, forceReactivate)
	}

	membership := duplicateSquawkCodeMembership(strips)
	for _, strip := range strips {
		if strip.Callsign == callsign {
			return s.applyDuplicateSquawkValidation(ctx, session, strip, membership, publish, forceReactivate)
		}
	}

	return nil
}

func (s *StripService) reevaluateDuplicateSquawkValidationsForSession(ctx context.Context, session int32, publish bool) error {
	strips, available, err := s.listStripsForDuplicateSquawkValidation(ctx, session)
	if err != nil {
		return err
	}
	if !available {
		return nil
	}

	membership := duplicateSquawkCodeMembership(strips)
	for _, strip := range strips {
		if err := s.applyDuplicateSquawkValidation(ctx, session, strip, membership, publish, false); err != nil {
			return err
		}
	}

	return nil
}

func (s *StripService) reevaluateStoredDuplicateSquawkValidation(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
	strip, available, err := s.getStripForDuplicateSquawkValidation(ctx, session, callsign)
	if err != nil {
		return err
	}
	if !available {
		return nil
	}
	return s.applyDuplicateSquawkValidationState(ctx, session, strip, isDuplicateSquawkValidation(strip.ValidationStatus), publish, forceReactivate)
}

func (s *StripService) reevaluateSquawkValidation(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
	strips, available, err := s.listStripsForDuplicateSquawkValidation(ctx, session)
	if err != nil {
		return err
	}
	if !available {
		return s.reevaluateStoredSquawkValidation(ctx, session, callsign, publish, forceReactivate)
	}

	membership := duplicateSquawkCodeMembership(strips)
	for _, strip := range strips {
		if strip.Callsign != callsign {
			continue
		}
		if err := s.applyDuplicateSquawkValidation(ctx, session, strip, membership, publish, forceReactivate); err != nil {
			return err
		}
		if !duplicateSquawkPresentForStrip(strip, membership) && !isDuplicateSquawkValidation(strip.ValidationStatus) {
			if err := s.applyWrongSquawkValidation(ctx, session, strip, publish, forceReactivate); err != nil {
				return err
			}
			return s.applyCtotValidation(ctx, session, strip, ctotValidationNow(), publish, forceReactivate)
		}

		refreshed, refreshedAvailable, err := s.getStripForDuplicateSquawkValidation(ctx, session, callsign)
		if err != nil {
			return err
		}
		if !refreshedAvailable {
			if duplicateSquawkPresentForStrip(strip, membership) || isDuplicateSquawkValidation(strip.ValidationStatus) {
				return nil
			}
			refreshed = strip
		}

		if err := s.applyWrongSquawkValidation(ctx, session, refreshed, publish, forceReactivate); err != nil {
			return err
		}
		return s.applyCtotValidation(ctx, session, refreshed, ctotValidationNow(), publish, forceReactivate)
	}

	return nil
}

func (s *StripService) reevaluateSquawkValidationsForSession(ctx context.Context, session int32, publish bool) error {
	strips, available, err := s.listStripsForDuplicateSquawkValidation(ctx, session)
	if err != nil {
		return err
	}
	if !available {
		return nil
	}

	membership := duplicateSquawkCodeMembership(strips)
	for _, strip := range strips {
		if err := s.applyDuplicateSquawkValidation(ctx, session, strip, membership, publish, false); err != nil {
			return err
		}
	}

	for _, strip := range strips {
		refreshed, refreshedAvailable, err := s.getStripForDuplicateSquawkValidation(ctx, session, strip.Callsign)
		if err != nil {
			return err
		}
		if !refreshedAvailable {
			if duplicateSquawkPresentForStrip(strip, membership) || isDuplicateSquawkValidation(strip.ValidationStatus) {
				continue
			}
			refreshed = strip
		}

		if err := s.applyWrongSquawkValidation(ctx, session, refreshed, publish, false); err != nil {
			return err
		}
	}

	return s.ReevaluateCtotValidationsForSession(ctx, session, publish)
}

func (s *StripService) reevaluateStripValidationPrecedence(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
	if err := s.reevaluateSquawkValidation(ctx, session, callsign, publish, forceReactivate); err != nil {
		return err
	}

	return s.ReevaluateNoStandValidation(ctx, session, callsign, publish, forceReactivate)
}

func (s *StripService) reevaluateStoredSquawkValidation(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
	strip, available, err := s.getStripForDuplicateSquawkValidation(ctx, session, callsign)
	if err != nil {
		return err
	}
	if !available {
		return nil
	}

	if err := s.applyDuplicateSquawkValidationState(ctx, session, strip, isDuplicateSquawkValidation(strip.ValidationStatus), publish, forceReactivate); err != nil {
		return err
	}
	if !isDuplicateSquawkValidation(strip.ValidationStatus) {
		if err := s.applyWrongSquawkValidation(ctx, session, strip, publish, forceReactivate); err != nil {
			return err
		}
		return s.applyCtotValidation(ctx, session, strip, ctotValidationNow(), publish, forceReactivate)
	}

	refreshed, refreshedAvailable, err := s.getStripForDuplicateSquawkValidation(ctx, session, callsign)
	if err != nil {
		return err
	}
	if !refreshedAvailable {
		if isDuplicateSquawkValidation(strip.ValidationStatus) {
			return nil
		}
		refreshed = strip
	}

	if err := s.applyWrongSquawkValidation(ctx, session, refreshed, publish, forceReactivate); err != nil {
		return err
	}
	return s.applyCtotValidation(ctx, session, refreshed, ctotValidationNow(), publish, forceReactivate)
}

func (s *StripService) setOwnerAndReevaluateDuplicateSquawkValidation(ctx context.Context, session int32, callsign string, owner *string, version int32) (int64, error) {
	count, err := s.stripRepo.SetOwner(ctx, session, callsign, owner, version)
	if err != nil || count != 1 {
		return count, err
	}
	if err := s.ReevaluatePdcRequestValidations(ctx, session, callsign, true, true); err != nil {
		return 0, err
	}
	if err := s.reevaluateStripValidationPrecedence(ctx, session, callsign, true, true); err != nil {
		return 0, err
	}
	return count, nil
}
