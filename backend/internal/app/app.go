package app

import (
	"FlightStrips/internal/alb"
	"FlightStrips/internal/cdm"
	appconfig "FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/internal/ecfmp"
	ecfmpWebAPI "FlightStrips/internal/ecfmp/webapi"
	"FlightStrips/internal/euroscope"
	"FlightStrips/internal/frontend"
	"FlightStrips/internal/metar"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/pilot"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/server"
	"FlightStrips/internal/services"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/vatsim"
	"FlightStrips/internal/websocket"
	pkgEuroscope "FlightStrips/pkg/events/euroscope"
	pkgFrontend "FlightStrips/pkg/events/frontend"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
)

type Config struct {
	DatabaseConnectionString string
	OIDCSigningAlgorithm     string
	OIDCAuthority            string
	OIDCAudience             string
	Environment              string
	EnablePostgresTracing    bool
	EnableHTTPTracing        bool

	CDMKey                   string
	CDMConfigDir             string
	CDMConfigRefreshInterval time.Duration
	EnableCDMConfigStore     bool

	HoppieLogon          string
	PDCWebLookupLiveOnly bool
	EnablePDC            bool

	ECFMPBaseURL          string
	EnableECFMP           bool
	EnableECFMPAPI        bool
	EnablePilotAPI        bool
	EnableALB             bool
	EnableMetar           bool
	EnableVATSIM          bool
	EnableTransceivers    bool
	EnableTraffic         bool
	EnableStandAssignment bool
	EnableDBSeed          bool
	CloseDBOnClose        bool
}

type Dependencies struct {
	DBPool                *pgxpool.Pool
	AuthenticationService shared.AuthenticationService
	PDCClient             pdc.HoppieClientInterface
	VATSIMStatusURL       string
	VATSIMPollInterval    time.Duration
	TransceiversURL       string
	TransceiversInterval  time.Duration
}

type App struct {
	dbpool                   *pgxpool.Pool
	closeDB                  bool
	handler                  http.Handler
	workers                  []func(context.Context)
	startWorkers             sync.Once
	standAssignmentReadiness appconfig.StandAssignmentReadiness
}

