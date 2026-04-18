package services

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/events/frontend"
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
)

// UpdateAssignedSquawk updates the assigned squawk for a strip and notifies the frontend.
func (s *StripService) UpdateAssignedSquawk(ctx context.Context, session int32, callsign string, squawk string) error {
	count, err := s.stripRepo.UpdateAssignedSquawk(ctx, session, callsign, &squawk, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		slog.DebugContext(ctx, "Strip being updated does not exist in database", slog.String("callsign", callsign), slog.String("event", "AssignedSquawk"))
	} else {
		s.publisher.SendAssignedSquawkEvent(session, callsign, squawk)
		if err := s.reevaluateSquawkValidationsForSession(ctx, session, true); err != nil {
			return err
		}
	}
	return nil
}

// UpdateSquawk updates the current squawk for a strip and notifies the frontend.
func (s *StripService) UpdateSquawk(ctx context.Context, session int32, callsign string, squawk string) error {
	count, err := s.stripRepo.UpdateSquawk(ctx, session, callsign, &squawk, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		slog.DebugContext(ctx, "Strip being updated does not exist in database", slog.String("callsign", callsign), slog.String("event", "Squawk"))
	} else {
		s.publisher.SendSquawkEvent(session, callsign, squawk)
		if err := s.reevaluateSquawkValidationsForSession(ctx, session, true); err != nil {
			return err
		}
	}
	return nil
}

// UpdateRequestedAltitude updates the requested altitude for a strip and notifies the frontend.
func (s *StripService) UpdateRequestedAltitude(ctx context.Context, session int32, callsign string, altitude int32) error {
	count, err := s.stripRepo.UpdateRequestedAltitude(ctx, session, callsign, &altitude, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		slog.DebugContext(ctx, "Strip being updated does not exist in database", slog.String("callsign", callsign), slog.String("event", "RequestedAltitude"))
	} else {
		s.publisher.SendRequestedAltitudeEvent(session, callsign, altitude)
	}
	return nil
}

// UpdateClearedAltitude updates the cleared altitude for a strip and notifies the frontend.
func (s *StripService) UpdateClearedAltitude(ctx context.Context, session int32, callsign string, altitude int32) error {
	count, err := s.stripRepo.UpdateClearedAltitude(ctx, session, callsign, &altitude, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		slog.DebugContext(ctx, "Strip being updated does not exist in database", slog.String("callsign", callsign), slog.String("event", "ClearedAltitude"))
	} else {
		s.publisher.SendClearedAltitudeEvent(session, callsign, altitude)
	}
	return nil
}

// UpdateCommunicationType updates the communication type for a strip and notifies the frontend.
func (s *StripService) UpdateCommunicationType(ctx context.Context, session int32, callsign string, commType string) error {
	count, err := s.stripRepo.UpdateCommunicationType(ctx, session, callsign, &commType, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		slog.DebugContext(ctx, "Strip being updated does not exist in database", slog.String("callsign", callsign), slog.String("event", "CommunicationType"))
		return nil
	}
	s.publisher.SendCommunicationTypeEvent(session, callsign, commType)
	return nil
}

// UpdateHeading updates the heading for a strip and notifies the frontend.
func (s *StripService) UpdateHeading(ctx context.Context, session int32, callsign string, heading int32) error {
	count, err := s.stripRepo.UpdateHeading(ctx, session, callsign, &heading, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		slog.DebugContext(ctx, "Strip being updated does not exist in database", slog.String("callsign", callsign), slog.String("event", "SetHeading"))
		return nil
	}
	s.publisher.SendSetHeadingEvent(session, callsign, heading)
	return nil
}

// UpdateGroundState updates the ground state for a strip, recomputes the bay, and moves
// the strip if the bay changed.
func (s *StripService) UpdateGroundState(ctx context.Context, session int32, callsign string, groundState string, airport string) error {
	existingStrip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.DebugContext(ctx, "Strip being updated does not exist in database", slog.String("callsign", callsign), slog.String("event", "GroundState"))
			return nil
		}
		return err
	}

	if existingStrip.State != nil && *existingStrip.State == groundState {
		return nil
	}

	dbStrip := database.Strip{
		Origin:      existingStrip.Origin,
		Destination: existingStrip.Destination,
		Cleared:     existingStrip.Cleared,
		Bay:         existingStrip.Bay,
	}
	bay := shared.GetDepartureBayFromGroundState(groundState, dbStrip, airport, s.isGndOnline(ctx, session))

	_, err = s.stripRepo.UpdateGroundState(ctx, session, callsign, &groundState, bay, nil)
	if err != nil {
		return err
	}

	if existingStrip.Bay != bay {
		if err := s.MoveToBay(context.Background(), session, callsign, bay, true); err != nil {
			return err
		}
	}

	if s.cdmService != nil {
		if err := s.cdmService.SyncAsatForGroundState(ctx, session, callsign, groundState); err != nil {
			return err
		}
	}

	return nil
}

