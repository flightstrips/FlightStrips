package services

import (
	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
)

func isMissedApproachReturn(fromPosition string, assumingPosition string) bool {
	fromPos, fromErr := config.GetPositionBasedOnFrequency(fromPosition)
	assumingPos, assumingErr := config.GetPositionBasedOnFrequency(assumingPosition)
	return fromErr == nil && assumingErr == nil && fromPos.Section == "TWR" && assumingPos.Section == "APP"
}

// applyMissedApproachOwnerFix cleans up owners after a missed-approach assume:
//   - removes towerPosition from PreviousOwners (TWR should become next controller again)
//   - recalculates NextOwners via UpdateRouteForStrip
func (s *StripService) applyMissedApproachOwnerFix(ctx context.Context, session int32, callsign string, assumingPosition string, towerPosition string) {
	updated, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		slog.WarnContext(ctx, "missed approach owner fix: failed to re-fetch strip", slog.String("callsign", callsign), slog.Any("error", err))
		return
	}

	cleanedPrev := slices.DeleteFunc(slices.Clone(updated.PreviousOwners), func(p string) bool {
		return p == towerPosition
	})
	if len(cleanedPrev) != len(updated.PreviousOwners) {
		if setErr := s.stripRepo.SetPreviousOwners(ctx, session, callsign, cleanedPrev); setErr != nil {
			slog.WarnContext(ctx, "missed approach owner fix: failed to clear TWR from previous owners", slog.String("callsign", callsign), slog.Any("error", setErr))
		} else {
			s.publisher.SendOwnersUpdate(session, callsign, assumingPosition, updated.NextOwners, cleanedPrev)
		}
	}

	if srv := s.publisher.GetServer(); srv != nil {
		_ = srv.UpdateRouteForStrip(callsign, session, true)
	}
}

// MissedApproach handles the frontend "missed approach" action for an aircraft on final.
// The requesting controller must own the strip, which must be in FINAL or RWY_ARR.
// The strip is moved to AIRBORNE and a coordination transfer is initiated to the
// approach controller configured for the active arrival runway.
func (s *StripService) MissedApproach(ctx context.Context, session int32, callsign string, position string) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	if strip.Bay != shared.BAY_FINAL && strip.Bay != shared.BAY_RWY_ARR {
		return errors.New("missed approach: strip is not in FINAL or RWY_ARR bay")
	}

	if strip.Owner == nil || *strip.Owner == "" || *strip.Owner != position {
		return errors.New("missed approach: strip is not owned by you")
	}

	if s.publisher == nil {
		return errors.New("missed approach: frontend hub not configured")
	}
	server := s.publisher.GetServer()
	if server == nil {
		return errors.New("missed approach: server not configured")
	}

	// Get the active arrival runway from the session.
	sessionObj, err := server.GetSessionRepository().GetByID(ctx, session)
	if err != nil {
		return fmt.Errorf("missed approach: failed to get session: %w", err)
	}

	// Find the first configured arrival runway that has a handover mapping.
	toPositionName := ""
	for _, runway := range sessionObj.ActiveRunways.ArrivalRunways {
		if pos, ok := config.GetMissedApproachHandoverPosition(runway); ok {
			toPositionName = pos
			break
		}
	}
	if toPositionName == "" {
		return errors.New("missed approach: no approach controller configured for the active arrival runway")
	}

	// Convert the configured position name to its frequency.
	posConfig, configErr := config.GetPositionByName(toPositionName)
	if configErr != nil {
		return fmt.Errorf("missed approach: approach controller %q not found in config: %w", toPositionName, configErr)
	}
	configuredFrequency := posConfig.Frequency

	// Look up online controllers to resolve the actual handover target.
	// The configured controller may not be staffed, so fall back through the
	// airborne_owners priority list (same order used for departure airborne transfers).
	controllers, err := server.GetControllerRepository().ListBySession(ctx, session)
	if err != nil {
		return fmt.Errorf("missed approach: failed to list controllers: %w", err)
	}

	// Build a quick lookup: frequency → controller.
	controllerByFreq := make(map[string]*internalModels.Controller, len(controllers))
	for _, c := range controllers {
		controllerByFreq[c.Position] = c
	}

	var ownerCid *string
	toPosition := configuredFrequency
	var targetCallsign string

	// Find the strip owner's CID.
	if c, ok := controllerByFreq[position]; ok && c.Cid != nil && *c.Cid != "" {
		ownerCid = c.Cid
	}

	// Prefer the configured controller; fall back through airborne_owners in priority order.
	if c, ok := controllerByFreq[configuredFrequency]; ok {
		targetCallsign = c.Callsign
	} else {
		for _, posName := range config.GetAirborneOwners() {
			p, posErr := config.GetPositionByName(posName)
			if posErr != nil {
				continue
			}
			if c, ok := controllerByFreq[p.Frequency]; ok {
				targetCallsign = c.Callsign
				toPosition = p.Frequency
				slog.InfoContext(ctx, "missed approach: configured APP controller offline, falling back via airborne_owners",
					slog.String("callsign", callsign),
					slog.String("configured_position", toPositionName),
					slog.String("fallback_position_name", posName),
					slog.String("fallback_callsign", targetCallsign),
				)
				break
			}
		}
	}

	// Move strip to AIRBORNE, keeping the current owner.
	if err := s.MoveToBay(ctx, session, callsign, shared.BAY_AIRBORNE, true); err != nil {
		return fmt.Errorf("missed approach: failed to move strip to AIRBORNE: %w", err)
	}

	// Create the coordination record and notify frontends.
	coord := &internalModels.Coordination{
		Session:      session,
		StripID:      strip.ID,
		FromPosition: position,
		ToPosition:   toPosition,
	}
	if err := server.GetCoordinationRepository().Create(ctx, coord); err != nil {
		return fmt.Errorf("missed approach: failed to create coordination record: %w", err)
	}
	s.publisher.SendCoordinationTransfer(session, callsign, position, toPosition)

	// Send ES coordination handover to the TWR controller's ES client.
	if s.esCommander != nil && ownerCid != nil && targetCallsign != "" {
		slog.InfoContext(ctx, "missed approach: sending ES coordination handover",
			slog.String("callsign", callsign),
			slog.String("owner_cid", *ownerCid),
			slog.String("target_callsign", targetCallsign),
		)
		s.esCommander.SendCoordinationHandover(session, *ownerCid, callsign, targetCallsign)
	} else {
		slog.WarnContext(ctx, "missed approach: ES handover skipped",
			slog.String("callsign", callsign),
			slog.Bool("euroscope_hub_present", s.esCommander != nil),
			slog.Bool("owner_cid_found", ownerCid != nil),
			slog.String("target_callsign", targetCallsign),
		)
	}

	return nil
}
