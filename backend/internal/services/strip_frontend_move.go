package services

import (
	"FlightStrips/internal/cdm"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

var validFrontendMoveBays = map[string]bool{
	shared.BAY_NOT_CLEARED: true,
	shared.BAY_CLEARED:     true,
	shared.BAY_PUSH:        true,
	shared.BAY_TAXI:        true,
	shared.BAY_TAXI_LWR:    true,
	shared.BAY_TAXI_TWR:    true,
	shared.BAY_DEPART:      true,
	shared.BAY_AIRBORNE:    true,
	shared.BAY_FINAL:       true,
	shared.BAY_RWY_ARR:     true,
	shared.BAY_TWY_ARR:     true,
	shared.BAY_STAND:       true,
	shared.BAY_HIDDEN:      true,
	shared.BAY_ARR_HIDDEN:  true,
	shared.BAY_CONTROLZONE: true,
}

func (s *StripService) MoveFrontendStrip(ctx context.Context, session int32, callsign string, targetBay string, cid string, airport string, clientPosition string, clearance bool, confirmedRemoval bool) error {
	if !validFrontendMoveBays[targetBay] {
		slog.WarnContext(ctx, "MoveFrontendStrip: rejecting move event with invalid bay",
			slog.String("callsign", callsign),
			slog.String("bay", targetBay),
			slog.String("cid", cid),
		)
		return errors.New("invalid bay value: " + targetBay)
	}
	if confirmedRemoval && targetBay != shared.BAY_HIDDEN {
		return errors.New("confirmed removal must target the hidden bay")
	}

	strip, err := s.stripReader.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	// A confirmed EST VACANT/CLEAR FPL operation must remain available for
	// removing an obsolete stand occupant even when another validation currently
	// owns the master-caution slot. Ordinary moves to HIDDEN remain locked.
	if strip.IsValidationLocked() && !confirmedRemoval && (strip.ValidationStatus == nil || strip.ValidationStatus.IssueType != pushbackTsatValidationIssueType || targetBay != shared.BAY_PUSH) {
		return errors.New("strip is locked by an active validation")
	}

	if err := validateFrontendMoveBayTransition(strip, airport, targetBay, clearance); err != nil {
		return err
	}

	if err := s.authorizeFrontendMove(ctx, session, strip, callsign, airport, targetBay, clientPosition, confirmedRemoval); err != nil {
		return err
	}
	if targetBay == shared.BAY_PUSH && strip.Bay != targetBay && strings.EqualFold(strip.Origin, airport) {
		ctx = context.WithValue(ctx, pushbackTransitionContextKey{}, pushbackTransitionContext{source: "frontend", initial: snapshotPushbackTransitionTiming(strip)})
		if err := s.validatePushbackTiming(ctx, session, strip, clientPosition); err != nil {
			return err
		}
		ctx = withValidatedPushback(ctx, session, callsign)
		strip, err = s.stripReader.GetByCallsign(ctx, session, callsign)
		if err != nil {
			return err
		}
	}
	if strip.IsValidationLocked() && !confirmedRemoval {
		return errors.New("strip is locked by an active validation")
	}

	if strip.Bay == targetBay {
		return nil
	}

	previousBay := strip.Bay
	previousCleared := strip.Cleared
	shouldConfirmVoiceClearance := targetBay == shared.BAY_CLEARED &&
		strip.PdcState != "" &&
		strip.PdcState != internalModels.PdcStateNone

	groundState, err := s.applyFrontendMoveState(ctx, session, strip, targetBay, cid, airport)
	if err != nil {
		return err
	}

	if err := s.MoveToBay(ctx, session, callsign, targetBay, true); err != nil {
		return err
	}

	if targetBay == shared.BAY_CLEARED {
		s.ClearMandatoryRouteCdm(ctx, session, callsign)
	}

	if shouldConfirmVoiceClearance {
		pdcService := s.getPdcService()
		if pdcService == nil {
			return errors.New("PDC service not available")
		}

		if err := pdcService.ConfirmVoiceClearance(ctx, callsign, session); err != nil {
			if rollbackErr := s.MoveToBay(ctx, session, callsign, previousBay, true); rollbackErr != nil {
				return errors.Join(err, rollbackErr)
			}
			if rollbackErr := s.applyClearedFlagForMoveWithOptions(ctx, session, callsign, previousCleared, previousBay, previousBay == shared.BAY_NOT_CLEARED, cid, false, false); rollbackErr != nil {
				return errors.Join(err, rollbackErr)
			}
			return err
		}
	}
	if targetBay == shared.BAY_CLEARED && !previousCleared {
		if err := s.adjustTobtForClearance(ctx, session, callsign); err != nil {
			return err
		}
	}

	s.syncAsatForGroundStateBestEffort(ctx, session, callsign, groundState)
	return nil
}

const pushbackTsatValidationIssueType = "TSAT PUSHBACK"

type validatedPushbackContextKey struct{}

type validatedPushbackFlight struct {
	session  int32
	callsign string
}

func withValidatedPushback(ctx context.Context, session int32, callsign string) context.Context {
	return context.WithValue(ctx, validatedPushbackContextKey{}, validatedPushbackFlight{session: session, callsign: callsign})
}

func pushbackWasValidated(ctx context.Context, session int32, callsign string) bool {
	flight, ok := ctx.Value(validatedPushbackContextKey{}).(validatedPushbackFlight)
	return ok && flight.session == session && flight.callsign == callsign
}

func pushbackTsatWithinWindow(tsat string, now time.Time) bool {
	ts := strings.TrimSpace(tsat)
	if len(ts) < 4 {
		return false
	}
	ts = ts[:4]
	parsed, ok := parseValidationClockUTC(ts, now.UTC())
	if !ok {
		return false
	}
	delta := parsed.Sub(now.UTC())
	if delta > 12*time.Hour {
		delta -= 24 * time.Hour
	}
	return delta >= -5*time.Minute && delta <= 6*time.Minute
}

func pushbackTimingContext(strip *internalModels.Strip, remoteCtot string, confirmed bool, now time.Time) string {
	if strip == nil {
		return ""
	}
	value := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	remoteState := remoteCtot
	if !confirmed {
		remoteState = "UNCONFIRMED"
	}
	return value(strip.EffectiveTobt()) + "|" + value(strip.EffectiveTsat()) + "|" + value(strip.EffectiveCtot()) + "|" + remoteState + "|" + pushbackWindowState(value(strip.EffectiveTsat()), now)
}

func pushbackWindowState(tsat string, now time.Time) string {
	if pushbackTsatWithinWindow(tsat, now) {
		return "valid"
	}
	ts := strings.TrimSpace(tsat)
	if len(ts) < 4 {
		return "unknown"
	}
	parsed, ok := parseValidationClockUTC(ts[:4], now.UTC())
	if !ok {
		return "unknown"
	}
	delta := parsed.Sub(now.UTC())
	if delta > 12*time.Hour {
		delta -= 24 * time.Hour
	}
	if delta > 6*time.Minute {
		return "early"
	}
	return "late"
}

func (s *StripService) validatePushbackTiming(ctx context.Context, session int32, strip *internalModels.Strip, position string) error {
	service, ok := s.cdmService.(interface {
		PreparePushback(context.Context, int32, string) (string, string, bool, error)
		ReadPushbackCtot(context.Context, int32, string) (string, error)
	})
	if !ok || s.validationStore == nil {
		return nil
	}
	now := time.Now().UTC()
	initialTobt := valueOrEmptyStripTime(strip.EffectiveTobt())
	initialTsat := valueOrEmptyStripTime(strip.EffectiveTsat())
	initialCtot := valueOrEmptyStripTime(strip.EffectiveCtot())
	remoteCtot, readErr := service.ReadPushbackCtot(ctx, session, strip.Callsign)
	confirmed := readErr == nil
	logRejection := func(updated *internalModels.Strip, reason string, verified bool, prepareErr error) {
		slog.WarnContext(ctx, "Pushback timing validation blocked attempt",
			slog.Int("session", int(session)), slog.String("callsign", strip.Callsign), slog.String("position", position),
			slog.String("reason", reason), slog.String("initial_tobt", initialTobt), slog.String("initial_tsat", initialTsat),
			slog.String("initial_ctot", initialCtot), slog.String("tobt", valueOrEmptyStripTime(updated.EffectiveTobt())),
			slog.String("tsat", valueOrEmptyStripTime(updated.EffectiveTsat())), slog.String("ctot", valueOrEmptyStripTime(updated.EffectiveCtot())),
			slog.String("remote_ctot", remoteCtot), slog.String("initial_window_state", pushbackWindowState(initialTsat, now)),
			slog.String("window_state", pushbackWindowState(valueOrEmptyStripTime(updated.EffectiveTsat()), time.Now().UTC())),
			slog.Bool("remote_confirmed", confirmed), slog.Bool("verified", verified),
			slog.Any("read_error", readErr), slog.Any("verification_error", prepareErr))
	}
	contextKey := pushbackTimingContext(strip, remoteCtot, confirmed, now)
	if strip.ValidationStatus != nil && strip.ValidationStatus.IssueType == pushbackTsatValidationIssueType && strip.ValidationStatus.ContextKey == contextKey {
		if strip.ValidationStatus.Active {
			logRejection(strip, "existing_validation", false, nil)
			return errors.New("pushback requires TSAT override")
		}
		return nil
	}
	if confirmed && remoteCtot == "" && pushbackTsatWithinWindow(valueOrEmptyStripTime(strip.EffectiveTsat()), now) {
		if strip.ValidationStatus != nil && strip.ValidationStatus.IssueType == pushbackTsatValidationIssueType {
			if err := s.validationStore.ClearValidationStatus(ctx, session, strip.Callsign); err != nil {
				return err
			}
			shared.PublishStripUpdate(ctx, s.publisher, session, strip.Callsign)
		}
		return nil
	}
	verified := false
	var prepareErr error
	if !pushbackTsatWithinWindow(valueOrEmptyStripTime(strip.EffectiveTsat()), now) {
		var preparedCtot string
		_, preparedCtot, verified, prepareErr = service.PreparePushback(ctx, session, strip.Callsign)
		if verified {
			remoteCtot, confirmed = preparedCtot, true
		}
	} else {
		verified = confirmed
	}
	updated, err := s.stripReader.GetByCallsign(ctx, session, strip.Callsign)
	if err != nil {
		return err
	}
	if prepareErr != nil || !verified || remoteCtot != "" || !pushbackTsatWithinWindow(valueOrEmptyStripTime(updated.EffectiveTsat()), time.Now().UTC()) || valueOrEmptyStripTime(updated.EffectiveCtot()) != "" {
		reason, message := pushbackRejectionDetails(updated, remoteCtot, verified, prepareErr, time.Now().UTC())
		logRejection(updated, reason, verified, prepareErr)
		if validationCandidateIsInhibited(updated.ValidationStatus, pushbackTsatValidationIssueType) {
			return errors.New("pushback timing warning is waiting for another validation")
		}
		owner := position
		if updated.Owner != nil && *updated.Owner != "" {
			owner = *updated.Owner
		}
		status := &internalModels.ValidationStatus{
			IssueType:      pushbackTsatValidationIssueType,
			Message:        message,
			OwningPosition: owner,
			Active:         true,
			ActivationKey:  uuid.New().String(),
			ContextKey:     pushbackTimingContext(updated, remoteCtot, confirmed, time.Now().UTC()),
		}
		if err := s.validationStore.SetValidationStatus(ctx, session, strip.Callsign, status); err != nil {
			return err
		}
		shared.PublishStripUpdate(ctx, s.publisher, session, strip.Callsign)
		return errors.New("pushback requires TSAT override")
	}
	if updated.ValidationStatus != nil && updated.ValidationStatus.IssueType == pushbackTsatValidationIssueType {
		if err := s.validationStore.ClearValidationStatus(ctx, session, strip.Callsign); err != nil {
			return err
		}
		shared.PublishStripUpdate(ctx, s.publisher, session, strip.Callsign)
	}
	return nil
}

func pushbackRejectionDetails(strip *internalModels.Strip, remoteCtot string, verified bool, prepareErr error, now time.Time) (string, string) {
	if prepareErr != nil && !errors.Is(prepareErr, cdm.ErrPushbackVerification) {
		return "preparation_failed", "Pushback requires override: timing could not be prepared."
	}
	if prepareErr != nil || !verified {
		return "viff_unconfirmed", "Pushback requires override: vIFF could not confirm the timing update or CTOT."
	}
	if remoteCtot != "" || valueOrEmptyStripTime(strip.EffectiveCtot()) != "" {
		return "ctot_present", "Pushback requires override: aircraft has a CTOT."
	}
	if pushbackWindowState(valueOrEmptyStripTime(strip.EffectiveTsat()), now) == "unknown" {
		return "tsat_unknown", "Pushback requires override: TSAT is missing or invalid."
	}
	return "tsat_outside_window", "Pushback requires override: aircraft is outside the TSAT window."
}

func valueOrEmptyStripTime(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func validateFrontendMoveBayTransition(strip *internalModels.Strip, airport string, targetBay string, clearance bool) error {
	if clearance && targetBay != shared.BAY_CLEARED {
		return errors.New("clearance moves must target the cleared bay")
	}
	if strip.Bay == shared.BAY_NOT_CLEARED && targetBay != shared.BAY_NOT_CLEARED && targetBay != shared.BAY_HIDDEN && !clearance {
		return errors.New("non-cleared strips cannot be moved out of the not-cleared bay")
	}

	isArrivalStrip := strip.Destination == airport && strip.Origin != airport
	if isArrivalStrip && targetBay == shared.BAY_NOT_CLEARED {
		return errors.New("arrival strips cannot be moved to the not-cleared bay")
	}

	return nil
}

func (s *StripService) authorizeFrontendMove(ctx context.Context, session int32, strip *internalModels.Strip, callsign string, airport string, targetBay string, clientPosition string, confirmedRemoval bool) error {
	// EST's confirmed VACANT/CLEAR FPL commands intentionally remove whatever
	// strip occupies the stand, irrespective of its current controller owner.
	if confirmedRemoval && targetBay == shared.BAY_HIDDEN {
		return nil
	}
	if strip.Owner == nil || *strip.Owner == "" || *strip.Owner == clientPosition {
		return nil
	}

	isArrivalStrip := strip.Destination == airport && strip.Origin != airport
	if isArrivalStrip && shared.IsArrivalBay(targetBay) {
		return nil
	}

	coordRepo := s.getCoordinationRepository()
	if coordRepo == nil {
		return errors.New("not authorized: strip is owned by another controller")
	}

	coord, err := coordRepo.GetByStripCallsign(ctx, session, callsign)
	if err != nil || coord == nil || coord.ToPosition != clientPosition {
		return errors.New("not authorized: strip is owned by another controller")
	}

	return nil
}

func (s *StripService) applyFrontendMoveState(ctx context.Context, session int32, strip *internalModels.Strip, targetBay string, cid string, airport string) (*string, error) {
	if targetBay == shared.BAY_NOT_CLEARED || targetBay == shared.BAY_CLEARED {
		return nil, s.applyClearedFlagForMoveWithOptions(ctx, session, strip.Callsign, targetBay == shared.BAY_CLEARED, strip.Bay, targetBay == shared.BAY_NOT_CLEARED, cid, false, true)
	}

	return s.updateGroundStateForMoveWithOptions(ctx, session, strip.Callsign, targetBay, cid, airport, strip.Bay, false)
}
