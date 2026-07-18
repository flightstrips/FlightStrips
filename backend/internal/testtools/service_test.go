package testtools

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/services"
	"FlightStrips/internal/vatsim"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scenarioNotifier struct{}

func (scenarioNotifier) SendStripUpdate(int32, string) {}

type repositoryStripDeleter struct {
	repository.StripRepository
	deleted []string
}

func (d *repositoryStripDeleter) DeleteStrip(ctx context.Context, session int32, callsign string) error {
	d.deleted = append(d.deleted, callsign)
	return d.Delete(ctx, session, callsign)
}

func TestScenariosUseRealReconciliationAndLifecycle(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()
	require.NoError(t, queries.InsertAirport(ctx, "EKCH"))

	sessionRepo := postgres.NewSessionRepository(pool)
	stripRepo := postgres.NewStripRepository(pool)
	assignmentRepo := postgres.NewStandAssignmentRepository(pool)
	sessionID, err := sessionRepo.Create(ctx, &models.Session{Name: "SAT-TEST", Airport: "EKCH", CdmMaster: true})
	require.NoError(t, err)
	otherSessionID, err := sessionRepo.Create(ctx, &models.Session{Name: "SAT-OTHER", Airport: "EKCH"})
	require.NoError(t, err)

	configDir := filepath.Join("..", "..", "config", "ekch")
	aircraft, err := sat.LoadAircraftReferenceFile(filepath.Join(configDir, "GRpluginAircraftInfo.txt"))
	require.NoError(t, err)
	engines, err := sat.LoadAircraftEngineReferenceFile(filepath.Join("..", "..", "config", "test", "ICAO_Aircraft.json"), aircraft)
	require.NoError(t, err)
	stands, err := sat.LoadStandCapabilityFile(filepath.Join(configDir, "GRpluginStands.txt"))
	require.NoError(t, err)
	policy, err := sat.LoadAirlineAssignmentFile(filepath.Join(configDir, "airline_assignment.json"), stands)
	require.NoError(t, err)
	borders := sat.NewAirportCountryRegistry()

	clock := NewClock()
	allocations, err := services.NewStandAllocationService(pool, stripRepo, assignmentRepo, stands, policy, services.WithStandAllocationClock(clock.Now))
	require.NoError(t, err)
	departures, err := services.NewDepartureLifecycleService(
		allocations, assignmentRepo, stripRepo, sessionRepo, stands, aircraft, engines, borders,
		services.WithDepartureLifecycleClock(clock.Now),
	)
	require.NoError(t, err)
	arrivals, err := services.NewArrivalLifecycleService(
		allocations, assignmentRepo, stripRepo, sessionRepo, stands, aircraft, engines, borders,
		services.WithArrivalLifecycleClock(clock.Now),
	)
	require.NoError(t, err)
	source := vatsim.NewSyntheticSource()
	reconciler, err := vatsim.NewReconciler(vatsim.ReconcilerDependencies{
		Cache: source, Sessions: sessionRepo, Strips: stripRepo, Assignments: assignmentRepo,
		DepartureLifecycle: departures, ArrivalLifecycle: arrivals, Notifier: scenarioNotifier{},
	}, time.Second, vatsim.WithClock(clock.Now))
	require.NoError(t, err)
	deleter := &repositoryStripDeleter{StripRepository: stripRepo}
	service := NewService(ServiceConfig{
		Source: source, Reconciler: reconciler, Departures: departures, Arrivals: arrivals,
		Allocations: allocations, Sessions: sessionRepo, Strips: stripRepo,
		StripDeleter: deleter, Assignments: assignmentRepo, Stands: stands, Clock: clock,
	})
	departures.SetWrongStandMessenger(NewFallbackMessenger(nil, service))

	scenario, err := service.CreateScenario(ctx, CreateScenarioRequest{
		SessionID: sessionID, Preset: PresetDeparture, Callsign: "TST101", AircraftType: "A320",
	})
	require.NoError(t, err)
	require.NotNil(t, scenario.Assignment)
	assert.Equal(t, services.StageReserved, scenario.Assignment.Stage)
	assert.Equal(t, testSource, scenario.Assignment.Source)
	_, err = stripRepo.GetByCallsign(ctx, otherSessionID, "TST101")
	assert.ErrorIs(t, err, pgx.ErrNoRows)
	_, err = service.CreateScenario(ctx, CreateScenarioRequest{
		SessionID: otherSessionID, Preset: PresetDeparture, Callsign: "TST101", AircraftType: "A320",
	})
	assert.ErrorIs(t, err, ErrConflict)

	listDone := make(chan error, 1)
	go func() {
		for range 20 {
			if _, listErr := service.ListScenarios(ctx, sessionID); listErr != nil {
				listDone <- listErr
				return
			}
		}
		listDone <- nil
	}()
	scenario, err = service.Command(ctx, scenario.ID, ScenarioCommand{Command: "advance"})
	require.NoError(t, err)
	require.NoError(t, <-listDone)
	require.NotNil(t, scenario.Assignment)
	assert.Equal(t, services.StageDepartureBlock, scenario.Assignment.Stage)
	assert.Equal(t, testSource, scenario.Assignment.Source)
	assert.Equal(t, "online", scenario.FeedState)

	require.NoError(t, service.Reset(ctx))
	_, err = stripRepo.GetByCallsign(ctx, sessionID, "TST101")
	assert.ErrorIs(t, err, pgx.ErrNoRows)
	_, err = assignmentRepo.GetAssignment(ctx, sessionID, "TST101")
	assert.ErrorIs(t, err, pgx.ErrNoRows)
	assert.Contains(t, deleter.deleted, "TST101")

	customEOBT := clock.Now().Add(5 * time.Minute).Format("1504")
	arrival, err := service.CreateScenario(ctx, CreateScenarioRequest{
		SessionID: sessionID, Preset: PresetArrival, Callsign: "TST201", AircraftType: "A320",
		EOBT: customEOBT, EnrouteTime: "0030",
	})
	require.NoError(t, err)
	require.NotNil(t, arrival.Assignment)
	assert.Equal(t, services.StageEstimated, arrival.Assignment.Stage)
	assert.Equal(t, testSource, arrival.Assignment.Source)

	arrival, err = service.Command(ctx, arrival.ID, ScenarioCommand{Command: "advance"})
	require.NoError(t, err)
	require.NotNil(t, arrival.Assignment)
	assert.Equal(t, services.StageAssigned, arrival.Assignment.Stage)
	arrival, err = service.Command(ctx, arrival.ID, ScenarioCommand{Command: "advance"})
	require.NoError(t, err)
	require.NotNil(t, arrival.Assignment)
	assert.Equal(t, services.StageConfirmed, arrival.Assignment.Stage)
	flight, ok := source.Snapshot().FlightByCallsign("TST201")
	require.True(t, ok)
	assert.Equal(t, customEOBT, flight.FlightPlan.EOBT)
	assert.Equal(t, "0030", flight.FlightPlan.EnrouteDuration)
	require.NoError(t, service.Reset(ctx))

	wrongStand, err := service.CreateScenario(ctx, CreateScenarioRequest{
		SessionID: sessionID, Preset: PresetWrongStand, Callsign: "TST301", AircraftType: "A320",
	})
	require.NoError(t, err)
	wrongStand, err = service.Command(ctx, wrongStand.ID, ScenarioCommand{Command: "advance"})
	require.NoError(t, err)
	require.NotNil(t, wrongStand.Assignment)
	require.NotNil(t, wrongStand.Assignment.ConflictReason)
	assert.Contains(t, *wrongStand.Assignment.ConflictReason, "WRONG_STAND_PENDING")
	assert.Contains(t, wrongStand.GeneratedMessage, "PLEASE RELOCATE")
	assignedStand := wrongStand.Assignment.Stand

	wrongStand, err = service.Command(ctx, wrongStand.ID, ScenarioCommand{Command: "advance"})
	require.NoError(t, err)
	require.NotNil(t, wrongStand.Assignment)
	assert.Equal(t, assignedStand, wrongStand.Assignment.Stand)
	require.NotNil(t, wrongStand.Assignment.ConflictReason)
	assert.Contains(t, *wrongStand.Assignment.ConflictReason, "WRONG_STAND_PENDING")
	assert.Equal(t, "wrong-stand assignment retained", wrongStand.LastAction)
}