// UpdateClearedFlag updates the cleared flag for a strip, recomputes the bay,
// triggers auto-assumption if cleared, and moves the strip if the bay changed.
func (s *StripService) UpdateClearedFlag(ctx context.Context, session int32, callsign string, cleared bool) error {
	existingStrip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.DebugContext(ctx, "Strip being updated does not exist in database", slog.String("callsign", callsign), slog.String("event", "FlightStripOnline"))
			return nil
		}
		return err
	}

	if existingStrip.Cleared == cleared {
		return nil
	}

	bay := existingStrip.Bay
	if bay == shared.BAY_NOT_CLEARED || bay == shared.BAY_UNKNOWN {
		bay = shared.BAY_CLEARED
	}
	if bay == "" {
		bay = shared.BAY_HIDDEN
	}

	_, err = s.stripRepo.UpdateClearedFlag(ctx, session, callsign, cleared, bay, nil)
	if err != nil {
		return err
	}

	if cleared {
		// Skip auto-assume if PDC clearance is pending pilot WILCO (strip is in CLEARED pdc state)
		if existingStrip.PdcState != "CLEARED" {
			if err := s.AutoAssumeForClearedStrip(ctx, session, callsign); err != nil {
				slog.ErrorContext(ctx, "Failed to auto-assume cleared strip from EuroScope", slog.Any("error", err))
			}
		}
	}

	if existingStrip.Bay != bay {
		return s.MoveToBay(ctx, session, callsign, bay, true)
	}

	return nil
}

// UpdateStand updates the stand for a strip, notifies the frontend, and triggers route recalculation.
func (s *StripService) UpdateStand(ctx context.Context, session int32, callsign string, stand string) error {
	count, err := s.stripRepo.UpdateStand(ctx, session, callsign, &stand, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		slog.DebugContext(ctx, "Strip being updated does not exist in database", slog.String("callsign", callsign), slog.String("event", "Stand"))
		return nil
	}
	s.publisher.SendStandEvent(session, callsign, stand)

	server := s.publisher.GetServer()
	if server != nil {
		if err := server.UpdateRouteForStrip(callsign, session, true); err != nil {
			slog.ErrorContext(ctx, "Error updating route after stand assignment", slog.String("callsign", callsign), slog.Any("error", err))
		}
	}
	if err := s.reevaluateStripValidationPrecedence(ctx, session, callsign, true, false); err != nil {
		return err
	}
	return nil
}

// notifyStripUpdate broadcasts a strip_update to frontend clients.
func (s *StripService) notifyStripUpdate(session int32, callsign string) {
	if s.publisher == nil {
		return
	}
	s.publisher.SendStripUpdate(session, callsign)
}

// UpdateClearedFlagForMove handles the frontend "move to cleared/not-cleared bay" action.
// It updates the cleared flag and bay in the DB, triggers auto-assumption, and notifies EuroScope.
func (s *StripService) UpdateClearedFlagForMove(ctx context.Context, session int32, callsign string, isCleared bool, bay string, cid string) error {
	return s.applyClearedFlagForMove(ctx, session, callsign, isCleared, bay, cid, false)
}

// ConfirmPdcClearance marks a strip cleared as part of pilot confirmation.
// EuroScope notification is handled by the PDC service so it can send the
// confirmed state before the cleared flag.
func (s *StripService) ConfirmPdcClearance(ctx context.Context, session int32, callsign string, bay string, cid string) error {
	return s.applyClearedFlagForMoveWithOptions(ctx, session, callsign, true, bay, cid, false, false)
}

func (s *StripService) applyClearedFlagForMove(ctx context.Context, session int32, callsign string, isCleared bool, bay string, cid string, forceEuroscopeNotification bool) error {
	return s.applyClearedFlagForMoveWithOptions(ctx, session, callsign, isCleared, bay, cid, forceEuroscopeNotification, true)
}

