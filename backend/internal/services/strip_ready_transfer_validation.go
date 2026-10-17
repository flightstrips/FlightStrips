package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

const readyTransferTsatValidationMessage = "TSAT is outside the permitted start-request transfer window. Acknowledge to override if TSAT is malfunctioning."

func (s *StripService) SetReadyTransferValidation(ctx context.Context, session int32, callsign string, position string) error {
	if s.stripReader == nil || s.validationStore == nil || s.publisher == nil {
		return errors.New("ready-transfer validation is not configured")
	}
	strip, err := s.stripReader.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}
	if validationCandidateIsInhibited(strip.ValidationStatus, models.ValidationIssueTypeReadyTransferTsat) {
		return errors.New("ready-transfer validation is inhibited by a higher-priority validation")
	}

	owner := strings.TrimSpace(position)
	if strip.Owner != nil && strings.TrimSpace(*strip.Owner) != "" {
		owner = strings.TrimSpace(*strip.Owner)
	}
	if owner == "" {
		return errors.New("ready-transfer validation requires an owning position")
	}
	contextKey := ""
	if strip.EffectiveTsat() != nil {
		contextKey = strings.TrimSpace(*strip.EffectiveTsat())
	}
	status := &models.ValidationStatus{
		IssueType:      models.ValidationIssueTypeReadyTransferTsat,
		Message:        readyTransferTsatValidationMessage,
		OwningPosition: owner,
		Active:         true,
		ActivationKey:  uuid.NewString(),
		ContextKey:     contextKey,
	}
	if err := s.validationStore.SetValidationStatus(ctx, session, callsign, status); err != nil {
		return err
	}
	shared.PublishStripUpdate(ctx, s.publisher, session, callsign)
	return nil
}