func Build(ctx context.Context, cfg Config, deps Dependencies) (*App, error) {
	cfg = cfg.withDefaults()
	standAssignmentReadiness := configureStandAssignment(cfg.EnableStandAssignment)

	dbpool, closeDB, err := buildDBPool(ctx, cfg, deps.DBPool)
	if err != nil {
		return nil, err
	}

	authService, err := buildAuthenticationService(cfg, deps.AuthenticationService)
	if err != nil {
		if closeDB {
			dbpool.Close()
		}
		return nil, err
	}

	stripRepo := postgres.NewStripRepository(dbpool)
	controllerRepo := postgres.NewControllerRepository(dbpool)
	sessionRepo := postgres.NewSessionRepository(dbpool)
	sectorRepo := postgres.NewSectorOwnerRepository(dbpool)
	coordRepo := postgres.NewCoordinationRepository(dbpool)
	tacticalStripRepo := postgres.NewTacticalStripRepository(dbpool)

	stripService := services.NewStripService(
		stripRepo,
		services.WithTacticalStripRepository(tacticalStripRepo),
		services.WithCoordinationStore(coordRepo),
		services.WithControllerReader(controllerRepo),
		services.WithSessionReader(sessionRepo),
		services.WithSectorOwnerRepository(sectorRepo),
	)
	stripValidationService := services.NewStripValidationService(stripRepo, stripRepo)
	controllerService := services.NewControllerService(controllerRepo)
	cdmClient := cdm.NewClient(cdm.WithAPIKey(cfg.CDMKey))
	cdmService := cdm.NewCdmService(cdmClient, stripRepo, sessionRepo, controllerRepo)
	stripService.SetCdmService(cdmService)
	cdmService.SetValidationReevaluator(stripService)

	pdcService := buildPDCService(cfg, deps.PDCClient, sessionRepo, stripRepo, sectorRepo, controllerRepo)
	requireLiveCIDVerification := isLiveEnvironment(cfg.Environment)
	vatsimCache := buildVATSIMCache(cfg, deps, requireLiveCIDVerification)

	var fsServer *server.Server
	var transceiverCache *vatsim.TransceiverCache
	var transceiverLookup server.TransceiverLookup = noopTransceiverLookup{}
	if cfg.EnableTransceivers {
		transceiverCache = vatsim.NewTransceiverCache(
			deps.TransceiversURL,
			deps.TransceiversInterval,
			nil,
			func(ctx context.Context) error {
				if fsServer == nil {
					return nil
				}
				return fsServer.RefreshAllSectors(ctx)
			},
		)
		transceiverLookup = transceiverCache
		slog.Info("VATSIM transceiver cache enabled for sector ownership refresh")
	}

	frontendHub := frontend.NewHub(stripService, authService)
	euroscopeHub := euroscope.NewHub(stripService, controllerService, authService)
	albHub := alb.NewHub()
	ecfmpService := ecfmp.NewService(ecfmp.NewClient(ecfmp.WithBaseURL(cfg.ECFMPBaseURL)), stripRepo, sessionRepo, frontendHub, euroscopeHub)

	stripService.SetFrontendHub(frontendHub)
	stripValidationService.SetFrontendHub(frontendHub)
	frontendHub.SetValidationService(stripValidationService)
	stripService.SetEuroscopeHub(euroscopeHub)
	stripService.SetSectorOwnerRepo(sectorRepo)
	cdmService.SetFrontendHub(frontendHub)
	cdmService.SetEuroscopeHub(euroscopeHub)

	sequenceService, configStore := configureCDM(cfg, cdmClient, cdmService, stripRepo, sessionRepo, frontendHub, euroscopeHub)

	if pdcService != nil {
		stripService.SetPdcService(pdcService)
		pdcService.SetStripService(stripService)
		pdcService.SetFrontendHub(frontendHub)
		pdcService.SetEuroscopeHub(euroscopeHub)
		pdcService.SetTransceiverLookup(transceiverLookup)
	}

	fsServer = server.NewServer(dbpool, euroscopeHub, frontendHub, cdmService, pdcService, transceiverLookup, stripRepo, controllerRepo, sessionRepo, sectorRepo, coordRepo, tacticalStripRepo)

	frontendHub.SetServer(fsServer)
	euroscopeHub.SetServer(fsServer)
	stripService.SetRouteRecalculator(fsServer)
	controllerService.SetFrontendNotifier(frontendHub)
	controllerService.SetSessionRecalculator(fsServer)
	controllerService.SetStripService(stripService)

	if cfg.EnableDBSeed {
		db := database.New(dbpool)
		_ = db.InsertAirport(context.Background(), "EKCH")
	}

	app := &App{
		dbpool:                   dbpool,
		closeDB:                  closeDB,
		standAssignmentReadiness: standAssignmentReadiness,
		handler: buildHandler(buildHandlerConfig{
			authService:                authService,
			frontendHub:                frontendHub,
			euroscopeHub:               euroscopeHub,
			albHub:                     albHub,
			pdcService:                 pdcService,
			sessionRepo:                sessionRepo,
			sequenceService:            sequenceService,
			vatsimCache:                vatsimCache,
			requireLiveCIDVerification: requireLiveCIDVerification,
			enableHTTPTracing:          cfg.EnableHTTPTracing,
			enableALB:                  cfg.EnableALB,
			enableCDMAPI:               sequenceService != nil,
			enableECFMPAPI:             cfg.EnableECFMPAPI,
			enablePilotAPI:             cfg.EnablePilotAPI,
			enablePDCAPI:               pdcService != nil,
			ecfmpService:               ecfmpService,
		}),
	}

	app.addWorker(cdmService.Start)
	if configStore != nil {
		app.addWorker(configStore.Start)
	}
	if cfg.EnablePDC && pdcService != nil {
		app.addWorker(pdcService.Start)
	}
	if cfg.EnableVATSIM && vatsimCache != nil {
		app.addWorker(vatsimCache.Start)
	}
	if transceiverCache != nil {
		app.addWorker(transceiverCache.Start)
	}
	if cfg.EnableECFMP {
		app.addWorker(ecfmpService.Start)
	}
	if cfg.EnableALB {
		app.addWorker(func(context.Context) { albHub.Run() })
	}
	if cfg.EnableMetar {
		metarPoller := metar.NewPoller(sessionRepo, frontendHub)
		app.addWorker(metarPoller.Start)
	}
	if cfg.EnableTraffic {
		trafficMetrics := services.NewTrafficMetricsService(sessionRepo, stripRepo)
		app.addWorker(trafficMetrics.Start)
	}

	return app, nil
}

