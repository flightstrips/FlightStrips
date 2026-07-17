package pdc

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/testutil"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func emptyFrequencyTestControllerRepository() *testutil.MockControllerRepository {
	return &testutil.MockControllerRepository{
		ListBySessionFn: func(context.Context, int32) ([]*models.Controller, error) {
			return nil, errors.New("no online controllers")
		},
	}
}

func TestMain(m *testing.M) {
	// Change to backend root so config/ekch.yaml is found.
	if err := os.Chdir("../.."); err != nil {
		panic("failed to chdir to backend root: " + err.Error())
	}
	if err := config.InitConfig(); err != nil {
		panic("failed to initialize config: " + err.Error())
	}
	os.Exit(m.Run())
}

type staticTransceiverLookup struct {
	frequenciesByCallsign map[string][]string
}

func (s staticTransceiverLookup) GetFrequencies(callsign string) []string {
	return append([]string(nil), s.frequenciesByCallsign[callsign]...)
}

// ── getNextFrequency (NEXT FRQ in clearance = SQ / DEL controller) ────────────

func TestGetNextFrequency_SQOwnerOnline(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		{Sector: []string{"SQ", "DEL"}, Position: "119.905", Identifier: "DEL"},
		// Airborne also online — must NOT affect NEXT FRQ
		{Sector: []string{"K_DEP"}, Position: "124.980", Identifier: "K_DEP"},
	})

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool), controllerRepo: emptyFrequencyTestControllerRepository()}

	freq, err := svc.getNextFrequency(context.Background(), sessionID)
	require.NoError(t, err)
	assert.Equal(t, "119.905", freq)
}

func TestGetNextFrequency_FallbackToDEL(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		// Only DEL, no SQ
		{Sector: []string{"DEL"}, Position: "119.905", Identifier: "DEL"},
	})

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool), controllerRepo: emptyFrequencyTestControllerRepository()}

	freq, err := svc.getNextFrequency(context.Background(), sessionID)
	require.NoError(t, err)
	assert.Equal(t, "119.905", freq)
}

func TestGetNextFrequency_NoSQOrDEL_ReturnsError(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	// Only airborne, no ground controller with SQ or DEL
	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		{Sector: []string{"K_DEP"}, Position: "124.980", Identifier: "K_DEP"},
	})

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool), controllerRepo: emptyFrequencyTestControllerRepository()}

	_, err := svc.getNextFrequency(context.Background(), sessionID)
	assert.Error(t, err)
}

// ── getAirborneFrequency (Departure frequency in clearance = SID-specific airborne sector) ─

func TestGetAirborneFrequency_UsesSidSpecificSectorPriority(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		{Sector: []string{"K_DEP"}, Position: "124.980", Identifier: "K_DEP"},
		{Sector: []string{"R_DEP"}, Position: "120.255", Identifier: "R_DEP"},
		{Sector: []string{"SQ", "DEL"}, Position: "119.905", Identifier: "DEL"}, // not airborne
	})

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool), controllerRepo: emptyFrequencyTestControllerRepository()}
	sid := "BETUD2A"

	freq, err := svc.getAirborneFrequency(context.Background(), sessionID, &sid)
	require.NoError(t, err)
	assert.Equal(t, "124.980", freq)
}

func TestGetAirborneFrequency_UsesDifferentSidSpecificSectorPriority(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		{Sector: []string{"K_DEP"}, Position: "124.980", Identifier: "K_DEP"},
		{Sector: []string{"R_DEP"}, Position: "120.255", Identifier: "R_DEP"},
	})

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool), controllerRepo: emptyFrequencyTestControllerRepository()}
	sid := "GOLGA2C"

	freq, err := svc.getAirborneFrequency(context.Background(), sessionID, &sid)
	require.NoError(t, err)
	assert.Equal(t, "120.255", freq)
}

