package server

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	pkgModels "FlightStrips/pkg/models"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	if err := os.Chdir("../.."); err != nil {
		panic("failed to chdir to backend root: " + err.Error())
	}
	config.InitConfig()
	os.Exit(m.Run())
}

func TestUpdateRouteForStrip_ArrivalOutsideSupportedRegionFallsBackToTowerOwner(t *testing.T) {
	t.Parallel()

	arrivalRunway, towerSector := mustArrivalRunwayAndTowerSector(t)

	frontendHub := &testutil.MockFrontendHub{}
	stripRepo := &testutil.MockStripRepository{}
	sessionRepo := &testutil.MockSessionRepository{}
	sectorRepo := &testutil.MockSectorOwnerRepository{}

	strip := &models.Strip{
		Callsign:          "SAS123",
		Session:           42,
		Destination:       "EKCH",
		Stand:             stringPtr("A12"),
		PositionLatitude:  float64Ptr(0),
		PositionLongitude: float64Ptr(0),
	}

	var updatedNextOwners []string

	stripRepo.GetByCallsignFn = func(_ context.Context, session int32, callsign string) (*models.Strip, error) {
		require.Equal(t, int32(42), session)
		require.Equal(t, "SAS123", callsign)
		return strip, nil
	}
	stripRepo.SetNextOwnersFn = func(_ context.Context, session int32, callsign string, nextOwners []string) error {
		require.Equal(t, int32(42), session)
		require.Equal(t, "SAS123", callsign)
		updatedNextOwners = append([]string(nil), nextOwners...)
		return nil
	}

	sessionRepo.GetByIDFn = func(_ context.Context, id int32) (*models.Session, error) {
		require.Equal(t, int32(42), id)
		return &models.Session{
			ID:      42,
			Airport: "EKCH",
			ActiveRunways: pkgModels.ActiveRunways{
				ArrivalRunways: []string{arrivalRunway},
			},
		}, nil
	}

	sectorRepo.ListBySessionFn = func(_ context.Context, session int32) ([]*models.SectorOwner, error) {
		require.Equal(t, int32(42), session)
		return []*models.SectorOwner{
			{
				Session:  42,
				Sector:   []string{towerSector},
				Position: "EKCH_TWR",
			},
		}, nil
	}

	srv := &Server{
		frontendHub: frontendHub,
		stripRepo:   stripRepo,
		sessionRepo: sessionRepo,
		sectorRepo:  sectorRepo,
	}

	err := srv.UpdateRouteForStrip("SAS123", 42, true)
	require.NoError(t, err)

	assert.Equal(t, []string{"EKCH_TWR"}, updatedNextOwners)
	require.Len(t, frontendHub.OwnersUpdates, 1)
	assert.Equal(t, []string{"EKCH_TWR"}, frontendHub.OwnersUpdates[0].NextOwners)
	assert.Equal(t, "SAS123", frontendHub.OwnersUpdates[0].Callsign)
}

