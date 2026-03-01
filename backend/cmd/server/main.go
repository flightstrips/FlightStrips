package main

import (
	"FlightStrips/internal/cdm"
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/internal/euroscope"
	"FlightStrips/internal/frontend"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/server"
	"FlightStrips/internal/services"
	"FlightStrips/internal/telemetry"
	"FlightStrips/internal/websocket"
	pkgEuroscope "FlightStrips/pkg/events/euroscope"
	pkgFrontend "FlightStrips/pkg/events/frontend"
	"context"
	_ "database/sql"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/exaring/otelpgx"
	_ "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	_ "embed"

	_ "github.com/jackc/pgx/v5/pgtype"

	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
)

var addr = flag.String("addr", "0.0.0.0:2994", "http service address")

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func main() {
	flag.Parse()

	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	err := godotenv.Load()
	if err != nil {
		slog.Error("Error loading .env file", slog.Any("error", err))
		os.Exit(1)
	}

	ctx := context.Background()
	config.InitConfig()

	// Initialize OpenTelemetry if endpoint is configured
	otlpEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	var tel *telemetry.Telemetry
	if otlpEndpoint != "" {
		tel, err = telemetry.Initialize(ctx, telemetry.Config{
			ServiceName:    "FlightStrips",
			ServiceVersion: "1.0.0",
			Environment:    getEnv("ENVIRONMENT", "development"),
			OTLPEndpoint:   otlpEndpoint,
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

		// Setup dual logger (stdout + OTEL)
		telemetry.SetupDualLogger()
		slog.Info("OpenTelemetry initialized", slog.String("endpoint", otlpEndpoint))
	}

	// Configure pgxpool with OTEL tracing
	poolConfig, err := pgxpool.ParseConfig(os.Getenv("DATABASE_CONNECTIONSTRING"))
	if err != nil {
		slog.Error("Failed to parse database connection string", slog.Any("error", err))
		os.Exit(1)
	}

	if otlpEndpoint != "" {
		poolConfig.ConnConfig.Tracer = otelpgx.NewTracer(
			otelpgx.WithTracerProvider(otel.GetTracerProvider()),
			otelpgx.WithTrimSQLInSpanName(),
		)
	}

	dbpool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		slog.Error("Failed to connect to database", slog.Any("error", err))
		os.Exit(1)
	}
	defer dbpool.Close()

	if otlpEndpoint != "" {
		if err := otelpgx.RecordStats(dbpool); err != nil {
			slog.Warn("Failed to record database stats", slog.Any("error", err))
		}
	}

	authenticationService, err := services.NewAuthenticationService(os.Getenv("OIDC_SIGNING_ALGO"), os.Getenv("OIDC_AUTHORITY"))
	if err != nil {
		slog.Error("Failed to initialize authentication service", slog.Any("error", err))
		os.Exit(1)
	}

	cdmKey := os.Getenv("CDM_KEY")
	cdmClient := cdm.NewClient(cdm.WithAPIKey(cdmKey))

	// Initialize repositories
	stripRepo := postgres.NewStripRepository(dbpool)
	controllerRepo := postgres.NewControllerRepository(dbpool)
	sessionRepo := postgres.NewSessionRepository(dbpool)
	sectorRepo := postgres.NewSectorOwnerRepository(dbpool)
	coordRepo := postgres.NewCoordinationRepository(dbpool)

	// Initialize services
	stripService := services.NewStripService(stripRepo)
	cdmService := cdm.NewCdmService(cdmClient, stripRepo, sessionRepo)

	// Initialize PDC Service
	hoppieLogon := os.Getenv("HOPPIE_LOGON")
	var pdcService *pdc.Service
	if hoppieLogon != "" {
		hoppieClient := pdc.NewClient(hoppieLogon)
		pdcService = pdc.NewPDCService(hoppieClient, sessionRepo, stripRepo, sectorRepo)
		pdcService.SetStripService(stripService)
		slog.Info("PDC Service initialized")
	} else {
		slog.Warn("PDC Service not initialized - HOPPIE_LOGON")
	}

	frontendHub := frontend.NewHub(stripService)
	euroscopeHub := euroscope.NewHub(stripService)

	stripService.SetFrontendHub(frontendHub)
	stripService.SetEuroscopeHub(euroscopeHub)
	stripService.SetSectorOwnerRepo(sectorRepo)
	cdmService.SetFrontendHub(frontendHub)
	if pdcService != nil {
		pdcService.SetFrontendHub(frontendHub)
	}

	fsServer := server.NewServer(dbpool, euroscopeHub, frontendHub, cdmService, pdcService, stripRepo, controllerRepo, sessionRepo, sectorRepo, coordRepo)

	frontendHub.SetServer(fsServer)
	euroscopeHub.SetServer(fsServer)

	frontendUpgrader := websocket.NewConnectionUpgrader[pkgFrontend.EventType, *frontend.Client](frontendHub, authenticationService)
	euroscopeUpgrader := websocket.NewConnectionUpgrader[pkgEuroscope.EventType, *euroscope.Client](euroscopeHub, authenticationService)

	go cdmService.Start(ctx)
	if pdcService != nil {
		go pdcService.Start(ctx)
	}

	// TODO remove
	db := database.New(dbpool)
	_ = db.InsertAirport(context.Background(), "EKCH")

	// Health Function for local Dev
	http.HandleFunc("/healthz", healthz)
	http.HandleFunc("/euroscopeEvents", euroscopeUpgrader.Upgrade)
	http.HandleFunc("/frontEndEvents", frontendUpgrader.Upgrade)

	// Wrap with OTEL HTTP instrumentation
	var handler http.Handler = http.DefaultServeMux
	if otlpEndpoint != "" {
		handler = otelhttp.NewHandler(http.DefaultServeMux, "http.server")
	}

	// Setup graceful shutdown
	server := &http.Server{
		Addr:    *addr,
		Handler: handler,
	}

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("Server started", slog.String("address", *addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	<-sigChan
	slog.Info("Shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server shutdown failed", slog.Any("error", err))
		os.Exit(1)
	}

	slog.Info("Server shutdown complete")
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
