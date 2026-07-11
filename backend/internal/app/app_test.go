package app

import (
	"FlightStrips/internal/services"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestBuildKeepsUnrelatedApplicationAvailableWhenStandAssignmentIsUnavailable(t *testing.T) {
	poolConfig, err := pgxpool.ParseConfig("postgres://user:password@127.0.0.1:1/test")
	require.NoError(t, err)
	dbPool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	require.NoError(t, err)
	t.Cleanup(dbPool.Close)

	application, err := Build(context.Background(), Config{
		Environment:           "test",
		EnableStandAssignment: true,
		EnableCDMConfigStore:  false,
		EnablePDC:             false,
		EnableECFMP:           false,
		EnableECFMPAPI:        false,
		EnablePilotAPI:        false,
		EnableALB:             false,
		EnableMetar:           false,
		EnableVATSIM:          false,
		EnableTraffic:         false,
		EnableDBSeed:          false,
	}, Dependencies{
		DBPool:                dbPool,
		AuthenticationService: services.NewTestAuthenticationService(),
	})
	require.NoError(t, err)
	require.NotNil(t, application)
	require.False(t, application.StandAssignmentReadiness().Ready)
	require.True(t, application.StandAssignmentReadiness().Enabled)

	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	require.Equal(t, http.StatusOK, response.Code)

	workersContext, cancelWorkers := context.WithCancel(context.Background())
	application.StartWorkers(workersContext)
	cancelWorkers()

	// CDM is the only normal worker enabled by this fixture. SAT must not add
	// a worker or timer when its configuration is unavailable.
	require.Len(t, application.workers, 1)
}

func TestBuildDisablesStandAssignmentByDefault(t *testing.T) {
	poolConfig, err := pgxpool.ParseConfig("postgres://user:password@127.0.0.1:1/test")
	require.NoError(t, err)
	dbPool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	require.NoError(t, err)
	t.Cleanup(dbPool.Close)

	application, err := Build(context.Background(), Config{
		Environment:          "production",
		EnableCDMConfigStore: false,
		EnablePDC:            false,
		EnableECFMP:          false,
		EnableECFMPAPI:       false,
		EnablePilotAPI:       false,
		EnableALB:            false,
		EnableMetar:          false,
		EnableVATSIM:         false,
		EnableTraffic:        false,
		EnableDBSeed:         false,
	}, Dependencies{
		DBPool:                dbPool,
		AuthenticationService: services.NewTestAuthenticationService(),
	})
	require.NoError(t, err)
	require.False(t, application.StandAssignmentReadiness().Enabled)
	require.False(t, application.StandAssignmentReadiness().Ready)
	require.Len(t, application.workers, 1)
}
