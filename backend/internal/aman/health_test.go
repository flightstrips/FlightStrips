package aman

import (
	"context"
	"testing"
	"time"
)

type healthTestComponent struct{ report TechnicalHealth }

func (healthTestComponent) Name() string                                      { return "test health" }
func (c healthTestComponent) TechnicalHealth(context.Context) TechnicalHealth { return c.report }

func TestEvaluateTechnicalHealthNamesTechnicalAuthorityBlockers(t *testing.T) {
	report := EvaluateTechnicalHealth(ModeAuthoritative,
		ComponentHealth{Status: HealthReady},
		ComponentHealth{Status: HealthDegraded, Reason: "terminal_geometry_invalid"},
		ComponentHealth{Status: HealthReady},
		ComponentHealth{Status: HealthReady},
		ComponentHealth{Status: HealthUnavailable, Reason: "model_not_loaded"},
		ComponentHealth{Status: HealthReady},
	)
	if report.Ready || report.Status != HealthDegraded {
		t.Fatalf("unexpected readiness: %#v", report)
	}
	want := []string{"navigation:terminal_geometry_invalid", "predictor:model_not_loaded"}
	if len(report.BlockedReasons) != len(want) {
		t.Fatalf("blocked reasons = %#v, want %#v", report.BlockedReasons, want)
	}
	for index, reason := range want {
		if report.BlockedReasons[index] != reason {
			t.Fatalf("blocked reasons = %#v, want %#v", report.BlockedReasons, want)
		}
	}
}

func TestEvaluateTechnicalHealthRestoresAfterFreshInput(t *testing.T) {
	stale := EvaluateTechnicalHealth(ModeAuthoritative,
		ComponentHealth{Status: HealthDegraded, Reason: "snapshot_stale"}, ComponentHealth{Status: HealthReady},
		ComponentHealth{Status: HealthReady}, ComponentHealth{Status: HealthReady}, ComponentHealth{Status: HealthReady}, ComponentHealth{Status: HealthReady},
	)
	fresh := EvaluateTechnicalHealth(ModeAuthoritative,
		ComponentHealth{Status: HealthReady}, ComponentHealth{Status: HealthReady},
		ComponentHealth{Status: HealthReady}, ComponentHealth{Status: HealthReady}, ComponentHealth{Status: HealthReady}, ComponentHealth{Status: HealthReady},
	)
	if stale.Ready || stale.BlockedReasons[0] != "vatsim:snapshot_stale" {
		t.Fatalf("stale report = %#v", stale)
	}
	if !fresh.Ready || fresh.Status != HealthReady || len(fresh.BlockedReasons) != 0 {
		t.Fatalf("fresh report = %#v", fresh)
	}
}

func TestRuntimeHealthUsesEffectiveModeAndReporter(t *testing.T) {
	now := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)
	reporter := healthTestComponent{report: EvaluateTechnicalHealth(ModeShadow,
		ComponentHealth{Status: HealthReady, UpdatedAt: &now}, ComponentHealth{Status: HealthReady},
		ComponentHealth{Status: HealthReady}, ComponentHealth{Status: HealthReady},
		ComponentHealth{Status: HealthReady}, ComponentHealth{Status: HealthReady},
	)}
	runtime, err := NewRuntime(RuntimeConfig{Mode: ModeAuthoritative, EnabledAirports: []string{"EKCH"}, TerminalGeometryPath: "terminal.json", NavigationSourceAdapter: NavigationAdapterAIRACNet}, Dependencies{
		Repositories: reporter, NavigationMaterializer: reporter, NavigationReader: reporter, Predictor: reporter,
		StateEngine: reporter, SequenceService: reporter, Publisher: reporter, ValidationService: reporter,
		HealthService: reporter, ReconciliationWorker: healthTestWorker{}, ObservationSink: healthTestSink{},
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	report := runtime.Health(context.Background())
	if !report.Ready || report.Mode != ModeAuthoritative || !report.Enabled || report.Status != HealthReady {
		t.Fatalf("unexpected report: %#v", report)
	}
	if report.DesiredMode != ModeAuthoritative || report.EffectiveMode != EffectiveAuthoritative || !report.AuthorityAllowed {
		t.Fatalf("unexpected rollout decision fields: %#v", report)
	}
}

type healthTestWorker struct{}

func (healthTestWorker) Run(context.Context, time.Duration) {}

type healthTestSink struct{}

func (healthTestSink) Observe(context.Context, FlightObservation) error { return nil }
