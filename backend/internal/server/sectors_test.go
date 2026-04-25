package server

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeSectorChanges_UsesPublicSectorNameForVariants(t *testing.T) {
	t.Parallel()

	changes := computeSectorChanges(
		[]*models.SectorOwner{
			{Sector: []string{"GWA"}, Position: frequencyForPosition(t, "EKCH_A_TWR")},
		},
		[]*models.SectorOwner{
			{Sector: []string{"GWA"}, Position: frequencyForPosition(t, "EKCH_D_TWR")},
		},
	)

	require.Len(t, changes, 1)
	assert.Equal(t, "GW", changes[0].SectorName)
	assert.Equal(t, "EKCH_A_TWR", changes[0].FromPosition)
	assert.Equal(t, "EKCH_D_TWR", changes[0].ToPosition)
}

func TestComputeSectorChanges_ReportsBothGWVariantsWithPublicName(t *testing.T) {
	t.Parallel()

	changes := computeSectorChanges(
		[]*models.SectorOwner{
			{
				Sector:   []string{"GWA"},
				Position: frequencyForPosition(t, "EKCH_A_TWR"),
			},
			{
				Sector:   []string{"GWD"},
				Position: frequencyForPosition(t, "EKCH_C_TWR"),
			},
		},
		[]*models.SectorOwner{
			{
				Sector:   []string{"GWA"},
				Position: frequencyForPosition(t, "EKCH_D_TWR"),
			},
			{
				Sector:   []string{"GWD"},
				Position: frequencyForPosition(t, "EKCH_D_TWR"),
			},
		},
	)

	require.Len(t, changes, 2)
	assert.ElementsMatch(t, []string{"GW", "GW"}, []string{changes[0].SectorName, changes[1].SectorName})
	assert.ElementsMatch(t,
		[]string{"EKCH_A_TWR", "EKCH_C_TWR"},
		[]string{changes[0].FromPosition, changes[1].FromPosition},
	)
	assert.ElementsMatch(t,
		[]string{"EKCH_D_TWR", "EKCH_D_TWR"},
		[]string{changes[0].ToPosition, changes[1].ToPosition},
	)
}

func TestComputeSectorChanges_UnknownSectorNamePassesThrough(t *testing.T) {
	t.Parallel()

	changes := computeSectorChanges(
		nil,
		[]*models.SectorOwner{
			{Sector: []string{"CUSTOM"}, Position: frequencyForPosition(t, "EKCH_A_TWR")},
		},
	)

	require.Len(t, changes, 1)
	assert.Equal(t, "CUSTOM", changes[0].SectorName)
	assert.Equal(t, "", changes[0].FromPosition)
	assert.Equal(t, "EKCH_A_TWR", changes[0].ToPosition)
}

func TestEkchGwArrivalVariantMatchesEkchConfig(t *testing.T) {
	t.Parallel()

	assertEkchGwOwner(t, []string{"22L", "22R"}, "GWA", "EKCH_A_TWR")
	assertEkchGwOwner(t, []string{"04L", "04R"}, "GWA", "EKCH_A_TWR")
	assertEkchGwOwner(t, []string{"30"}, "GWA", "EKCH_D_TWR")
	assertEkchGwOwner(t, []string{"30", "22R"}, "GWA", "EKCH_D_TWR")
}

func TestEkchGwDepartureVariantAlwaysAssignsToDTower(t *testing.T) {
	t.Parallel()

	assertEkchGwOwner(t, []string{"22L", "22R"}, "GWD", "EKCH_D_TWR")
	assertEkchGwOwner(t, []string{"04L", "04R"}, "GWD", "EKCH_D_TWR")
	assertEkchGwOwner(t, []string{"30"}, "GWD", "EKCH_D_TWR")
	assertEkchGwOwner(t, []string{"30", "22R"}, "GWD", "EKCH_D_TWR")
}

