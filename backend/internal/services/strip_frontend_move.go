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

func (s *StripService) MoveFrontendStrip(ctx context.Context, session int32, callsign string, targetBay string, cid string, airport string, clientPosition string) error {
	if !validFrontendMoveBays[targetBay] {
		slog.WarnContext(ctx, "MoveFrontendStrip: rejecting move event with invalid bay",
			slog.String("callsign", callsign),
			slog.String("bay", targetBay),
			slog.String("cid", cid),
		)
		return errors.New("invalid bay value: " + targetBay)
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	if strip.IsValidationLocked() {
		return errors.New("strip is locked by an active validation")
	}

	if err := validateFrontendMoveBayTransition(strip, airport, targetBay); err != nil {
		return err
	}

	if err := s.authorizeFrontendMove(ctx, session, strip, callsign, targetBay, clientPosition); err != nil {
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

	if err := s.applyFrontendMoveState(ctx, session, strip, targetBay, cid, airport); err != nil {
		return err
	}

	if err := s.MoveToBay(ctx, session, callsign, targetBay, true); err != nil {
		return err
	}

	if targetBay == shared.BAY_CLEARED {
		s.ClearMandatoryRouteCdm(ctx, session, callsign)
	}

	if !shouldConfirmVoiceClearance {
		return nil
	}

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

	return nil
}

func validateFrontendMoveBayTransition(strip *internalModels.Strip, airport string, targetBay string) error {
	isDepartureStrip := strip.Origin == airport && strip.Destination != airport
	isArrivalStrip := strip.Destination == airport && strip.Origin != airport

	if isDepartureStrip && shared.IsArrivalBay(targetBay) {
		return errors.New("departure strips cannot be moved to arrival bays")
	}
	if isArrivalStrip && shared.IsDepartureBay(targetBay) {
		return errors.New("arrival strips cannot be moved to departure bays")
	}

	return nil
}

func (s *StripService) authorizeFrontendMove(ctx context.Context, session int32, strip *internalModels.Strip, callsign string, targetBay string, clientPosition string) error {
	if strip.Owner == nil || *strip.Owner == "" || *strip.Owner == clientPosition {
		return nil
	}

	if shared.IsArrivalBay(targetBay) {
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

func (s *StripService) applyFrontendMoveState(ctx context.Context, session int32, strip *internalModels.Strip, targetBay string, cid string, airport string) error {
	if targetBay == shared.BAY_NOT_CLEARED || targetBay == shared.BAY_CLEARED {
		return s.applyClearedFlagForMoveWithOptions(ctx, session, strip.Callsign, targetBay == shared.BAY_CLEARED, strip.Bay, targetBay == shared.BAY_NOT_CLEARED, cid, false, true)
	}

	return s.updateGroundStateForMoveWithOptions(ctx, session, strip.Callsign, targetBay, cid, airport, strip.Bay, false)
}