func (s *StripService) applyClearedFlagForMoveWithOptions(ctx context.Context, session int32, callsign string, isCleared bool, bay string, cid string, forceEuroscopeNotification bool, autoAssumeOnClear bool) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	count, err := s.stripRepo.UpdateClearedFlag(ctx, session, callsign, isCleared, bay, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("failed to update strip cleared flag")
	}

	if !isCleared && bay == shared.BAY_NOT_CLEARED {
		if err := s.clearOwnerForNotCleared(ctx, session, callsign); err != nil {
			return err
		}
	}

	// Only trigger side-effects when the cleared flag actually changed value.
	if strip.Cleared != isCleared {
		if isCleared {
			if autoAssumeOnClear {
				if err := s.AutoAssumeForClearedStripByCid(ctx, session, callsign, cid); err != nil {
					slog.ErrorContext(ctx, "Failed to auto-assume cleared strip", slog.Any("error", err))
				}
			}
		}
	}
	if (strip.Cleared != isCleared || forceEuroscopeNotification) && s.esCommander != nil {
		s.esCommander.SendClearedFlag(session, cid, callsign, isCleared)
	}
	return nil
}

// UpdateGroundStateForMove handles the frontend "move to general bay" action.
// It computes the new ground state, updates the DB, and notifies EuroScope.
func (s *StripService) UpdateGroundStateForMove(ctx context.Context, session int32, callsign string, bay string, cid string, airport string) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	state := strip.State
	if strip.Origin == airport {
		groundState := shared.GetGroundState(bay)
		if groundState != euroscope.GroundStateUnknown && bay != shared.BAY_STAND {
			state = &groundState
		}
	} else if strip.Destination == airport && bay == shared.BAY_STAND {
		parked := euroscope.GroundStateParked
		state = &parked
	}

	count, err := s.stripRepo.UpdateGroundState(ctx, session, callsign, state, bay, nil)
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("failed to update strip bay/ground state")
	}

	if state != strip.State && state != nil && s.esCommander != nil {
		s.esCommander.SendGroundState(session, cid, callsign, *state)
	}

	// If the strip is moved backward from rwy-dep to a non-airborne bay, reset runway_cleared.
	if strip.Bay == shared.BAY_DEPART && bay != shared.BAY_DEPART && bay != shared.BAY_AIRBORNE && strip.RunwayCleared {
		if _, err := s.stripRepo.ResetRunwayClearance(ctx, session, callsign); err != nil {
			return err
		}
		s.publisher.SendStripUpdate(session, callsign)
	}

	if err := s.reevaluateStripValidationPrecedence(ctx, session, callsign, true, true); err != nil {
		return err
	}

	return nil
}

// UpdateReleasePoint updates the release point for a strip and broadcasts to all frontend clients.
func (s *StripService) UpdateReleasePoint(ctx context.Context, session int32, callsign string, releasePoint string) error {
	affected, err := s.stripRepo.UpdateReleasePoint(ctx, session, callsign, &releasePoint)
	if err != nil {
		return err
	}
	if affected != 1 {
		return errors.New("failed to update release point")
	}
	s.publisher.Broadcast(session, frontend.ReleasePointEvent{
		Callsign:     callsign,
		ReleasePoint: releasePoint,
	})
	return nil
}

// ApplyReleasePoint updates the release point with ownership enforcement and broadcasts.
// Non-owners may overwrite an existing value (marks the cell yellow for both controllers).
// Non-owners setting a value on a strip that has none are rejected.
func (s *StripService) ApplyReleasePoint(ctx context.Context, session int32, callsign string, releasePoint string, clientPosition string) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	isOwner := strip.Owner == nil || *strip.Owner == "" || *strip.Owner == clientPosition
	unexpectedChange := false

	if !isOwner {
		hasExisting := strip.ReleasePoint != nil && *strip.ReleasePoint != ""
		if hasExisting && *strip.ReleasePoint != releasePoint {
			// Non-owner overwriting existing value — allow, mark as unexpected change.
			if err := s.stripRepo.AppendUnexpectedChangeField(ctx, session, callsign, "release_point"); err != nil {
				return err
			}
			unexpectedChange = true
		} else if !hasExisting {
			// Non-owner setting a value that didn't exist — reject.
			return errors.New("cannot modify holding point on unowned strip")
		}
		// Non-owner setting same value as existing — silently allow.
	}

	if err := s.UpdateReleasePoint(ctx, session, callsign, releasePoint); err != nil {
		return err
	}

	if unexpectedChange {
		s.publisher.SendStripUpdate(session, callsign)
	}
	return nil
}

// UpdateMarked updates the marked flag for a strip and broadcasts to all frontend clients.
func (s *StripService) UpdateMarked(ctx context.Context, session int32, callsign string, marked bool) error {
	affected, err := s.stripRepo.UpdateMarked(ctx, session, callsign, marked, nil)
	if err != nil {
		return err
	}
	if affected != 1 {
		return errors.New("failed to update marked flag")
	}
	s.publisher.Broadcast(session, frontend.MarkedEvent{
		Callsign: callsign,
		Marked:   marked,
	})
	return nil
}
