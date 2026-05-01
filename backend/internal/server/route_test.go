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
	if err := config.InitConfig(); err != nil {
		panic("failed to initialize config: " + err.Error())
	}
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

func TestUpdateRouteForStrip_ArrivalOutsideSupportedRegionUsesTowerAsRouteStart(t *testing.T) {
	t.Parallel()

	frontendHub := &testutil.MockFrontendHub{}
	stripRepo := &testutil.MockStripRepository{}
	sessionRepo := &testutil.MockSessionRepository{}
	sectorRepo := &testutil.MockSectorOwnerRepository{}

	towerPosition := "118.105"
	apronPosition := "121.630"

	strip := &models.Strip{
		Callsign:          "NSZ3097",
		Session:           76,
		Destination:       "EKCH",
		Runway:            stringPtr("22L"),
		Stand:             stringPtr("B3"),
		Owner:             stringPtr(towerPosition),
		PositionLatitude:  float64Ptr(0),
		PositionLongitude: float64Ptr(0),
	}

	var updatedNextOwners []string

	stripRepo.GetByCallsignFn = func(_ context.Context, session int32, callsign string) (*models.Strip, error) {
		require.Equal(t, int32(76), session)
		require.Equal(t, "NSZ3097", callsign)
		return strip, nil
	}
	stripRepo.SetNextOwnersFn = func(_ context.Context, session int32, callsign string, nextOwners []string) error {
		require.Equal(t, int32(76), session)
		require.Equal(t, "NSZ3097", callsign)
		updatedNextOwners = append([]string(nil), nextOwners...)
		return nil
	}

	sessionRepo.GetByIDFn = func(_ context.Context, id int32) (*models.Session, error) {
		require.Equal(t, int32(76), id)
		return &models.Session{
			ID:      76,
			Airport: "EKCH",
			ActiveRunways: pkgModels.ActiveRunways{
				ArrivalRunways: []string{"22L"},
			},
		}, nil
	}

	sectorRepo.ListBySessionFn = func(_ context.Context, session int32) ([]*models.SectorOwner, error) {
		require.Equal(t, int32(76), session)
		return []*models.SectorOwner{
			{
				Session:  76,
				Sector:   []string{"TE"},
				Position: towerPosition,
			},
			{
				Session:  76,
				Sector:   []string{"AA"},
				Position: apronPosition,
			},
		}, nil
	}

	srv := &Server{
		frontendHub: frontendHub,
		stripRepo:   stripRepo,
		sessionRepo: sessionRepo,
		sectorRepo:  sectorRepo,
	}

	err := srv.UpdateRouteForStrip("NSZ3097", 76, true)
	require.NoError(t, err)

	assert.Equal(t, []string{apronPosition}, updatedNextOwners)
	require.Len(t, frontendHub.OwnersUpdates, 1)
	assert.Equal(t, []string{apronPosition}, frontendHub.OwnersUpdates[0].NextOwners)
	assert.Equal(t, "NSZ3097", frontendHub.OwnersUpdates[0].Callsign)
}

func TestUpdateRouteForStrip_ArrivalUsesConfigDrivenCrossingSectorSplit(t *testing.T) {
	t.Parallel()

	frontendHub := &testutil.MockFrontendHub{}
	stripRepo := &testutil.MockStripRepository{}
	sessionRepo := &testutil.MockSessionRepository{}
	sectorRepo := &testutil.MockSectorOwnerRepository{}

	aTowerPosition := frequencyForPosition(t, "EKCH_A_TWR")
	apronPosition := frequencyForPosition(t, "EKCH_A_GND")

	strip := &models.Strip{
		Callsign:          "SAS789",
		Session:           91,
		Destination:       "EKCH",
		Runway:            stringPtr("22L"),
		Stand:             stringPtr("A17"),
		Owner:             stringPtr(aTowerPosition),
		PositionLatitude:  float64Ptr(0),
		PositionLongitude: float64Ptr(0),
	}

	var updatedNextOwners []string

	stripRepo.GetByCallsignFn = func(_ context.Context, session int32, callsign string) (*models.Strip, error) {
		require.Equal(t, int32(91), session)
		require.Equal(t, "SAS789", callsign)
		return strip, nil
	}
	stripRepo.SetNextOwnersFn = func(_ context.Context, session int32, callsign string, nextOwners []string) error {
		require.Equal(t, int32(91), session)
		require.Equal(t, "SAS789", callsign)
		updatedNextOwners = append([]string(nil), nextOwners...)
		return nil
	}

	sessionRepo.GetByIDFn = func(_ context.Context, id int32) (*models.Session, error) {
		require.Equal(t, int32(91), id)
		return &models.Session{
			ID:      91,
			Airport: "EKCH",
			ActiveRunways: pkgModels.ActiveRunways{
				ArrivalRunways: []string{"22L"},
			},
		}, nil
	}

	sectorRepo.ListBySessionFn = func(_ context.Context, session int32) ([]*models.SectorOwner, error) {
		require.Equal(t, int32(91), session)
		return []*models.SectorOwner{
			{
				Session:  91,
				Sector:   []string{"TE"},
				Position: aTowerPosition,
			},
			{
				Session:  91,
				Sector:   []string{"GWA"},
				Position: aTowerPosition,
			},
			{
				Session:  91,
				Sector:   []string{"AA"},
				Position: apronPosition,
			},
		}, nil
	}

	srv := &Server{
		frontendHub: frontendHub,
		stripRepo:   stripRepo,
		sessionRepo: sessionRepo,
		sectorRepo:  sectorRepo,
	}

	err := srv.UpdateRouteForStrip("SAS789", 91, true)
	require.NoError(t, err)

	assert.Equal(t, []string{apronPosition}, updatedNextOwners)
	require.Len(t, frontendHub.OwnersUpdates, 1)
	assert.Equal(t, []string{apronPosition}, frontendHub.OwnersUpdates[0].NextOwners)
	assert.Equal(t, "SAS789", frontendHub.OwnersUpdates[0].Callsign)
}

