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
	pdcInvalidValidationIssueType   = "PDC INVALID"
	pdcInvalidValidationActionKind  = "open_dcl_menu"
	pdcInvalidValidationActionLabel = "OPEN DCL MENU"
)

func isPdcInvalidValidation(status *internalModels.ValidationStatus) bool {
	return status != nil && status.IssueType == pdcInvalidValidationIssueType
}

func pdcInvalidValidationAction() *internalModels.ValidationAction {
	return &internalModels.ValidationAction{
		Label:      pdcInvalidValidationActionLabel,
		ActionKind: pdcInvalidValidationActionKind,
	}
}

func pdcInvalidValidationApplies(strip *internalModels.Strip) bool {
	if strip == nil || strip.PdcState != string(pdc.StateRequestedWithFaults) {
		return false
	}
	if !pdcRequestValidationAppliesInBay(strip.Bay) {
		return false
	}

	return true
}

func pdcInvalidValidationMessage(faults []pdc.FlightPlanValidationFault) string {
	lines := []string{
		"Pilot requested PDC, but the clearance is invalid:",
	}
	for _, fault := range faults {
		lines = append(lines, "• "+fault.Message)
	}
	lines = append(lines, "Open DCL menu to review the request and correct the issue.")
	return strings.Join(lines, "\n")
}

func (s *StripService) applyPdcInvalidValidation(ctx context.Context, session int32, strip *internalModels.Strip, activeDepartureRunways []string, availableSids pkgModels.AvailableSids, publish bool, forceReactivate bool) error {
	if strip == nil {
		return nil
	}

	current := strip.ValidationStatus
	if current != nil && !isPdcInvalidValidation(current) && !isPdcCustomValidation(current) {
		return nil
	}

	faults := pdc.PDCStripValidationFaults(strip, activeDepartureRunways, availableSids)
	if !pdcInvalidValidationApplies(strip) || len(faults) == 0 {
		if !isPdcInvalidValidation(current) {
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
		IssueType:      pdcInvalidValidationIssueType,
		Message:        pdcInvalidValidationMessage(faults),
		OwningPosition: owner,
		Active:         true,
		CustomAction:   pdcInvalidValidationAction(),
	}

	if isPdcInvalidValidation(current) && current.OwningPosition == owner && current.Message == desired.Message && !forceReactivate {
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

func (s *StripService) ReevaluatePdcInvalidValidationForStrip(ctx context.Context, session int32, strip *internalModels.Strip, activeDepartureRunways []string, publish bool, forceReactivate bool) error {
	sessionData, err := s.getCachedSession(ctx, session)
	if err != nil {
		return err
	}
	var availableSids pkgModels.AvailableSids
	if sessionData != nil {
		availableSids = sessionData.AvailableSids
	}
	return s.applyPdcInvalidValidation(ctx, session, strip, activeDepartureRunways, availableSids, publish, forceReactivate)
}

func (s *StripService) ReevaluatePdcInvalidValidation(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
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

	return s.applyPdcInvalidValidation(ctx, session, strip, sessionData.ActiveRunways.DepartureRunways, sessionData.AvailableSids, publish, forceReactivate)
}
