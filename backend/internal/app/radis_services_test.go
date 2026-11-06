package app

import (
	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/aman"
	appconfig "FlightStrips/internal/config"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func TestRADISServiceFixturesUseProductionTypedLoaderAndAirportInputs(t *testing.T) {
	root, err := filepath.Abs("../../..")
	require.NoError(t, err)
	paths := []string{"backend/internal/config/testdata/radis-services/ekbi.airport.yaml", "backend/internal/config/testdata/radis-services/ekyt.airport.yaml"}
	definitions, err := appconfig.LoadAirportFiles(appconfig.ProductionAirportCatalog(), paths, appconfig.AirportLoadOptions{RepositoryRoot: root})
	require.NoError(t, err)
	sat := configureSATInputs(Config{Airports: definitions})
	cdm := configureCDMInputs(Config{Airports: definitions})
	for index, airport := range []string{"EKBI", "EKYT"} {
		definition, ok := definitions.Lookup(airport)
		require.True(t, ok)
		workflow, err := airportworkflow.Bind(definition, airportworkflow.RADISDescriptor())
		require.NoError(t, err)
		require.Equal(t, "radis", workflow.Definition().ViewID())
		require.True(t, sat.readiness[airport].Ready)
		expectedStands := 1
		if airport == "EKBI" {
			expectedStands = 2
		}
		require.Len(t, sat.registry.Stands().Stands(airport), expectedStands)
		require.Equal(t, 31+index, cdm.ConfigForAirport(airport).DefaultRate)
		require.Equal(t, 3+index, cdm.ConfigForAirport(airport).DefaultTaxiMinutes)
		require.Equal(t, "Synthetic "+airport+" hold.", definition.TextPilotInputs().Presets["si"][0].Text)
		require.False(t, definition.ServiceEnabled("pdc"))
		require.False(t, definition.ServiceEnabled("aman"))
	}
	a, _ := sat.registry.Lookup("EKBI")
	b, _ := sat.registry.Lookup("EKYT")
	require.Same(t, a.Aircraft, b.Aircraft)
	require.Same(t, a.Engines, b.Engines)
	require.NotSame(t, a.Policy, b.Policy)
	require.Nil(t, cdm.ConfigForAirport("EKCH"))
	require.Empty(t, sat.registry.Stands().Stands("EKCH"))
}

func TestRADISFixturesRejectAMANActivationCommandsAndPublication(t *testing.T) {
	root, err := filepath.Abs("../../..")
	require.NoError(t, err)
	defs, err := appconfig.LoadAirportFiles(appconfig.ProductionAirportCatalog(), []string{"backend/internal/config/testdata/radis-services/ekbi.airport.yaml", "backend/internal/config/testdata/radis-services/ekyt.airport.yaml"}, appconfig.AirportLoadOptions{RepositoryRoot: root})
	require.NoError(t, err)
	cfg := Config{Airports: defs, EnablePDC: true, AMAN: aman.RuntimeConfig{Mode: aman.ModeAuthoritative, EnabledAirports: []string{"EKBI", "EKYT"}}}
	filtered, err := enforceAirportCapabilities(cfg)
	require.NoError(t, err)
	require.False(t, filtered.EnablePDC)
	require.Equal(t, aman.ModeDisabled, filtered.AMAN.Mode)
	require.Empty(t, filtered.AMAN.EnabledAirports)
	scope := amanAirportScope(cfg, nil)
	gate := &amanCommandGate{scope: scope}
	transport := &amanTransport{scope: scope}
	for _, airport := range []string{"EKBI", "EKYT"} {
		_, err = gate.MoveFlight(t.Context(), aman.CommandContext{SessionID: 1, Airport: airport}, aman.MoveFlightCommand{})
		require.ErrorContains(t, err, "disabled")
		_, err = transport.CurrentAMANState(t.Context(), airport)
		require.ErrorContains(t, err, "disabled")
		require.ErrorContains(t, transport.PublishAMANState(t.Context(), aman.AirportState{Airport: airport, SessionID: 1}), "disabled")
	}
}
