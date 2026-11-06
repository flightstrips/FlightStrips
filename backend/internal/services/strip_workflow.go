package services

import (
	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
	"fmt"
)

func WithAirportWorkflows(registry *airportworkflow.Registry) StripServiceOption {
	return func(s *StripService) { s.workflows = registry }
}

// workflowFor binds an immutable startup implementation once per session.
// It never decodes configuration, discovers handlers or opens a resource.
func (s *StripService) workflowFor(ctx context.Context, session int32, airport string) (*airportworkflow.Bound, error) {
	if session <= 0 {
		return nil, fmt.Errorf("invalid workflow session")
	}
	if bound, ok := s.sessionWorkflows.Load(session); ok {
		b := bound.(*airportworkflow.Bound)
		if airport != "" && b.Definition().Airport() != airport {
			return nil, fmt.Errorf("workflow airport/session mismatch")
		}
		return b, nil
	}
	reader := s.sessionRepo
	if s.workflowSessions != nil {
		reader = s.workflowSessions
	}
	if reader == nil {
		return nil, fmt.Errorf("session is required to bind workflow")
	}
	model, err := reader.GetByID(ctx, session)
	if err != nil {
		return nil, err
	}
	if model == nil || model.ID != session {
		return nil, fmt.Errorf("invalid workflow session identity")
	}
	if airport != "" && model.Airport != airport {
		return nil, fmt.Errorf("workflow airport/session mismatch")
	}
	airport = model.Airport

	b, ok := s.workflows.Lookup(airport)
	if !ok {
		return nil, fmt.Errorf("no bound workflow for airport %s", airport)
	}
	actual, _ := s.sessionWorkflows.LoadOrStore(session, b)
	b = actual.(*airportworkflow.Bound)
	if b.Definition().Airport() != airport {
		return nil, fmt.Errorf("workflow airport/session mismatch")
	}
	return b, nil
}

func workflowSnapshot(strip *models.Strip) airportworkflow.Snapshot {
	return airportworkflow.SnapshotOf(strip)
}

func workflowValue[T any](p *T) (v T) {
	if p != nil {
		return *p
	}
	return v
}

func (s *StripService) observeWorkflow(ctx context.Context, session int32, airport string, strip *models.Strip, observation airportworkflow.Observation, groundOnline bool) (airportworkflow.Decision, error) {
	b, err := s.workflowFor(ctx, session, airport)
	if err != nil {
		return airportworkflow.Decision{}, err
	}
	return b.Observe(airportworkflow.Context{Session: session, Strip: workflowSnapshot(strip), GroundOnline: groundOnline}, observation)
}

func workflowInitialCFL(bound *airportworkflow.Bound, runway string) (int, bool) {
	return bound.Definition().InitialCFLForRunway(runway)
}

type WorkflowRevision struct {
	StripID, Version int32
	Definition       string
}
type workflowRevisionKey struct{}

func WithWorkflowRevision(ctx context.Context, revision WorkflowRevision) context.Context {
	return context.WithValue(ctx, workflowRevisionKey{}, revision)
}
func validateWorkflowRevision(ctx context.Context, bound *airportworkflow.Bound, session int32, strip *models.Strip) error {
	if strip.Session != 0 && strip.Session != session {
		return fmt.Errorf("strip belongs to another session")
	}
	if revision, ok := ctx.Value(workflowRevisionKey{}).(WorkflowRevision); ok {
		if revision.StripID != strip.ID || revision.Version != strip.Version || revision.Definition != bound.Definition().Revision() {
			return fmt.Errorf("stale workflow revision")
		}
	}
	return nil
}

func workflowSource(ctx context.Context) airportworkflow.Source {
	if _, human := shared.HumanCommand(ctx); human {
		return airportworkflow.HumanSource
	}
	return airportworkflow.AutomaticSource
}

// UsesRevisionedStripState lets the existing publisher retain the bound board's
// concurrency contract for ordinary ES and service field notifications. It
// reuses the session workflow binding; it creates no second state cache.
func (s *StripService) UsesRevisionedStripState(ctx context.Context, session int32) bool {
	bound, err := s.workflowFor(ctx, session, "")
	return err == nil && bound.BoardStatePolicy().Revisioned
}
