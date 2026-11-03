package services

import (
	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events/euroscope"
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

type workflowFixtureHandler struct{ admit bool }

func (workflowFixtureHandler) Observe(c airportworkflow.Context, _ airportworkflow.Observation) (airportworkflow.Decision, error) {
	bay := c.Strip.Bay
	return airportworkflow.Decision{Bay: &bay}, nil
}
func (h workflowFixtureHandler) Lifecycle(airportworkflow.Context, airportworkflow.Lifecycle) (airportworkflow.LifecycleDecision, error) {
	return airportworkflow.LifecycleDecision{Admit: h.admit}, nil
}
func (workflowFixtureHandler) Coordinate(airportworkflow.Context, airportworkflow.Coordination) (airportworkflow.CoordinationDecision, error) {
	return airportworkflow.CoordinationDecision{}, nil
}

func fixtureWorkflow(t *testing.T, h airportworkflow.Handler, actions ...airportworkflow.ActionDescriptor) *airportworkflow.Bound {
	t.Helper()
	original, _ := testWorkflows.Lookup("EKCH")
	b, err := airportworkflow.Bind(original.Definition(), airportworkflow.Descriptor{ID: "ekch/v1", Bind: func(*config.AirportDefinition) (airportworkflow.Handler, []airportworkflow.ActionDescriptor, error) {
		return h, actions, nil
	}})
	require.NoError(t, err)
	return b
}

func TestWorkflowExecutesFieldOnlyActionAndRejectsInvalidIntent(t *testing.T) {
	f := newFrontendMoveFixture(&models.Strip{Callsign: "SAS123", Origin: "EKCH", Destination: "EGLL", Bay: shared.BAY_CLEARED, PdcState: "REQUESTED"})
	type request struct{ State string }
	b := fixtureWorkflow(t, workflowFixtureHandler{true}, airportworkflow.Action("custom.state", func(_ airportworkflow.Context, r request) (airportworkflow.Decision, error) {
		return airportworkflow.Decision{GroundState: &r.State}, nil
	}))
	f.svc.sessionWorkflows.Store(int32(1), b)
	require.NoError(t, f.svc.ExecuteWorkflowAction(f.ctx, 1, "SAS123", "own", "EKCH", "GND", "custom.state", request{"LINEUP"}))
	require.Equal(t, []string{"LINEUP"}, f.updateGroundStates)
	require.Empty(t, f.updateBayTargets, "field-only action must preserve ordering")
	require.Empty(t, f.pdcSpy.confirmCalls, "field-only action must not confirm a PDC clearance")
	require.Error(t, f.svc.ExecuteWorkflowAction(f.ctx, 1, "SAS123", "own", "EKCH", "GND", "custom.state", request{"BOGUS"}))
	require.Len(t, f.updateGroundStates, 1, "invalid complete decision must not mutate")
}

func TestWorkflowConsumesRefreshedDecisionAfterProviderPreparation(t *testing.T) {
	f := newFrontendMoveFixture(&models.Strip{Callsign: "SAS123", Origin: "EKCH", Destination: "EGLL", Bay: shared.BAY_CLEARED, CdmData: &models.CdmData{}})
	b := fixtureWorkflow(t, workflowFixtureHandler{true}, airportworkflow.Action("custom.depart", func(c airportworkflow.Context, _ struct{}) (airportworkflow.Decision, error) {
		bay, state := shared.BAY_PUSH, "PUSH"
		if c.Strip.Cleared {
			bay, state = shared.BAY_TAXI, "TAXI"
		}
		return airportworkflow.Decision{Bay: &bay, GroundState: &state}, nil
	}))
	f.svc.sessionWorkflows.Store(int32(1), b)
	f.svc.validationStore = &validationStoreFake{}
	f.svc.cdmService = pushbackCdmStub{StripCdmService: &spyStripCdmService{}, prepare: func(context.Context, int32, string) (string, string, bool, error) {
		f.strip.Cleared = true
		now := time.Now().UTC().Format("1504")
		f.strip.CdmData.Tsat = &now
		return now, "", true, nil
	}}
	require.NoError(t, f.svc.ExecuteWorkflowAction(f.ctx, 1, "SAS123", "own", "EKCH", "GND", "custom.depart", struct{}{}))
	require.Equal(t, shared.BAY_TAXI, f.strip.Bay)
	require.Equal(t, []string{"TAXI"}, f.updateGroundStates)
}

func TestWorkflowBindsOnlyTrustedSessionAirport(t *testing.T) {
	s := NewStripService(nil, WithAirportWorkflows(testWorkflows), WithSessionReader(&testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*models.Session, error) {
		return &models.Session{ID: 42, Airport: "OTHER"}, nil
	}}))
	_, err := s.workflowFor(context.Background(), 42, "EKCH")
	require.Error(t, err)
	_, cached := s.sessionWorkflows.Load(int32(42))
	require.False(t, cached)
	_, err = s.workflowFor(context.Background(), 0, "EKCH")
	require.Error(t, err)
}

func TestWorkflowAdmissionRejectionDoesNotMarkOrPublish(t *testing.T) {
	marked, created := false, false
	repo := &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return nil, pgx.ErrNoRows }, CreateFn: func(context.Context, *models.Strip) error { created = true; return nil }, MarkEuroscopeSeenFn: func(context.Context, int32, string) error { marked = true; return nil }}
	s := newWorkflowTestService(repo)
	s.sessionWorkflows.Store(int32(1), fixtureWorkflow(t, workflowFixtureHandler{false}))
	s.SetSessionRepo(&testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*models.Session, error) {
		return &models.Session{ID: 1, Airport: "EKCH"}, nil
	}})
	hub := &testutil.MockFrontendHub{}
	s.SetFrontendHub(hub)
	require.NoError(t, s.SyncStrip(context.Background(), 1, "master", euroscope.Strip{Callsign: "DENIED", Origin: "EKCH"}, "EKCH"))
	require.False(t, marked)
	require.False(t, created)
	require.Empty(t, hub.StripUpdates)
}
