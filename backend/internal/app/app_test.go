package app

import (
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/services"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type appTestHoppieClient struct{}

func (appTestHoppieClient) Poll(context.Context, string) ([]pdc.Message, error) { return nil, nil }
func (appTestHoppieClient) SendCPDLC(context.Context, string, string, string) error {
	return nil
}
func (appTestHoppieClient) SendTelex(context.Context, string, string, string) error {
	return nil
}

func TestBuildFailsWhenPDCIsEnabledWithoutHoppieConfiguration(t *testing.T) {
	poolConfig, err := pgxpool.ParseConfig("postgres://user:password@127.0.0.1:1/test")
	require.NoError(t, err)
	dbPool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	require.NoError(t, err)
	t.Cleanup(dbPool.Close)

	application, err := Build(context.Background(), Config{
		Environment:          "test",
		EnablePDC:            true,
		EnableCDMConfigStore: false,
		EnableECFMP:          false,
		EnableVATSIM:         false,
	}, Dependencies{
		DBPool:                dbPool,
		AuthenticationService: services.NewTestAuthenticationService(),
	})

	require.Nil(t, application)
	require.Error(t, err)
	require.Contains(t, err.Error(), "PDC is enabled but no Hoppie client or HOPPIE_LOGON is configured")
}

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

	// CDM and the core session monitor are the only normal workers enabled by
	// this fixture. SAT must not add a worker or timer when unavailable.
	require.Len(t, application.workers, 4)
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
	require.Len(t, application.workers, 4)

	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/pdc/status", nil))
	require.Equal(t, http.StatusNotFound, response.Code)

	statusRequest := httptest.NewRequest(http.MethodGet, "/api/stand/status", nil)
	statusResponse := httptest.NewRecorder()
	application.Handler().ServeHTTP(statusResponse, statusRequest)
	require.Equal(t, http.StatusUnauthorized, statusResponse.Code)
}

func TestBuildAssemblesPDCOnlyWithInjectedRealClientBoundary(t *testing.T) {
	poolConfig, err := pgxpool.ParseConfig("postgres://user:password@127.0.0.1:1/test")
	require.NoError(t, err)
	dbPool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	require.NoError(t, err)
	t.Cleanup(dbPool.Close)

	application, err := Build(context.Background(), Config{
		Environment:          "test",
		EnablePDC:            true,
		EnableCDMConfigStore: false,
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
		PDCClient:             appTestHoppieClient{},
	})
	require.NoError(t, err)
	require.Len(t, application.workers, 5)

	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/pdc/status", nil))
	require.NotEqual(t, http.StatusNotFound, response.Code)
}

func TestBuildGatesEFBAPIBehindFeatureFlag(t *testing.T) {
	poolConfig, err := pgxpool.ParseConfig("postgres://user:password@127.0.0.1:1/test")
	require.NoError(t, err)
	dbPool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	require.NoError(t, err)
	t.Cleanup(dbPool.Close)

	build := func(enabled bool) *App {
		application, buildErr := Build(context.Background(), Config{
			Environment:          "test",
			EnableEFB:            enabled,
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
		}, Dependencies{DBPool: dbPool, AuthenticationService: services.NewTestAuthenticationService()})
		require.NoError(t, buildErr)
		return application
	}

	disabled := httptest.NewRecorder()
	build(false).Handler().ServeHTTP(disabled, httptest.NewRequest(http.MethodGet, "/api/efb/me", nil))
	require.Equal(t, http.StatusNotFound, disabled.Code)

	enabled := httptest.NewRecorder()
	build(true).Handler().ServeHTTP(enabled, httptest.NewRequest(http.MethodGet, "/api/efb/me", nil))
	require.Equal(t, http.StatusUnauthorized, enabled.Code)
}
