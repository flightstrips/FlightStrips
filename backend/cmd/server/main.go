package main

import (
	"FlightStrips/internal/alb"
	"FlightStrips/internal/cdm"
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/internal/euroscope"
	"FlightStrips/internal/frontend"
	"FlightStrips/internal/metar"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/pilot"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/server"
	"FlightStrips/internal/services"
	"FlightStrips/internal/telemetry"
	"FlightStrips/internal/vatsim"
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
	"path/filepath"
	"strconv"
	"strings"
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

var addr = flag.String("addr", "", "http service address (overrides SERVER_ADDR env var)")

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func main() {
	flag.Parse()

	if *addr == "" {
		*addr = getEnv("SERVER_ADDR", "127.0.0.1:8090")
	}

	var logLevel slog.LevelVar
	switch strings.ToUpper(os.Getenv("LOG_LEVEL")) {
	case "DEBUG":
		logLevel.Set(slog.LevelDebug)
	case "WARN":
		logLevel.Set(slog.LevelWarn)
	case "ERROR":
		logLevel.Set(slog.LevelError)
	default: // "INFO" or unset
		logLevel.Set(slog.LevelInfo)
	}
	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: &logLevel}))
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

	authenticationService, err := services.NewAuthenticationService(os.Getenv("OIDC_SIGNING_ALGO"), os.Getenv("OIDC_AUTHORITY"), getEnv("OIDC_AUDIENCE", "backend-dev"))
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
	tacticalStripRepo := postgres.NewTacticalStripRepository(dbpool)

	// Initialize services
	stripService := services.NewStripService(stripRepo)
	controllerService := services.NewControllerService(controllerRepo)
	cdmService := cdm.NewCdmService(cdmClient, stripRepo, sessionRepo, controllerRepo)

	stripService.SetTacticalStripRepo(tacticalStripRepo)
	stripService.SetCoordinationRepo(coordRepo)
	stripService.SetControllerRepo(controllerRepo)
	stripService.SetCdmService(cdmService)
	cdmService.SetValidationReevaluator(stripService)

	// Initialize PDC Service
	hoppieLogon := os.Getenv("HOPPIE_LOGON")
	pdcClient := pdc.HoppieClientInterface(pdc.NoopHoppieClient{})
	if hoppieLogon != "" {
		pdcClient = pdc.NewClient(hoppieLogon)
		slog.Info("PDC Hoppie client initialized")
	} else {
		slog.Warn("PDC Hoppie client disabled - HOPPIE_LOGON not set")
	}
	pdcService := pdc.NewPDCService(pdcClient, sessionRepo, stripRepo, sectorRepo, controllerRepo)
	pdcService.SetStripService(stripService)
	pdcService.SetWebLookupLiveOnly(isLiveEnvironment(getEnv("ENVIRONMENT", "development")))

	requireLiveCIDVerification := isLiveEnvironment(getEnv("ENVIRONMENT", "development"))
	var vatsimCache *vatsim.Cache
	if requireLiveCIDVerification {
		vatsimCache = vatsim.NewCache(
			getEnv("VATSIM_STATUS_URL", ""),
			envDuration("VATSIM_POLL_INTERVAL", 30*time.Second),
			nil,
		)
		slog.Info("VATSIM cache enabled for live web PDC ownership verification")
	}

	var fsServer *server.Server
	transceiverCache := vatsim.NewTransceiverCache(
		getEnv("VATSIM_TRANSCEIVERS_URL", ""),
		envDuration("VATSIM_TRANSCEIVER_POLL_INTERVAL", 30*time.Second),
		nil,
		func(ctx context.Context) error {
			if fsServer == nil {
				return nil
			}
			return fsServer.RefreshAllSectors(ctx)
		},
	)
	slog.Info("VATSIM transceiver cache enabled for sector ownership refresh")

	frontendHub := frontend.NewHub(stripService, authenticationService)
	euroscopeHub := euroscope.NewHub(stripService, controllerService, authenticationService)
	albHub := alb.NewHub()

	stripService.SetFrontendHub(frontendHub)
	stripService.SetEuroscopeHub(euroscopeHub)
	stripService.SetSectorOwnerRepo(sectorRepo)
	cdmService.SetFrontendHub(frontendHub)
	cdmService.SetEuroscopeHub(euroscopeHub)

	cdmCfg := config.GetCdmConfig()
	resolveURI := func(uri string) string {
		if uri == "" || strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
			return uri
		}
		return filepath.Join(config.GetConfigDir(), uri)
	}
	configStore := cdm.NewCdmConfigStore(
		resolveURI(cdmCfg.RateUri),
		resolveURI(cdmCfg.SidIntervalUri),
		resolveURI(cdmCfg.TaxizonesUri),
		envDuration("CDM_CONFIG_REFRESH_INTERVAL", 15*time.Minute),
		cdm.CdmConfigDefaults{
			Rate:        cdmCfg.Rate,
			RateLvo:     cdmCfg.RateLvo,
			TaxiMinutes: cdm.DefaultCDMTaxiMinutes,
		},
		nil,
	)
	// Seed deice config and default rates from YAML before first refresh.
	configStore.SeedAirportConfig("EKCH", cdmCfg.Rate, cdmCfg.RateLvo, cdm.CdmDeiceConfig{
		Light:  cdmCfg.Deice.Light,
		Medium: cdmCfg.Deice.Medium,
		Heavy:  cdmCfg.Deice.Heavy,
		Super:  cdmCfg.Deice.Super,
		Platform: func() []cdm.CdmDeicePlatformConfig {
			platforms := make([]cdm.CdmDeicePlatformConfig, len(cdmCfg.Deice.Platform))
			for i, p := range cdmCfg.Deice.Platform {
				platforms[i] = cdm.CdmDeicePlatformConfig{Name: p.Name, Time: p.Time}
			}
			return platforms
		}(),
	})
	sequenceService := cdm.NewSequenceService(stripRepo, sessionRepo, configStore, frontendHub, euroscopeHub)
	cdmService.SetConfigProvider(configStore)
	cdmService.SetSequenceService(sequenceService)
	go configStore.Start(ctx)
	slog.Info("CDM local calculation enabled", slog.String("rateUri", resolveURI(cdmCfg.RateUri)))

	if pdcService != nil {
		pdcService.SetFrontendHub(frontendHub)
		pdcService.SetEuroscopeHub(euroscopeHub)
	}

	fsServer = server.NewServer(dbpool, euroscopeHub, frontendHub, cdmService, pdcService, transceiverCache, stripRepo, controllerRepo, sessionRepo, sectorRepo, coordRepo, tacticalStripRepo)

	frontendHub.SetServer(fsServer)
	euroscopeHub.SetServer(fsServer)
	controllerService.SetServer(fsServer)
	controllerService.SetStripService(stripService)

	frontendUpgrader := websocket.NewConnectionUpgrader[pkgFrontend.EventType, *frontend.Client](frontendHub, authenticationService)
	euroscopeUpgrader := websocket.NewConnectionUpgrader[pkgEuroscope.EventType, *euroscope.Client](euroscopeHub, authenticationService)

	go cdmService.Start(ctx)
	go pdcService.Start(ctx)
	if vatsimCache != nil {
		go vatsimCache.Start(ctx)
	}
	go transceiverCache.Start(ctx)
	go albHub.Run()

	metarPoller := metar.NewPoller(sessionRepo, frontendHub)
	go metarPoller.Start(ctx)

	trafficMetrics := services.NewTrafficMetricsService(sessionRepo, stripRepo)
	go trafficMetrics.Start(ctx)

	// TODO remove
	db := database.New(dbpool)
	_ = db.InsertAirport(context.Background(), "EKCH")

	// Health Function for local Dev
	http.HandleFunc("/healthz", healthz)
	http.HandleFunc("/euroscopeEvents", euroscopeUpgrader.Upgrade)
	http.HandleFunc("/frontEndEvents", frontendUpgrader.Upgrade)
	http.HandleFunc("/albEvents", albHub.Upgrade)
	apiMux := http.NewServeMux()
	flightLookup := pdc.NewFlightLookupAdapter(pdcService, sessionRepo)
	pilot.NewWebAPI(authenticationService, vatsimCache, flightLookup, requireLiveCIDVerification).RegisterRoutes(apiMux)
	pdc.NewWebAPI(authenticationService, pdcService, vatsimCache, requireLiveCIDVerification).RegisterRoutes(apiMux)
	http.Handle("/api/", server.APIMiddleware(http.StripPrefix("/api", apiMux)))

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
