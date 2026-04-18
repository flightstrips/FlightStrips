package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"

	"github.com/google/uuid"
)

const (
	taxiwayTypeValidationIssueType   = "TAXIWAY TYPE"
	taxiwayTypeValidationActionKind  = "assign_holding_point"
	taxiwayTypeValidationActionLabel = "ASSIGN HS"
)

type taxiwayTypeValidationMatch struct {
	releasePoint string
	reasonKind   string
	reasonValue  string
}

func isTaxiwayTypeValidation(status *internalModels.ValidationStatus) bool {
	return status != nil && status.IssueType == taxiwayTypeValidationIssueType
}

func taxiwayTypeValidationAction() *internalModels.ValidationAction {
	return &internalModels.ValidationAction{
		Label:      taxiwayTypeValidationActionLabel,
		ActionKind: taxiwayTypeValidationActionKind,
	}
}

func normalizeTaxiwayTypeValidationString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(*value))
}

func taxiwayTypeValidationApplies(strip *internalModels.Strip) bool {
	if strip == nil || strip.Owner == nil || *strip.Owner == "" || normalizeTaxiwayTypeValidationString(strip.ReleasePoint) == "" {
		return false
	}

	switch strip.Bay {
	case shared.BAY_PUSH, shared.BAY_TAXI, shared.BAY_TAXI_LWR, shared.BAY_TAXI_TWR, shared.BAY_DEPART:
		return true
	default:
		return false
	}
}

func taxiwayTypeValidationScopeForStrip(strip *internalModels.Strip) (config.TaxiwayTypeValidationScopeConfig, bool) {
	if strip == nil || strip.Owner == nil || *strip.Owner == "" {
		return config.TaxiwayTypeValidationScopeConfig{}, false
	}

	position, err := config.GetPositionBasedOnFrequency(*strip.Owner)
	if err != nil {
		position, err = config.GetPositionByName(*strip.Owner)
		if err != nil {
			return config.TaxiwayTypeValidationScopeConfig{}, false
		}
	}

	return config.GetTaxiwayTypeValidationScopeForPosition(position.Name)
}

func taxiwayTypeValidationMatchForStrip(strip *internalModels.Strip) (*taxiwayTypeValidationMatch, bool) {
	if !taxiwayTypeValidationApplies(strip) {
		return nil, false
	}

	scope, ok := taxiwayTypeValidationScopeForStrip(strip)
	if !ok {
		return nil, false
	}

	releasePoint := normalizeTaxiwayTypeValidationString(strip.ReleasePoint)
	aircraftType := normalizeTaxiwayTypeValidationString(strip.AircraftType)
	if releasePoints, found := scope.AircraftTypes[aircraftType]; found && containsTaxiwayTypeValidationValue(releasePoints, releasePoint) {
		return &taxiwayTypeValidationMatch{
			releasePoint: releasePoint,
			reasonKind:   "aircraft type",
			reasonValue:  aircraftType,
		}, true
	}

	aircraftCategory := normalizeTaxiwayTypeValidationString(strip.AircraftCategory)
	if releasePoints, found := scope.Categories[aircraftCategory]; found && containsTaxiwayTypeValidationValue(releasePoints, releasePoint) {
		return &taxiwayTypeValidationMatch{
			releasePoint: releasePoint,
			reasonKind:   "aircraft category",
			reasonValue:  aircraftCategory,
		}, true
	}

	return nil, false
}

func containsTaxiwayTypeValidationValue(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

func taxiwayTypeValidationMessage(match *taxiwayTypeValidationMatch) string {
	if match == nil {
		return "Assigned holding point is incompatible with the current aircraft."
	}

	return fmt.Sprintf("Assigned holding point %s is incompatible with %s %s.", match.releasePoint, match.reasonKind, match.reasonValue)
}

func (s *StripService) applyTaxiwayTypeValidation(ctx context.Context, session int32, strip *internalModels.Strip, publish bool, forceReactivate bool) error {
	if strip == nil {
		return nil
	}

	current := strip.ValidationStatus
	if current != nil && !isTaxiwayTypeValidation(current) && !isCtotValidation(current) {
		return nil
	}

	match, present := taxiwayTypeValidationMatchForStrip(strip)
	if !present {
		if !isTaxiwayTypeValidation(current) {
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
		IssueType:      taxiwayTypeValidationIssueType,
		Message:        taxiwayTypeValidationMessage(match),
		OwningPosition: owner,
		Active:         true,
		CustomAction:   taxiwayTypeValidationAction(),
	}

	if isTaxiwayTypeValidation(current) && current.OwningPosition == owner && current.Message == desired.Message && !forceReactivate {
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

func (s *StripService) ReevaluateTaxiwayTypeValidation(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	return s.applyTaxiwayTypeValidation(ctx, session, strip, publish, forceReactivate)
}

func (s *StripService) applyTaxiwayTypeAndCtotValidation(ctx context.Context, session int32, strip *internalModels.Strip, now time.Time, publish bool, forceReactivate bool) error {
	if err := s.applyTaxiwayTypeValidation(ctx, session, strip, publish, forceReactivate); err != nil {
		return err
	}
	if isTaxiwayTypeValidation(strip.ValidationStatus) {
		return nil
	}

	refreshed, available, err := s.getStripForDuplicateSquawkValidation(ctx, session, strip.Callsign)
	if err != nil {
		return err
	}
	if !available {
		refreshed = strip
	}

	return s.applyCtotValidation(ctx, session, refreshed, now, publish, forceReactivate)
}

func (s *StripService) reevaluateTaxiwayTypeAndCtotValidation(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	return s.applyTaxiwayTypeAndCtotValidation(ctx, session, strip, ctotValidationNow(), publish, forceReactivate)
}
