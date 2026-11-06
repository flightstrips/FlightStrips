package app

import (
	appconfig "FlightStrips/internal/config"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"
)

// These fictional definitions test service assembly without activating a new
// production handler or supplying real airport operating values.
func syntheticServiceAirports(t *testing.T) *appconfig.AirportRegistry {
	t.Helper()
	root := t.TempDir()
	project, err := filepath.Abs("../../..")
	require.NoError(t, err)
	template, err := os.ReadFile(filepath.Join(project, ".github/specs/multi-airport-support/examples/zzzz.airport.yaml"))
	require.NoError(t, err)
	for source, target := range map[string]string{"backend/config/ekch/GRpluginAircraftInfo.txt": "aircraft.txt", "backend/config/test/ICAO_Aircraft.json": "aircraft.json"} {
		data, err := os.ReadFile(filepath.Join(project, source))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(root, target), data, 0600))
	}
	require.NoError(t, os.Mkdir(filepath.Join(root, "cdm"), 0700))
	for _, name := range []string{"rates.json", "sid.json", "taxi.json"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, "cdm", name), []byte("[]"), 0600))
	}
	var selected []string
	for i, airport := range []string{"AAAA", "BBBB"} {
		var doc map[string]any
		require.NoError(t, yaml.Unmarshal([]byte(strings.ReplaceAll(string(template), "ZZZZ", airport)), &doc))
		geography := doc["geography"].(map[string]any)
		geography["runways"] = []string{"09", "27"}
		geography["initial_cfl_ft"] = map[string]any{"09": 3000, "27": 3000}
		services := doc["services"].(map[string]any)
		standFile, policyFile, sceneryFile := airport+".stands", airport+".policy.json", airport+".scenery.json"
		require.NoError(t, os.WriteFile(filepath.Join(root, standFile), []byte("STAND:"+airport+":A1:N000.00.00.000:E000.00.00.000:30\n"), 0600))
		fallbacks := []string{}
		for _, name := range []string{"airliner_default", "business_vip", "cargo", "military", "military_helicopter", "helicopter", "ga_private", "unknown"} {
			fallbacks = append(fallbacks, fmt.Sprintf(`%q:{"stands":{"tier1":{"A1":100}}}`, name))
		}
		require.NoError(t, os.WriteFile(filepath.Join(root, policyFile), []byte(`{"rules":[],"stand_groups":{},"fallback_rules":{`+strings.Join(fallbacks, ",")+`}}`), 0600))
		require.NoError(t, os.WriteFile(filepath.Join(root, sceneryFile), []byte(`{"icao":"`+airport+`","gates":{}}`), 0600))
		asset := func(path string) map[string]any { return map[string]any{"repo_path": path} }
		services["stand_assignment"] = map[string]any{"availability": "enabled", "stands": asset(standFile), "aircraft_reference": asset("aircraft.txt"), "aircraft_icao": asset("aircraft.json"), "airline_assignment": asset(policyFile), "scenery": asset(sceneryFile)}
		textFile := airport + ".text.json"
		require.NoError(t, os.WriteFile(filepath.Join(root, textFile), []byte(`{"airport":"`+airport+`","preset_provider":"static/v1","layouts":{},"presets":{"si":[{"id":"hold","label":"HOLD","text":"Synthetic `+airport+` hold.","tone":"phrase"}]}}`), 0600))
		services["text_pilot"] = map[string]any{"availability": "enabled", "provider": "frequency-message/v1", "layout_data": asset(textFile)}
		services["cdm"] = map[string]any{"availability": "enabled", "config_root": "cdm", "config": map[string]any{"rateUri": "rates.json", "sidIntervalUri": "sid.json", "taxizonesUri": "taxi.json", "rate": 31 + i, "rateLvo": 11 + i, "defaultTaxiTime": 3 + i, "deice": map[string]any{"light": 1 + i, "medium": 2 + i, "heavy": 3 + i, "super": 4 + i, "platform": []any{}}}}
		data, err := yaml.Marshal(doc)
		require.NoError(t, err)
		path := filepath.Join(root, airport+".yaml")
		require.NoError(t, os.WriteFile(path, data, 0600))
		selected = append(selected, path)
	}
	catalog, err := appconfig.NewAirportCatalog(appconfig.HandlerDescriptor{ID: "example-paper/v1", View: "example-paper", Layouts: []string{"example-paper"}, DecodeSettings: appconfig.TypedAirportSettings[map[string]any], Validate: func(*appconfig.AirportDocument) error { return nil }})
	require.NoError(t, err)
	registry, err := appconfig.LoadAirportFiles(catalog, selected, appconfig.AirportLoadOptions{RepositoryRoot: root})
	require.NoError(t, err)
	return registry
}

func TestProductionServiceInputsAreAirportBoundAndReferencesShared(t *testing.T) {
	registry := syntheticServiceAirports(t)
	inputs := configureSATInputs(Config{Airports: registry})
	require.True(t, inputs.readiness["AAAA"].Ready)
	require.True(t, inputs.readiness["BBBB"].Ready)
	a, ok := inputs.registry.Lookup("AAAA")
	require.True(t, ok)
	b, ok := inputs.registry.Lookup("BBBB")
	require.True(t, ok)
	require.Same(t, a.Aircraft, b.Aircraft)
	require.Same(t, a.Engines, b.Engines)
	require.NotSame(t, a.Policy, b.Policy)
	require.Len(t, inputs.registry.Stands().Stands("AAAA"), 1)
	require.Empty(t, inputs.registry.Stands().Stands("EKCH"))
	store := configureCDMInputs(Config{Airports: registry})
	require.Equal(t, 31, store.ConfigForAirport("AAAA").DefaultRate)
	require.Equal(t, 32, store.ConfigForAirport("BBBB").DefaultRate)
	require.Equal(t, 4, store.ConfigForAirport("BBBB").DefaultTaxiMinutes)
	require.Nil(t, store.ConfigForAirport("EKCH"))
	definition, _ := registry.Lookup("AAAA")
	path, _ := definition.Resource("services.stand_assignment.stands")
	require.NoError(t, os.Remove(path))
	failed := configureSATInputs(Config{Airports: registry})
	require.False(t, failed.readiness["AAAA"].Ready)
	require.True(t, failed.readiness["BBBB"].Ready)
	require.True(t, failed.aggregate.Ready)
	_, ok = failed.registry.Lookup("AAAA")
	require.False(t, ok)
	_, ok = failed.registry.Lookup("BBBB")
	require.True(t, ok)
}

func TestTextPilotInputsStayBoundToDefinitionAndSnapshot(t *testing.T) {
	registry := syntheticServiceAirports(t)
	a, _ := registry.Lookup("AAAA")
	b, _ := registry.Lookup("BBBB")
	first := a.TextPilotInputs()
	require.Equal(t, "Synthetic AAAA hold.", first.Presets["si"][0].Text)
	require.Equal(t, "Synthetic BBBB hold.", b.TextPilotInputs().Presets["si"][0].Text)
	first.Presets["si"][0].Text = "changed"
	require.Equal(t, "Synthetic AAAA hold.", a.TextPilotInputs().Presets["si"][0].Text)
}
