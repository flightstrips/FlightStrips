package app

import (
	appconfig "FlightStrips/internal/config"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealRegionalDefinitionsAssembleReadyStandServicesWithoutDeferredFeatures(t *testing.T) {
	root, err := filepath.Abs("../../..")
	require.NoError(t, err)
	definitions, err := appconfig.LoadAirportFiles(appconfig.ProductionAirportCatalog(), []string{
		"backend/config/airports/ekch.airport.yaml", "backend/config/airports/ekbi.airport.yaml", "backend/config/airports/ekyt.airport.yaml",
	}, appconfig.AirportLoadOptions{RepositoryRoot: root, GSXEnabled: true,
		DeploymentServices: map[string]bool{"stand_assignment": true, "cdm": true, "text_pilot": true},
		LookupEnv:          func(string) (string, bool) { return "backend/config/test/ICAO_Aircraft.json", true }})
	require.NoError(t, err)
	inputs := configureSATInputs(Config{Airports: definitions})
	for _, airport := range []string{"EKCH", "EKBI", "EKYT"} {
		require.True(t, inputs.readiness[airport].Ready, "%s: %s", airport, inputs.readiness[airport].Reason)
	}
	for _, airport := range []string{"EKBI", "EKYT"} {
		definition, _ := definitions.Lookup(airport)
		for _, service := range []string{"text_pilot", "cdm", "pdc", "aman"} {
			require.False(t, definition.ServiceEnabled(service), "%s %s must remain disabled", airport, service)
		}
		input, ok := inputs.registry.Lookup(airport)
		require.True(t, ok)
		require.True(t, input.Policy.EnforceStandUse)
	}
	first, _ := inputs.registry.Lookup("EKCH")
	second, _ := inputs.registry.Lookup("EKBI")
	require.Same(t, first.Aircraft, second.Aircraft)
	require.False(t, first.Policy.EnforceStandUse, "preserve existing EKCH policy semantics")
}
