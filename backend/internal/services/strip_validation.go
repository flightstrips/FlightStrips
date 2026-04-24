package services

import (
	internalModels "FlightStrips/internal/models"
	"context"
	"errors"

	"github.com/google/uuid"
)

// SetValidationStatus sets a new validation status on a strip. A fresh activation key is
// generated so that any outstanding acknowledgement from a previous trigger is ignored.
func (s *StripService) SetValidationStatus(ctx context.Context, session int32, callsign string, status *internalModels.ValidationStatus) error {
	if status == nil {
		return errors.New("SetValidationStatus: status must not be nil; use ClearValidationStatus to remove")
	}
	status.ActivationKey = uuid.New().String()
	if err := s.stripRepo.SetValidationStatus(ctx, session, callsign, status); err != nil {
		return err
	}
	s.publisher.SendStripUpdate(session, callsign)
	return nil
}

// AcknowledgeValidationStatus marks the validation status as inactive if the activation key
// matches and the requesting position is allowed to acknowledge it. Most validations remain
// owner-scoped; PDC validations are visible and acknowledgeable for all online positions.
// Uses a conditional DB update so concurrent triggers cannot be accidentally dismissed.
func (s *StripService) AcknowledgeValidationStatus(ctx context.Context, session int32, callsign string, activationKey string, requestingPosition string) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}
	if strip.ValidationStatus == nil {
		return nil
	}
	if strip.ValidationStatus.OwningPosition != requestingPosition &&
		!isPdcInvalidValidation(strip.ValidationStatus) &&
		!isPdcCustomValidation(strip.ValidationStatus) {
		return errors.New("acknowledge_validation_status: requesting position is not the owning position")
	}
	rows, err := s.stripRepo.AcknowledgeValidationStatus(ctx, session, callsign, activationKey)
	if err != nil {
		return err
	}
	if rows == 0 {
		// Key mismatch or already acknowledged — not an error, just a no-op.
		return nil
	}
	s.publisher.SendStripUpdate(session, callsign)
	return nil
}

// ClearValidationStatus removes the validation status from a strip entirely.
func (s *StripService) ClearValidationStatus(ctx context.Context, session int32, callsign string) error {
	if err := s.stripRepo.ClearValidationStatus(ctx, session, callsign); err != nil {
		return err
	}
	s.publisher.SendStripUpdate(session, callsign)
	return nil
}

// IsValidationBlocking returns true when the strip has an active (unacknowledged) validation.
func (s *StripService) IsValidationBlocking(ctx context.Context, session int32, callsign string) (bool, error) {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return false, err
	}
	return strip.IsValidationLocked(), nil
}
