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

// ── getAirborneFrequency (Departure frequency in clearance = airborne controller) ─

func TestGetAirborneFrequency_AirborneOnline(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	// EKCH_K_DEP (priority 4 in airborne_owners) is the only airborne controller online.
	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		{Sector: []string{"K_DEP"}, Position: "124.980", Identifier: "K_DEP"},
		{Sector: []string{"SQ", "DEL"}, Position: "119.905", Identifier: "DEL"}, // not airborne
	})

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool)}

	freq := svc.getAirborneFrequency(context.Background(), sessionID)
	assert.Equal(t, "124.980", freq)
}

func TestGetAirborneFrequency_NoAirborneOnline_ReturnsUNICOM(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	// Only ground/tower — none are in airborne_owners.
	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		{Sector: []string{"SQ", "DEL"}, Position: "119.905", Identifier: "DEL"},
		{Sector: []string{"TE", "TW"}, Position: "118.105", Identifier: "TE"},
	})

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool)}

	freq := svc.getAirborneFrequency(context.Background(), sessionID)
	assert.Equal(t, "122.8", freq)
}

func TestGetAirborneFrequency_Priority(t *testing.T) {
	t.Parallel()

	dbPool, queries := testdata.SetupTestDB(t)

	// Both EKCH_W_APP (priority 1) and EKCH_K_DEP (priority 4) are online.
	// EKCH_W_APP must win.
	sessionID := testdata.SeedTestSessionWithSectors(t, queries, []database.InsertSectorOwnersParams{
		{Sector: []string{"K_DEP"}, Position: "124.980", Identifier: "K_DEP"},
		{Sector: []string{"W_APP"}, Position: "119.805", Identifier: "W_APP"},
	})

	svc := &Service{sectorRepo: postgres.NewSectorOwnerRepository(dbPool)}

	freq := svc.getAirborneFrequency(context.Background(), sessionID)
	assert.Equal(t, "119.805", freq) // EKCH_W_APP is first in airborne_owners
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
				{Session: session, Position: "119.8050"},
			}, nil
		},
	}

	svc := &Service{
		sectorRepo:     postgres.NewSectorOwnerRepository(dbPool),
		controllerRepo: controllerRepo,
	}

	freq := svc.getAirborneFrequency(context.Background(), sessionID)
	assert.Equal(t, "119.805", freq)
}
