package services

import (
	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	pkg "FlightStrips/pkg/models"
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *StripService) RemoveStripBoardEntry(ctx context.Context, session int32, callsign, cid, airport, position string, revision pkg.StripRevision) error {
	b, strip, err := s.authorizeStripBoard(ctx, session, callsign, cid, airport, position, revision)
	if err != nil {
		return err
	}
	d, err := b.Action(airportworkflow.Context{Session: session, Source: airportworkflow.HumanSource, Now: s.boardTime(), Strip: workflowSnapshot(strip)}, "strip.remove", airportworkflow.StripRemoval{})
	if err != nil {
		return err
	}
	return s.applyStripBoardDecision(ctx, session, strip, d)
}
func (s *StripService) RestoreStripBoardEntry(ctx context.Context, session int32, callsign, cid, airport, position string, revision pkg.StripRevision) error {
	b, strip, err := s.authorizeStripBoard(ctx, session, callsign, cid, airport, position, revision)
	if err != nil {
		return err
	}
	d, err := b.Action(airportworkflow.Context{Session: session, Source: airportworkflow.HumanSource, Now: s.boardTime(), Strip: workflowSnapshot(strip)}, "strip.restore", airportworkflow.StripRestoration{})
	if err != nil {
		return err
	}
	return s.applyStripBoardDecision(ctx, session, strip, d)
}

// AircraftDisconnectDelay lets the existing scoped disconnect worker use a
// bound workflow's explicit delay. EKCH returns no override and keeps its grace.
func (s *StripService) AircraftDisconnectDelay(ctx context.Context, session int32) (time.Duration, bool, error) {
	b, err := s.workflowFor(ctx, session, "")
	if err != nil {
		return 0, false, err
	}
	d, err := b.Lifecycle(airportworkflow.Context{Session: session}, airportworkflow.Lifecycle{Kind: airportworkflow.Removal})
	if err != nil {
		return 0, false, err
	}
	if d.RemovalDelay == nil {
		return 0, false, nil
	}
	return *d.RemovalDelay, true, nil
}

func (s *StripService) HandleRetainedAircraftDisconnect(ctx context.Context, session int32, callsign string) (bool, error) {
	b, err := s.workflowFor(ctx, session, "")
	if err != nil {
		return false, err
	}
	if !b.BoardStatePolicy().Revisioned {
		return false, nil
	}
	return true, s.removeBoardStripAutomatic(ctx, session, callsign, "disconnect")
}
func (s *StripService) removeBoardStripAutomatic(ctx context.Context, session int32, callsign, reason string) error {
	if _, human := shared.HumanCommand(ctx); human {
		return shared.RejectAction("use revisioned strip removal")
	}
	bound, err := s.workflowFor(ctx, session, "")
	if err != nil {
		return err
	}
	clearPresence := func() error {
		if store, available := s.stripReader.(interface {
			ClearEuroscopeSeen(context.Context, int32, string) error
		}); available {
			return store.ClearEuroscopeSeen(ctx, session, callsign)
		}
		return nil
	}
	for range 2 {
		strip, err := s.stripReader.GetByCallsign(ctx, session, callsign)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if strip.BoardState == nil {
			return clearPresence()
		}
		d, err := bound.Lifecycle(airportworkflow.Context{Session: session, Source: airportworkflow.AutomaticSource, Now: s.boardTime(), Strip: workflowSnapshot(strip)}, airportworkflow.Lifecycle{Kind: airportworkflow.Removal, Reason: reason})
		if err != nil {
			return err
		}
		if d.Visibility == nil {
			return clearPresence()
		}
		state := *strip.BoardState
		if d.Visibility.Visible {
			state.RemovedAt = nil
			state.RemovalReason = ""
		} else {
			if state.RemovedAt != nil {
				return clearPresence()
			}
			now := s.boardTime()
			state.RemovedAt = &now
			state.RemovalReason = d.Visibility.Reason
		}
		ok, err := s.commitStripVisibility(ctx, session, strip, &state)
		if err != nil {
			return err
		}
		if ok {
			return clearPresence()
		}
	}
	return shared.RejectAction("strip board strip changed during removal; retry")
}
func (s *StripService) reconcileStripVisibility(ctx context.Context, session int32, airport, callsign string, o airportworkflow.Observation) error {
	b, err := s.workflowFor(ctx, session, airport)
	if err != nil {
		return err
	}
	if !b.BoardStatePolicy().Revisioned {
		return nil
	}
	for range 2 {
		strip, err := s.stripReader.GetByCallsign(ctx, session, callsign)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		d, err := b.Observe(airportworkflow.Context{Session: session, Now: s.boardTime(), Strip: workflowSnapshot(strip)}, o)
		if err != nil {
			return err
		}
		if d.Visibility == nil || strip.BoardState == nil {
			return nil
		}
		state := *strip.BoardState
		if d.Visibility.Visible {
			state.RemovedAt = nil
			state.RemovalReason = ""
		} else {
			now := s.boardTime()
			state.RemovedAt = &now
			state.RemovalReason = d.Visibility.Reason
		}
		ok, err := s.commitStripVisibility(ctx, session, strip, &state)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
	}
	return shared.RejectAction("strip board strip changed during lifecycle reconciliation; retry")
}
func (s *StripService) commitStripVisibility(ctx context.Context, session int32, strip *models.Strip, state *models.StripBoardState) (bool, error) {
	store, ok := s.stripReader.(stripBoardStore)
	if !ok {
		return false, shared.RejectAction("strip board persistence unavailable")
	}
	updated, err := store.UpdateStripBoardState(ctx, session, strip.ID, strip.Version, state)
	if err != nil || !updated {
		return updated, err
	}
	strip.BoardState = state
	strip.Version++
	s.cacheStrip(ctx, strip)
	if sync := shared.GetSyncState(ctx); sync != nil {
		if sync.ExistingStrips != nil {
			sync.ExistingStrips[strip.Callsign] = strip
		}
		sync.MarkStripUpdate(strip.Callsign)
	} else {
		shared.PublishStripUpdate(ctx, s.publisher, session, strip.Callsign)
	}
	return true, nil
}
