package aman

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type runtimeTestComponent string

func (c runtimeTestComponent) Name() string { return string(c) }

type runtimeTestWorker struct {
	started chan time.Duration
	stopped chan struct{}
	once    sync.Once
}

func newRuntimeTestWorker() *runtimeTestWorker {
	return &runtimeTestWorker{started: make(chan time.Duration, 1), stopped: make(chan struct{})}
}

func (w *runtimeTestWorker) Run(ctx context.Context, interval time.Duration) {
	w.started <- interval
	<-ctx.Done()
	w.once.Do(func() { close(w.stopped) })
}

func validRuntimeConfig(mode RolloutMode) RuntimeConfig {
	return RuntimeConfig{
		Mode:                    mode,
		EnabledAirports:         []string{"EKCH"},
		TerminalGeometryPath:    "testdata/terminal.geojson",
		NavigationSourceAdapter: NavigationAdapterAIRACNet,
		ReconciliationInterval:  3 * time.Second,
		SurveillanceInterval:    4 * time.Second,
	}
}

func runtimeTestDependencies() Dependencies {
	component := runtimeTestComponent("test component")
	return Dependencies{
		Repositories:           component,
		NavigationMaterializer: component,
		NavigationReader:       component,
		Predictor:              component,
		StateEngine:            component,
		SequenceService:        component,
		Publisher:              component,
		ValidationService:      component,
		HealthService:          component,
		SurveillanceWorker:     newRuntimeTestWorker(),
		ReconciliationWorker:   newRuntimeTestWorker(),
	}
}

func TestDefaultRuntimeConfigIsDisabledAndReleaseSafe(t *testing.T) {
	config := DefaultRuntimeConfig()
	require.Equal(t, ModeDisabled, config.Mode)
	require.NoError(t, config.Validate())

	runtime, err := NewRuntime(RuntimeConfig{}, Dependencies{})
	require.NoError(t, err)
	require.False(t, runtime.Enabled())
	require.True(t, runtime.Ownership().LegacyArrivalETAWriter)
}

func TestRuntimeAcceptsAllConfiguredModes(t *testing.T) {
	for _, mode := range []RolloutMode{ModeShadow, ModeReadOnly, ModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			runtime, err := NewRuntime(validRuntimeConfig(mode), runtimeTestDependencies())
			require.NoError(t, err)
			require.True(t, runtime.Enabled())
		})
	}
}

func TestRuntimeRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RuntimeConfig)
		want   string
	}{
		{"airport", func(c *RuntimeConfig) { c.EnabledAirports = []string{"bad"} }, "ICAO"},
		{"duplicate airport", func(c *RuntimeConfig) { c.EnabledAirports = []string{" EKCH ", "ekch"} }, "unique"},
		{"reconciliation timing", func(c *RuntimeConfig) { c.ReconciliationInterval = -time.Second }, "reconciliation interval"},
		{"surveillance timing", func(c *RuntimeConfig) { c.SurveillanceInterval = -time.Second }, "surveillance interval"},
		{"source adapter", func(c *RuntimeConfig) { c.NavigationSourceAdapter = "other" }, "source adapter"},
		{"geometry", func(c *RuntimeConfig) { c.TerminalGeometryPath = "" }, "geometry path"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validRuntimeConfig(ModeShadow)
			test.mutate(&config)
			require.ErrorContains(t, config.Validate(), test.want)
		})
	}
}

func TestDisabledRuntimeRejectsAnExplicitInvalidSourceAdapter(t *testing.T) {
	err := (RuntimeConfig{NavigationSourceAdapter: "other"}).Validate()
	require.ErrorContains(t, err, "source adapter")
}

func TestRuntimeRequiresExplicitDependenciesWhenEnabled(t *testing.T) {
	_, err := NewRuntime(validRuntimeConfig(ModeShadow), Dependencies{})
	require.EqualError(t, err, "AMAN runtime requires repositories")
}

func TestRuntimeWorkersUseApplicationContextAndConfiguredIntervals(t *testing.T) {
	deps := runtimeTestDependencies()
	runtime, err := NewRuntime(validRuntimeConfig(ModeShadow), deps)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	runtime.Start(ctx)
	require.Equal(t, 4*time.Second, <-deps.SurveillanceWorker.(*runtimeTestWorker).started)
	require.Equal(t, 3*time.Second, <-deps.ReconciliationWorker.(*runtimeTestWorker).started)
	cancel()
	require.Eventually(t, func() bool {
		select {
		case <-deps.SurveillanceWorker.(*runtimeTestWorker).stopped:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		select {
		case <-deps.ReconciliationWorker.(*runtimeTestWorker).stopped:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
}

func TestRuntimeStoresNormalizedConfiguration(t *testing.T) {
	config := validRuntimeConfig(ModeShadow)
	config.Mode = " SHADOW "
	config.EnabledAirports = []string{" ekch ", "ekrn"}
	config.NavigationSourceAdapter = " AIRACNET "
	config.TerminalGeometryPath = " testdata/terminal.geojson "

	runtime, err := NewRuntime(config, runtimeTestDependencies())
	require.NoError(t, err)
	require.Equal(t, RuntimeConfig{
		Mode:                    ModeShadow,
		EnabledAirports:         []string{"EKCH", "EKRN"},
		ReconciliationInterval:  3 * time.Second,
		SurveillanceInterval:    4 * time.Second,
		TerminalGeometryPath:    "testdata/terminal.geojson",
		NavigationSourceAdapter: NavigationAdapterAIRACNet,
	}, runtime.Config())
}

func TestRuntimeOwnershipFollowsRolloutMode(t *testing.T) {
	tests := []struct {
		mode             RolloutMode
		legacy, amanETA  bool
		sequence, mutate bool
	}{
		{mode: ModeDisabled, legacy: true},
		{mode: ModeShadow, legacy: true},
		{mode: ModeReadOnly, amanETA: true},
		{mode: ModeAuthoritative, amanETA: true, sequence: true, mutate: true},
	}
	for _, test := range tests {
		t.Run(string(test.mode), func(t *testing.T) {
			config := validRuntimeConfig(test.mode)
			if test.mode == ModeDisabled {
				config = RuntimeConfig{}
			}
			runtime, err := NewRuntime(config, runtimeTestDependencies())
			require.NoError(t, err)
			ownership := runtime.Ownership()
			require.Equal(t, test.legacy, ownership.LegacyArrivalETAWriter)
			require.Equal(t, test.amanETA, ownership.AMANArrivalETAWriter)
			require.Equal(t, test.sequence, ownership.SequenceAuthoritative)
			require.Equal(t, test.mutate, ownership.ControllerMutationAuthorized)
		})
	}
}