func TestGetAirborneFrequency_FallsBackWithinSidSpecificPriority(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		{Sector: []string{"R_DEP"}, Position: "120.255", Identifier: "R_DEP"},
	})

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool), controllerRepo: emptyFrequencyTestControllerRepository()}
	sid := "BETUD2A"

	freq, err := svc.getAirborneFrequency(context.Background(), sessionID, &sid)
	require.NoError(t, err)
	assert.Equal(t, "120.255", freq)
}

func TestGetAirborneFrequency_NoAirborneOnline_ReturnsUNICOM(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		{Sector: []string{"SQ", "DEL"}, Position: "119.905", Identifier: "DEL"},
		{Sector: []string{"TE", "TW"}, Position: "118.105", Identifier: "TE"},
	})

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool), controllerRepo: emptyFrequencyTestControllerRepository()}
	sid := "BETUD2A"

	freq, err := svc.getAirborneFrequency(context.Background(), sessionID, &sid)
	require.NoError(t, err)
	assert.Equal(t, "122.8", freq)
}

func TestGetAirborneFrequency_UsesDefaultSectorWhenSidMissing(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		{Sector: []string{"K_DEP"}, Position: "124.980", Identifier: "K_DEP"},
		{Sector: []string{"R_DEP"}, Position: "120.255", Identifier: "R_DEP"},
	})

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool), controllerRepo: emptyFrequencyTestControllerRepository()}

	freq, err := svc.getAirborneFrequency(context.Background(), sessionID, nil)
	require.NoError(t, err)
	assert.Equal(t, "124.980", freq)
}

func TestGetAirborneFrequency_UsesOnlineControllersWhenSectorOwnersAreStale(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		{Sector: []string{"SQ", "DEL"}, Position: "119.905", Identifier: "DEL"},
	})

	controllerRepo := &testutil.MockControllerRepository{
		ListBySessionFn: func(ctx context.Context, session int32) ([]*models.Controller, error) {
			return []*models.Controller{
				{Session: session, Position: "120.2550"},
			}, nil
		},
	}

	svc := &Service{
		sectorRepo:     postgres.NewSectorOwnerRepository(dbPool),
		controllerRepo: controllerRepo,
	}
	sid := "GOLGA2C"

	freq, err := svc.getAirborneFrequency(context.Background(), sessionID, &sid)
	require.NoError(t, err)
	assert.Equal(t, "120.255", freq)
}

func TestGetAirborneFrequency_UsesCoveredFrequencyForCrossCoupledController(t *testing.T) {
	t.Parallel()

	sessionID := int32(1)

	controllerRepo := &testutil.MockControllerRepository{
		ListBySessionFn: func(ctx context.Context, session int32) ([]*models.Controller, error) {
			return []*models.Controller{
				{Session: session, Callsign: "EKCH_O_APP", Position: "118.455"},
			}, nil
		},
	}

	svc := &Service{
		sectorRepo: &testutil.MockSectorOwnerRepository{
			ListBySessionFn: func(ctx context.Context, session int32) ([]*models.SectorOwner, error) {
				return []*models.SectorOwner{
					{Session: session, Sector: []string{"SQ", "DEL"}, Position: "119.905", Identifier: "DEL"},
				}, nil
			},
		},
		controllerRepo: controllerRepo,
		transceiverLookups: []TransceiverLookup{staticTransceiverLookup{frequenciesByCallsign: map[string][]string{
			"EKCH_O_APP": {"124.980"},
		}}},
	}
	sid := "BETUD2A"

	freq, err := svc.getAirborneFrequency(context.Background(), sessionID, &sid)
	require.NoError(t, err)
	assert.Equal(t, "124.980", freq)
}

func TestGetAirborneFrequency_UsesDefaultSectorForUnknownSid(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		{Sector: []string{"K_DEP"}, Position: "124.980", Identifier: "K_DEP"},
		{Sector: []string{"R_DEP"}, Position: "120.255", Identifier: "R_DEP"},
	})

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool), controllerRepo: emptyFrequencyTestControllerRepository()}
	sid := "UNKNOWN1A"

	freq, err := svc.getAirborneFrequency(context.Background(), sessionID, &sid)
	require.NoError(t, err)
	assert.Equal(t, "124.980", freq)
}
