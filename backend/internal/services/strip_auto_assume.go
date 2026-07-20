package services

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"github.com/jackc/pgx/v5"
)

// AutoAssumeForClearedStrip resolves the configured clearance handover target for a
// cleared departure strip and assigns it directly as the strip owner. It sends an
// owners update broadcast to all frontend clients. If no target is sensed, the
// strip is left unchanged.
func (s *StripService) AutoAssumeForClearedStrip(ctx context.Context, session int32, callsign string) error {
	return s.autoAssumeForClearedStrip(ctx, session, callsign, "")
}

func (s *StripService) AutoAssumeForClearedStripByCid(ctx context.Context, session int32, callsign string, cid string) error {
	actingPosition, err := s.resolveControllerPositionByCid(ctx, cid)
	if err != nil {
		return err
	}

	return s.autoAssumeForClearedStrip(ctx, session, callsign, actingPosition)
}

func (s *StripService) autoAssumeForClearedStrip(ctx context.Context, session int32, callsign string, actingPosition string) error {
	if s.sectorOwnerRepo == nil && s.clearedOwnerResolver == nil {
		return nil
	}

	strip, err := s.syncStripByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}
	if !s.shouldAutoAssumeClearedDeparture(ctx, session, strip) {
		slog.DebugContext(ctx, "Skipping auto-assume for cleared non-departure strip",
			slog.String("callsign", callsign),
			slog.String("origin", strip.Origin),
			slog.String("destination", strip.Destination),
			slog.String("bay", strip.Bay),
		)
		return nil
	}

	targetPosition, resolved, err := s.resolveClearedStripOwner(ctx, session, strip)
	if err != nil {
		return err
	}
	if !resolved {
		slog.DebugContext(ctx, "No clearance handover target sensed for auto-assume", slog.String("callsign", callsign))
		return nil
	}

	slog.DebugContext(ctx, "Auto-assuming cleared departure strip", slog.String("callsign", callsign), slog.String("position", targetPosition))

	nextOwners, previousOwners := prepareOwnersForAutomaticTransfer(strip, targetPosition, actingPosition)

	if err := s.ownerStore.SetNextAndPreviousOwners(ctx, session, callsign, nextOwners, previousOwners); err != nil {
		return err
	}

	count, err := s.setOwnerAndReevaluateDuplicateSquawkValidation(ctx, session, callsign, &targetPosition, strip.Version)
	if err != nil {
		return err
	}

	if count == 1 {
		resolvedOwner := targetPosition
		strip.Owner = &resolvedOwner
		strip.NextOwners = slices.Clone(nextOwners)
		strip.PreviousOwners = slices.Clone(previousOwners)

		var nextDisplay *models.NextDisplay
		if routeRecalculator := s.getRouteRecalculator(); routeRecalculator != nil {
			if err := routeRecalculator.UpdateRouteForStripContext(ctx, callsign, session, false); err != nil {
				slog.ErrorContext(ctx, "Error updating route after auto-assume", slog.String("callsign", callsign), slog.Any("error", err))
			}
			if refreshed, err := s.syncStripByCallsign(ctx, session, callsign); err == nil {
				nextOwners = refreshed.NextOwners
				nextDisplay = refreshed.NextDisplay
				if s.routeDisplayComputer != nil {
					if computed, err := s.routeDisplayComputer.ComputeNextDisplayForStripContext(ctx, refreshed, session); err == nil {
						nextDisplay = computed
					} else {
						slog.ErrorContext(ctx, "Error computing route display after auto-assume",
							slog.String("callsign", callsign),
							slog.Any("error", err))
					}
				}
			}
		}
		s.publisher.SendOwnersUpdate(session, callsign, targetPosition, nextOwners, previousOwners, nextDisplay)
	}

	return nil
}

func (s *StripService) resolveClearedStripOwner(ctx context.Context, session int32, strip *models.Strip) (string, bool, error) {
	if s.clearedOwnerResolver != nil {
		return s.clearedOwnerResolver.ResolveClearedStripOwnerContext(ctx, strip, session)
	}

	// Preserve the legacy fallback for isolated service users that do not wire the
	// server resolver (primarily narrow unit tests and test tools).
	owners, err := s.syncSectorOwners(ctx, session)
	if err != nil {
		return "", false, err
	}
	for _, sector := range []string{"SQ", "DEL"} {
		for _, owner := range owners {
			if slices.Contains(owner.Sector, sector) {
				return owner.Position, true, nil
			}
		}
	}
	return "", false, nil
}

func (s *StripService) resolveControllerPositionByCid(ctx context.Context, cid string) (string, error) {
	if cid == "" {
		return "", nil
	}

	controllerRepo := s.getControllerRepository()
	if controllerRepo == nil {
		return "", nil
	}

	controller, err := controllerRepo.GetByCid(ctx, cid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}

	return controller.Position, nil
}

