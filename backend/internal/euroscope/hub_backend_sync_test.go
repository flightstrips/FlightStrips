package euroscope

import (
	"context"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	outgoing "FlightStrips/pkg/events"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var backendFixtureOnce sync.Once
var backendFixtureWorkflow *airportworkflow.Bound
var backendFixtureDefinitions *config.AirportRegistry

func backendWorkflowFixture() *airportworkflow.Bound {
	backendFixtureOnce.Do(func() {
		definitions, err := config.LoadAirportFiles(config.ProductionAirportCatalog(), []string{"backend/config/airports/ekch.airport.yaml"}, config.AirportLoadOptions{RepositoryRoot: "../../.."})
		if err != nil {
			panic(err)
		}
		backendFixtureDefinitions = definitions
		definition, _ := definitions.Lookup("EKCH")
		backendFixtureWorkflow, err = airportworkflow.Bind(definition, airportworkflow.EKCHDescriptor())
		if err != nil {
			panic(err)
		}
	})
	return backendFixtureWorkflow
}

func shouldIncludeInBackendSync(strip *internalModels.Strip) bool {
	d, err := backendWorkflowFixture().Lifecycle(airportworkflow.Context{Session: 1, Strip: airportworkflow.SnapshotOf(strip)}, airportworkflow.Lifecycle{Kind: airportworkflow.BackendAdmission})
	return err == nil && d.Admit
}
func backendSyncGroundState(strip *internalModels.Strip) string {
	d, err := backendWorkflowFixture().Observe(airportworkflow.Context{Session: 1, Strip: airportworkflow.SnapshotOf(strip)}, airportworkflow.Observation{Kind: airportworkflow.BackendState})
	if err != nil || d.ReconciledGroundState == nil {
		return ""
	}
	return *d.ReconciledGroundState
}

func TestSendBackendSyncIfNeeded_SendsLineupForDepartBayWithoutStoredState(t *testing.T) {
	const session = int32(1)

	departState := euroscopeEvents.GroundStateDepart
	seenAt := time.Now().UTC()
	stripRepo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, gotSession int32) ([]*internalModels.Strip, error) {
			assert.Equal(t, session, gotSession)
			return []*internalModels.Strip{
				{
					Callsign:        "SAS101",
					Origin:          "EKCH",
					Bay:             shared.BAY_DEPART,
					EuroscopeSeenAt: &seenAt,
				},
				{
					Callsign:        "SAS102",
					Origin:          "EKCH",
					Bay:             shared.BAY_DEPART,
					State:           &departState,
					EuroscopeSeenAt: &seenAt,
				},
			}, nil
		},
	}

	hub := &Hub{
		server: &testutil.MockServer{StripRepoVal: stripRepo},
	}
	client := startQueuedTestClient(&Client{workflow: backendWorkflowFixture(),
		session: session,
		user:    shared.NewAuthenticatedUser("1234567", 0, nil),
	})

	hub.sendBackendSyncIfNeeded(client)

	message := <-client.send
	syncEvent, ok := message.(euroscopeEvents.BackendSyncEvent)
	require.True(t, ok)
	require.Len(t, syncEvent.Strips, 2)

	groundStates := map[string]string{}
	for _, strip := range syncEvent.Strips {
		groundStates[strip.Callsign] = strip.GroundState
	}

	assert.Equal(t, euroscopeEvents.GroundStateLineup, groundStates["SAS101"])
	assert.Equal(t, euroscopeEvents.GroundStateDepart, groundStates["SAS102"])
}

func TestSendBackendSyncIfNeeded_ExcludesVatsimOnlyPlanningStrips(t *testing.T) {
	const session = int32(1)
	seenAt := time.Now().UTC()
	stripRepo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, gotSession int32) ([]*internalModels.Strip, error) {
			assert.Equal(t, session, gotSession)
			return []*internalModels.Strip{
				{Callsign: "PREFILE1", Bay: shared.BAY_DEP_HIDDEN},
				{Callsign: "INBOUND1", Bay: shared.BAY_ARR_HIDDEN},
				{Callsign: "UNSYNCED1", Bay: shared.BAY_NOT_CLEARED},
				{Callsign: "SAS101", Bay: shared.BAY_NOT_CLEARED, EuroscopeSeenAt: &seenAt},
			}, nil
		},
	}

	hub := &Hub{server: &testutil.MockServer{StripRepoVal: stripRepo}}
	client := startQueuedTestClient(&Client{workflow: backendWorkflowFixture(),
		send:    make(chan outgoing.OutgoingMessage, 4),
		session: session,
		user:    shared.NewAuthenticatedUser("1234567", 0, nil),
	})

	hub.sendBackendSyncIfNeeded(client)

	for _, callsign := range []string{"PREFILE1", "INBOUND1", "UNSYNCED1"} {
		textEvent, ok := (<-client.send).(euroscopeEvents.FsScratchPadEvent)
		require.True(t, ok)
		require.Equal(t, callsign, textEvent.Callsign)
		require.Empty(t, textEvent.Text)
	}
	message := <-client.send
	syncEvent, ok := message.(euroscopeEvents.BackendSyncEvent)
	require.True(t, ok)
	require.Len(t, syncEvent.Strips, 1)
	assert.Equal(t, "SAS101", syncEvent.Strips[0].Callsign)
}

