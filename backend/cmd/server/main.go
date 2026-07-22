package main

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/app"
	"FlightStrips/internal/config"
	"FlightStrips/internal/telemetry"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
)

var addr = flag.String("addr", "", "http service address (overrides SERVER_ADDR env var)")

func main() {
	flag.Parse()

	if *addr == "" {
		*addr = getEnv("SERVER_ADDR", "127.0.0.1:8090")
	}

	configureLogging()
	loadEnvFiles()

	ctx, cancelWorkers := context.WithCancel(context.Background())
	defer cancelWorkers()

	if err := config.InitConfig(); err != nil {
		slog.Error("Failed to initialize config", slog.Any("error", err))
		os.Exit(1)
	}
	environment := getEnv("ENVIRONMENT", "development")
	enableTestTools := envBool("ENABLE_TEST_TOOLS", false)
	if enableTestTools && isLiveEnvironment(environment) {
		slog.Error("ENABLE_TEST_TOOLS cannot be enabled in a live environment")
		os.Exit(1)
	}
	standAssignmentAircraftJSON := standAssignmentAircraftFile(os.Getenv("GRPLUGIN_ICAO_AIRCRAFT_JSON"))
	amanConfig, err := amanConfigFromEnv()
	if err != nil {
		slog.Error("Failed to configure AMAN", slog.Any("error", err))
		os.Exit(1)
	}

	otlpEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if otlpEndpoint != "" {
		tel, err := telemetry.Initialize(ctx, telemetry.Config{
			ServiceName:    "flightstrips-backend",
			ServiceVersion: "1.0.0",
			Environment:    environment,
		})
		if err != nil {
			slog.Error("Failed to initialize telemetry", slog.Any("error", err))
			os.Exit(1)
		}
		defer func() {
			if err := telemetry.WaitForShutdown(tel.Shutdown); err != nil {
				slog.Error("Failed to shutdown telemetry", slog.Any("error", err))
			}
		}()

		telemetry.SetupDualLogger()
		slog.Info("OpenTelemetry initialized", slog.String("endpoint", otlpEndpoint))
	}

	application, err := app.Build(ctx, app.Config{
		DatabaseConnectionString:        os.Getenv("DATABASE_CONNECTIONSTRING"),
		OIDCSigningAlgorithm:            os.Getenv("OIDC_SIGNING_ALGO"),
		OIDCAuthority:                   os.Getenv("OIDC_AUTHORITY"),
		OIDCAudience:                    getEnv("OIDC_AUDIENCE", "backend-dev"),
		Environment:                     environment,
		EnablePostgresTracing:           otlpEndpoint != "",
		EnableHTTPTracing:               otlpEndpoint != "",
		CDMKey:                          os.Getenv("CDM_KEY"),
		CDMConfigRefreshInterval:        envDuration("CDM_CONFIG_REFRESH_INTERVAL", 15*time.Minute),
		EnableCDMConfigStore:            true,
		HoppieLogon:                     os.Getenv("HOPPIE_LOGON"),
		PDCWebLookupLiveOnly:            envBool("PDC_WEB_LOOKUP_LIVE_ONLY", isLiveEnvironment(environment)),
		EnablePDC:                       true,
		ECFMPBaseURL:                    getEnv("ECFMP_BASE_URL", ""),
		EnableECFMP:                     true,
		EnableECFMPAPI:                  !isLiveEnvironment(environment),
		EnablePilotAPI:                  true,
		EnableEFB:                       envBool("ENABLE_EFB", false),
		EnableALB:                       true,
		EnableMetar:                     true,
		EnableVATSIM:                    true,
		EnableTransceivers:              true,
		EnableTraffic:                   true,
		EnableStandAssignment:           envBool("ENABLE_STAND_ASSIGNMENT", false),
		EnableStandAssignmentESMessages: envBool("ENABLE_STAND_ASSIGNMENT_ES_MESSAGES", false),
		EnableTestTools:                 enableTestTools,
		EnableDBSeed:                    true,
		StandAssignmentAircraftJSON:     standAssignmentAircraftJSON,
		AMAN:                            amanConfig,
	}, app.Dependencies{
		VATSIMStatusURL:      getEnv("VATSIM_STATUS_URL", ""),
		VATSIMPollInterval:   envDuration("VATSIM_POLL_INTERVAL", 15*time.Second),
		TransceiversURL:      getEnv("VATSIM_TRANSCEIVERS_URL", ""),
		TransceiversInterval: envDuration("VATSIM_TRANSCEIVER_POLL_INTERVAL", 30*time.Second),
	})
	if err != nil {
		slog.Error("Failed to build application", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		if err := application.Close(context.Background()); err != nil {
			slog.Error("Failed to close application", slog.Any("error", err))
		}
	}()
	application.StartWorkers(ctx)

	httpServer := &http.Server{
		Addr:    *addr,
		Handler: application.Handler(),
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("Server started", slog.String("address", *addr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	<-sigChan
	slog.Info("Shutting down server...")
	cancelWorkers()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server shutdown failed", slog.Any("error", err))
		os.Exit(1)
	}

	slog.Info("Server shutdown complete")
}

func standAssignmentAircraftFile(configured string) string {
	return strings.TrimSpace(configured)
}

func configureLogging() {
	var logLevel slog.LevelVar
	switch strings.ToUpper(os.Getenv("LOG_LEVEL")) {
	case "DEBUG":
		logLevel.Set(slog.LevelDebug)
	case "WARN":
		logLevel.Set(slog.LevelWarn)
	case "ERROR":
		logLevel.Set(slog.LevelError)
	default:
		logLevel.Set(slog.LevelInfo)
	}

	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: &logLevel}))
	slog.SetDefault(logger)
}

