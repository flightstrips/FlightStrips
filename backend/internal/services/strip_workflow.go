package services

import (
	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/models"
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

func workflowSnapshot(s *models.Strip) airportworkflow.Snapshot {
	if s == nil {
		return airportworkflow.Snapshot{}
	}
	return airportworkflow.Snapshot{GroundStateKnown: s.State != nil, Exists: true, ID: s.ID, Version: s.Version, Callsign: s.Callsign, Origin: s.Origin, Destination: s.Destination, Bay: s.Bay, GroundState: workflowValue(s.State), Owner: workflowValue(s.Owner), Cleared: s.Cleared, PDCState: s.PdcState, AssignedSquawk: workflowValue(s.AssignedSquawk), TrackingController: s.TrackingController, StartRequested: s.StartReq, Latitude: workflowValue(s.PositionLatitude), Longitude: workflowValue(s.PositionLongitude), Altitude: workflowValue(s.PositionAltitude), EuroscopeSeen: s.EuroscopeSeenAt != nil, Runway: workflowValue(s.Runway), Landed: s.CdmData != nil && s.CdmData.Aldt != nil}
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