func TestSendBackendSyncIfNeeded_IncludesPersistedHold(t *testing.T) {
	const session = int32(1)
	seenAt := time.Now().UTC()
	stripRepo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, gotSession int32) ([]*internalModels.Strip, error) {
			assert.Equal(t, session, gotSession)
			return []*internalModels.Strip{{
				Callsign:        "SAS105",
				Bay:             shared.BAY_AIRBORNE,
				EuroscopeSeenAt: &seenAt,
				Hold:            "OLPIB",
				HoldType:        "enroute",
				HoldEat:         "1422",
			}}, nil
		},
	}

	hub := &Hub{server: &testutil.MockServer{StripRepoVal: stripRepo}}
	client := startQueuedTestClient(&Client{workflow: backendWorkflowFixture(),
		session: session,
		user:    shared.NewAuthenticatedUser("1234567", 0, nil),
	})

	hub.sendBackendSyncIfNeeded(client)

	message := <-client.send
	syncEvent, ok := message.(euroscopeEvents.BackendSyncEvent)
	require.True(t, ok)
	require.Len(t, syncEvent.Strips, 1)
	assert.Equal(t, "OLPIB", syncEvent.Strips[0].Hold)
	assert.Equal(t, "enroute", syncEvent.Strips[0].HoldType)
	assert.Equal(t, "1422", syncEvent.Strips[0].HoldEat)
}

func TestBackendSyncGroundState_DepartIgnoresStaleTaxiState(t *testing.T) {
	taxiState := euroscopeEvents.GroundStateTaxi
	strip := &internalModels.Strip{
		Callsign: "SAS103",
		Origin:   "EKCH",
		Bay:      shared.BAY_DEPART,
		State:    &taxiState,
	}

	assert.Equal(t, euroscopeEvents.GroundStateLineup, backendSyncGroundState(strip))
}

func TestBackendSyncGroundState_AirborneDoesNotSendStaleTaxiState(t *testing.T) {
	taxiState := euroscopeEvents.GroundStateTaxi
	strip := &internalModels.Strip{
		Callsign: "SAS104",
		Origin:   "EKCH",
		Bay:      shared.BAY_AIRBORNE,
		State:    &taxiState,
	}

	assert.Equal(t, euroscopeEvents.GroundStateUnknown, backendSyncGroundState(strip))
}

// Admission policy cannot make a missing repository record materializable.
type admitAllBackendHandler struct{ airportworkflow.Handler }

func (admitAllBackendHandler) Lifecycle(airportworkflow.Context, airportworkflow.Lifecycle) (airportworkflow.LifecycleDecision, error) {
	return airportworkflow.LifecycleDecision{Admit: true}, nil
}
func TestBackendSyncSkipsNilRecordsWithCustomAdmission(t *testing.T) {
	original := backendWorkflowFixture()
	bound, err := airportworkflow.Bind(original.Definition(), airportworkflow.Descriptor{ID: "ekch/v1", Bind: func(*config.AirportDefinition) (airportworkflow.Handler, []airportworkflow.ActionDescriptor, error) {
		return admitAllBackendHandler{original}, nil, nil
	}})
	require.NoError(t, err)
	hub := &Hub{server: &testutil.MockServer{StripRepoVal: &testutil.MockStripRepository{ListFn: func(context.Context, int32) ([]*internalModels.Strip, error) {
		return []*internalModels.Strip{nil, {Callsign: "CUSTOM123"}}, nil
	}}}}
	client := startQueuedTestClient(&Client{workflow: bound, session: 1, send: make(chan outgoing.OutgoingMessage, 2)})
	hub.sendBackendSyncIfNeeded(client)
	select {
	case message := <-client.send:
		event, ok := message.(euroscopeEvents.BackendSyncEvent)
		require.True(t, ok)
		require.Len(t, event.Strips, 1)
		require.Equal(t, "CUSTOM123", event.Strips[0].Callsign)
	case <-time.After(time.Second):
		t.Fatal("backend sync did not complete")
	}
}
