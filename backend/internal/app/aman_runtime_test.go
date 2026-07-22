package app

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/services"
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type amanAppTestComponent string

func (c amanAppTestComponent) Name() string { return string(c) }

type amanAppTestObservationSink struct{}

func (amanAppTestObservationSink) Observe(context.Context, aman.FlightObservation) error { return nil }

type amanAppTestWorker struct {
	started chan struct{}
	stopped chan struct{}
	once    sync.Once
}

func newAMANAppTestWorker() *amanAppTestWorker {
	return &amanAppTestWorker{started: make(chan struct{}, 1), stopped: make(chan struct{})}
}

func (w *amanAppTestWorker) Run(ctx context.Context, _ time.Duration) {
	w.started <- struct{}{}
	<-ctx.Done()
	w.once.Do(func() { close(w.stopped) })
}

func amanAppTestDependencies() aman.Dependencies {
	component := amanAppTestComponent("injected test component")
	return aman.Dependencies{
		Repositories:           component,
		NavigationMaterializer: component,
		NavigationReader:       component,
		Predictor:              component,
		StateEngine:            component,
		SequenceService:        component,
		Publisher:              component,
		ValidationService:      component,
		HealthService:          component,
		ObservationSink:        amanAppTestObservationSink{},
		ReconciliationWorker:   newAMANAppTestWorker(),
	}
}

func TestValidateAMANTerminalGeometry(t *testing.T) {
	tempFile, err := os.CreateTemp(t.TempDir(), "terminal-*.geojson")
	require.NoError(t, err)
	require.NoError(t, tempFile.Close())

	require.NoError(t, validateAMANTerminalGeometry(aman.RuntimeConfig{}))
	require.NoError(t, validateAMANTerminalGeometry(aman.RuntimeConfig{Mode: aman.ModeShadow, TerminalGeometryPath: tempFile.Name()}))
	require.ErrorContains(t, validateAMANTerminalGeometry(aman.RuntimeConfig{Mode: aman.ModeShadow, TerminalGeometryPath: t.TempDir()}), "must name a file")
	require.ErrorContains(t, validateAMANTerminalGeometry(aman.RuntimeConfig{Mode: aman.ModeShadow, TerminalGeometryPath: "missing.geojson"}), "terminal geometry path")
	require.ErrorContains(t, validateAMANTerminalGeometry(aman.RuntimeConfig{TerminalGeometryPath: "missing.geojson"}), "terminal geometry path")
}

func TestApplicationConfigDefaultsAMANToDisabled(t *testing.T) {
	config := (Config{}).withDefaults()
	require.Equal(t, aman.ModeDisabled, config.AMAN.Mode)
}

func TestBuildRejectsAuthoritativeAMANWithoutTypedCommandService(t *testing.T) {
	geometry, err := os.CreateTemp(t.TempDir(), "terminal-*.geojson")
	require.NoError(t, err)
	require.NoError(t, geometry.Close())

	_, err = Build(context.Background(), Config{AMAN: aman.RuntimeConfig{
		Mode: aman.ModeAuthoritative, EnabledAirports: []string{"EKCH"},
		TerminalGeometryPath: geometry.Name(), NavigationSourceAdapter: aman.NavigationAdapterAIRACNet,
	}}, Dependencies{AMAN: amanAppTestDependencies()})

	require.EqualError(t, err, "initialize AMAN commands: authoritative runtime requires typed command service")
}

func TestBuildAddsAMANWorkersOnlyWhenEnabled(t *testing.T) {
	poolConfig, err := pgxpool.ParseConfig("postgres://user:password@127.0.0.1:1/test")
	require.NoError(t, err)
	dbPool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	require.NoError(t, err)
	t.Cleanup(dbPool.Close)
	geometry, err := os.CreateTemp(t.TempDir(), "terminal-*.geojson")
	require.NoError(t, err)
	require.NoError(t, geometry.Close())
	amanDeps := amanAppTestDependencies()

	application, err := Build(context.Background(), Config{
		Environment:          "test",
		EnableCDMConfigStore: false,
		EnablePDC:            false,
		EnableECFMP:          false,
		EnableECFMPAPI:       false,
		EnablePilotAPI:       false,
		EnableALB:            false,
		EnableMetar:          false,
		EnableVATSIM:         true,
		EnableTraffic:        false,
		EnableDBSeed:         false,
		AMAN: aman.RuntimeConfig{
			Mode:                    aman.ModeShadow,
			EnabledAirports:         []string{"EKCH"},
			TerminalGeometryPath:    geometry.Name(),
			NavigationSourceAdapter: aman.NavigationAdapterAIRACNet,
		},
	}, Dependencies{
		DBPool:                dbPool,
		AuthenticationService: services.NewTestAuthenticationService(),
		AMAN:                  amanDeps,
	})
	require.NoError(t, err)
	require.NotNil(t, application.AMANRuntime())
	require.True(t, application.AMANRuntime().Ownership().LegacyArrivalETAWriter)
	require.Len(t, application.workers, 7)

	ctx, cancel := context.WithCancel(context.Background())
	application.StartWorkers(ctx)
	reconcile := amanDeps.ReconciliationWorker.(*amanAppTestWorker)
	require.Eventually(t, func() bool { return len(reconcile.started) == 1 }, time.Second, 10*time.Millisecond)
	cancel()
	require.Eventually(t, func() bool {
		select {
		case <-reconcile.stopped:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
}
