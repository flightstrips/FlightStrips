package app

import (
	"FlightStrips/internal/aman"
	appconfig "FlightStrips/internal/config"
	"FlightStrips/internal/services"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestBuildConsumesSelectedProductionEKCH(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	poolConfig, err := pgxpool.ParseConfig("postgres://user:password@127.0.0.1:1/test")
	require.NoError(t, err)
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	application, err := Build(context.Background(), Config{
		Environment: "test", AirportRepositoryRoot: root, AirportFiles: []string{"backend/config/airports/ekch.airport.yaml"},
		EnablePDC: true, AMAN: aman.DefaultRuntimeConfig(),
	}, Dependencies{DBPool: pool, AuthenticationService: services.NewTestAuthenticationService()})
	require.NoError(t, err)
	require.NotNil(t, application.Airports())
	definition, ok := application.Airports().Lookup("EKCH")
	require.True(t, ok)
	require.NotEmpty(t, definition.Revision())
	require.Len(t, definition.Document().Workflow.Bays, 18)
	require.True(t, definition.ServiceEnabled("pdc"))
	require.False(t, definition.ServiceEnabled("aman"))
	require.False(t, definition.ServiceEnabled("stand_assignment"))
	require.False(t, application.AMANRuntime().Enabled())
	require.False(t, application.StandAssignmentReadiness().Enabled)
	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/pdc/status", nil))
	require.NotEqual(t, http.StatusNotFound, response.Code)
	// Loading another explicitly selected definition cannot rebind EKCH consumers.
	beforeLat, beforeLon := appconfig.GetAirportCoordinates()
	_, err = appconfig.LoadAirportFiles(appconfig.ProductionAirportCatalog(), []string{"backend/config/airports/ekch.airport.yaml"}, appconfig.AirportLoadOptions{RepositoryRoot: root})
	require.NoError(t, err)
	afterLat, afterLon := appconfig.GetAirportCoordinates()
	require.Equal(t, beforeLat, afterLat)
	require.Equal(t, beforeLon, afterLon)
}
func TestBuildRejectsInvalidSelectionBeforeOpeningDatabase(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	for _, selected := range []string{"missing.yaml", ".github/specs/multi-airport-support/examples/zzzz.airport.yaml"} {
		_, err := Build(context.Background(), Config{AirportRepositoryRoot: root, AirportFiles: []string{selected}}, Dependencies{})
		require.ErrorContains(t, err, "initialize selected airports")
	}
}