func (s *StripService) shouldAutoAssumeClearedDeparture(ctx context.Context, session int32, strip *models.Strip) bool {
	if strip == nil {
		return false
	}
	if shared.IsArrivalBay(strip.Bay) {
		return false
	}
	sessionRepo := s.getSessionRepository()
	if sessionRepo == nil {
		return true
	}
	sessionModel := s.syncSession(ctx, session)
	if sessionModel == nil {
		var err error
		sessionModel, err = sessionRepo.GetByID(ctx, session)
		if err != nil {
			slog.WarnContext(ctx, "Failed to resolve session airport for cleared-strip auto-assume; falling back to bay classification",
				slog.String("callsign", strip.Callsign),
				slog.Any("error", err),
			)
			return true
		}
	}

	airport := strings.TrimSpace(sessionModel.Airport)
	if airport == "" {
		return true
	}

	if strings.EqualFold(strings.TrimSpace(strip.Destination), airport) {
		return false
	}

	origin := strings.TrimSpace(strip.Origin)
	if origin == "" {
		return true
	}

	return strings.EqualFold(origin, airport)
}

// AutoAssumeForControllerOnline finds all cleared, unowned strips in the session whose
// next owner matches controllerPosition and assigns that controller as the strip owner.
// This is called when a controller comes online so they automatically inherit strips
// that were already cleared and waiting for them.
func (s *StripService) AutoAssumeForControllerOnline(ctx context.Context, session int32, controllerPosition string) error {
	strips, err := s.syncStrips(ctx, session)
	if err != nil {
		return err
	}

	for _, strip := range strips {
		if !strip.Cleared {
			continue
		}
		if strip.Owner != nil {
			continue
		}
		if len(strip.NextOwners) == 0 || strip.NextOwners[0] != controllerPosition {
			continue
		}
		if !s.shouldAutoAssumeClearedDeparture(ctx, session, strip) {
			slog.DebugContext(ctx, "Skipping controller-online auto-assume for cleared non-departure strip",
				slog.String("callsign", strip.Callsign),
				slog.String("origin", strip.Origin),
				slog.String("destination", strip.Destination),
				slog.String("bay", strip.Bay),
				slog.String("position", controllerPosition),
			)
			continue
		}

		count, err := s.setOwnerAndReevaluateDuplicateSquawkValidation(ctx, session, strip.Callsign, &controllerPosition, strip.Version)
		if err != nil {
			slog.ErrorContext(ctx, "AutoAssumeForControllerOnline: SetOwner failed",
				slog.String("callsign", strip.Callsign),
				slog.Any("error", err))
			continue
		}

		if count == 1 {
			slog.DebugContext(ctx, "Auto-assumed strip on controller online",
				slog.String("callsign", strip.Callsign),
				slog.String("position", controllerPosition))
			nextOwners := strip.NextOwners
			if routeRecalculator := s.getRouteRecalculator(); routeRecalculator != nil {
				if err := routeRecalculator.UpdateRouteForStripContext(ctx, strip.Callsign, session, false); err != nil {
					slog.ErrorContext(ctx, "Error updating route after auto-assume on controller online", slog.String("callsign", strip.Callsign), slog.Any("error", err))
				}
				if refreshed, err := s.syncStripByCallsign(ctx, session, strip.Callsign); err == nil {
					nextOwners = refreshed.NextOwners
				}
			}
			s.publisher.SendOwnersUpdate(session, strip.Callsign, controllerPosition, nextOwners, strip.PreviousOwners, nil)
		}
	}

	return nil
}

func (s *StripService) syncStripByCallsign(ctx context.Context, session int32, callsign string) (*models.Strip, error) {
	if syncState := shared.GetSyncState(ctx); syncState != nil && syncState.ExistingStrips != nil {
		if strip := syncState.ExistingStrips[callsign]; strip != nil {
			return strip, nil
		}
		return nil, pgx.ErrNoRows
	}
	return s.stripReader.GetByCallsign(ctx, session, callsign)
}

func (s *StripService) syncStrips(ctx context.Context, session int32) ([]*models.Strip, error) {
	if syncState := shared.GetSyncState(ctx); syncState != nil && syncState.ExistingStrips != nil {
		strips := make([]*models.Strip, 0, len(syncState.ExistingStrips))
		for _, strip := range syncState.ExistingStrips {
			strips = append(strips, strip)
		}
		return strips, nil
	}
	return s.stripReader.List(ctx, session)
}

func (s *StripService) syncSession(ctx context.Context, session int32) *models.Session {
	if syncState := shared.GetSyncState(ctx); syncState != nil && syncState.Session != nil && syncState.Session.ID == session {
		return syncState.Session
	}
	return nil
}

func (s *StripService) syncSectorOwners(ctx context.Context, session int32) ([]*models.SectorOwner, error) {
	if syncState := shared.GetSyncState(ctx); syncState != nil && syncState.SectorOwners != nil {
		owners := make([]*models.SectorOwner, 0, len(syncState.SectorOwners))
		for _, owner := range syncState.SectorOwners {
			owners = append(owners, owner)
		}
		return owners, nil
	}

	owners, err := s.sectorOwnerRepo.ListBySession(ctx, session)
	if err != nil {
		return nil, err
	}
	if syncState := shared.GetSyncState(ctx); syncState != nil {
		syncState.SectorOwners = make(map[string]*models.SectorOwner, len(owners))
		for _, owner := range owners {
			syncState.SectorOwners[owner.Position] = owner
		}
	}
	return owners, nil
}
