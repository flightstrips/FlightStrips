package app

import (
	appconfig "FlightStrips/internal/config"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/services"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"
)

func loadAssemblyAirports(t *testing.T, mutate func(map[string]any), options appconfig.AirportLoadOptions) *appconfig.AirportRegistry {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(root, "backend", "config", "airports", "ekch.airport.yaml"))
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, yaml.Unmarshal(data, &document))
	mutate(document)
	data, err = yaml.Marshal(document)
	require.NoError(t, err)
	selected := filepath.Join(t.TempDir(), "selected.yaml")
	require.NoError(t, os.WriteFile(selected, data, 0600))
	options.RepositoryRoot = root
	registry, err := appconfig.LoadAirportFiles(appconfig.ProductionAirportCatalog(), []string{selected}, options)
	require.NoError(t, err)
	return registry
}

func TestDisabledCDMDoesNotRegisterBackgroundWorker(t *testing.T) {
	registry := loadAssemblyAirports(t, func(c map[string]any) {
		c["services"].(map[string]any)["cdm"] = map[string]any{"availability": "disabled"}
	}, appconfig.AirportLoadOptions{})
	definition, _ := registry.Lookup("EKCH")
	require.False(t, definition.ServiceEnabled("cdm"))
	poolConfig, err := pgxpool.ParseConfig("postgres://user:password@127.0.0.1:1/test")
	require.NoError(t, err)
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	application, err := Build(context.Background(), Config{Environment: "test", Airports: registry, EKCH: definition, CDMKey: "configured-deployment-key"}, Dependencies{DBPool: pool, AuthenticationService: services.NewTestAuthenticationService()})
	require.NoError(t, err)
	require.Len(t, application.workers, 3, "only the session monitor and two transport hubs should start")
}

func TestAMANUsesSelectedAircraftDatabaseWithoutSAT(t *testing.T) {
	t.Setenv("GRPLUGIN_ICAO_AIRCRAFT_JSON", filepath.Join(t.TempDir(), "missing-legacy.json"))
	registry := loadAssemblyAirports(t, func(c map[string]any) {
		c["services"].(map[string]any)["stand_assignment"] = map[string]any{"availability": "disabled", "aircraft_icao": map[string]any{"repo_path": "backend/config/test/ICAO_Aircraft.json"}}
	}, appconfig.AirportLoadOptions{DeploymentServices: map[string]bool{"aman": true}})
	definition, _ := registry.Lookup("EKCH")
	require.False(t, definition.ServiceEnabled("stand_assignment"))
	require.True(t, definition.ServiceEnabled("aman"))
	_, ok := definition.Resource("services.stand_assignment.stands")
	require.False(t, ok, "AMAN must not acquire disabled SAT assets")
	engines, err := loadAMANAircraftEngineReference(definition)
	require.NoError(t, err)
	engine, ok := engines.Lookup("A320")
	require.True(t, ok)
	require.Equal(t, sat.EngineJet, engine)
}

func TestGSXOnlyRejectsAnotherAirportsScenery(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(root, "backend", "config", "airports", "ekch.airport.yaml"))
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, yaml.Unmarshal(data, &document))
	document["services"].(map[string]any)["stand_assignment"] = map[string]any{"availability": "disabled", "scenery": map[string]any{"environment": "GSX_TEST_SCENERY", "default_repo_path": "backend/config/ekch/gsx_sceneries.json"}}
	data, err = yaml.Marshal(document)
	require.NoError(t, err)
	selected := filepath.Join(t.TempDir(), "selected.yaml")
	require.NoError(t, os.WriteFile(selected, data, 0600))
	sceneryPath := filepath.Join(t.TempDir(), "scenery.json")
	options := appconfig.AirportLoadOptions{RepositoryRoot: root, GSXEnabled: true, LookupEnv: func(string) (string, bool) { return sceneryPath, true }}
	wrong := []byte(`{"icao":"EKBI","gates":{"A31":{"test":{"stand":"WRONG-AIRPORT"}}}}`)
	require.NoError(t, os.WriteFile(sceneryPath, wrong, 0600))
	registry, err := appconfig.LoadAirportFiles(appconfig.ProductionAirportCatalog(), []string{selected}, options)
	require.ErrorContains(t, err, "must describe airport EKCH")
	require.Nil(t, registry)
	require.NoError(t, os.WriteFile(sceneryPath, []byte(`{"icao":"EKCH","gates":{"A31":{"test":{"stand":"CORRECT"}}}}`), 0600))
	registry, err = appconfig.LoadAirportFiles(appconfig.ProductionAirportCatalog(), []string{selected}, options)
	require.NoError(t, err)
	definition, _ := registry.Lookup("EKCH")
	loaded, err := loadGSXSceneriesDefinition(Config{EKCH: definition, EnableGSXStandFeed: true})
	require.NoError(t, err)
	stand, _ := loaded.Resolve("EKCH", "A31", "test", "")
	require.Equal(t, "CORRECT", stand)
	require.NoError(t, os.WriteFile(sceneryPath, wrong, 0600))
	_, err = loadGSXSceneriesDefinition(Config{EKCH: definition, EnableGSXStandFeed: true})
	require.ErrorContains(t, err, "must describe airport EKCH")
}
