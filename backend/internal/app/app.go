package app

import (
	"FlightStrips/internal/aman"
	appconfig "FlightStrips/internal/config"
	"FlightStrips/internal/ecfmp"
	"FlightStrips/internal/gsx"
	"FlightStrips/internal/navigation"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/services"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/standstatus"
	"FlightStrips/internal/vatsim"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	NATS NATSConfig

	OIDCSigningAlgorithm string
	OIDCAuthority        string
	OIDCAudience         string
	Environment          string

	EnableHTTPTracing bool

	CDMKey                   string
	CDMConfigDir             string
	CDMConfigRefreshInterval time.Duration
	EnableCDMConfigStore     bool

	HoppieLogon          string
	PDCWebLookupLiveOnly bool
	EnablePDC            bool

	ECFMPBaseURL                    string
	EnableECFMP                     bool
	EnableECFMPAPI                  bool
	EnablePilotAPI                  bool
	EnableEFB                       bool
	EnableGSXStandFeed              bool
	EnableALB                       bool
	EnableMetar                     bool
	EnableVATSIM                    bool
	EnableTransceivers              bool
	EnableTraffic                   bool
	EnableStandAssignment           bool
	EnableStandAssignmentESMessages bool
	EnableStandAssignmentPrefiles   bool
	EnableTestTools                 bool

	StandAssignmentAircraftJSON string

	StandAssignmentHoldDuration   time.Duration
	StandAssignmentBlockExtension time.Duration
	StandAssignmentSweepInterval  time.Duration

	AMAN       aman.RuntimeConfig
	Navigation navigation.Config
}

type Dependencies struct {
	NATS NATSDependencies

	AuthenticationService shared.AuthenticationService
	PDCClient             pdc.HoppieClientInterface
	VATSIMStatusURL       string
	VATSIMPollInterval    time.Duration
	TransceiversURL       string
	TransceiversInterval  time.Duration
}

type App struct {
	natsRuntime *natsRuntime

	handler http.Handler

	standAssignmentReadiness appconfig.StandAssignmentReadiness
}

func configureStandAssignment(enabled bool, aircraftFile string) appconfig.StandAssignmentReadiness {
	readiness := appconfig.InitializeStandAssignmentWithAircraftFile(enabled, aircraftFile)
	switch {
	case !readiness.Enabled:
		slog.Info("Stand Assignment Tool disabled")
	case readiness.Ready:
		slog.Info("Stand Assignment Tool ready")
	default:
		slog.Error("Stand Assignment Tool unavailable", slog.String("reason", readiness.Reason))
	}
	return readiness
}

func standAssignmentDiagnostics() standstatus.WebAPIDiagnostics {
	diagnostics := standstatus.WebAPIDiagnostics{}
	if registry := appconfig.GetAircraftReference(); registry != nil {
		diagnostics.AircraftTypes = len(registry.Types())
	}
	if registry := appconfig.GetStandCapabilities(); registry != nil {
		stands := registry.AllStands()
		diagnostics.Stands = len(stands)
		for _, stand := range stands {
			diagnostics.StandVariants += len(stand.Variants)
		}
	}
	if policy := appconfig.GetAirlineAssignment(); policy != nil {
		diagnostics.AirlineRules = len(policy.Rules)
		diagnostics.StandGroups = len(policy.StandGroups)
		diagnostics.FallbackRules = len(policy.FallbackRules)
	}
	return diagnostics
}

func buildAuthenticationService(cfg Config, dependency shared.AuthenticationService) (shared.AuthenticationService, error) {
	if dependency != nil {
		return dependency, nil
	}

	authService, err := services.NewAuthenticationService(cfg.OIDCSigningAlgorithm, cfg.OIDCAuthority, cfg.OIDCAudience)
	if err != nil {
		return nil, fmt.Errorf("initialize authentication service: %w", err)
	}
	return authService, nil
}

type healthResponse struct {
	Status          string               `json:"status"`
	StandAssignment satHealth            `json:"stand_assignment"`
	AMAN            aman.TechnicalHealth `json:"aman"`
}

type satHealth struct {
	Enabled            bool     `json:"enabled"`
	Ready              bool     `json:"ready"`
	Status             string   `json:"status"`
	Reason             string   `json:"reason,omitempty"`
	SnapshotAgeSeconds *float64 `json:"snapshot_age_seconds,omitempty"`
}

