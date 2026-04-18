package services

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	"github.com/jackc/pgx/v5"
)

// AutoAssumeForClearedStrip finds the SQ (or fallback DEL) sector owner for the
// session and assigns them as the strip owner. It sends an owners update broadcast
// to all frontend clients. If no SQ/DEL owner is found, the strip is left unowned.
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
	if s.sectorOwnerRepo == nil {
		return nil
	}

	owners, err := s.sectorOwnerRepo.ListBySession(ctx, session)
	if err != nil {
		return err
	}

	sqPosition := ""
	for _, owner := range owners {
		if slices.Contains(owner.Sector, "SQ") {
			sqPosition = owner.Position
			break
		}
	}
	if sqPosition == "" {
		for _, owner := range owners {
			if slices.Contains(owner.Sector, "DEL") {
				sqPosition = owner.Position
				break
			}
		}
	}

	if sqPosition == "" {
		slog.DebugContext(ctx, "No SQ/DEL owner found for auto-assume", slog.String("callsign", callsign))
		return nil
	}

	slog.DebugContext(ctx, "Auto-assuming cleared strip", slog.String("callsign", callsign), slog.String("position", sqPosition))

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	nextOwners, previousOwners := prepareOwnersForAutomaticTransfer(strip, sqPosition, actingPosition)

	if err := s.stripRepo.SetNextAndPreviousOwners(ctx, session, callsign, nextOwners, previousOwners); err != nil {
		return err
	}

	count, err := s.stripRepo.SetOwner(ctx, session, callsign, &sqPosition, strip.Version)
	if err != nil {
		return err
	}

	if count == 1 {
		if s.publisher != nil {
			if server := s.publisher.GetServer(); server != nil {
				if err := server.UpdateRouteForStrip(callsign, session, false); err != nil {
					slog.ErrorContext(ctx, "Error updating route after auto-assume", slog.String("callsign", callsign), slog.Any("error", err))
				}
				if refreshed, err := s.stripRepo.GetByCallsign(ctx, session, callsign); err == nil {
					nextOwners = refreshed.NextOwners
				}
			}
		}
		s.publisher.SendOwnersUpdate(session, callsign, sqPosition, nextOwners, previousOwners)
	}

	return nil
}

func (s *StripService) resolveControllerPositionByCid(ctx context.Context, cid string) (string, error) {
	if cid == "" {
		return "", nil
	}

	controllerRepo := s.controllerRepo
	if controllerRepo == nil && s.publisher != nil {
		if server := s.publisher.GetServer(); server != nil {
			controllerRepo = server.GetControllerRepository()
		}
	}
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

// AutoAssumeForControllerOnline finds all cleared, unowned strips in the session whose
// next owner matches controllerPosition and assigns that controller as the strip owner.
// This is called when a controller comes online so they automatically inherit strips
// that were already cleared and waiting for them.
func (s *StripService) AutoAssumeForControllerOnline(ctx context.Context, session int32, controllerPosition string) error {
	strips, err := s.stripRepo.List(ctx, session)
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

		count, err := s.stripRepo.SetOwner(ctx, session, strip.Callsign, &controllerPosition, strip.Version)
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
			if s.publisher != nil {
				if server := s.publisher.GetServer(); server != nil {
					if err := server.UpdateRouteForStrip(strip.Callsign, session, false); err != nil {
						slog.ErrorContext(ctx, "Error updating route after auto-assume on controller online", slog.String("callsign", strip.Callsign), slog.Any("error", err))
					}
					if refreshed, err := s.stripRepo.GetByCallsign(ctx, session, strip.Callsign); err == nil {
						nextOwners = refreshed.NextOwners
					}
				}
			}
			s.publisher.SendOwnersUpdate(session, strip.Callsign, controllerPosition, nextOwners, strip.PreviousOwners)
		}
	}

	return nil
}
