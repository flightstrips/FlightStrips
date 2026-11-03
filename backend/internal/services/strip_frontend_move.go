package services

import (
	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/cdm"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *StripService) MoveFrontendStrip(ctx context.Context, session int32, callsign string, targetBay string, cid string, airport string, clientPosition string, clearance bool, confirmedRemoval bool) error {
	return s.executeControllerWorkflow(ctx, session, callsign, cid, airport, clientPosition, "strip.move", airportworkflow.Move{Bay: targetBay, Clearance: clearance, ConfirmedRemoval: confirmedRemoval})
}

// ExecuteWorkflowAction dispatches any handler-registered typed action through
// the same guards and move/field persistence as normal controller movements.
func (s *StripService) ExecuteWorkflowAction(ctx context.Context, session int32, callsign, cid, airport, position, action string, request any) error {
	return s.executeControllerWorkflow(ctx, session, callsign, cid, airport, position, action, request)
}
func (s *StripService) executeControllerWorkflow(ctx context.Context, session int32, callsign, cid, airport, clientPosition, action string, request any) error {
	bound, err := s.workflowFor(ctx, session, airport)
	if err != nil {
		return err
	}

	strip, err := s.stripReader.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	if err := validateWorkflowRevision(ctx, bound, session, strip); err != nil {
		return err
	}
	decision, err := bound.Action(airportworkflow.Context{Session: session, Source: airportworkflow.HumanSource, Strip: workflowSnapshot(strip)}, action, request)
	if err != nil {
		return err
	}
	if decision.Bay == nil {
		bay := strip.Bay
		decision.Bay = &bay
	}
	if err := bound.Validate(airportworkflow.Context{Session: session, Source: airportworkflow.HumanSource}, decision); err != nil {
		return err
	}
	targetBay := *decision.Bay
	confirmedRemoval := false
	if move, ok := request.(airportworkflow.Move); ok {
		confirmedRemoval = move.ConfirmedRemoval
	}
	// A confirmed EST VACANT/CLEAR FPL operation must remain available for
	// removing an obsolete stand occupant even when another validation currently
	// owns the master-caution slot. Ordinary moves to HIDDEN remain locked.
	if strip.IsValidationLocked() && !confirmedRemoval && (strip.ValidationStatus == nil || strip.ValidationStatus.IssueType != pushbackTsatValidationIssueType || targetBay != shared.BAY_PUSH) {
		return shared.RejectAction("strip is locked by an active validation")
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
	if err := validateWorkflowRevision(ctx, bound, session, strip); err != nil {
		return err
	}
	// Provider preparation ran without a session/strip lock. Recheck the pure
	// action and ownership against the refreshed strip before any move mutation.
	decision, err = bound.Action(airportworkflow.Context{Session: session, Source: airportworkflow.HumanSource, Strip: workflowSnapshot(strip)}, action, request)
	if err != nil {
		return err
	}
	if decision.Bay == nil {
		bay := strip.Bay
		decision.Bay = &bay
	}
	if err := bound.Validate(airportworkflow.Context{Session: session, Source: airportworkflow.HumanSource}, decision); err != nil {
		return err
	}
	targetBay = *decision.Bay
	if err := s.authorizeFrontendMove(ctx, session, strip, callsign, airport, targetBay, clientPosition, confirmedRemoval); err != nil {
		return err
	}
	if strip.IsValidationLocked() && !confirmedRemoval {
		return shared.RejectAction("strip is locked by an active validation")
	}

	if strip.Bay == targetBay && (decision.Cleared == nil || *decision.Cleared == strip.Cleared) && (decision.GroundState == nil || *decision.GroundState == workflowValue(strip.State)) {
		return s.acknowledgeDeicePlatformInBay(ctx, session, callsign, targetBay)
	}

	previousBay := strip.Bay
	previousCleared := strip.Cleared
	clearing := decision.Cleared != nil && *decision.Cleared
	shouldConfirmVoiceClearance := clearing && targetBay == shared.BAY_CLEARED &&
		strip.PdcState != "" &&
		strip.PdcState != internalModels.PdcStateNone

	groundState, err := s.applyFrontendMoveDecision(ctx, session, strip, decision, cid, airport)
	if err != nil {
		return err
	}

	// State preparation preserves the source bay. MoveToBay saves the target bay
	// before initializing runway status and reevaluating arrival validations.
	if previousBay != targetBay {
		if err := s.MoveToBay(ctx, session, callsign, targetBay, true); err != nil {
			return err
		}
	} else if s.publisher != nil {
		shared.PublishStripUpdate(ctx, s.publisher, session, callsign)
	}

	if clearing && targetBay == shared.BAY_CLEARED {
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
	if clearing && targetBay == shared.BAY_CLEARED && !previousCleared {
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
		slog.Log(ctx, pushbackRejectionLogLevel(reason, readErr, prepareErr), "Pushback timing validation blocked attempt",
			slog.Int("session", int(session)), slog.String("callsign", strip.Callsign), slog.String("position", position),
			slog.String("reason", reason), slog.String("initial_tobt", initialTobt), slog.String("initial_tsat", initialTsat),
			slog.String("initial_ctot", initialCtot), slog.String("tobt", valueOrEmptyStripTime(updated.EffectiveTobt())),
			slog.String("tsat", valueOrEmptyStripTime(updated.EffectiveTsat())), slog.String("ctot", valueOrEmptyStripTime(updated.EffectiveCtot())),
			slog.String("remote_ctot", remoteCtot), slog.String("initial_window_state", pushbackWindowState(initialTsat, now)),
			slog.String("window_state", pushbackWindowState(valueOrEmptyStripTime(updated.EffectiveTsat()), time.Now().UTC())),
			slog.Bool("remote_confirmed", confirmed), slog.Bool("verified", verified),
			slog.Any("read_error", readErr), slog.Any("verification_error", prepareErr))
	}
	rejection := func(reason, message string, prepareErr error) error {
		if pushbackRejectionLogLevel(reason, readErr, prepareErr) == slog.LevelInfo {
			return shared.RejectAction(message)
		}
		return errors.New(message)
	}
	contextKey := pushbackTimingContext(strip, remoteCtot, confirmed, now)
	if strip.ValidationStatus != nil && strip.ValidationStatus.IssueType == pushbackTsatValidationIssueType && strip.ValidationStatus.ContextKey == contextKey {
		if strip.ValidationStatus.Active {
			logRejection(strip, "existing_validation", false, nil)
			return rejection("existing_validation", "pushback requires TSAT override", nil)
		}
		return nil
	}
	if confirmed && pushbackTsatWithinWindow(valueOrEmptyStripTime(strip.EffectiveTsat()), now) {
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
			return rejection(reason, "pushback timing warning is waiting for another validation", prepareErr)
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
		return rejection(reason, "pushback requires TSAT override", prepareErr)
	}
	if updated.ValidationStatus != nil && updated.ValidationStatus.IssueType == pushbackTsatValidationIssueType {
		if err := s.validationStore.ClearValidationStatus(ctx, session, strip.Callsign); err != nil {
			return err
		}
		shared.PublishStripUpdate(ctx, s.publisher, session, strip.Callsign)
	}
	return nil
}

func pushbackRejectionLogLevel(reason string, readErr, prepareErr error) slog.Level {
	if readErr != nil || prepareErr != nil {
		return slog.LevelWarn
	}
	switch reason {
	case "existing_validation", "ctot_present", "tsat_unknown", "tsat_outside_window":
		return slog.LevelInfo
	default:
		return slog.LevelWarn
	}
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

func (s *StripService) validateFrontendMoveBayTransition(ctx context.Context, session int32, strip *internalModels.Strip, airport, bay string, clearance bool) error {
	bound, err := s.workflowFor(ctx, session, airport)
	if err != nil {
		return err
	}
	_, err = bound.Action(airportworkflow.Context{Session: session, Source: airportworkflow.HumanSource, Strip: workflowSnapshot(strip)}, "strip.move", airportworkflow.Move{Bay: bay, Clearance: clearance})
	return err
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
		return shared.RejectAction("not authorized: strip is owned by another controller")
	}

	coord, err := coordRepo.GetByStripCallsign(ctx, session, callsign)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err != nil || coord == nil || coord.ToPosition != clientPosition {
		return shared.RejectAction("not authorized: strip is owned by another controller")
	}

	return nil
}

func (s *StripService) applyFrontendMoveDecision(ctx context.Context, session int32, strip *internalModels.Strip, decision airportworkflow.Decision, cid string, airport string) (*string, error) {
	targetBay := *decision.Bay
	if decision.Cleared != nil {
		return nil, s.applyClearedFlagForMoveWithOptions(ctx, session, strip.Callsign, *decision.Cleared, strip.Bay, !*decision.Cleared, cid, false, true)
	}
	ctx = context.WithValue(ctx, workflowGroundStateKey{}, decision.GroundState)
	return s.updateGroundStateForMoveWithOptions(ctx, session, strip.Callsign, targetBay, cid, airport, strip.Bay, false)
}

type workflowGroundStateKey struct{}
