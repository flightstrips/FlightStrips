package pdc

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/testutil"
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// Change to backend root so config/ekch.yaml is found.
	if err := os.Chdir("../.."); err != nil {
		panic("failed to chdir to backend root: " + err.Error())
	}
	config.InitConfig()
	os.Exit(m.Run())
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

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool)}

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

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool)}

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

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool)}

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

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool)}
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

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool)}
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

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool)}
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

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool)}
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

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool)}

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

func TestGetAirborneFrequency_UsesDefaultSectorForUnknownSid(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		{Sector: []string{"K_DEP"}, Position: "124.980", Identifier: "K_DEP"},
		{Sector: []string{"R_DEP"}, Position: "120.255", Identifier: "R_DEP"},
	})

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool)}
	sid := "UNKNOWN1A"

	freq, err := svc.getAirborneFrequency(context.Background(), sessionID, &sid)
	require.NoError(t, err)
	assert.Equal(t, "124.980", freq)
}