func TestUpdateRoutesForSession_RecalculatesEachStrip(t *testing.T) {
	t.Parallel()

	arrivalRunway, towerSector := mustArrivalRunwayAndTowerSector(t)

	frontendHub := &testutil.MockFrontendHub{}
	stripRepo := &testutil.MockStripRepository{}
	sessionRepo := &testutil.MockSessionRepository{}
	sectorRepo := &testutil.MockSectorOwnerRepository{}

	strips := []*models.Strip{
		{Callsign: "SAS123", Session: 42, Destination: "EKCH"},
		{Callsign: "KLM456", Session: 42, Destination: "EKCH"},
	}

	var updatedCallsigns []string

	stripRepo.ListFn = func(_ context.Context, session int32) ([]*models.Strip, error) {
		require.Equal(t, int32(42), session)
		return strips, nil
	}
	stripRepo.SetNextOwnersFn = func(_ context.Context, session int32, callsign string, nextOwners []string) error {
		require.Equal(t, int32(42), session)
		assert.Equal(t, []string{"EKCH_TWR"}, nextOwners)
		updatedCallsigns = append(updatedCallsigns, callsign)
		return nil
	}

	sessionRepo.GetByIDFn = func(_ context.Context, id int32) (*models.Session, error) {
		require.Equal(t, int32(42), id)
		return &models.Session{
			ID:      42,
			Airport: "EKCH",
			ActiveRunways: pkgModels.ActiveRunways{
				ArrivalRunways: []string{arrivalRunway},
			},
		}, nil
	}

	sectorRepo.ListBySessionFn = func(_ context.Context, session int32) ([]*models.SectorOwner, error) {
		require.Equal(t, int32(42), session)
		return []*models.SectorOwner{
			{
				Session:  42,
				Sector:   []string{towerSector},
				Position: "EKCH_TWR",
			},
		}, nil
	}

	srv := &Server{
		frontendHub: frontendHub,
		stripRepo:   stripRepo,
		sessionRepo: sessionRepo,
		sectorRepo:  sectorRepo,
	}

	err := srv.UpdateRoutesForSession(42, false)
	require.NoError(t, err)

	assert.Equal(t, []string{"SAS123", "KLM456"}, updatedCallsigns)
	assert.Empty(t, frontendHub.OwnersUpdates)
}

func TestUpdateRoutesForSession_ReturnsFirstStripError(t *testing.T) {
	t.Parallel()

	arrivalRunway, towerSector := mustArrivalRunwayAndTowerSector(t)
	expectedErr := errors.New("set next owners failed")

	stripRepo := &testutil.MockStripRepository{}
	sessionRepo := &testutil.MockSessionRepository{}
	sectorRepo := &testutil.MockSectorOwnerRepository{}

	strips := []*models.Strip{
		{Callsign: "SAS123", Session: 42, Destination: "EKCH"},
		{Callsign: "KLM456", Session: 42, Destination: "EKCH"},
	}

	var updatedCallsigns []string

	stripRepo.ListFn = func(_ context.Context, session int32) ([]*models.Strip, error) {
		require.Equal(t, int32(42), session)
		return strips, nil
	}
	stripRepo.SetNextOwnersFn = func(_ context.Context, session int32, callsign string, nextOwners []string) error {
		require.Equal(t, int32(42), session)
		assert.Equal(t, []string{"EKCH_TWR"}, nextOwners)
		updatedCallsigns = append(updatedCallsigns, callsign)
		if callsign == "KLM456" {
			return expectedErr
		}
		return nil
	}

	sessionRepo.GetByIDFn = func(_ context.Context, id int32) (*models.Session, error) {
		require.Equal(t, int32(42), id)
		return &models.Session{
			ID:      42,
			Airport: "EKCH",
			ActiveRunways: pkgModels.ActiveRunways{
				ArrivalRunways: []string{arrivalRunway},
			},
		}, nil
	}

	sectorRepo.ListBySessionFn = func(_ context.Context, session int32) ([]*models.SectorOwner, error) {
		require.Equal(t, int32(42), session)
		return []*models.SectorOwner{
			{
				Session:  42,
				Sector:   []string{towerSector},
				Position: "EKCH_TWR",
			},
		}, nil
	}

	srv := &Server{
		stripRepo:   stripRepo,
		sessionRepo: sessionRepo,
		sectorRepo:  sectorRepo,
	}

	err := srv.UpdateRoutesForSession(42, false)
	require.ErrorIs(t, err, expectedErr)
	assert.Equal(t, []string{"SAS123", "KLM456"}, updatedCallsigns)
}

func mustArrivalRunwayAndTowerSector(t *testing.T) (string, string) {
	t.Helper()

	for _, runway := range config.GetRunways() {
		if towerSector, ok := config.GetArrivalTowerSector([]string{runway}); ok {
			return runway, towerSector
		}
	}

	t.Fatal("expected at least one configured arrival runway with a tower sector")
	return "", ""
}

func stringPtr(value string) *string {
	return &value
}

func float64Ptr(value float64) *float64 {
	return &value
}
