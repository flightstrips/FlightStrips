package services

import (
	"context"
	"strings"

	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/pdc"

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

func pdcCustomValidationApplies(strip *internalModels.Strip) bool {
	if strip == nil || strip.Owner == nil || *strip.Owner == "" || strip.PdcState != string(pdc.StateRequested) {
		return false
	}

	if strings.TrimSpace(pdcRequestRemarksValue(strip.PdcRequestRemarks)) == "" {
		return false
	}

	position, err := config.GetPositionBasedOnFrequency(*strip.Owner)
	if err != nil {
		position, err = config.GetPositionByName(*strip.Owner)
		if err != nil {
			return false
		}
	}

	return position.Section == "DEL" || position.Section == "CLR"
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
	if current != nil && !isPdcCustomValidation(current) && !isPdcInvalidValidation(current) {
		return nil
	}

	remarks := strings.TrimSpace(pdcRequestRemarksValue(strip.PdcRequestRemarks))
	if !pdcCustomValidationApplies(strip) {
		if !isPdcCustomValidation(current) {
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

	if err := s.stripRepo.SetValidationStatus(ctx, session, strip.Callsign, desired); err != nil {
		return err
	}
	strip.ValidationStatus = desired
	if publish && s.publisher != nil {
		s.publisher.SendStripUpdate(session, strip.Callsign)
	}
	return nil
}

func (s *StripService) ReevaluatePdcCustomValidationForStrip(ctx context.Context, session int32, strip *internalModels.Strip, publish bool, forceReactivate bool) error {
	return s.applyPdcCustomValidation(ctx, session, strip, publish, forceReactivate)
}

func (s *StripService) ReevaluatePdcCustomValidation(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	return s.applyPdcCustomValidation(ctx, session, strip, publish, forceReactivate)
}

func (s *StripService) ReevaluatePdcRequestValidationsForStrip(ctx context.Context, session int32, strip *internalModels.Strip, activeDepartureRunways []string, publish bool, forceReactivate bool) error {
	if err := s.applyPdcInvalidValidation(ctx, session, strip, activeDepartureRunways, publish, forceReactivate); err != nil {
		return err
	}

	return s.applyPdcCustomValidation(ctx, session, strip, publish, forceReactivate)
}

func (s *StripService) ReevaluatePdcRequestValidations(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
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

	return s.ReevaluatePdcRequestValidationsForStrip(ctx, session, strip, sessionData.ActiveRunways.DepartureRunways, publish, forceReactivate)
}

func pdcRequestRemarksValue(remarks *string) string {
	if remarks == nil {
		return ""
	}

	return *remarks
}
