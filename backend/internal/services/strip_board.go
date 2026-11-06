package services

import (
	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	fe "FlightStrips/pkg/events/frontend"
	pkg "FlightStrips/pkg/models"
	"context"
)

type stripBoardStore interface {
	UpdateStripBoardState(context.Context, int32, int32, int32, *models.StripBoardState) (bool, error)
}

func (s *StripService) reorderBoardStrip(ctx context.Context, session int32, strip *models.Strip, after *fe.StripRef, bay string) error {
	r, ok := ctx.Value(workflowRevisionKey{}).(WorkflowRevision)
	if !ok {
		return shared.RejectAction("strip board ordering revision required")
	}
	b, err := s.workflowFor(ctx, session, "")
	if err != nil {
		return err
	}
	if err = validateWorkflowRevision(ctx, b, session, strip); err != nil {
		return err
	}
	if _, ok := shared.HumanCommand(ctx); !ok {
		return shared.RejectAction("exact own plugin binding required")
	}
	if strip.BoardState.RemovedAt != nil {
		return shared.RejectAction("removed strip cannot be reordered")
	}
	var predecessor *string
	if after != nil {
		if after.Kind != "flight" || after.Callsign == nil || *after.Callsign == strip.Callsign {
			return shared.RejectAction("invalid strip predecessor")
		}
		predecessor = after.Callsign
	}
	store, ok := s.stripReader.(interface {
		ReorderStripBoard(context.Context, int32, int32, int32, string, *string) error
	})
	if !ok {
		return shared.RejectAction("strip board ordering unavailable")
	}
	if err = shared.ValidateCommandAuthority(ctx, session); err != nil {
		return err
	}
	if err = store.ReorderStripBoard(ctx, session, strip.ID, r.Version, bay, predecessor); err != nil {
		return err
	}
	strips, err := s.stripReader.List(ctx, session)
	if err != nil {
		return err
	}
	for _, v := range strips {
		if v.Bay == bay {
			shared.PublishStripUpdate(ctx, s.publisher, session, v.Callsign)
		}
	}
	return nil
}

func (s *StripService) authorizeStripBoard(ctx context.Context, session int32, callsign, cid, airport, position string, revision pkg.StripRevision) (*airportworkflow.Bound, *models.Strip, error) {
	// Shared board-only writes still require a live exact own-plugin binding.
	if _, ok := shared.HumanCommand(ctx); !ok {
		return nil, nil, shared.RejectAction("exact own plugin binding required")
	}
	if _, err := shared.EnsureHumanCommand(ctx, s.esCommander, session, cid); err != nil {
		return nil, nil, err
	}
	b, err := s.workflowFor(ctx, session, airport)
	if err != nil {
		return nil, nil, err
	}
	if !b.BoardStatePolicy().Revisioned {
		return nil, nil, shared.RejectAction("strip board action unavailable")
	}
	strip, err := s.stripReader.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return nil, nil, err
	}
	if strip.BoardState == nil {
		return nil, nil, shared.RejectAction("strip is not admitted to this board")
	}
	if strip.ID != revision.StripID || strip.Version != revision.Version || b.Definition().Revision() != revision.Definition || strip.Session != session {
		return nil, nil, shared.RejectAction("stale strip revision")
	}
	if b.Definition().UsesStripOwnership() && strip.Owner != nil && *strip.Owner != "" && (position == "" || position != *strip.Owner) {
		return nil, nil, shared.RejectAction("not authorized: strip is owned by another controller")
	}
	return b, strip, nil
}

func (s *StripService) AnnotateStrip(ctx context.Context, session int32, callsign, cid, airport, position string, revision pkg.StripRevision, edit airportworkflow.StripAnnotation) error {
	b, strip, err := s.authorizeStripBoard(ctx, session, callsign, cid, airport, position, revision)
	if err != nil {
		return err
	}
	if strip.BoardState != nil && strip.BoardState.RemovedAt != nil {
		return shared.RejectAction("restore the removed strip before editing")
	}
	d, err := b.Action(airportworkflow.Context{Session: session, Source: airportworkflow.HumanSource, Strip: workflowSnapshot(strip), Now: s.boardTime()}, "strip.annotate", edit)
	if err != nil {
		return err
	}
	return s.applyStripBoardDecision(ctx, session, strip, d)
}
func (s *StripService) persistStripBoardState(ctx context.Context, session int32, strip *models.Strip, state *models.StripBoardState) error {
	store, ok := s.stripReader.(stripBoardStore)
	if !ok {
		return shared.RejectAction("strip board persistence unavailable")
	}
	if err := shared.ValidateCommandAuthority(ctx, session); err != nil {
		return err
	}
	updated, err := store.UpdateStripBoardState(ctx, session, strip.ID, strip.Version, state)
	if err != nil {
		return err
	}
	if !updated {
		return shared.RejectAction("strip board strip changed; retry from current board")
	}
	shared.PublishStripUpdate(ctx, s.publisher, session, strip.Callsign)
	return nil
}

// A planning row may predate its first ES observation. Mark its board state before
// publication, without replacing annotations or treating that planning row as a
// separate flight identity.
func (s *StripService) initializeStripBoardState(ctx context.Context, session int32, airport, callsign string) error {
	b, err := s.workflowFor(ctx, session, airport)
	if err != nil {
		return err
	}
	if !b.BoardStatePolicy().Revisioned {
		return nil
	}
	store, ok := s.stripReader.(stripBoardStore)
	if !ok {
		return shared.RejectAction("strip board persistence unavailable")
	}
	for range 2 {
		strip, err := s.stripReader.GetByCallsign(ctx, session, callsign)
		if err != nil {
			return err
		}
		if strip.BoardState != nil {
			return nil
		}
		created := s.boardTime()
		updated, err := store.UpdateStripBoardState(ctx, session, strip.ID, strip.Version, &models.StripBoardState{CreatedAt: &created})
		if err != nil {
			return err
		}
		if updated {
			return nil
		}
	}
	return shared.RejectAction("strip board strip changed during admission; retry")
}

// The shared executor applies concrete board-state effects. It does not route
// annotation/admission/recovery through movement authorization or early returns.
func (s *StripService) applyStripBoardDecision(ctx context.Context, session int32, strip *models.Strip, d airportworkflow.Decision) error {
	if d.Annotations == nil && d.Visibility == nil {
		return nil
	}
	state := models.StripBoardState{}
	if strip.BoardState != nil {
		state = *strip.BoardState
	}
	if d.Annotations != nil {
		state.Annotations = *d.Annotations
	}
	if d.Visibility != nil {
		if d.Visibility.Visible {
			state.RemovedAt = nil
			state.RemovalReason = ""
		} else {
			now := s.boardTime()
			state.RemovedAt = &now
			state.RemovalReason = d.Visibility.Reason
		}
	}
	return s.persistStripBoardState(ctx, session, strip, &state)
}
