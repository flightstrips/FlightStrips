package app

import (
	appconfig "FlightStrips/internal/config"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGSXRemainsEKCHOnlyWhenRegionalsAreSelected(t *testing.T) {
	root, err := filepath.Abs("../../..")
	require.NoError(t, err)
	definitions, err := appconfig.LoadAirportFiles(appconfig.ProductionAirportCatalog(), []string{
		"backend/config/airports/ekch.airport.yaml", "backend/config/airports/ekbi.airport.yaml", "backend/config/airports/ekyt.airport.yaml",
	}, appconfig.AirportLoadOptions{RepositoryRoot: root, GSXEnabled: true, LookupEnv: func(string) (string, bool) { return "backend/config/test/ICAO_Aircraft.json", true }})
	require.NoError(t, err)
	sceneries, err := loadGSXSceneriesDefinition(Config{Airports: definitions, EnableGSXStandFeed: true})
	require.NoError(t, err)
	require.Contains(t, sceneries, "EKCH")
	require.NotContains(t, sceneries, "EKBI")
	require.NotContains(t, sceneries, "EKYT")
}