func evaluateSATHealth(readiness appconfig.StandAssignmentReadiness, snapshot vatsim.Snapshot, staleAfter time.Duration, now func() time.Time) satHealth {
	result := satHealth{Enabled: readiness.Enabled, Ready: readiness.Ready, Status: "ready"}
	ageDuration := now().Sub(snapshot.Timestamp)
	if ageDuration < 0 {
		ageDuration = 0
	}
	age := ageDuration.Seconds()
	result.SnapshotAgeSeconds = &age
	switch {
	case snapshot.Timestamp.IsZero():
		result.Status, result.Ready, result.Reason = "feed_unavailable", false, "VATSIM feed has not produced a snapshot"
	case snapshot.LastRefreshError != nil:
		result.Status, result.Ready, result.Reason = "feed_failed", false, snapshot.LastRefreshError.Error()
	case ageDuration > staleAfter:
		result.Status, result.Ready, result.Reason = "feed_stale", false, "VATSIM snapshot is stale"
	}
	return result
}

func satStaleAfter(poll time.Duration) time.Duration {
	if poll <= 0 {
		poll = 15 * time.Second
	}
	threshold := 2 * poll
	if threshold < time.Minute {
		return time.Minute
	}
	return threshold
}

func (cfg Config) withDefaults() Config {
	cfg.AMAN = cfg.AMAN.Normalize()
	if cfg.OIDCAudience == "" {
		cfg.OIDCAudience = "backend-dev"
	}
	if cfg.Environment == "" {
		cfg.Environment = "development"
	}
	if cfg.CDMConfigDir == "" {
		cfg.CDMConfigDir = appconfig.GetConfigDir()
	}
	if cfg.CDMConfigRefreshInterval <= 0 {
		cfg.CDMConfigRefreshInterval = 15 * time.Minute
	}
	if cfg.ECFMPBaseURL == "" {
		cfg.ECFMPBaseURL = ecfmp.DefaultBaseURL
	}
	if cfg.StandAssignmentHoldDuration <= 0 {
		cfg.StandAssignmentHoldDuration = 15 * time.Minute
	}
	if cfg.StandAssignmentBlockExtension <= 0 {
		cfg.StandAssignmentBlockExtension = 10 * time.Minute
	}
	if cfg.StandAssignmentSweepInterval <= 0 {
		cfg.StandAssignmentSweepInterval = 30 * time.Second
	}
	return cfg
}

func isLiveEnvironment(environment string) bool {
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "live", "prod", "production":
		return true
	default:
		return false
	}
}

// loadGSXSceneries reads the per-airport gate/scenery mapping that translates a
// controller's stand and release point into the names a particular add-on uses.
// A missing file is not an error: the feed then publishes the controller's own
// stand name and no pushback point.
func loadGSXSceneries(enabled bool) (gsx.Sceneries, error) {
	if !enabled {
		return nil, nil
	}

	sceneries := gsx.Sceneries{}
	// Mirrors the SAT configuration layout: config/<icao>/<file>.
	path := filepath.Join(appconfig.GetConfigDir(), "ekch", "gsx_sceneries.json")
	cfg, err := gsx.LoadSceneryConfig(path)
	if err != nil {
		return nil, err
	}
	if cfg != nil {
		sceneries[cfg.ICAO] = cfg
		slog.Info("loaded GSX scenery config", "icao", cfg.ICAO, "gates", len(cfg.Gates))
	} else {
		slog.Info("no GSX scenery config found; publishing controller stand names only", "path", path)
	}
	return sceneries, nil
}

func (a *App) Handler() http.Handler            { return a.handler }
func (a *App) StartWorkers(ctx context.Context) { a.natsRuntime.startWorkers(ctx) }
func (a *App) Close(ctx context.Context) error  { return a.natsRuntime.close(ctx) }

// DrainPositions runs while replay and ownership fencing are still live.
func (a *App) DrainPositions(ctx context.Context) error { return a.natsRuntime.deadlines.Close(ctx) }
func (a *App) StandAssignmentReadiness() appconfig.StandAssignmentReadiness {
	return a.standAssignmentReadiness
}
