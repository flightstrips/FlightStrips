package services

import (
	"context"
	"strings"

	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/pdc"

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

func (s *StripService) applyPdcInvalidValidation(ctx context.Context, session int32, strip *internalModels.Strip, activeDepartureRunways []string, publish bool, forceReactivate bool) error {
	if strip == nil {
		return nil
	}

	current := strip.ValidationStatus
	if current != nil && !isPdcInvalidValidation(current) && !isPdcCustomValidation(current) {
		return nil
	}

	faults := pdc.PDCStripValidationFaults(strip, activeDepartureRunways)
	if !pdcInvalidValidationApplies(strip) || len(faults) == 0 {
		if !isPdcInvalidValidation(current) {
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

	if err := s.stripRepo.SetValidationStatus(ctx, session, strip.Callsign, desired); err != nil {
		return err
	}
	strip.ValidationStatus = desired
	if publish && s.publisher != nil {
		s.publisher.SendStripUpdate(session, strip.Callsign)
	}
	return nil
}

func (s *StripService) ReevaluatePdcInvalidValidationForStrip(ctx context.Context, session int32, strip *internalModels.Strip, activeDepartureRunways []string, publish bool, forceReactivate bool) error {
	return s.applyPdcInvalidValidation(ctx, session, strip, activeDepartureRunways, publish, forceReactivate)
}

func (s *StripService) ReevaluatePdcInvalidValidation(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
	sessionRepo := s.getSessionRepository()
	if sessionRepo == nil {
		return nil
	}
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}
	sessionData, err := sessionRepo.GetByID(ctx, session)
	if err != nil {
		return err
	}

	return s.applyPdcInvalidValidation(ctx, session, strip, sessionData.ActiveRunways.DepartureRunways, publish, forceReactivate)
}
