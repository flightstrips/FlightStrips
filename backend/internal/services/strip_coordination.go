package services

import (
	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
	"errors"
	"log/slog"
	"slices"

	"github.com/jackc/pgx/v5"
)

func (s *StripService) maybeSyncEsCoordinationAcceptance(session int32, callsign string, coordination *internalModels.Coordination, assumingPosition string) {
	if coordination == nil || !coordination.FromEs || coordination.EsHandoverCid == nil || *coordination.EsHandoverCid == "" || s.esCommander == nil {
		return
	}

	s.esCommander.SendAssumeOnly(session, *coordination.EsHandoverCid, callsign)
}

func (s *StripService) CreateCoordinationTransfer(ctx context.Context, session int32, callsign string, from string, to string) error {
	if s.publisher == nil {
		return errors.New("frontend hub not configured")
	}

	server := s.publisher.GetServer()
	if server == nil {
		return errors.New("server not configured")
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	coord := &internalModels.Coordination{
		Session:      session,
		StripID:      strip.ID,
		FromPosition: from,
		ToPosition:   to,
	}

	if err := server.GetCoordinationRepository().Create(ctx, coord); err != nil {
		return err
	}

	s.maybeMoveToLowerTwyDepOnTowerTransfer(ctx, session, callsign, strip.Bay, to)
	s.publisher.SendCoordinationTransfer(session, callsign, from, to)

	// For strips in the AIRBORNE bay, also send an ES handover so the owning
	// EuroScope client initiates the transfer to the target controller.
	if strip.Bay == shared.BAY_AIRBORNE && s.esCommander != nil {
		controllers, err := server.GetControllerRepository().ListBySession(ctx, session)
		if err == nil {
			var ownerCid *string
			var targetCallsign string
			for _, c := range controllers {
				if c.Position == from && c.Cid != nil && *c.Cid != "" {
					ownerCid = c.Cid
				}
				if c.Position == to {
					targetCallsign = c.Callsign
				}
			}
			if ownerCid != nil && targetCallsign != "" {
				s.esCommander.SendCoordinationHandover(session, *ownerCid, callsign, targetCallsign)
			}
		}
	}

	return nil
}

func (s *StripService) CreateEsArrivalCoordination(ctx context.Context, session int32, callsign string, from string, to string, esHandoverCid *string) error {
	if s.publisher == nil {
		return errors.New("frontend hub not configured")
	}

	server := s.publisher.GetServer()
	if server == nil {
		return errors.New("server not configured")
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	coord := &internalModels.Coordination{
		Session:       session,
		StripID:       strip.ID,
		FromPosition:  from,
		ToPosition:    to,
		FromEs:        true,
		EsHandoverCid: esHandoverCid,
	}

	if err := server.GetCoordinationRepository().Create(ctx, coord); err != nil {
		return err
	}

	s.publisher.SendCoordinationTransfer(session, callsign, from, to)
	return nil
}

// AcceptCoordination accepts a pending coordination for a strip by the given assumingPosition.
// It deletes the coordination, updates the next/previous owners, sets the strip owner, and
// sends frontend notifications. Returns nil without error if no coordination exists.
func (s *StripService) AcceptCoordination(ctx context.Context, session int32, callsign string, assumingPosition string) error {
	if s.publisher == nil {
		return errors.New("frontend hub not configured")
	}
	server := s.publisher.GetServer()
	if server == nil {
		return errors.New("server not configured")
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	coordination, err := server.GetCoordinationRepository().GetByStripID(ctx, session, strip.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	if err := server.GetCoordinationRepository().Delete(ctx, coordination.ID); err != nil {
		return err
	}

	nextOwners := strip.NextOwners
	index := slices.Index(nextOwners, assumingPosition)
	if index >= 0 {
		nextOwners = nextOwners[index+1:]
	}

	previousOwners := append(strip.PreviousOwners, coordination.FromPosition)

	if err := s.stripRepo.SetNextAndPreviousOwners(ctx, session, callsign, nextOwners, previousOwners); err != nil {
		return err
	}

	count, err := s.setOwnerAndReevaluateDuplicateSquawkValidation(ctx, session, callsign, &assumingPosition, strip.Version)
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("failed to set strip owner after accepting coordination")
	}

	s.publisher.SendCoordinationAssume(session, callsign, assumingPosition)
	s.publisher.SendOwnersUpdate(session, callsign, assumingPosition, nextOwners, previousOwners)
	return nil
}

func (s *StripService) AutoTransferAirborneStrip(ctx context.Context, session int32, callsign string) error {
	slog.DebugContext(ctx, "Attempting automatic AIRBORNE transfer", slog.String("callsign", callsign), slog.Int("session", int(session)))
	if s.publisher == nil {
		return errors.New("frontend hub not configured")
	}

	server := s.publisher.GetServer()
	if server == nil {
		return errors.New("server not configured")
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	if strip.Bay != shared.BAY_AIRBORNE {
		slog.DebugContext(ctx, "Skipping automatic AIRBORNE transfer because strip is not in AIRBORNE bay", slog.String("callsign", callsign))
		return nil
	}

	if strip.Owner == nil || *strip.Owner == "" {
		slog.DebugContext(ctx, "Skipping automatic AIRBORNE transfer without strip owner", slog.String("callsign", callsign))
		return nil
	}

	// fetch controllers once
	controllers, err := server.GetControllerRepository().ListBySession(ctx, session)
	if err != nil {
		return err
	}

	targetController, err := s.resolveAirborneController(strip, controllers)
	if err != nil {
		return err
	}
	if targetController == nil {
		slog.DebugContext(ctx, "No suitable target controller found for automatic AIRBORNE transfer", slog.String("callsign", callsign))
		return nil
	}

	var ownerController *internalModels.Controller
	for _, c := range controllers {
		if c.Position == *strip.Owner {
			ownerController = c
			break
		}
	}

	if ownerController == nil || ownerController.Cid == nil || *ownerController.Cid == "" {
		slog.DebugContext(ctx, "Owner controller CID not available; skipping euroscope handover send", slog.String("callsign", callsign))
		return nil
	}

	if ownerController.Position == targetController.Position {
		slog.DebugContext(ctx, "Skipping automatic AIRBORNE transfer because target controller is the same as strip owner", slog.String("callsign", callsign))
		return nil
	}

	coordination, err := server.GetCoordinationRepository().GetByStripID(ctx, session, strip.ID)
	if err == nil {
		slog.DebugContext(ctx, "Skipping automatic AIRBORNE transfer because coordination already exists",
			slog.String("callsign", callsign),
			slog.String("existing_to", coordination.ToPosition))
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	if err := s.CreateCoordinationTransfer(ctx, session, callsign, *strip.Owner, targetController.Position); err != nil {
		slog.ErrorContext(ctx, "Failed to create coordination transfer for automatic AIRBORNE transfer", slog.String("callsign", callsign), slog.String("from", *strip.Owner), slog.String("to", targetController.Position), slog.Any("error", err))
		return err
	}

	return nil
}

func (s *StripService) resolveAirborneController(strip *internalModels.Strip, controllers []*internalModels.Controller) (*internalModels.Controller, error) {
	if strip.Sid == nil || *strip.Sid == "" {
		return nil, nil
	}

	controllerPriority, err := config.GetAirborneControllerPriority(*strip.Sid)
	if err != nil {
		if errors.Is(err, config.ErrUnknownAirborneRoute) {
			slog.Debug("Unknown SID for AIRBORNE transfer; no controller priority defined", slog.String("sid", *strip.Sid))
			return nil, nil
		}
		return nil, err
	}

	slog.Debug("Resolving airborne controller",
		slog.String("sid", *strip.Sid),
		slog.Any("priority_list", controllerPriority),
	)

	for _, callsign := range controllerPriority {
		position, err := config.GetPositionByName(callsign)
		if err != nil {
			continue
		}
		for _, controller := range controllers {
			if controller.Position == position.Frequency {
				slog.Debug("Matched airborne controller",
					slog.String("sid", *strip.Sid),
					slog.String("matched_callsign", callsign),
					slog.String("controller_position", controller.Position),
				)
				return controller, nil
			}
		}
	}

	slog.Debug("No controller found for AIRBORNE transfer", slog.String("sid", *strip.Sid))
	return nil, nil
}

func prepareOwnersForAutomaticTransfer(strip *internalModels.Strip, newOwner string, additionalPreviousOwners ...string) ([]string, []string) {
	nextOwners := strip.NextOwners
	if index := slices.Index(nextOwners, newOwner); index >= 0 {
		nextOwners = nextOwners[index+1:]
	}

	previousOwners := make([]string, 0, len(strip.PreviousOwners)+1+len(additionalPreviousOwners))
	appendPreviousOwner := func(position string) {
		if position == "" || position == newOwner || slices.Contains(previousOwners, position) {
			return
		}
		previousOwners = append(previousOwners, position)
	}

	for _, owner := range strip.PreviousOwners {
		if owner == newOwner {
			continue
		}
		if strip.Owner != nil && *strip.Owner == owner {
			continue
		}
		previousOwners = append(previousOwners, owner)
	}

	if strip.Owner != nil && *strip.Owner != "" && *strip.Owner != newOwner {
		appendPreviousOwner(*strip.Owner)
	}
	for _, owner := range additionalPreviousOwners {
		appendPreviousOwner(owner)
	}

	return nextOwners, previousOwners
}

// autoAcceptPendingCoordination accepts any pending coordination for the strip automatically,
// e.g. when the aircraft is detected landing on the runway.
func (s *StripService) autoAcceptPendingCoordination(ctx context.Context, session int32, strip *internalModels.Strip) {
	if s.coordRepo == nil {
		return
	}
	coordination, err := s.coordRepo.GetByStripID(ctx, session, strip.ID)
	if err != nil {
		return // no coordination pending
	}

	if err := s.AcceptCoordination(ctx, session, strip.Callsign, coordination.ToPosition); err != nil {
		slog.ErrorContext(ctx, "autoAcceptPendingCoordination: failed to accept", slog.String("callsign", strip.Callsign), slog.Any("error", err))
		return
	}

	s.maybeSyncEsCoordinationAcceptance(session, strip.Callsign, coordination, coordination.ToPosition)
}

// HandleCoordinationReceived processes an EuroScope coordination-received event,
// ensuring the strip is in FINAL and creating an arrival coordination record.
func (s *StripService) HandleCoordinationReceived(ctx context.Context, session int32, callsign string, controllerCallsign string) error {
	if s.controllerRepo == nil {
		return errors.New("controller repository not configured")
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.DebugContext(ctx, "Strip not found for coordination_received event", slog.String("callsign", callsign))
			return nil
		}
		return err
	}

	if strip.Bay != shared.BAY_ARR_HIDDEN && strip.Bay != shared.BAY_FINAL && strip.Bay != shared.BAY_RWY_ARR {
		slog.DebugContext(ctx, "coordination_received on strip not in arrival bay, ignoring", slog.String("callsign", callsign), slog.String("bay", strip.Bay))
		return nil
	}

	controller, err := s.controllerRepo.GetByCallsign(ctx, session, controllerCallsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.DebugContext(ctx, "Controller not found for coordination_received event", slog.String("controller_callsign", controllerCallsign))
			return nil
		}
		return err
	}

	slog.DebugContext(ctx, "Received coordination received event", slog.String("callsign", callsign), slog.String("from_controller", controllerCallsign))

	if strip.Bay == shared.BAY_ARR_HIDDEN {
		if _, err := s.stripRepo.UpdateBay(ctx, session, callsign, shared.BAY_FINAL, nil); err != nil {
			return err
		}

		if err := s.MoveToBay(ctx, session, callsign, shared.BAY_FINAL, true); err != nil {
			return err
		}
	}

	fromPosition := ""
	if strip.Owner != nil {
		fromPosition = *strip.Owner
	}

	return s.CreateEsArrivalCoordination(ctx, session, callsign, fromPosition, controller.Position, controller.Cid)
}

// AssumeStripCoordination handles the full frontend assume flow:
// accepts a pending coordination, or directly assumes an unowned strip.
func (s *StripService) AssumeStripCoordination(ctx context.Context, session int32, callsign string, position string) error {
	if s.coordRepo == nil {
		return errors.New("coordination repository not configured")
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	// Check for a pending coordination first.
	coordination, err := s.coordRepo.GetByStripID(ctx, session, strip.ID)
	if err == nil {
		// Coordination exists — validate it targets this position.
		if coordination.ToPosition != position {
			// Special case: strip is unowned and this position is next in line.
			// The coordination is stale (e.g. from an ES arrival push to a different position).
			// Delete it and allow direct assumption.
			if (strip.Owner == nil || *strip.Owner == "") && slices.Contains(strip.NextOwners, position) {
				_ = s.coordRepo.Delete(ctx, coordination.ID)

				nextOwners := strip.NextOwners
				index := slices.Index(nextOwners, position)
				if index >= 0 {
					nextOwners = nextOwners[index+1:]
				}

				if err := s.stripRepo.SetNextAndPreviousOwners(ctx, session, callsign, nextOwners, strip.PreviousOwners); err != nil {
					return err
				}

				count, err := s.setOwnerAndReevaluateDuplicateSquawkValidation(ctx, session, callsign, &position, strip.Version)
				if err != nil {
					return err
				}
				if count != 1 {
					return errors.New("failed to set strip owner")
				}
				s.publisher.SendCoordinationAssume(session, callsign, position)
				s.publisher.SendOwnersUpdate(session, callsign, position, nextOwners, strip.PreviousOwners)
				return nil
			}
			return errors.New("cannot assume strip which is not transferred to you")
		}

		if err := s.AcceptCoordination(ctx, session, callsign, position); err != nil {
			return err
		}

		// Missed approach return: if an APP controller assumes an AIRBORNE strip that was
		// handed over by a TWR controller, automatically move it back to FINAL so TWR
		// becomes the next controller again on the next approach.
		if strip.Bay == shared.BAY_AIRBORNE && isMissedApproachReturn(coordination.FromPosition, position) {
			if moveErr := s.MoveToBay(ctx, session, callsign, shared.BAY_FINAL, true); moveErr != nil {
				slog.WarnContext(ctx, "missed approach assume: failed to auto-move strip to FINAL",
					slog.String("callsign", callsign),
					slog.Any("error", moveErr))
			} else {
				s.applyMissedApproachOwnerFix(ctx, session, callsign, position, coordination.FromPosition)
			}
		}

		s.maybeSyncEsCoordinationAcceptance(session, callsign, coordination, position)

		return nil
	}

	// Strip is not owned by anyone and has no pending coordination — assume directly.
	if strip.Owner == nil || *strip.Owner == "" {
		nextOwners := strip.NextOwners
		index := slices.Index(nextOwners, position)
		if index >= 0 {
			nextOwners = nextOwners[index+1:]
		}

		if err := s.stripRepo.SetNextAndPreviousOwners(ctx, session, callsign, nextOwners, strip.PreviousOwners); err != nil {
			return err
		}

		count, err := s.setOwnerAndReevaluateDuplicateSquawkValidation(ctx, session, callsign, &position, strip.Version)
		if err != nil {
			return err
		}
		if count != 1 {
			return errors.New("failed to set strip owner")
		}

		s.publisher.SendCoordinationAssume(session, callsign, position)

		if s.publisher != nil {
			if server := s.publisher.GetServer(); server != nil {
				if err := server.UpdateRouteForStrip(callsign, session, false); err != nil {
					slog.ErrorContext(ctx, "Error updating route after direct assume", slog.String("callsign", callsign), slog.Any("error", err))
				}
				if refreshed, err := s.stripRepo.GetByCallsign(ctx, session, callsign); err == nil {
					nextOwners = refreshed.NextOwners
				}
			}
		}

		s.publisher.SendOwnersUpdate(session, callsign, position, nextOwners, strip.PreviousOwners)
		return nil
	}

	return errors.New("cannot assume strip which is not transferred to you")
}

// ForceAssumeStrip forcibly takes ownership of a strip, overriding any existing owner.
// Unlike AssumeStripCoordination it does not check NextOwners — any controller
// may force-assume any strip regardless of current ownership.
//
// After assuming:
//   - The route is recalculated starting from the new owner.
//   - The displaced owner is removed from previous controllers.
//   - Any controllers that appear in the recalculated next owners are removed from previous controllers.
func (s *StripService) ForceAssumeStrip(ctx context.Context, session int32, callsign string, position string) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	// Delete any stale coordination so it does not block future operations.
	if s.coordRepo != nil {
		if coord, err := s.coordRepo.GetByStripID(ctx, session, strip.ID); err == nil {
			_ = s.coordRepo.Delete(ctx, coord.ID)
		}
	}

	// Trim NextOwners: remove the new owner and all positions before it.
	nextOwners := strip.NextOwners
	index := slices.Index(nextOwners, position)
	isExpectedHandoff := index >= 0
	if isExpectedHandoff {
		nextOwners = nextOwners[index+1:]
	}

	// Build the new previous owners list.
	// Filter out the displaced owner first (avoids duplicates), then — for an
	// expected handoff — append them so they appear as a past controller.
	previousOwners := make([]string, 0, len(strip.PreviousOwners)+1)
	for _, p := range strip.PreviousOwners {
		if strip.Owner == nil || *strip.Owner != p {
			previousOwners = append(previousOwners, p)
		}
	}
	if isExpectedHandoff && strip.Owner != nil && *strip.Owner != "" {
		previousOwners = append(previousOwners, *strip.Owner)
	}

	if err := s.stripRepo.SetNextAndPreviousOwners(ctx, session, callsign, nextOwners, previousOwners); err != nil {
		return err
	}

	count, err := s.setOwnerAndReevaluateDuplicateSquawkValidation(ctx, session, callsign, &position, strip.Version)
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("failed to set strip owner")
	}

	// Recalculate the route starting from the new owner. This updates NextOwners in the
	// DB. We pass sendUpdate=false because we send the owners update ourselves below with
	// the fully cleaned previous owners.
	if s.publisher != nil {
		if server := s.publisher.GetServer(); server != nil {
			_ = server.UpdateRouteForStrip(callsign, session, false)

			// Re-read to pick up the recalculated NextOwners.
			if updated, err := s.stripRepo.GetByCallsign(ctx, session, callsign); err == nil {
				nextOwners = updated.NextOwners
			}
		}
	}

	// Any controller that now appears in NextOwners must be removed from PreviousOwners
	// — they are upcoming controllers, not past ones.
	cleanedPrevious := make([]string, 0, len(previousOwners))
	for _, p := range previousOwners {
		if !slices.Contains(nextOwners, p) {
			cleanedPrevious = append(cleanedPrevious, p)
		}
	}
	if !slices.Equal(cleanedPrevious, previousOwners) {
		if err := s.stripRepo.SetPreviousOwners(ctx, session, callsign, cleanedPrevious); err != nil {
			return err
		}
		previousOwners = cleanedPrevious
	}

	s.publisher.SendCoordinationAssume(session, callsign, position)
	s.publisher.SendOwnersUpdate(session, callsign, position, nextOwners, previousOwners)
	return nil
}

// RejectCoordination rejects a pending coordination transfer.
func (s *StripService) RejectCoordination(ctx context.Context, session int32, callsign string, position string) error {
	if s.coordRepo == nil {
		return errors.New("coordination repository not configured")
	}

	coordination, err := s.coordRepo.GetByStripCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	if coordination.ToPosition != position {
		return errors.New("cannot reject strip which is not transferred to you")
	}

	if err = s.coordRepo.Delete(ctx, coordination.ID); err != nil {
		return err
	}
	s.publisher.SendCoordinationReject(session, callsign, position)
	return nil
}

// CreateTagRequest creates a tag-request coordination, allowing a non-owner to request the strip tag from the current owner.
func (s *StripService) CreateTagRequest(ctx context.Context, session int32, callsign string, requesterPosition string) error {
	if s.publisher == nil {
		return errors.New("frontend hub not configured")
	}
	server := s.publisher.GetServer()
	if server == nil {
		return errors.New("server not configured")
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	if strip.Owner == nil || *strip.Owner == "" {
		return errors.New("cannot request tag of an unowned strip")
	}

	if *strip.Owner == requesterPosition {
		return errors.New("cannot request tag of a strip you already own")
	}

	coordRepo := server.GetCoordinationRepository()

	// Check no active coordination already exists
	if _, err := coordRepo.GetByStripCallsign(ctx, session, callsign); err == nil {
		return errors.New("an active coordination already exists for this strip")
	}

	coord := &internalModels.Coordination{
		Session:      session,
		StripID:      strip.ID,
		FromPosition: *strip.Owner,
		ToPosition:   requesterPosition,
		IsTagRequest: true,
	}

	if err := coordRepo.Create(ctx, coord); err != nil {
		return err
	}

	s.publisher.SendCoordinationTagRequest(session, callsign, *strip.Owner, requesterPosition)
	return nil
}

// AcceptTagRequest allows the strip owner to accept a pending tag request, transferring ownership to the requester.
func (s *StripService) AcceptTagRequest(ctx context.Context, session int32, callsign string, ownerPosition string) error {
	if s.publisher == nil {
		return errors.New("frontend hub not configured")
	}
	server := s.publisher.GetServer()
	if server == nil {
		return errors.New("server not configured")
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	if strip.Owner == nil || *strip.Owner != ownerPosition {
		return errors.New("only the strip owner can accept a tag request")
	}

	coordRepo := server.GetCoordinationRepository()
	coordination, err := coordRepo.GetByStripCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	if !coordination.IsTagRequest {
		return errors.New("no pending tag request for this strip")
	}

	requesterPosition := coordination.ToPosition

	if err := coordRepo.Delete(ctx, coordination.ID); err != nil {
		return err
	}

	// Trim next owners: remove the requester and all positions before it.
	nextOwners := strip.NextOwners
	index := slices.Index(nextOwners, requesterPosition)
	isExpectedHandoff := index >= 0
	if isExpectedHandoff {
		nextOwners = nextOwners[index+1:]
	}

	// Build the new previous owners list.
	// Filter out the displaced owner first (avoids duplicates), then — for an
	// expected handoff — append them so they appear as a past controller.
	previousOwners := make([]string, 0, len(strip.PreviousOwners)+1)
	for _, p := range strip.PreviousOwners {
		if p != ownerPosition {
			previousOwners = append(previousOwners, p)
		}
	}
	if isExpectedHandoff {
		previousOwners = append(previousOwners, ownerPosition)
	}

	if err := s.stripRepo.SetNextAndPreviousOwners(ctx, session, callsign, nextOwners, previousOwners); err != nil {
		return err
	}

	count, err := s.setOwnerAndReevaluateDuplicateSquawkValidation(ctx, session, callsign, &requesterPosition, strip.Version)
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("failed to set strip owner after accepting tag request")
	}

	// Recalculate route from the new owner (same as ForceAssumeStrip).
	if server := s.publisher.GetServer(); server != nil {
		_ = server.UpdateRouteForStrip(callsign, session, false)

		if updated, err := s.stripRepo.GetByCallsign(ctx, session, callsign); err == nil {
			nextOwners = updated.NextOwners
		}
	}

	// Remove from previous owners anyone who now appears in next owners.
	cleanedPrevious := make([]string, 0, len(previousOwners))
	for _, p := range previousOwners {
		if !slices.Contains(nextOwners, p) {
			cleanedPrevious = append(cleanedPrevious, p)
		}
	}
	if !slices.Equal(cleanedPrevious, previousOwners) {
		if err := s.stripRepo.SetPreviousOwners(ctx, session, callsign, cleanedPrevious); err != nil {
			return err
		}
		previousOwners = cleanedPrevious
	}

	s.publisher.SendCoordinationAssume(session, callsign, requesterPosition)
	s.publisher.SendOwnersUpdate(session, callsign, requesterPosition, nextOwners, previousOwners)
	return nil
}

// CancelCoordinationTransfer cancels a pending coordination transfer initiated by the caller.
func (s *StripService) CancelCoordinationTransfer(ctx context.Context, session int32, callsign string, position string) error {
	if s.coordRepo == nil {
		return errors.New("coordination repository not configured")
	}

	coordination, err := s.coordRepo.GetByStripCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	if coordination.FromPosition != position {
		return errors.New("cannot cancel a transfer that you did not initiate")
	}

	if err := s.coordRepo.Delete(ctx, coordination.ID); err != nil {
		return err
	}
	s.publisher.SendCoordinationReject(session, callsign, position)
	return nil
}

// FreeStrip releases ownership of a strip and notifies all frontend clients.
func (s *StripService) FreeStrip(ctx context.Context, session int32, callsign string, position string) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	if strip.Owner == nil || *strip.Owner != position {
		return errors.New("cannot free strip which is not owned by you")
	}

	previousOwners := append(strip.PreviousOwners, position)

	if err := s.stripRepo.SetPreviousOwners(ctx, session, callsign, previousOwners); err != nil {
		return err
	}

	count, err := s.setOwnerAndReevaluateDuplicateSquawkValidation(ctx, session, callsign, nil, strip.Version)
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("failed to set strip owner")
	}

	s.publisher.SendCoordinationFree(session, callsign)
	s.publisher.SendOwnersUpdate(session, callsign, "", strip.NextOwners, previousOwners)
	return nil
}