// StandAssignmentReadiness returns the SAT configuration state created with
// this application. Future SAT routes, workers, and contracts must use this
// state instead of independently reading environment configuration.
func (a *App) StandAssignmentReadiness() appconfig.StandAssignmentReadiness {
	return a.standAssignmentReadiness
}

func configureStandAssignment(enabled bool) appconfig.StandAssignmentReadiness {
	readiness := appconfig.InitializeStandAssignment(enabled)
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

func (a *App) Handler() http.Handler {
	return a.handler
}

func (a *App) StartWorkers(ctx context.Context) {
	a.startWorkers.Do(func() {
		for _, worker := range a.workers {
			go worker(ctx)
		}
	})
}

func (a *App) Close(context.Context) error {
	if a.closeDB && a.dbpool != nil {
		a.dbpool.Close()
	}
	return nil
}

func (a *App) DBPool() *pgxpool.Pool {
	return a.dbpool
}

func (a *App) addWorker(worker func(context.Context)) {
	if worker != nil {
		a.workers = append(a.workers, worker)
	}
}

func buildDBPool(ctx context.Context, cfg Config, dbpool *pgxpool.Pool) (*pgxpool.Pool, bool, error) {
	if dbpool != nil {
		return dbpool, cfg.CloseDBOnClose, nil
	}

	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseConnectionString)
	if err != nil {
		return nil, false, fmt.Errorf("parse database connection string: %w", err)
	}

	if cfg.EnablePostgresTracing {
		poolConfig.ConnConfig.Tracer = otelpgx.NewTracer(
			otelpgx.WithTracerProvider(otel.GetTracerProvider()),
			otelpgx.WithTrimSQLInSpanName(),
		)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, false, fmt.Errorf("connect to database: %w", err)
	}

	if cfg.EnablePostgresTracing {
		if err := otelpgx.RecordStats(pool); err != nil {
			slog.Warn("Failed to record database stats", slog.Any("error", err))
		}
	}

	return pool, true, nil
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

func buildPDCService(
	cfg Config,
	client pdc.HoppieClientInterface,
	sessionRepo repository.SessionRepository,
	stripRepo repository.StripRepository,
	sectorRepo repository.SectorOwnerRepository,
	controllerRepo repository.ControllerRepository,
) *pdc.Service {
	if !cfg.EnablePDC {
		return nil
	}

	if client == nil {
		if cfg.HoppieLogon != "" {
			client = pdc.NewClient(cfg.HoppieLogon)
			slog.Info("PDC Hoppie client initialized")
		} else {
			client = pdc.NoopHoppieClient{}
			slog.Warn("PDC Hoppie client disabled - HOPPIE_LOGON not set")
		}
	}

	service := pdc.NewPDCService(client, sessionRepo, stripRepo, sectorRepo, controllerRepo)
	service.SetWebLookupLiveOnly(cfg.PDCWebLookupLiveOnly)
	return service
}

func buildVATSIMCache(cfg Config, deps Dependencies, requireLiveCIDVerification bool) *vatsim.Cache {
	if !cfg.EnableVATSIM || !requireLiveCIDVerification {
		return nil
	}

	cache := vatsim.NewCache(deps.VATSIMStatusURL, deps.VATSIMPollInterval, nil)
	slog.Info("VATSIM cache enabled for live web PDC ownership verification")
	return cache
}

func configureCDM(
	cfg Config,
	cdmClient *cdm.Client,
	cdmService *cdm.Service,
	stripRepo repository.StripRepository,
	sessionRepo repository.SessionRepository,
	frontendHub *frontend.Hub,
	euroscopeHub *euroscope.Hub,
) (*cdm.SequenceService, *cdm.CdmConfigStore) {
	if !cfg.EnableCDMConfigStore {
		return nil, nil
	}

	cdmCfg := appconfig.GetCdmConfig()
	resolveURI := func(uri string) string {
		if uri == "" || strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
			return uri
		}
		return filepath.Join(cfg.CDMConfigDir, uri)
	}
	configStore := cdm.NewCdmConfigStore(
		resolveURI(cdmCfg.RateUri),
		resolveURI(cdmCfg.SidIntervalUri),
		resolveURI(cdmCfg.TaxizonesUri),
		cfg.CDMConfigRefreshInterval,
		cdm.CdmConfigDefaults{
			Rate:        cdmCfg.Rate,
			RateLvo:     cdmCfg.RateLvo,
			TaxiMinutes: cdmCfg.DefaultTaxiTime,
		},
		nil,
	)
	configStore.SetCdmClient(cdmClient)
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
	configStore.SetOnAirportConfigChanged(func(airport string) {
		if err := cdmService.TriggerRecalculateForAirport(context.Background(), airport); err != nil {
			slog.Warn("CDM config change recalculation failed",
				slog.String("airport", airport),
				slog.Any("error", err),
			)
		}
	})
	slog.Info("CDM local calculation enabled", slog.String("rateUri", resolveURI(cdmCfg.RateUri)))

	return sequenceService, configStore
}

