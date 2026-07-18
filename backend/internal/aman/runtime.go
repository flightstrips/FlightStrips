package aman

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	defaultReconciliationInterval = 15 * time.Second
	defaultSurveillanceInterval   = 15 * time.Second

	// NavigationAdapterAIRACNet is the approved navigation-data adapter. Its
	// implementation belongs to the navigation task; this package only names
	// the configured boundary.
	NavigationAdapterAIRACNet = "airacnet"
)

var airportIdentifier = regexp.MustCompile(`^[A-Z]{4}$`)

// RuntimeConfig is the application configuration for one AMAN runtime. The
// zero value is intentionally release-safe: AMAN remains disabled.
type RuntimeConfig struct {
	EnabledAirports             []string
	Mode                        RolloutMode
	ReconciliationInterval      time.Duration
	SurveillanceInterval        time.Duration
	TerminalGeometryPath        string
	NavigationSourceAdapter     string
	EnableEuroScopeGainLoseTags bool
}

// DefaultRuntimeConfig returns the release-safe runtime configuration.
func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		Mode:                   ModeDisabled,
		ReconciliationInterval: defaultReconciliationInterval,
		SurveillanceInterval:   defaultSurveillanceInterval,
	}
}

func (c RuntimeConfig) withDefaults() RuntimeConfig {
	defaults := DefaultRuntimeConfig()
	if c.Mode == "" {
		c.Mode = defaults.Mode
	}
	if c.ReconciliationInterval == 0 {
		c.ReconciliationInterval = defaults.ReconciliationInterval
	}
	if c.SurveillanceInterval == 0 {
		c.SurveillanceInterval = defaults.SurveillanceInterval
	}
	return c
}

func (c RuntimeConfig) normalize() RuntimeConfig {
	c = c.withDefaults()
	c.Mode = RolloutMode(strings.ToLower(strings.TrimSpace(string(c.Mode))))
	c.EnabledAirports = normalizeAirports(c.EnabledAirports)
	c.TerminalGeometryPath = strings.TrimSpace(c.TerminalGeometryPath)
	c.NavigationSourceAdapter = strings.ToLower(strings.TrimSpace(c.NavigationSourceAdapter))
	return c
}

// Validate rejects configuration that would make an enabled AMAN runtime
// ambiguous or unusable. Disabled AMAN accepts its zero-value configuration.
func (c RuntimeConfig) Validate() error {
	original := c.withDefaults()
	c = original.normalize()
	if !c.Mode.Valid() {
		return fmt.Errorf("AMAN mode %q is invalid", c.Mode)
	}
	if c.ReconciliationInterval <= 0 {
		return fmt.Errorf("AMAN reconciliation interval must be greater than 0")
	}
	if c.SurveillanceInterval <= 0 {
		return fmt.Errorf("AMAN surveillance interval must be greater than 0")
	}

	if len(c.EnabledAirports) != len(nonEmpty(original.EnabledAirports)) {
		return fmt.Errorf("AMAN enabled airports must be unique ICAO identifiers")
	}
	seenAirports := make(map[string]struct{}, len(c.EnabledAirports))
	for _, airport := range c.EnabledAirports {
		if !airportIdentifier.MatchString(airport) {
			return fmt.Errorf("AMAN airport %q must be a four-letter ICAO identifier", airport)
		}
		if _, exists := seenAirports[airport]; exists {
			return fmt.Errorf("AMAN enabled airports must be unique ICAO identifiers")
		}
		seenAirports[airport] = struct{}{}
	}
	if c.NavigationSourceAdapter != "" && !isSupportedNavigationAdapter(c.NavigationSourceAdapter) {
		return fmt.Errorf("AMAN navigation source adapter %q is unsupported", c.NavigationSourceAdapter)
	}
	if c.Mode == ModeDisabled {
		return nil
	}
	if len(c.EnabledAirports) == 0 {
		return fmt.Errorf("AMAN enabled airports are required when mode is %q", c.Mode)
	}
	if !isSupportedNavigationAdapter(c.NavigationSourceAdapter) {
		return fmt.Errorf("AMAN navigation source adapter %q is unsupported", c.NavigationSourceAdapter)
	}
	if strings.TrimSpace(c.TerminalGeometryPath) == "" {
		return fmt.Errorf("AMAN terminal geometry path is required when enabled")
	}
	return nil
}

func normalizeAirports(values []string) []string {
	airports := make([]string, 0, len(values))
	for _, value := range values {
		airport := strings.ToUpper(strings.TrimSpace(value))
		if airport != "" {
			airports = append(airports, airport)
		}
	}
	return airports
}

func nonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	return result
}

func isSupportedNavigationAdapter(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), NavigationAdapterAIRACNet)
}

// Component is a narrow constructor-time seam. The owning persistence,
// navigation, prediction, state, sequence, publishing, validation, and health
// tasks provide concrete components without a package-level registry.
type Component interface {
	Name() string
}

// Worker is a cancellation-aware AMAN reconciliation loop. The concrete source
// observation worker is assembled independently by the application.
type Worker interface {
	Run(context.Context, time.Duration)
}

// Dependencies are injected by application assembly. Keeping these boundaries
// explicit lets later AMAN tasks add concrete implementations without changing
// startup ownership or adding a service locator.
type Dependencies struct {
	Repositories           Component
	NavigationMaterializer Component
	NavigationReader       Component
	Predictor              Component
	StateEngine            Component
	SequenceService        Component
	Publisher              Component
	ValidationService      Component
	HealthService          Component
	ObservationSink        ObservationSink
	ReconciliationWorker   Worker
}

// Ownership describes the runtime authority selected by rollout mode.
type Ownership struct {
	LegacyArrivalETAWriter       bool
	AMANArrivalETAWriter         bool
	SequenceAuthoritative        bool
	ControllerMutationAuthorized bool
	EuroScopeGainLoseTagsEnabled bool
}

// Runtime owns AMAN mode and worker lifecycle. It deliberately does not
// implement navigation, prediction, persistence, or sequencing policy.
type Runtime struct {
	config    RuntimeConfig
	deps      Dependencies
	ownership Ownership
}

// NewRuntime validates the rollout configuration and all required explicit
// dependencies before workers can start.
func NewRuntime(config RuntimeConfig, deps Dependencies) (*Runtime, error) {
	config = config.normalize()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	runtime := &Runtime{config: config, deps: deps, ownership: ownershipFor(config)}
	if config.Mode == ModeDisabled {
		return runtime, nil
	}
	for _, dependency := range []struct {
		name  string
		value Component
	}{
		{"repositories", deps.Repositories},
		{"navigation materializer", deps.NavigationMaterializer},
		{"navigation reader", deps.NavigationReader},
		{"predictor", deps.Predictor},
		{"state engine", deps.StateEngine},
		{"sequence service", deps.SequenceService},
		{"publisher", deps.Publisher},
		{"validation service", deps.ValidationService},
		{"health service", deps.HealthService},
	} {
		if dependency.value == nil || strings.TrimSpace(dependency.value.Name()) == "" {
			return nil, fmt.Errorf("AMAN runtime requires %s", dependency.name)
		}
	}
	if deps.ReconciliationWorker == nil {
		return nil, fmt.Errorf("AMAN runtime requires reconciliation worker")
	}
	if deps.ObservationSink == nil {
		return nil, fmt.Errorf("AMAN runtime requires observation sink")
	}
	return runtime, nil
}

func ownershipFor(config RuntimeConfig) Ownership {
	ownership := Ownership{EuroScopeGainLoseTagsEnabled: config.EnableEuroScopeGainLoseTags}
	switch config.Mode {
	case ModeDisabled, ModeShadow:
		ownership.LegacyArrivalETAWriter = true
	case ModeReadOnly:
		ownership.AMANArrivalETAWriter = true
	case ModeAuthoritative:
		ownership.AMANArrivalETAWriter = true
		ownership.SequenceAuthoritative = true
		ownership.ControllerMutationAuthorized = true
	}
	return ownership
}

// Config returns a copy of the normalized configuration.
func (r *Runtime) Config() RuntimeConfig {
	if r == nil {
		return DefaultRuntimeConfig()
	}
	config := r.config
	config.EnabledAirports = slices.Clone(config.EnabledAirports)
	return config
}

func (r *Runtime) Ownership() Ownership {
	if r == nil {
		return ownershipFor(DefaultRuntimeConfig())
	}
	return r.ownership
}

func (r *Runtime) Enabled() bool {
	return r != nil && r.config.Mode != ModeDisabled
}

// Start runs the configured future AMAN reconciliation loop. The application
// owns the concrete VATSIM source-observation worker so it cannot be started a
// second time through this runtime.
func (r *Runtime) Start(ctx context.Context) {
	if !r.Enabled() {
		return
	}
	go r.deps.ReconciliationWorker.Run(ctx, r.config.ReconciliationInterval)
}
