package services

import (
	"context"
	"strings"

	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/shared"
	pkgModels "FlightStrips/pkg/models"

	"github.com/google/uuid"
)

const (
	pdcCustomValidationIssueType   = "CUSTOM PDC"
	pdcCustomValidationActionKind  = "open_dcl_menu"
	pdcCustomValidationActionLabel = "OPEN DCL MENU"
)

func isPdcCustomValidation(status *internalModels.ValidationStatus) bool {
	return status != nil && status.IssueType == pdcCustomValidationIssueType
}

func pdcCustomValidationAction() *internalModels.ValidationAction {
	return &internalModels.ValidationAction{
		Label:      pdcCustomValidationActionLabel,
		ActionKind: pdcCustomValidationActionKind,
	}
}

func pdcRequestValidationAppliesInBay(bay string) bool {
	return bay == shared.BAY_NOT_CLEARED
}

func pdcCustomValidationApplies(strip *internalModels.Strip) bool {
	if strip == nil || strip.PdcState != string(pdc.StateRequested) {
		return false
	}
	if !pdcRequestValidationAppliesInBay(strip.Bay) {
		return false
	}

	if strings.TrimSpace(pdcRequestRemarksValue(strip.PdcRequestRemarks)) == "" {
		return false
	}

	return true
}

func pdcValidationOwningPosition(strip *internalModels.Strip) string {
	if strip == nil || strip.Owner == nil {
		return ""
	}

	return strings.TrimSpace(*strip.Owner)
}

func pdcCustomValidationMessage(remarks string) string {
	normalizedRemarks := strings.TrimSpace(strings.ReplaceAll(remarks, "\r\n", "\n"))
	lines := []string{
		"Pilot requested PDC with free-text remarks that require manual handling.",
		"NITOS remarks:",
		normalizedRemarks,
		"Open DCL menu to review the request and handle the clearance manually.",
	}
	return strings.Join(lines, "\n")
}

func (s *StripService) applyPdcCustomValidation(ctx context.Context, session int32, strip *internalModels.Strip, publish bool, forceReactivate bool) error {
	if strip == nil {
		return nil
	}

	current := strip.ValidationStatus
	if validationCandidateIsInhibited(current, pdcCustomValidationIssueType) {
		return nil
	}

	remarks := strings.TrimSpace(pdcRequestRemarksValue(strip.PdcRequestRemarks))
	if !pdcCustomValidationApplies(strip) {
		if !isPdcCustomValidation(current) {
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

	owner := pdcValidationOwningPosition(strip)
	desired := &internalModels.ValidationStatus{
		IssueType:      pdcCustomValidationIssueType,
		Message:        pdcCustomValidationMessage(remarks),
		OwningPosition: owner,
		Active:         true,
		CustomAction:   pdcCustomValidationAction(),
	}

	if isPdcCustomValidation(current) && current.OwningPosition == owner && current.Message == desired.Message && !forceReactivate {
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

func (s *StripService) ReevaluatePdcCustomValidationForStrip(ctx context.Context, session int32, strip *internalModels.Strip, publish bool, forceReactivate bool) error {
	sessionData, err := s.getCachedSession(ctx, session)
	if err != nil {
		return err
	}
	var activeDepartureRunways []string
	if sessionData != nil {
		activeDepartureRunways = sessionData.ActiveRunways.DepartureRunways
	}
	return s.ReevaluatePdcRequestValidationsForStrip(ctx, session, strip, activeDepartureRunways, publish, forceReactivate)
}

func (s *StripService) ReevaluatePdcCustomValidation(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
	strip, available, err := s.getCachedStrip(ctx, session, callsign)
	if err != nil {
		return err
	}
	if !available {
		return nil
	}
	return s.ReevaluatePdcCustomValidationForStrip(ctx, session, strip, publish, forceReactivate)
}

func (s *StripService) ReevaluatePdcRequestValidationsForStrip(ctx context.Context, session int32, strip *internalModels.Strip, activeDepartureRunways []string, publish bool, forceReactivate bool) error {
	previousIssueType := ""
	if strip != nil && strip.ValidationStatus != nil {
		previousIssueType = strip.ValidationStatus.IssueType
	}
	if err := s.applyPdcRequestValidationsForStrip(ctx, session, strip, activeDepartureRunways, publish, forceReactivate); err != nil {
		return err
	}

	// PDC validations have higher precedence than the ordinary departure
	// validations. Once the last PDC issue clears, immediately restore any
	// lower-priority issue that is still applicable.
	if strip != nil && strip.ValidationStatus == nil &&
		(previousIssueType == pdcInvalidValidationIssueType || previousIssueType == pdcCustomValidationIssueType) {
		return s.reevaluateDepartureValidation(ctx, session, strip.Callsign, publish, false)
	}

	return nil
}

func (s *StripService) applyPdcRequestValidationsForStrip(ctx context.Context, session int32, strip *internalModels.Strip, activeDepartureRunways []string, publish bool, forceReactivate bool) error {

	sessionData, err := s.getCachedSession(ctx, session)
	if err != nil {
		return err
	}
	var availableSids pkgModels.AvailableSids
	if sessionData != nil {
		availableSids = sessionData.AvailableSids
	}

	if err := s.applyPdcInvalidValidation(ctx, session, strip, activeDepartureRunways, availableSids, publish, forceReactivate); err != nil {
		return err
	}

	if err := s.applyPdcCustomValidation(ctx, session, strip, publish, forceReactivate); err != nil {
		return err
	}
	return nil
}

// applyPdcValidationBeforeLowerDepartureValidations restores the PDC tier after
// a higher-priority issue (for example duplicate squawk) disappears. It uses
// the non-recursive PDC helper because the caller continues through the lower
// departure tiers itself.
func (s *StripService) applyPdcValidationBeforeLowerDepartureValidations(ctx context.Context, session int32, strip *internalModels.Strip, publish bool, forceReactivate bool) (bool, error) {
	sessionData, err := s.getCachedSession(ctx, session)
	if err != nil {
		return false, err
	}
	var activeDepartureRunways []string
	if sessionData != nil {
		activeDepartureRunways = sessionData.ActiveRunways.DepartureRunways
	}
	if err := s.applyPdcRequestValidationsForStrip(ctx, session, strip, activeDepartureRunways, publish, forceReactivate); err != nil {
		return false, err
	}
	return isPdcInvalidValidation(strip.ValidationStatus) || isPdcCustomValidation(strip.ValidationStatus), nil
}

func (s *StripService) ReevaluatePdcRequestValidations(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
	sessionRepo := s.getSessionRepository()
	if sessionRepo == nil {
		return nil
	}

	strip, available, err := s.getCachedStrip(ctx, session, callsign)
	if err != nil {
		return err
	}
	if !available {
		return nil
	}
	sessionData, err := s.getCachedSession(ctx, session)
	if err != nil {
		return err
	}
	if sessionData == nil {
		return nil
	}

	return s.ReevaluatePdcRequestValidationsForStrip(ctx, session, strip, sessionData.ActiveRunways.DepartureRunways, publish, forceReactivate)
}

func pdcRequestRemarksValue(remarks *string) string {
	if remarks == nil {
		return ""
	}

	return *remarks
}
