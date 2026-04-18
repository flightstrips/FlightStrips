package services

import (
	"context"
	"strings"
	"time"

	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"

	"github.com/google/uuid"
)

const (
	ctotValidationIssueType   = "CTOT"
	ctotValidationMessage     = "CTOT is more than 10 minutes in the future."
	ctotValidationActionKind  = "assign_holding_point"
	ctotValidationActionLabel = "ASSIGN HP"
	ctotValidationThreshold   = 10 * time.Minute
	ctotValidationRolloverGap = 12 * time.Hour
)

var ctotValidationNow = func() time.Time {
	return time.Now().UTC()
}

func isCtotValidation(status *internalModels.ValidationStatus) bool {
	return status != nil && status.IssueType == ctotValidationIssueType
}

func ctotValidationAction() *internalModels.ValidationAction {
	return &internalModels.ValidationAction{
		Label:      ctotValidationActionLabel,
		ActionKind: ctotValidationActionKind,
	}
}

func ctotValidationApplies(strip *internalModels.Strip) bool {
	if strip == nil || strip.Owner == nil || *strip.Owner == "" {
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

	switch strip.Bay {
	case shared.BAY_TAXI_LWR, shared.BAY_DEPART:
		return true
	default:
		return false
	}
}

func parseValidationClockUTC(hhmm string, now time.Time) (time.Time, bool) {
	trimmed := strings.TrimSpace(hhmm)
	if len(trimmed) != 4 {
		return time.Time{}, false
	}
	for _, digit := range trimmed {
		if digit < '0' || digit > '9' {
			return time.Time{}, false
		}
	}

	hour := int(trimmed[0]-'0')*10 + int(trimmed[1]-'0')
	minute := int(trimmed[2]-'0')*10 + int(trimmed[3]-'0')
	if hour > 23 || minute > 59 {
		return time.Time{}, false
	}

	candidate := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
	if now.Sub(candidate) > ctotValidationRolloverGap {
		candidate = candidate.Add(24 * time.Hour)
	}

	return candidate, true
}

func ctotMoreThanThresholdAhead(ctot string, now time.Time) bool {
	ctotTime, ok := parseValidationClockUTC(ctot, now.UTC())
	if !ok {
		return false
	}

	return ctotTime.Sub(now.UTC()) > ctotValidationThreshold
}

func (s *StripService) applyCtotValidation(ctx context.Context, session int32, strip *internalModels.Strip, now time.Time, publish bool, forceReactivate bool) error {
	if strip == nil {
		return nil
	}

	current := strip.ValidationStatus
	if current != nil && !isCtotValidation(current) {
		return nil
	}

	ctot := ""
	if effective := strip.EffectiveCtot(); effective != nil {
		ctot = *effective
	}

	if !ctotValidationApplies(strip) || !ctotMoreThanThresholdAhead(ctot, now) {
		if !isCtotValidation(current) {
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
		IssueType:      ctotValidationIssueType,
		Message:        ctotValidationMessage,
		OwningPosition: owner,
		Active:         true,
		CustomAction:   ctotValidationAction(),
	}

	if isCtotValidation(current) && current.OwningPosition == owner && !forceReactivate {
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

func (s *StripService) ReevaluateCtotValidation(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	return s.applyCtotValidation(ctx, session, strip, ctotValidationNow(), publish, forceReactivate)
}

func (s *StripService) ReevaluateCtotValidationsForSession(ctx context.Context, session int32, publish bool) error {
	strips, err := s.stripRepo.List(ctx, session)
	if err != nil {
		return err
	}

	now := ctotValidationNow()
	for _, strip := range strips {
		if err := s.applyCtotValidation(ctx, session, strip, now, publish, false); err != nil {
			return err
		}
	}

	return nil
}