func TestUpdateRouteForStrip_ArrivalResolvesGWAToTEOwnerWhenControllersAreSplit(t *testing.T) {
	t.Parallel()

	frontendHub := &testutil.MockFrontendHub{}
	stripRepo := &testutil.MockStripRepository{}
	sessionRepo := &testutil.MockSessionRepository{}
	sectorRepo := &testutil.MockSectorOwnerRepository{}

	const (
		lat = 55.6235
		lon = 12.6380
	)

	region, err := config.GetRegionForPosition(lat, lon)
	require.NoError(t, err)
	require.Equal(t, "GROUND_WEST", region.Name)

	aTowerPosition := frequencyForPosition(t, "EKCH_A_TWR")
	cTowerPosition := frequencyForPosition(t, "EKCH_C_TWR")
	apronPosition := frequencyForPosition(t, "EKCH_A_GND")

	strip := &models.Strip{
		Callsign:          "SAS790",
		Session:           92,
		Destination:       "EKCH",
		Runway:            stringPtr("22L"),
		Stand:             stringPtr("A17"),
		Owner:             stringPtr(aTowerPosition),
		PositionLatitude:  float64Ptr(lat),
		PositionLongitude: float64Ptr(lon),
	}

	var updatedNextOwners []string

	stripRepo.GetByCallsignFn = func(_ context.Context, session int32, callsign string) (*models.Strip, error) {
		require.Equal(t, int32(92), session)
		require.Equal(t, "SAS790", callsign)
		return strip, nil
	}
	stripRepo.SetNextOwnersFn = func(_ context.Context, session int32, callsign string, nextOwners []string) error {
		require.Equal(t, int32(92), session)
		require.Equal(t, "SAS790", callsign)
		updatedNextOwners = append([]string(nil), nextOwners...)
		return nil
	}

	sessionRepo.GetByIDFn = func(_ context.Context, id int32) (*models.Session, error) {
		require.Equal(t, int32(92), id)
		return &models.Session{
			ID:      92,
			Airport: "EKCH",
			ActiveRunways: pkgModels.ActiveRunways{
				ArrivalRunways: []string{"22L"},
			},
		}, nil
	}

	sectorRepo.ListBySessionFn = func(_ context.Context, session int32) ([]*models.SectorOwner, error) {
		require.Equal(t, int32(92), session)
		return []*models.SectorOwner{
			{
				Session:  92,
				Sector:   []string{"TE"},
				Position: aTowerPosition,
			},
			{
				Session:  92,
				Sector:   []string{"GWA"},
				Position: cTowerPosition,
			},
			{
				Session:  92,
				Sector:   []string{"AA"},
				Position: apronPosition,
			},
		}, nil
	}

	srv := &Server{
		frontendHub: frontendHub,
		stripRepo:   stripRepo,
		sessionRepo: sessionRepo,
		sectorRepo:  sectorRepo,
	}

	err = srv.UpdateRouteForStrip("SAS790", 92, true)
	require.NoError(t, err)

	assert.Equal(t, []string{apronPosition}, updatedNextOwners)
	require.Len(t, frontendHub.OwnersUpdates, 1)
	assert.Equal(t, []string{apronPosition}, frontendHub.OwnersUpdates[0].NextOwners)
	assert.Equal(t, "SAS790", frontendHub.OwnersUpdates[0].Callsign)
}

func TestResolveRouteSectorOwner_UsesOverrideTargetFirst(t *testing.T) {
	t.Parallel()

	owner, ok := resolveRouteSectorOwner(
		"GWA",
		map[string]string{
			"TE":  "EKCH_A_TWR",
			"GWA": "EKCH_C_TWR",
		},
		map[string]string{"GWA": "TE"},
	)

	require.True(t, ok)
	assert.Equal(t, "EKCH_A_TWR", owner)
}

func TestResolveRouteSectorOwner_FallsBackToOriginalSector(t *testing.T) {
	t.Parallel()

	owner, ok := resolveRouteSectorOwner(
		"GWA",
		map[string]string{
			"GWA": "EKCH_C_TWR",
		},
		map[string]string{"GWA": "TE"},
	)

	require.True(t, ok)
	assert.Equal(t, "EKCH_C_TWR", owner)
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

func frequencyForPosition(t *testing.T, name string) string {
	t.Helper()

	position, err := config.GetPositionByName(name)
	require.NoError(t, err)
	return position.Frequency
}

func stringPtr(value string) *string {
	return &value
}

func float64Ptr(value float64) *float64 {
	return &value
}
