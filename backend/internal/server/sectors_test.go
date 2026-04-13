package server

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
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
