package services

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/models"
	"context"
	"errors"
	"log/slog"
	"slices"
)

// RunwayClearance marks a strip as runway-cleared, moving it from TAXI_LWR to DEPART (if applicable),
// then broadcasts the full updated strip to all clients in the session.
// For departures from this airport in TAXI_LWR or DEPART bay, the ground state is set to 'DEPA'
// and sent to EuroScope.
func (s *StripService) RunwayClearance(ctx context.Context, session int32, callsign string, cid string, airport string) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}
	if strip.IsValidationLocked() {
		return errors.New("strip is locked by an active validation")
	}

	affected, err := s.stripRepo.UpdateRunwayClearance(ctx, session, callsign)
	if err != nil {
		return err
	}
	if affected != 1 {
		return errors.New("failed to update runway clearance")
	}

	// For departures moving to or already at rwy-dep, set state to DEPA and notify ES.
	isAtOrMovingToDepart := strip.Bay == shared.BAY_DEPART || strip.Bay == shared.BAY_TAXI_LWR
	if isAtOrMovingToDepart && strip.Origin == airport {
		state := euroscope.GroundStateDepart
		if _, err := s.stripRepo.UpdateGroundState(ctx, session, callsign, &state, shared.BAY_DEPART, nil); err != nil {
			return err
		}
		if s.esCommander != nil {
			s.esCommander.SendGroundState(session, cid, callsign, state)
		}
	}

	s.publisher.SendStripUpdate(session, callsign)
	return nil
}

// RunwayConfirmation marks a cleared strip as runway-confirmed (green) and broadcasts the update.
// A strip that is already confirmed stays confirmed; non-cleared strips are unaffected.
func (s *StripService) RunwayConfirmation(ctx context.Context, session int32, callsign string) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}
	if strip.IsValidationLocked() {
		return errors.New("strip is locked by an active validation")
	}

	affected, err := s.stripRepo.UpdateRunwayConfirmation(ctx, session, callsign)
	if err != nil {
		return err
	}
	if affected != 1 {
		return errors.New("failed to update runway confirmation")
	}
	s.publisher.SendStripUpdate(session, callsign)
	return nil
}

// PropagateRunwayChange updates the runway on strips that had an auto-assigned runway
// matching the old active runways.
func (s *StripService) PropagateRunwayChange(ctx context.Context, session int32, airport string, oldRunways models.ActiveRunways, newRunways models.ActiveRunways) error {
	strips, err := s.stripRepo.List(ctx, session)
	if err != nil {
		return err
	}

	for _, strip := range strips {
		if strip.Runway == nil || *strip.Runway == "" {
			continue
		}
		currentRunway := *strip.Runway
		isArrival := strip.Destination == airport

		var oldList []string
		var newList []string
		if isArrival {
			oldList = oldRunways.ArrivalRunways
			newList = newRunways.ArrivalRunways
		} else {
			oldList = oldRunways.DepartureRunways
			newList = newRunways.DepartureRunways
		}

		if !slices.Contains(oldList, currentRunway) {
			continue
		}
		if len(newList) == 0 {
			continue
		}

		newRunway := newList[0]
		if newRunway == currentRunway {
			continue
		}

		if _, err := s.stripRepo.UpdateRunway(ctx, session, strip.Callsign, &newRunway, nil); err != nil {
			slog.ErrorContext(ctx, "Failed to update auto-assigned runway on strip",
				slog.String("callsign", strip.Callsign),
				slog.String("old_runway", currentRunway),
				slog.String("new_runway", newRunway),
				slog.Any("error", err))
			continue
		}

		if err := s.ReevaluatePdcInvalidValidation(ctx, session, strip.Callsign, true, false); err != nil {
			return err
		}
		if err := s.reevaluateDepartureValidation(ctx, session, strip.Callsign, true, false); err != nil {
			return err
		}
	}
	return nil
}
