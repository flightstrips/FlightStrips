package services

import (
	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/euroscope"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

// scheduleStandAutoRemoval deletes the strip after its configured STAND delay,
// provided the same strip is still in BAY_STAND when the timer fires.
func (s *StripService) scheduleStandAutoRemoval(session int32, callsign string) {
	ctx := context.Background()
	bound, err := s.workflowFor(ctx, session, "")
	if err != nil {
		slog.ErrorContext(ctx, "Failed to schedule stand strip removal", slog.Any("error", err), slog.String("callsign", callsign))
		return
	}
	initial, err := s.stripReader.GetByCallsign(ctx, session, callsign)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to schedule stand strip removal", slog.Any("error", err), slog.String("callsign", callsign))
		return
	}
	policy, err := bound.Lifecycle(airportworkflow.Context{Session: session, Strip: workflowSnapshot(initial)}, airportworkflow.Lifecycle{Kind: airportworkflow.Admission})
	if err != nil {
		slog.ErrorContext(ctx, "Failed to schedule stand strip removal", slog.Any("error", err), slog.String("callsign", callsign))
		return
	}
	if policy.StandRemovalDelay <= 0 {
		return
	}
	id := initial.ID
	go func() {
		time.Sleep(policy.StandRemovalDelay)
		s.removeExpiredStandStrip(ctx, session, callsign, id, bound)
	}()
}

func (s *StripService) removeExpiredStandStrip(ctx context.Context, session int32, callsign string, id int32, bound *airportworkflow.Bound) {
	strip, err := s.stripReader.GetByCallsign(ctx, session, callsign)
	if errors.Is(err, pgx.ErrNoRows) {
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "Failed to load stand strip for removal", slog.Any("error", err), slog.String("callsign", callsign))
		return
	}
	if strip.ID != id {
		return
	}
	decision, err := bound.Lifecycle(airportworkflow.Context{Session: session, Strip: workflowSnapshot(strip)}, airportworkflow.Lifecycle{Kind: airportworkflow.StandExpiry})
	if err != nil {
		slog.ErrorContext(ctx, "Failed to evaluate stand strip removal", slog.Any("error", err), slog.String("callsign", callsign))
		return
	}
	if !decision.Remove {
		return
	}
	if err := s.DeleteStrip(ctx, session, callsign); err != nil {
		slog.ErrorContext(ctx, "Failed to remove expired stand strip", slog.Any("error", err), slog.String("callsign", callsign))
	}
}

// ClearStrip moves strip to cleared bay and notifies EuroScope to set cleared flag
func (s *StripService) ClearStrip(ctx context.Context, session int32, callsign string, cid string) error {
	if err := shared.ValidateCommandAuthority(ctx, session); err != nil {
		return err
	}
	bound, err := s.workflowFor(ctx, session, "")
	if err != nil {
		return err
	}
	strip, err := s.stripReader.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}
	decision, err := bound.Action(airportworkflow.Context{Session: session, Source: workflowSource(ctx), Strip: workflowSnapshot(strip)}, "strip.clearance", airportworkflow.Clearance{Cleared: true})
	if err != nil {
		return err
	}
	if decision.Bay == nil || decision.Cleared == nil {
		return fmt.Errorf("clearance workflow omitted fields")
	}
	bay, cleared := *decision.Bay, *decision.Cleared
	if err := s.MoveToBay(ctx, session, callsign, bay, true); err != nil {
		return fmt.Errorf("failed to move strip to cleared bay: %w", err)
	}

	if _, err := s.fieldStore.UpdateClearedFlag(ctx, session, callsign, cleared, bay, nil); err != nil {
		slog.ErrorContext(ctx, "ClearStrip: failed to update cleared flag", slog.Any("error", err))
	}

	if s.esCommander != nil {
		if dispatchErr := shared.DispatchCommand(ctx, session, euroscope.ClearedFlagEvent{Callsign: callsign, Cleared: cleared}, func() { s.esCommander.SendClearedFlag(session, cid, callsign, cleared) }); dispatchErr != nil {
			return dispatchErr
		}
	}

	return nil
}

