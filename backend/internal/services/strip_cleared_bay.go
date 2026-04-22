package services

import (
	"FlightStrips/internal/shared"
	"context"
	"fmt"
	"log/slog"
	"time"
)

// scheduleStandAutoHide starts a background goroutine that moves the strip to
// BAY_HIDDEN after a 240-second delay, provided the strip is still in BAY_STAND
// when the timer fires.
func (s *StripService) scheduleStandAutoHide(session int32, callsign string) {
	go func() {
		time.Sleep(240 * time.Second)

		ctx := context.Background()

		strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
		if err != nil {
			// Strip was deleted while we were waiting — nothing to do.
			slog.DebugContext(ctx, "Auto-hide from STAND: strip not found, skipping",
				slog.String("callsign", callsign),
				slog.Int("session", int(session)))
			return
		}

		if strip.Bay != shared.BAY_STAND {
			// Strip was moved to a different bay before the timer fired — do not override.
			slog.DebugContext(ctx, "Auto-hide from STAND: strip already moved, skipping",
				slog.String("callsign", callsign),
				slog.String("current_bay", strip.Bay))
			return
		}

		slog.InfoContext(ctx, "Auto-hiding arrival strip from STAND bay after 15 s",
			slog.String("callsign", callsign),
			slog.Int("session", int(session)))

		if err := s.MoveToBay(ctx, session, callsign, shared.BAY_HIDDEN, true); err != nil {
			slog.ErrorContext(ctx, "Auto-hide from STAND: failed to move strip to HIDDEN",
				slog.String("callsign", callsign),
				slog.Int("session", int(session)),
				slog.Any("error", err))
		}
	}()
}

// ClearStrip moves strip to cleared bay and notifies EuroScope to set cleared flag
func (s *StripService) ClearStrip(ctx context.Context, session int32, callsign string, cid string) error {
	if err := s.MoveToBay(ctx, session, callsign, shared.BAY_CLEARED, true); err != nil {
		return fmt.Errorf("failed to move strip to cleared bay: %w", err)
	}

	if _, err := s.stripRepo.UpdateClearedFlag(ctx, session, callsign, true, shared.BAY_CLEARED, nil); err != nil {
		slog.ErrorContext(ctx, "ClearStrip: failed to update cleared flag", slog.Any("error", err))
	}

	if s.esCommander != nil {
		s.esCommander.SendClearedFlag(session, cid, callsign, true)
	}

	return nil
}

// UnclearStrip moves strip back to not-cleared bay and notifies EuroScope to clear the cleared flag
func (s *StripService) UnclearStrip(ctx context.Context, session int32, callsign string, cid string) error {
	if err := s.MoveToBay(ctx, session, callsign, shared.BAY_NOT_CLEARED, true); err != nil {
		return fmt.Errorf("failed to move strip to not-cleared bay: %w", err)
	}

	if _, err := s.stripRepo.UpdateClearedFlag(ctx, session, callsign, false, shared.BAY_NOT_CLEARED, nil); err != nil {
		slog.ErrorContext(ctx, "UnclearStrip: failed to update cleared flag", slog.Any("error", err))
	}

	if err := s.clearOwnerForNotCleared(ctx, session, callsign); err != nil {
		return err
	}

	if s.esCommander != nil {
		s.esCommander.SendClearedFlag(session, cid, callsign, false)
	}

	return nil
}

func (s *StripService) clearOwnerForNotCleared(ctx context.Context, session int32, callsign string) error {
	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return fmt.Errorf("failed to get strip for owner reset: %w", err)
	}

	if err := s.stripRepo.SetPreviousOwners(ctx, session, callsign, []string{}); err != nil {
		return fmt.Errorf("failed to persist previous owners: %w", err)
	}

	if strip.Owner != nil && *strip.Owner != "" {
		count, err := s.stripRepo.SetOwner(ctx, session, callsign, nil, strip.Version)
		if err != nil {
			return fmt.Errorf("failed to clear strip owner: %w", err)
		}
		if count != 1 {
			return fmt.Errorf("failed to clear strip owner")
		}
	}

	if err := s.recalculateRouteForStrip(session, callsign); err != nil {
		return fmt.Errorf("failed to recalculate route after clearing owner: %w", err)
	}

	if s.publisher != nil {
		refreshedStrip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
		if err != nil {
			return fmt.Errorf("failed to reload strip after owner reset: %w", err)
		}
		s.publisher.SendOwnersUpdate(session, callsign, "", refreshedStrip.NextOwners, refreshedStrip.PreviousOwners)
	}

	return nil
}

// DeleteStrip removes a strip from the database and notifies the frontend.
func (s *StripService) DeleteStrip(ctx context.Context, session int32, callsign string) error {
	err := s.stripRepo.Delete(ctx, session, callsign)
	s.publisher.SendAircraftDisconnect(session, callsign)
	return err
}