type buildHandlerConfig struct {
	authService                shared.AuthenticationService
	frontendHub                *frontend.Hub
	euroscopeHub               *euroscope.Hub
	albHub                     *alb.Hub
	pdcService                 *pdc.Service
	sessionRepo                repository.SessionRepository
	sequenceService            *cdm.SequenceService
	vatsimCache                *vatsim.Cache
	requireLiveCIDVerification bool
	enableHTTPTracing          bool
	enableALB                  bool
	enableCDMAPI               bool
	enableECFMPAPI             bool
	enablePilotAPI             bool
	enablePDCAPI               bool
	ecfmpService               *ecfmp.Service
}

func buildHandler(cfg buildHandlerConfig) http.Handler {
	mux := http.NewServeMux()
	frontendUpgrader := websocket.NewConnectionUpgrader[pkgFrontend.EventType, *frontend.Client](cfg.frontendHub, cfg.authService)
	euroscopeUpgrader := websocket.NewConnectionUpgrader[pkgEuroscope.EventType, *euroscope.Client](cfg.euroscopeHub, cfg.authService)

	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("/euroscopeEvents", euroscopeUpgrader.Upgrade)
	mux.HandleFunc("/frontEndEvents", frontendUpgrader.Upgrade)
	if cfg.enableALB {
		mux.HandleFunc("/albEvents", cfg.albHub.Upgrade)
	}

	apiMux := http.NewServeMux()
	if cfg.enableCDMAPI {
		cdm.NewWebAPI(cfg.authService, cfg.sessionRepo, cfg.sequenceService).RegisterRoutes(apiMux)
	}
	if cfg.enableECFMPAPI {
		ecfmpWebAPI.NewWebAPI(cfg.ecfmpService).RegisterRoutes(apiMux)
	}
	if cfg.enablePilotAPI {
		flightLookup := pdc.NewFlightLookupAdapter(cfg.pdcService, cfg.sessionRepo)
		pilot.NewWebAPI(cfg.authService, cfg.vatsimCache, flightLookup, cfg.requireLiveCIDVerification).RegisterRoutes(apiMux)
	}
	if cfg.enablePDCAPI {
		pdc.NewWebAPI(cfg.authService, cfg.pdcService, cfg.vatsimCache, cfg.requireLiveCIDVerification).RegisterRoutes(apiMux)
	}
	if cfg.enableCDMAPI || cfg.enableECFMPAPI || cfg.enablePilotAPI || cfg.enablePDCAPI {
		mux.Handle("/api/", server.APIMiddleware(http.StripPrefix("/api", apiMux)))
	}

	var handler http.Handler = mux
	if cfg.enableHTTPTracing {
		handler = otelhttp.NewHandler(handler, "http.server")
	}
	return handler
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

type noopTransceiverLookup struct{}

func (noopTransceiverLookup) GetFrequencies(string) []string {
	return nil
}

func (cfg Config) withDefaults() Config {
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
