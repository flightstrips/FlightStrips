package services

import (
	"FlightStrips/internal/dependencies"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/shared"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

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

type StripValidationDependencies struct {
	Strips    validationStripReader
	Statuses  StripValidationStatusStore
	Publisher validationStripNotifier
}

func NewStripValidationService(deps StripValidationDependencies) (*StripValidationService, error) {
	required := []struct {
		name  string
		value any
	}{
		{"strip reader", deps.Strips},
		{"validation status store", deps.Statuses},
		{"strip update publisher", deps.Publisher},
	}
	for _, dependency := range required {
		if dependencies.IsNil(dependency.value) {
			return nil, errors.New("strip validation service requires " + dependency.name)
		}
	}
	return &StripValidationService{
		stripReader:     deps.Strips,
		validationStore: deps.Statuses,
		publisher:       deps.Publisher,
	}, nil
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
	s.sendStripUpdate(ctx, session, callsign)
	return nil
}

// AcknowledgeValidationStatus marks the validation status as inactive if the activation key
// matches and the requesting position is allowed to acknowledge it. Most validations remain
// owner-scoped; PDC validations are visible and acknowledgeable for all online positions.
// Uses a conditional DB update so concurrent triggers cannot be accidentally dismissed.
func (s *StripValidationService) AcknowledgeValidationStatus(ctx context.Context, session int32, callsign string, activationKey string, requestingPosition string) error {
	strip, err := s.stripReader.GetByCallsign(ctx, session, callsign)
	if errors.Is(err, pgx.ErrNoRows) {
		// The acknowledgement can race with strip/session removal.
		return nil
	}
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
	if strip.ValidationStatus.IssueType == pushbackTsatValidationIssueType {
		slog.InfoContext(ctx, "Pushback timing override acknowledged",
			slog.Int("session", int(session)), slog.String("callsign", callsign),
			slog.String("position", requestingPosition), slog.String("activation_key", activationKey),
			slog.String("timing_context", strip.ValidationStatus.ContextKey),
			slog.String("validation_message", strip.ValidationStatus.Message))
	}
	s.sendStripUpdate(ctx, session, callsign)
	return nil
}

// ClearValidationStatus removes the validation status from a strip entirely.
func (s *StripValidationService) ClearValidationStatus(ctx context.Context, session int32, callsign string) error {
	if err := s.validationStore.ClearValidationStatus(ctx, session, callsign); err != nil {
		return err
	}
	s.sendStripUpdate(ctx, session, callsign)
	return nil
}

// ReconcileStandAssignmentValidation keeps arrival SAT conflicts in the same
// durable validation workflow as other strip issues without overwriting a
// higher-priority validation produced by another subsystem. Departure
// assignments retain their SAT conflict metadata but do not produce strip
// validations.
func (s *StripValidationService) ReconcileStandAssignmentValidation(ctx context.Context, session int32, callsign string, direction string, blockedBy []string, conflictReason string, assignments internalModels.StandValidationAssignments) error {
	strip, err := s.stripReader.GetByCallsign(ctx, session, callsign)
	if errors.Is(err, pgx.ErrNoRows) {
		// SAT reconciliation can outlive a concurrently removed strip.
		return nil
	}
	if err != nil {
		return err
	}
	current := strip.ValidationStatus
	update := internalModels.StandValidationUpdate{Assignments: assignments, Owner: strip.Owner, Previous: current}
	suppress := strings.EqualFold(strings.TrimSpace(direction), string(sat.AssignmentDirectionDeparture)) ||
		isWrongStandConflictReason(conflictReason) ||
		strings.HasPrefix(strings.TrimSpace(conflictReason), observedDepartureConflictPrefix)
	blocked := len(blockedBy) > 0 || strings.TrimSpace(conflictReason) != ""
	if suppress || !blocked {
		if current != nil && current.IssueType == internalModels.ValidationIssueTypeStandAssignment {
			return s.writeStandValidation(ctx, session, callsign, update)
		}
		return nil
	}
	if validationCandidateIsInhibited(current, internalModels.ValidationIssueTypeStandAssignment) {
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
	payload, err := json.Marshal(internalModels.StandAssignmentActionPayload{AssignmentID: assignments.Target.ID, AssignmentVersion: assignments.Target.Version})
	if err != nil {
		return err
	}
	action := &internalModels.ValidationAction{
		Label: "REQUEST NEW STAND", ActionKind: "assign_stand", Payload: payload,
	}
	if current != nil && current.IssueType == internalModels.ValidationIssueTypeStandAssignment &&
		current.Message == message && current.OwningPosition == owner {
		if current.CustomAction != nil && current.CustomAction.ActionKind == action.ActionKind {
			var previous internalModels.StandAssignmentActionPayload
			if json.Unmarshal(current.CustomAction.Payload, &previous) == nil && previous.AssignmentID == assignments.Target.ID && previous.AssignmentVersion == assignments.Target.Version {
				return nil
			}
		}
		// Timing refreshes can bump the assignment version while the blocker
		// stays the same. Refresh the action without reopening the warning.
		next := *current
		next.CustomAction = action
		update.Next = &next
		return s.writeStandValidation(ctx, session, callsign, update)
	}
	update.Next = &internalModels.ValidationStatus{
		IssueType:      internalModels.ValidationIssueTypeStandAssignment,
		Message:        message,
		OwningPosition: owner,
		Active:         true,
		ActivationKey:  uuid.NewString(),
		CustomAction:   action,
	}
	return s.writeStandValidation(ctx, session, callsign, update)
}

func (s *StripValidationService) writeStandValidation(ctx context.Context, session int32, callsign string, update internalModels.StandValidationUpdate) error {
	changed, err := s.validationStore.CompareAndSetStandValidation(ctx, session, callsign, update)
	if err == nil && changed {
		s.sendStripUpdate(ctx, session, callsign)
	}
	return err
}

// IsValidationBlocking returns true when the strip has an active (unacknowledged) validation.
func (s *StripValidationService) IsValidationBlocking(ctx context.Context, session int32, callsign string) (bool, error) {
	strip, err := s.stripReader.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return false, err
	}
	return strip.IsValidationLocked(), nil
}

func (s *StripValidationService) sendStripUpdate(ctx context.Context, session int32, callsign string) {
	shared.PublishStripUpdate(ctx, s.publisher, session, callsign)
}