func assertEkchGwOwner(t *testing.T, active []string, sectorKey string, expectedPositionName string) {
	t.Helper()

	aTower, err := config.GetPositionByName("EKCH_A_TWR")
	require.NoError(t, err)
	dTower, err := config.GetPositionByName("EKCH_D_TWR")
	require.NoError(t, err)

	sectors := config.GetControllerSectors([]*config.Position{aTower, dTower}, active)

	expectedFrequency := frequencyForPosition(t, expectedPositionName)
	actualFrequency := ownerOfSectorKey(sectors, sectorKey)
	require.NotEmpty(t, actualFrequency, "expected %s to be assigned for active runways %v", sectorKey, active)
	assert.Equal(t, expectedFrequency, actualFrequency)
}

func ownerOfSectorKey(sectors map[string][]config.Sector, sectorKey string) string {
	for frequency, ownedSectors := range sectors {
		for _, sector := range ownedSectors {
			if sector.KeyOrName() == sectorKey {
				return frequency
			}
		}
	}

	return ""
}

func TestRefreshSessionSectors_UpdatesEverySession(t *testing.T) {
	t.Parallel()

	sessionRepo := &testutil.MockSessionRepository{
		ListFn: func(_ context.Context) ([]*models.Session, error) {
			return []*models.Session{
				{ID: 11},
				{ID: 42},
			}, nil
		},
	}

	var updated []int32
	err := refreshSessionSectors(context.Background(), sessionRepo, func(sessionID int32) error {
		updated = append(updated, sessionID)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, []int32{11, 42}, updated)
}

func TestGetCurrentControllerCoverage_IgnoresControllersWithoutMatchingPrefix(t *testing.T) {
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_A_TWR", Frequency: "118.100"},
		{Name: "EKDK_I_CTR", Frequency: "119.805"},
	}))

	controllerRepo := &testutil.MockControllerRepository{
		ListFn: func(_ context.Context, _ int32) ([]*models.Controller, error) {
			return []*models.Controller{
				{Callsign: "ESMS_TWR", Position: "118.100"},
				{Callsign: "EKCH_S_TWR", Position: "118.100"},
				{Callsign: "EKDK_OBS", Position: "119.805", Observer: true},
				{Callsign: "EKDK_W_CTR", Position: "119.805"},
			}, nil
		},
	}

	coverage, err := getCurrentControllerCoverage(controllerRepo, 1, nil)
	require.NoError(t, err)
	require.Len(t, coverage, 2)
	assert.ElementsMatch(t, []config.ControllerCoverage{
		{Name: "EKCH_A_TWR", Frequency: "118.100"},
		{Name: "EKDK_I_CTR", Frequency: "119.805"},
	}, coverage)
}

func TestSendControllerUpdates_DoesNotAssignSectorsToWrongPrefix(t *testing.T) {
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_A_TWR", Frequency: "118.100"},
	}))

	frontendHub := &testutil.MockFrontendHub{}
	controllerRepo := &testutil.MockControllerRepository{
		ListFn: func(_ context.Context, _ int32) ([]*models.Controller, error) {
			return []*models.Controller{
				{Callsign: "ESMS_TWR", Position: "118.100"},
				{Callsign: "EKCH_S_TWR", Position: "118.100"},
			}, nil
		},
	}

	server := &Server{frontendHub: frontendHub}
	err := server.sendControllerUpdates(1, []*models.SectorOwner{{
		Session:    1,
		Position:   "118.100",
		Sector:     []string{"TW"},
		Identifier: "EKCH_S_TWR",
	}}, controllerRepo)
	require.NoError(t, err)
	require.Len(t, frontendHub.ControllerOnlines, 2)

	assert.Equal(t, "ESMS_TWR", frontendHub.ControllerOnlines[0].Callsign)
	assert.Empty(t, frontendHub.ControllerOnlines[0].Identifier)
	assert.Empty(t, frontendHub.ControllerOnlines[0].OwnedSectors)

	assert.Equal(t, "EKCH_S_TWR", frontendHub.ControllerOnlines[1].Callsign)
	assert.Equal(t, "EKCH_S_TWR", frontendHub.ControllerOnlines[1].Identifier)
	assert.Equal(t, []string{"TW"}, frontendHub.ControllerOnlines[1].OwnedSectors)
}