// UnclearStrip moves strip back to not-cleared bay and notifies EuroScope to clear the cleared flag
func (s *StripService) UnclearStrip(ctx context.Context, session int32, callsign string, cid string) error {
	if err := shared.ValidateCommandAuthority(ctx, session); err != nil {
		return err
	}
	bound, err := s.workflowFor(ctx, session, "")
	if err != nil {
		return err
	}
	strip, err := s.stripReader.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}
	decision, err := bound.Action(airportworkflow.Context{Session: session, Source: workflowSource(ctx), Strip: workflowSnapshot(strip)}, "strip.clearance", airportworkflow.Clearance{Cleared: false})
	if err != nil {
		return err
	}
	if decision.Bay == nil || decision.Cleared == nil {
		return fmt.Errorf("clearance workflow omitted fields")
	}
	bay, cleared := *decision.Bay, *decision.Cleared
	if err := s.MoveToBay(ctx, session, callsign, bay, true); err != nil {
		return fmt.Errorf("failed to move strip to not-cleared bay: %w", err)
	}

	if _, err := s.fieldStore.UpdateClearedFlag(ctx, session, callsign, cleared, bay, nil); err != nil {
		slog.ErrorContext(ctx, "UnclearStrip: failed to update cleared flag", slog.Any("error", err))
	}

	if err := s.clearOwnerForNotCleared(ctx, session, callsign); err != nil {
		return err
	}

	if s.esCommander != nil {
		if dispatchErr := shared.DispatchCommand(ctx, session, euroscope.ClearedFlagEvent{Callsign: callsign, Cleared: cleared}, func() { s.esCommander.SendClearedFlag(session, cid, callsign, cleared) }); dispatchErr != nil {
			return dispatchErr
		}
	}

	return nil
}

func (s *StripService) clearOwnerForNotCleared(ctx context.Context, session int32, callsign string) error {
	strip, err := s.syncStripByCallsign(ctx, session, callsign)
	if err != nil {
		return fmt.Errorf("failed to get strip for owner reset: %w", err)
	}

	if err := s.ownerStore.SetPreviousOwners(ctx, session, callsign, []string{}); err != nil {
		return fmt.Errorf("failed to persist previous owners: %w", err)
	}
	if syncState := shared.GetSyncState(ctx); syncState != nil && syncState.ExistingStrips != nil {
		if existing := syncState.ExistingStrips[callsign]; existing != nil {
			existing.PreviousOwners = []string{}
		}
	}

	if strip.Owner != nil && *strip.Owner != "" {
		count, err := s.ownerStore.SetOwner(ctx, session, callsign, nil, strip.Version)
		if err != nil {
			return fmt.Errorf("failed to clear strip owner: %w", err)
		}
		if count != 1 {
			return fmt.Errorf("failed to clear strip owner")
		}
		if syncState := shared.GetSyncState(ctx); syncState != nil && syncState.ExistingStrips != nil {
			if existing := syncState.ExistingStrips[callsign]; existing != nil {
				existing.Owner = nil
				existing.NextControllerSector = nil
				existing.Version++
			}
		}
	}

	if err := s.recalculateRouteForStrip(ctx, session, callsign); err != nil {
		return fmt.Errorf("failed to recalculate route after clearing owner: %w", err)
	}

	if s.publisher != nil {
		refreshedStrip, err := s.syncStripByCallsign(ctx, session, callsign)
		if err != nil {
			return fmt.Errorf("failed to reload strip after owner reset: %w", err)
		}
		s.publisher.SendOwnersUpdate(session, callsign, "", refreshedStrip.NextOwners, refreshedStrip.PreviousOwners, nil)
	}

	return nil
}

// DeleteStrip removes a strip from the database and notifies the frontend.
func (s *StripService) DeleteStrip(ctx context.Context, session int32, callsign string) error {
	bound, err := s.workflowFor(ctx, session, "")
	if err != nil {
		return err
	}
	if bound.BoardStatePolicy().Revisioned {
		return s.removeBoardStripAutomatic(ctx, session, callsign, "disconnect")
	}

	decision, err := bound.Lifecycle(airportworkflow.Context{Session: session}, airportworkflow.Lifecycle{Kind: airportworkflow.Removal})
	if err != nil {
		return err
	}
	if !decision.Remove {
		return nil
	}
	err = s.lifecycleStore.Delete(ctx, session, callsign)
	s.publisher.SendAircraftDisconnect(session, callsign)
	if err != nil {
		return err
	}
	return s.RemoveEuroScopeAMANStrip(ctx, session, callsign)
}
