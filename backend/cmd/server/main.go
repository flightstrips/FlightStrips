package main

import (
	"FlightStrips/internal/app"
	"FlightStrips/internal/config"
	"FlightStrips/internal/telemetry"
	"context"
	"flag"
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

	otlpEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if otlpEndpoint != "" {
		tel, err := telemetry.Initialize(ctx, telemetry.Config{
			ServiceName:    "flightstrips-backend",
			ServiceVersion: "1.0.0",
			Environment:    getEnv("ENVIRONMENT", "development"),
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
		DatabaseConnectionString: os.Getenv("DATABASE_CONNECTIONSTRING"),
		OIDCSigningAlgorithm:     os.Getenv("OIDC_SIGNING_ALGO"),
		OIDCAuthority:            os.Getenv("OIDC_AUTHORITY"),
		OIDCAudience:             getEnv("OIDC_AUDIENCE", "backend-dev"),
		Environment:              getEnv("ENVIRONMENT", "development"),
		EnablePostgresTracing:    otlpEndpoint != "",
		EnableHTTPTracing:        otlpEndpoint != "",
		CDMKey:                   os.Getenv("CDM_KEY"),
		CDMConfigRefreshInterval: envDuration("CDM_CONFIG_REFRESH_INTERVAL", 15*time.Minute),
		EnableCDMConfigStore:     true,
		HoppieLogon:              os.Getenv("HOPPIE_LOGON"),
		PDCWebLookupLiveOnly:     envBool("PDC_WEB_LOOKUP_LIVE_ONLY", isLiveEnvironment(getEnv("ENVIRONMENT", "development"))),
		EnablePDC:                true,
		ECFMPBaseURL:             getEnv("ECFMP_BASE_URL", ""),
		EnableECFMP:              true,
		EnableECFMPAPI:           !isLiveEnvironment(getEnv("ENVIRONMENT", "development")),
		EnablePilotAPI:           true,
		EnableALB:                true,
		EnableMetar:              true,
		EnableVATSIM:             true,
		EnableTransceivers:       true,
		EnableTraffic:            true,
		EnableDBSeed:             true,
	}, app.Dependencies{
		VATSIMStatusURL:      getEnv("VATSIM_STATUS_URL", ""),
		VATSIMPollInterval:   envDuration("VATSIM_POLL_INTERVAL", 30*time.Second),
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

func isLiveEnvironment(environment string) bool {
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "live", "prod", "production":
		return true
	default:
		return false
	}
}