func loadEnvFiles() {
	for _, envFile := range []string{".env", ".env.dev"} {
		err := godotenv.Load(envFile)
		if err != nil && !os.IsNotExist(err) {
			slog.Error("Error loading env file", slog.String("file", envFile), slog.Any("error", err))
			os.Exit(1)
		}
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func amanConfigFromEnv() (aman.RuntimeConfig, error) {
	config := aman.DefaultRuntimeConfig()
	if value := strings.TrimSpace(os.Getenv("AMAN_MODE")); value != "" {
		config.Mode = aman.RolloutMode(strings.ToLower(value))
	}
	config.EnabledAirports = splitEnvList(os.Getenv("AMAN_ENABLED_AIRPORTS"))
	config.FMPRoles = splitEnvList(os.Getenv("AMAN_FMP_ROLES"))
	config.TerminalGeometryPath = strings.TrimSpace(os.Getenv("AMAN_TERMINAL_GEOMETRY_PATH"))
	config.NavigationSourceAdapter = strings.TrimSpace(os.Getenv("AMAN_NAVIGATION_SOURCE"))
	config.EnableEuroScopeGainLoseTags = envBool("ENABLE_AMAN_EUROSCOPE_GAIN_LOSE_TAGS", false)

	var err error
	if config.ReconciliationInterval, err = requiredEnvDuration("AMAN_RECONCILIATION_INTERVAL", config.ReconciliationInterval); err != nil {
		return aman.RuntimeConfig{}, err
	}
	if config.SurveillanceInterval, err = requiredEnvDuration("AMAN_SURVEILLANCE_INTERVAL", config.SurveillanceInterval); err != nil {
		return aman.RuntimeConfig{}, err
	}
	if err := config.Validate(); err != nil {
		return aman.RuntimeConfig{}, err
	}
	return config, nil
}

func requiredEnvDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", key, err)
	}
	return duration, nil
}

func splitEnvList(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return r == ',' })
}

func isLiveEnvironment(environment string) bool {
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "live", "prod", "production":
		return true
	default:
		return false
	}
}
