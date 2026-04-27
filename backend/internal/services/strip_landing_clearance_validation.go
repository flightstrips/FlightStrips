package services

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"

	"github.com/google/uuid"
)

const (
	landingClearanceValidationIssueType   = "LANDING CLEARANCE"
	landingClearanceValidationMessage     = "Aircraft is not marked as cleared to land."
	landingClearanceValidationActionKind  = "runway_clearance"
	landingClearanceValidationActionLabel = "CLEAR TO LAND"
	landingClearanceValidationDelay       = 15 * time.Second
	// Temporary kill-switch while landing validation activation is known-bad.
	landingClearanceValidationCreationEnabled = false
)

var landingClearanceValidationAfterFunc = func(delay time.Duration, fn func()) {
	time.AfterFunc(delay, fn)
}

func isLandingClearanceValidation(status *internalModels.ValidationStatus) bool {
	return status != nil && status.IssueType == landingClearanceValidationIssueType
}

func landingClearanceValidationAction() *internalModels.ValidationAction {
	return &internalModels.ValidationAction{
		Label:      landingClearanceValidationActionLabel,
		ActionKind: landingClearanceValidationActionKind,
	}
}

func landingClearanceValidationRelevantBay(bay string) bool {
	switch bay {
	case shared.BAY_FINAL, shared.BAY_RWY_ARR, shared.BAY_TWY_ARR:
		return true
	default:
		return false
	}
}

func landingClearanceValidationCandidate(strips []*internalModels.Strip) *internalModels.Strip {
	var candidate *internalModels.Strip
	for _, strip := range strips {
		if strip == nil {
			continue
		}
		switch strip.Bay {
		case shared.BAY_RWY_ARR, shared.BAY_FINAL:
		default:
			continue
		}
		if strip.RunwayCleared {
			continue
		}
		if candidate == nil || landingClearanceCandidateLess(candidate, strip) {
			candidate = strip
		}
	}
	return candidate
}

func landingClearanceCandidateLess(current *internalModels.Strip, next *internalModels.Strip) bool {
	if landingClearanceBayPriority(current.Bay) != landingClearanceBayPriority(next.Bay) {
		return landingClearanceBayPriority(current.Bay) < landingClearanceBayPriority(next.Bay)
	}
	return landingClearanceSequence(current) < landingClearanceSequence(next)
}

func landingClearanceBayPriority(bay string) int {
	switch bay {
	case shared.BAY_RWY_ARR:
		return 2
	case shared.BAY_FINAL:
		return 1
	default:
		return 0
	}
}

func landingClearanceSequence(strip *internalModels.Strip) int32 {
	if strip == nil || strip.Sequence == nil {
		return 0
	}
	return *strip.Sequence
}

func landingClearanceValidationApplies(strip *internalModels.Strip, activeArrivalRunways []string) bool {
	if strip == nil || strip.Owner == nil || strings.TrimSpace(*strip.Owner) == "" {
		return false
	}

	if strip.RunwayCleared {
		return false
	}

	switch strip.Bay {
	case shared.BAY_FINAL, shared.BAY_RWY_ARR:
	default:
		return false
	}

	position, err := config.GetPositionBasedOnFrequency(*strip.Owner)
	if err != nil {
		position, err = config.GetPositionByName(*strip.Owner)
		if err != nil {
			return false
		}
	}
	if position.Section != "TWR" {
		return false
	}

	return config.IsArrivalTowerOwner(position.Name, activeArrivalRunways)
}

func landingClearanceValidationDesiredStatus(
	strip *internalModels.Strip,
	current *internalModels.ValidationStatus,
	forceReactivate bool,
) *internalModels.ValidationStatus {
	desired := &internalModels.ValidationStatus{
		IssueType:      landingClearanceValidationIssueType,
		Message:        landingClearanceValidationMessage,
		OwningPosition: *strip.Owner,
		Active:         true,
		CustomAction:   landingClearanceValidationAction(),
	}

	if isLandingClearanceValidation(current) && !forceReactivate {
		desired.Active = current.Active
		desired.ActivationKey = current.ActivationKey
	} else {
		desired.ActivationKey = uuid.New().String()
	}

	return desired
}

func (s *StripService) activeArrivalRunways(ctx context.Context, session int32) []string {
	sessionRepo := s.getSessionRepository()
	if sessionRepo == nil {
		return nil
	}

	sessionData, err := sessionRepo.GetByID(ctx, session)
	if err != nil || sessionData == nil {
		return nil
	}

	return sessionData.ActiveRunways.ArrivalRunways
}

func (s *StripService) listStripsForLandingClearanceValidation(ctx context.Context, session int32) (strips []*internalModels.Strip, available bool, err error) {
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

func (s *StripService) ReevaluateLandingClearanceValidation(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
	return s.ReevaluateLandingClearanceValidationsForSession(ctx, session, publish, forceReactivate)
}

func (s *StripService) ReevaluateLandingClearanceValidationsForSession(ctx context.Context, session int32, publish bool, forceReactivate bool) error {
	strips, available, err := s.listStripsForLandingClearanceValidation(ctx, session)
	if err != nil {
		return err
	}
	if !available {
		return nil
	}

	activeArrivalRunways := s.activeArrivalRunways(ctx, session)
	candidate := landingClearanceValidationCandidate(strips)

	for _, strip := range strips {
		if !isLandingClearanceValidation(strip.ValidationStatus) {
			continue
		}
		if candidate != nil && strip.Callsign == candidate.Callsign {
			continue
		}
		if err := s.stripRepo.ClearValidationStatus(ctx, session, strip.Callsign); err != nil {
			return err
		}
		strip.ValidationStatus = nil
		if publish && s.publisher != nil {
			s.publisher.SendStripUpdate(session, strip.Callsign)
		}
	}

	if candidate == nil || !landingClearanceValidationApplies(candidate, activeArrivalRunways) {
		if candidate != nil && isLandingClearanceValidation(candidate.ValidationStatus) {
			if err := s.stripRepo.ClearValidationStatus(ctx, session, candidate.Callsign); err != nil {
				return err
			}
			candidate.ValidationStatus = nil
			if publish && s.publisher != nil {
				s.publisher.SendStripUpdate(session, candidate.Callsign)
			}
		}
		return nil
	}

	if !landingClearanceValidationCreationEnabled {
		return nil
	}

	if !forceReactivate && !isLandingClearanceValidation(candidate.ValidationStatus) {
		return nil
	}

	desired := landingClearanceValidationDesiredStatus(candidate, candidate.ValidationStatus, forceReactivate)
	if validationStatusEquals(candidate.ValidationStatus, desired) {
		return nil
	}

	if err := s.stripRepo.SetValidationStatus(ctx, session, candidate.Callsign, desired); err != nil {
		return err
	}
	candidate.ValidationStatus = desired
	if publish && s.publisher != nil {
		s.publisher.SendStripUpdate(session, candidate.Callsign)
	}
	return nil
}

func (s *StripService) scheduleLandingClearanceValidation(session int32) {
	if !landingClearanceValidationCreationEnabled {
		return
	}

	landingClearanceValidationAfterFunc(landingClearanceValidationDelay, func() {
		ctx := context.Background()
		if err := s.ReevaluateLandingClearanceValidationsForSession(ctx, session, true, true); err != nil {
			slog.ErrorContext(ctx, "Landing clearance validation timer failed",
				slog.Int("session", int(session)),
				slog.Any("error", err))
		}
	})
}
