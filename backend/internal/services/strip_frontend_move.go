package services

import (
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
	"errors"
	"log/slog"
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
	if strip.IsValidationLocked() && !confirmedRemoval {
		return errors.New("strip is locked by an active validation")
	}

	if err := validateFrontendMoveBayTransition(strip, airport, targetBay, clearance); err != nil {
		return err
	}

	if err := s.authorizeFrontendMove(ctx, session, strip, callsign, airport, targetBay, clientPosition, confirmedRemoval); err != nil {
		return err
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

	s.syncAsatForGroundStateBestEffort(ctx, session, callsign, groundState)
	return nil
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
