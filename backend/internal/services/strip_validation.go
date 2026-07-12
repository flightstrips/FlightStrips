package services

import (
	internalModels "FlightStrips/internal/models"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const standAssignmentValidationIssueType = "STAND ASSIGNMENT"

type validationStripReader interface {
	GetByCallsign(ctx context.Context, session int32, callsign string) (*internalModels.Strip, error)
}

type validationStripNotifier interface {
	SendStripUpdate(session int32, callsign string)
}

type StripValidationService struct {
	stripReader     validationStripReader
	validationStore StripValidationStatusStore
	publisher       validationStripNotifier
}

func NewStripValidationService(stripReader validationStripReader, validationStore StripValidationStatusStore) *StripValidationService {
	service := &StripValidationService{
		stripReader:     missingStripValidationReader{},
		validationStore: missingValidationStatusStore{},
	}
	if stripReader != nil {
		service.stripReader = stripReader
	}
	if validationStore != nil {
		service.validationStore = validationStore
	}
	return service
}

func (s *StripValidationService) SetFrontendHub(publisher validationStripNotifier) {
	s.publisher = publisher
}

// SetValidationStatus sets a new validation status on a strip. A fresh activation key is
// generated so that any outstanding acknowledgement from a previous trigger is ignored.
func (s *StripValidationService) SetValidationStatus(ctx context.Context, session int32, callsign string, status *internalModels.ValidationStatus) error {
	if status == nil {
		return errors.New("SetValidationStatus: status must not be nil; use ClearValidationStatus to remove")
	}
	status.ActivationKey = uuid.New().String()
	if err := s.validationStore.SetValidationStatus(ctx, session, callsign, status); err != nil {
		return err
	}
	s.sendStripUpdate(session, callsign)
	return nil
}

// AcknowledgeValidationStatus marks the validation status as inactive if the activation key
// matches and the requesting position is allowed to acknowledge it. Most validations remain
// owner-scoped; PDC validations are visible and acknowledgeable for all online positions.
// Uses a conditional DB update so concurrent triggers cannot be accidentally dismissed.
func (s *StripValidationService) AcknowledgeValidationStatus(ctx context.Context, session int32, callsign string, activationKey string, requestingPosition string) error {
	strip, err := s.stripReader.GetByCallsign(ctx, session, callsign)
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
	rows, err := s.validationStore.AcknowledgeValidationStatus(ctx, session, callsign, activationKey)
	if err != nil {
		return err
	}
	if rows == 0 {
		// Key mismatch or already acknowledged — not an error, just a no-op.
		return nil
	}
	s.sendStripUpdate(session, callsign)
	return nil
}

// ClearValidationStatus removes the validation status from a strip entirely.
func (s *StripValidationService) ClearValidationStatus(ctx context.Context, session int32, callsign string) error {
	if err := s.validationStore.ClearValidationStatus(ctx, session, callsign); err != nil {
		return err
	}
	s.sendStripUpdate(session, callsign)
	return nil
}

// ReconcileStandAssignmentValidation keeps SAT conflicts in the same durable,
// owner-scoped validation workflow as other strip issues without overwriting a
// higher-priority validation produced by another subsystem.
func (s *StripValidationService) ReconcileStandAssignmentValidation(ctx context.Context, session int32, callsign string, blockedBy []string, conflictReason string) error {
	strip, err := s.stripReader.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}
	current := strip.ValidationStatus
	blocked := len(blockedBy) > 0 || strings.TrimSpace(conflictReason) != ""
	if !blocked {
		if current != nil && current.IssueType == standAssignmentValidationIssueType {
			return s.ClearValidationStatus(ctx, session, callsign)
		}
		return nil
	}
	if current != nil && current.IssueType != standAssignmentValidationIssueType {
		return nil
	}

	message := strings.TrimSpace(conflictReason)
	if message == "" {
		message = "Assigned stand is blocked by " + strings.Join(blockedBy, ", ") + "."
	}
	owner := ""
	if strip.Owner != nil {
		owner = *strip.Owner
	}
	if current != nil && current.Message == message && current.OwningPosition == owner {
		return nil
	}
	return s.SetValidationStatus(ctx, session, callsign, &internalModels.ValidationStatus{
		IssueType:      standAssignmentValidationIssueType,
		Message:        message,
		OwningPosition: owner,
		Active:         true,
		CustomAction: &internalModels.ValidationAction{
			Label:      "REQUEST NEW STAND",
			ActionKind: "assign_stand",
		},
	})
}

// IsValidationBlocking returns true when the strip has an active (unacknowledged) validation.
func (s *StripValidationService) IsValidationBlocking(ctx context.Context, session int32, callsign string) (bool, error) {
	strip, err := s.stripReader.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return false, err
	}
	return strip.IsValidationLocked(), nil
}

func (s *StripValidationService) sendStripUpdate(session int32, callsign string) {
	if s.publisher != nil {
		s.publisher.SendStripUpdate(session, callsign)
	}
}

type missingStripValidationReader struct{}

func (missingStripValidationReader) GetByCallsign(ctx context.Context, session int32, callsign string) (*internalModels.Strip, error) {
	return nil, missingStripValidationDependency("strip reader")
}

type missingValidationStatusStore struct{}

func (missingValidationStatusStore) SetValidationStatus(ctx context.Context, session int32, callsign string, status *internalModels.ValidationStatus) error {
	return missingStripValidationDependency("validation status store")
}

func (missingValidationStatusStore) AcknowledgeValidationStatus(ctx context.Context, session int32, callsign string, activationKey string) (int64, error) {
	return 0, missingStripValidationDependency("validation status store")
}

func (missingValidationStatusStore) ClearValidationStatus(ctx context.Context, session int32, callsign string) error {
	return missingStripValidationDependency("validation status store")
}

func missingStripValidationDependency(name string) error {
	return fmt.Errorf("strip validation service missing %s dependency", name)
}
