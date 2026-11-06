package services

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/testutil"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFlightPlanEETOnlyObservationPersistsAndMissingValueRetainsKnownFact(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	ctx := t.Context()
	require.NoError(t, q.InsertAirport(ctx, "EKBI"))
	session, err := q.InsertSession(ctx, database.InsertSessionParams{Airport: "EKBI", Name: "LIVE"})
	require.NoError(t, err)
	repo := postgres.NewStripRepository(pool)
	service := NewStripService(repo, WithAirportWorkflows(radisWorkflowFixtures(t)), WithSessionReader(postgres.NewSessionRepository(pool)), WithStripEventPublisher(&productionWorkflowPublisher{}), WithControllerReader(postgres.NewControllerRepository(pool)), WithSectorOwnerRepository(postgres.NewSectorOwnerRepository(pool)), WithCoordinationStore(postgres.NewCoordinationRepository(pool)), WithEuroscopeCommander(&testutil.MockEuroscopeHub{}))
	event := es.Strip{Callsign: "EETONLY", Origin: "EKBI", Destination: "EKYT", HasFp: true, EnrouteDuration: "0100", GroundState: es.GroundStateTaxi, Position: &es.Position{Lat: 0, Lon: 0, Altitude: 100}}
	require.NoError(t, service.SyncStrip(ctx, session, "MASTER", event, "EKBI"))
	before, err := repo.GetByCallsign(ctx, session, event.Callsign)
	require.NoError(t, err)
	require.Equal(t, "0100", before.FlightPlanEET)
	event.EnrouteDuration = "0130"
	require.NoError(t, service.SyncStrip(ctx, session, "MASTER", event, "EKBI"))
	after, err := repo.GetByCallsign(ctx, session, event.Callsign)
	require.NoError(t, err)
	require.Equal(t, "0130", after.FlightPlanEET)
	require.Greater(t, after.Version, before.Version)
	cached := *before
	applySyncStripUpdate(&cached, after)
	require.Equal(t, "0130", cached.FlightPlanEET)
	event.EnrouteDuration = ""
	require.NoError(t, service.SyncStrip(ctx, session, "MASTER", event, "EKBI"))
	retained, err := repo.GetByCallsign(ctx, session, event.Callsign)
	require.NoError(t, err)
	require.Equal(t, "0130", retained.FlightPlanEET)
	require.Equal(t, after.Version, retained.Version)
}

func TestFlightPlanEETDoesNotCarryIntoRestartedFlightLifecycle(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	ctx := t.Context()
	require.NoError(t, q.InsertAirport(ctx, "EKCH"))
	session, err := q.InsertSession(ctx, database.InsertSessionParams{Airport: "EKCH", Name: "LIVE"})
	require.NoError(t, err)
	repo := postgres.NewStripRepository(pool)
	require.NoError(t, repo.Create(ctx, &models.Strip{Callsign: "TURNAROUND", Session: session, Origin: "EGLL", Destination: "EKCH", Bay: "STAND", HasFP: true, FlightPlanEET: "0130"}))
	service := NewStripService(repo, WithAirportWorkflows(testWorkflows), WithSessionReader(postgres.NewSessionRepository(pool)), WithStripEventPublisher(&productionWorkflowPublisher{}), WithControllerReader(postgres.NewControllerRepository(pool)), WithSectorOwnerRepository(postgres.NewSectorOwnerRepository(pool)), WithCoordinationStore(postgres.NewCoordinationRepository(pool)), WithEuroscopeCommander(&testutil.MockEuroscopeHub{}))
	event := es.Strip{Callsign: "TURNAROUND", Origin: "EKCH", Destination: "ESSA", HasFp: true, GroundState: es.GroundStateTaxi, Position: &es.Position{Lat: 55.6, Lon: 12.65, Altitude: 0}}
	require.NoError(t, service.SyncStrip(ctx, session, "MASTER", event, "EKCH"))
	restarted, err := repo.GetByCallsign(ctx, session, event.Callsign)
	require.NoError(t, err)
	require.Equal(t, "EKCH", restarted.Origin)
	require.Equal(t, "ESSA", restarted.Destination)
	require.Empty(t, restarted.FlightPlanEET)
}
