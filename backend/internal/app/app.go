package app

import (
	"FlightStrips/internal/alb"
	"FlightStrips/internal/aman"
	"FlightStrips/internal/cdm"
	appconfig "FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/internal/ecfmp"
	ecfmpWebAPI "FlightStrips/internal/ecfmp/webapi"
	"FlightStrips/internal/efb"
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
	"FlightStrips/internal/standdiagnostics"
	"FlightStrips/internal/standstatus"
	"FlightStrips/internal/testtools"
	"FlightStrips/internal/vatsim"
	"FlightStrips/internal/websocket"
	pkgEuroscope "FlightStrips/pkg/events/euroscope"
	pkgFrontend "FlightStrips/pkg/events/frontend"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
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

	ECFMPBaseURL                    string
	EnableECFMP                     bool
	EnableECFMPAPI                  bool
	EnablePilotAPI                  bool
	EnableEFB                       bool
	EnableALB                       bool
	EnableMetar                     bool
	EnableVATSIM                    bool
	EnableTransceivers              bool
	EnableTraffic                   bool
	EnableStandAssignment           bool
	EnableStandAssignmentESMessages bool
	EnableTestTools                 bool
	EnableDBSeed                    bool
	CloseDBOnClose                  bool

	StandAssignmentAircraftJSON string

	StandAssignmentHoldDuration   time.Duration
	StandAssignmentBlockExtension time.Duration
	StandAssignmentSweepInterval  time.Duration

	AMAN aman.RuntimeConfig
}

type Dependencies struct {
	DBPool                *pgxpool.Pool
	AuthenticationService shared.AuthenticationService
	PDCClient             pdc.HoppieClientInterface
	VATSIMStatusURL       string
	VATSIMPollInterval    time.Duration
	TransceiversURL       string
	TransceiversInterval  time.Duration
	AMAN                  aman.Dependencies
}

type App struct {
	dbpool                   *pgxpool.Pool
	closeDB                  bool
	handler                  http.Handler
	workers                  []func(context.Context)
	startWorkers             sync.Once
	standAssignmentReadiness appconfig.StandAssignmentReadiness
	amanRuntime              *aman.Runtime
}

func Build(ctx context.Context, cfg Config, deps Dependencies) (*App, error) {
	cfg = cfg.withDefaults()
	if cfg.EnableTestTools && isLiveEnvironment(cfg.Environment) {
		return nil, errors.New("ENABLE_TEST_TOOLS cannot be enabled in a live environment")
	}
	if err := validateAMANTerminalGeometry(cfg.AMAN); err != nil {
		return nil, err
	}
	if err := cfg.AMAN.Validate(); err != nil {
		return nil, fmt.Errorf("initialize AMAN runtime: %w", err)
	}
	amanEnabled := cfg.AMAN.Mode != "" && cfg.AMAN.Mode != aman.ModeDisabled
	amanOwnership := aman.OwnershipForRolloutGate(cfg.AMAN.Mode, true)
	if amanOwnership.ControllerMutationAuthorized && deps.AMAN.ObservationSink != nil {
		if _, ok := deps.AMAN.SequenceService.(aman.CommandService); !ok {
			return nil, errors.New("initialize AMAN commands: authoritative runtime requires typed command service")
		}
	}
	standAssignmentReadiness := configureStandAssignment(cfg.EnableStandAssignment, cfg.StandAssignmentAircraftJSON)
	var testClock *testtools.Clock
	satNow := time.Now
	if cfg.EnableTestTools {
		testClock = testtools.NewClock()
		satNow = testClock.Now
	}

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

	core := assembleCoreRepositories(dbpool)
	stripRepo := core.strips
	controllerRepo := core.controllers
	sessionRepo := core.sessions
	sectorRepo := core.sectors
	coordRepo := core.coordinations
	tacticalStripRepo := core.tacticalStrips

	satGraph, err := assembleSAT(cfg, standAssignmentReadiness, dbpool, core, satNow)
	if err != nil {
		if closeDB {
			dbpool.Close()
		}
		return nil, err
	}
	standAssignmentRepo := satGraph.assignments
	standAllocationService := satGraph.allocations
	standActionService := satGraph.actions
	departureLifecycle := satGraph.departures
	arrivalLifecycle := satGraph.arrivals
	standAssignmentFailures := satGraph.failures

	stripService := services.NewStripService(
		stripRepo,
		services.WithTacticalStripRepository(tacticalStripRepo),
		services.WithCoordinationStore(coordRepo),
		services.WithControllerReader(controllerRepo),
		services.WithSessionReader(sessionRepo),
		services.WithSectorOwnerRepository(sectorRepo),
	)
	if departureLifecycle != nil {
		stripService.SetDeparturePositionObserver(departureLifecycle)
	}
	controllerService := services.NewControllerService(controllerRepo)
	cdmClient := cdm.NewClient(cdm.WithAPIKey(cfg.CDMKey))

	requireLiveCIDVerification := isLiveEnvironment(cfg.Environment)
	vatsimGraph := assembleVATSIMSource(cfg, deps, requireLiveCIDVerification, standAssignmentReadiness.Ready || amanEnabled)

	var fsServer *server.Server
	transports := assembleTransports(cfg, deps, func(ctx context.Context) error {
		return fsServer.RefreshAllSectors(ctx)
	})
	transceiverCache := transports.transceivers
	serverFrequencyProviders := transports.serverFrequencyProviders
	pdcFrequencyProviders := transports.pdcFrequencyProviders

	amanDependencies := deps.AMAN
	var defaultAMAN operationalAMANAssembly
	if amanEnabled && amanDependencies.ObservationSink == nil {
		defaultAMAN, err = assembleOperationalAMAN(cfg.AMAN, dbpool)
		if err != nil {
			if closeDB {
				dbpool.Close()
			}
			return nil, err
		}
		amanDependencies = defaultAMAN.dependencies
	}
	amanRuntime, err := aman.NewRuntime(cfg.AMAN, amanDependencies)
	if err != nil {
		if closeDB {
			dbpool.Close()
		}
		return nil, fmt.Errorf("initialize AMAN runtime: %w", err)
	}
	amanCommands, hasAMANCommands := amanDependencies.SequenceService.(aman.CommandService)
	if amanRuntime.Ownership().ControllerMutationAuthorized && !hasAMANCommands {
		if closeDB {
			dbpool.Close()
		}
		return nil, errors.New("initialize AMAN commands: authoritative runtime requires typed command service")
	}
	if amanRuntime.Ownership().ControllerMutationAuthorized {
		amanCommands = &amanCommandGate{health: amanRuntime.Health, commands: amanCommands}
	}
	var amanStateProvider frontend.AMANStateProvider
	if defaultAMAN.transport != nil {
		amanStateProvider = defaultAMAN.transport
	} else if provider, ok := amanDependencies.Publisher.(frontend.AMANStateProvider); ok {
		amanStateProvider = provider
	}
	realtime, err := assembleRealtime(stripService, controllerService, authService, amanStateProvider, amanCommands, cfg.AMAN.FMPRoles, amanRuntime.Ownership().ControllerMutationAuthorized)
	if err != nil {
		if closeDB {
			dbpool.Close()
		}
		return nil, err
	}
	frontendHub := realtime.frontend
	euroscopeHub := realtime.euroscope
	if defaultAMAN.transport != nil {
		defaultAMAN.transport.setHub(frontendHub)
	}
	stripValidationService, err := services.NewStripValidationService(services.StripValidationDependencies{
		Strips: stripRepo, Statuses: stripRepo, Publisher: frontendHub,
	})
	if err != nil {
		if closeDB {
			dbpool.Close()
		}
		return nil, fmt.Errorf("initialize strip validation service: %w", err)
	}
	if standAllocationService != nil {
		frontendHub.SetStandActionService(standActionService)
		standAllocationService.SetPublisher(frontendHub.PublishStandAllocation)
	}
	cdmService, err := cdm.NewCdmService(cdm.ServiceDependencies{
		Client:                cdmClient,
		Strips:                stripRepo,
		Sessions:              sessionRepo,
		Controllers:           controllerRepo,
		Frontend:              frontendHub,
		Euroscope:             euroscopeHub,
		ValidationReevaluator: stripService,
	})
	if err != nil {
		if closeDB {
			dbpool.Close()
		}
		return nil, fmt.Errorf("initialize CDM service: %w", err)
	}
	stripService.SetCdmService(cdmService)
	var vatsimReconciler *vatsim.Reconciler
	if standAssignmentReadiness.Ready && vatsimGraph.source != nil && standAssignmentRepo != nil {
		latitude, longitude := appconfig.GetAirportCoordinates()
		vatsimReconciler, err = vatsim.NewReconciler(vatsim.ReconcilerDependencies{
			Cache:              vatsimGraph.source,
			Sessions:           sessionRepo,
			Strips:             stripRepo,
			Assignments:        standAssignmentRepo,
			DepartureLifecycle: departureLifecycle,
			ArrivalLifecycle:   arrivalLifecycle,
			Notifier:           frontendHub,
		}, deps.VATSIMPollInterval, vatsim.WithAirportCoordinates(latitude, longitude), vatsim.WithClock(satNow), vatsim.WithLegacyArrivalETAWriter(amanOwnership.LegacyArrivalETAWriter))
		if err != nil {
			if closeDB {
				dbpool.Close()
			}
			return nil, fmt.Errorf("initialize VATSIM reconciler: %w", err)
		}
		euroscopeHub.SetAircraftDisconnectRetainer(vatsimReconciler.RetainsStrip)
	}
	var amanObservationWorker *vatsim.ObservationWorker
	if amanRuntime.Enabled() {
		if vatsimGraph.source == nil {
			if closeDB {
				dbpool.Close()
			}
			return nil, errors.New("initialize AMAN VATSIM observations: VATSIM source is unavailable")
		}
		amanObservationWorker, err = vatsim.NewObservationWorker(vatsim.ObservationWorkerDependencies{
			Cache: vatsimGraph.source, Identities: postgres.NewAMANRepository(dbpool), Sink: amanDependencies.ObservationSink,
			EnabledAirports: cfg.AMAN.EnabledAirports, StaleAfter: satStaleAfter(deps.VATSIMPollInterval), Now: satNow,
		})
		if err != nil {
			if closeDB {
				dbpool.Close()
			}
			return nil, fmt.Errorf("initialize AMAN VATSIM observations: %w", err)
		}
	}
	testToolsGraph := assembleTestTools(cfg.EnableTestTools, testToolsAssemblyDependencies{
		auth: authService, readiness: standAssignmentReadiness,
		source: vatsimGraph.synthetic, reconciler: vatsimReconciler,
		sat: satGraph, core: core, stripDeleter: stripService,
		clock: testClock, euroscope: euroscopeHub, enableStandAssignmentESMessages: cfg.EnableStandAssignmentESMessages,
	})
	var albHub *alb.Hub
	if cfg.EnableALB {
		albHub = alb.NewHub()
	}
	var ecfmpService *ecfmp.Service
	if cfg.EnableECFMP || cfg.EnableECFMPAPI {
		ecfmpService, err = ecfmp.NewService(ecfmp.ServiceDependencies{
			Client:    ecfmp.NewClient(ecfmp.WithBaseURL(cfg.ECFMPBaseURL)),
			Strips:    stripRepo,
			Sessions:  sessionRepo,
			Frontend:  frontendHub,
			Euroscope: euroscopeHub,
		})
		if err != nil {
			if closeDB {
				dbpool.Close()
			}
			return nil, fmt.Errorf("initialize ECFMP service: %w", err)
		}
	}

	stripService.SetFrontendHub(frontendHub)
	frontendHub.SetValidationService(stripValidationService)
	stripService.SetEuroscopeHub(euroscopeHub)
	stripService.SetSectorOwnerRepo(sectorRepo)

	sequenceService, configStore, err := configureCDM(cfg, cdmClient, cdmService, stripRepo, sessionRepo, frontendHub, euroscopeHub)
	if err != nil {
		if closeDB {
			dbpool.Close()
		}
		return nil, err
	}

	var pdcService *pdc.Service
	if cfg.EnablePDC {
		pdcService, err = buildPDCService(
			cfg,
			deps.PDCClient,
			sessionRepo,
			stripRepo,
			sectorRepo,
			controllerRepo,
			frontendHub,
			euroscopeHub,
			stripService,
			pdcFrequencyProviders,
		)
		if err != nil {
			if closeDB {
				dbpool.Close()
			}
			return nil, err
		}
		stripService.SetPdcService(pdcService)
		if err := frontendHub.RegisterPDCHandlers(pdcService); err != nil {
			if closeDB {
				dbpool.Close()
			}
			return nil, fmt.Errorf("register frontend PDC handlers: %w", err)
		}
		if err := euroscopeHub.RegisterPDCHandlers(pdcService); err != nil {
			if closeDB {
				dbpool.Close()
			}
			return nil, fmt.Errorf("register EuroScope PDC handlers: %w", err)
		}
	}

	fsServer, err = server.NewServer(server.Dependencies{
		DBPool:             dbpool,
		Euroscope:          euroscopeHub,
		Frontend:           frontendHub,
		CDM:                cdmService,
		FrequencyProviders: serverFrequencyProviders,
		Strips:             stripRepo,
		Controllers:        controllerRepo,
		Sessions:           sessionRepo,
		Sectors:            sectorRepo,
		Coordinations:      coordRepo,
		TacticalStrips:     tacticalStripRepo,
		StandAssignments:   standAssignmentRepo,
	})
	if err != nil {
		if closeDB {
			dbpool.Close()
		}
		return nil, fmt.Errorf("initialize server: %w", err)
	}

	frontendHub.SetServer(fsServer)
	euroscopeHub.SetServer(fsServer)
	stripService.SetRouteRecalculator(fsServer)
	if departureLifecycle != nil {
		departureLifecycle.SetRouteRecalculator(fsServer)
		departureLifecycle.SetStandPublisher(frontendHub)
	}
	controllerService.SetFrontendNotifier(frontendHub)
	controllerService.SetSessionRecalculator(fsServer)
	controllerService.SetStripService(stripService)

	if cfg.EnableDBSeed {
		db := database.New(dbpool)
		_ = db.InsertAirport(context.Background(), "EKCH")
	}

	metarPoller := metar.NewPoller(sessionRepo, frontendHub)
	efbFlightFinder := efb.NewFlightQuery(sessionRepo, stripRepo, requireLiveCIDVerification)
	app := &App{
		dbpool:                   dbpool,
		closeDB:                  closeDB,
		standAssignmentReadiness: standAssignmentReadiness,
		amanRuntime:              amanRuntime,
		handler: buildHandler(buildHandlerConfig{
			amanRuntime:  amanRuntime,
			authService:  authService,
			frontendHub:  frontendHub,
			euroscopeHub: euroscopeHub,
			albHub:       albHub,
			pdcService:   pdcService,
			efbAPI: efb.NewWebAPI(efb.WebAPIConfig{
				Auth: authService, Callsigns: vatsimGraph.source, Flights: efbFlightFinder, Sessions: sessionRepo,
				Assignments: standAssignmentRepo, CDM: cdmService, CDMReady: sequenceService != nil,
				Stands: standActionService, ATIS: metarPoller, Departures: fsServer, PDCReady: pdcService != nil, Live: requireLiveCIDVerification,
			}),
			sessionRepo:                sessionRepo,
			sequenceService:            sequenceService,
			vatsimSource:               vatsimGraph.source,
			standAssignmentRepo:        standAssignmentRepo,
			standAssignmentReadiness:   standAssignmentReadiness,
			standAssignmentDiagnostics: standAssignmentDiagnostics(),
			standAssignmentFailures:    standAssignmentFailures,
			standAssignmentStaleAfter:  satStaleAfter(deps.VATSIMPollInterval),
			requireLiveCIDVerification: requireLiveCIDVerification,
			enableHTTPTracing:          cfg.EnableHTTPTracing,
			enableALB:                  cfg.EnableALB,
			enableCDMAPI:               sequenceService != nil,
			enableECFMPAPI:             cfg.EnableECFMPAPI,
			enablePilotAPI:             cfg.EnablePilotAPI,
			enableEFBAPI:               cfg.EnableEFB,
			enablePDCAPI:               pdcService != nil,
			enableTestTools:            cfg.EnableTestTools,
			ecfmpService:               ecfmpService,
			testToolsAPI:               testToolsGraph.api,
		}),
	}

	app.addWorker(cdmService.Start)
	app.addWorker(fsServer.StartSessionMonitor)
	app.addWorker(frontendHub.Run)
	app.addWorker(euroscopeHub.Run)
	if configStore != nil {
		app.addWorker(configStore.Start)
	}
	if cfg.EnablePDC && pdcService != nil {
		app.addWorker(pdcService.Start)
	}
	if cfg.EnableVATSIM && vatsimGraph.cache != nil {
		app.addWorker(vatsimGraph.cache.Start)
	}
	if vatsimReconciler != nil && !cfg.EnableTestTools {
		app.addWorker(vatsimReconciler.Start)
	}
	if amanObservationWorker != nil {
		app.addWorker(func(ctx context.Context) { amanObservationWorker.Run(ctx, cfg.AMAN.SurveillanceInterval) })
	}
	if departureLifecycle != nil {
		app.addWorker(departureLifecycle.StartSweep)
	}
	if arrivalLifecycle != nil {
		app.addWorker(arrivalLifecycle.StartSweep)
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
		app.addWorker(metarPoller.Start)
	}
	if cfg.EnableTraffic {
		trafficMetrics := services.NewTrafficMetricsService(sessionRepo, stripRepo)
		app.addWorker(trafficMetrics.Start)
	}
	if amanRuntime.Enabled() {
		app.addWorker(amanRuntime.Start)
	}

	return app, nil
}

type coreRepositories struct {
	strips         repository.StripRepository
	controllers    repository.ControllerRepository
	sessions       repository.SessionRepository
	sectors        repository.SectorOwnerRepository
	coordinations  repository.CoordinationRepository
	tacticalStrips repository.TacticalStripRepository
}

func assembleCoreRepositories(dbpool *pgxpool.Pool) coreRepositories {
	return coreRepositories{
		strips:         postgres.NewStripRepository(dbpool),
		controllers:    postgres.NewControllerRepository(dbpool),
		sessions:       postgres.NewSessionRepository(dbpool),
		sectors:        postgres.NewSectorOwnerRepository(dbpool),
		coordinations:  postgres.NewCoordinationRepository(dbpool),
		tacticalStrips: postgres.NewTacticalStripRepository(dbpool),
	}
}

type satAssembly struct {
	assignments repository.StandAssignmentRepository
	allocations *services.StandAllocationService
	actions     *services.StandActionService
	departures  *services.DepartureLifecycleService
	arrivals    *services.ArrivalLifecycleService
	failures    *standdiagnostics.AllocationFailureLog
}

func assembleSAT(cfg Config, readiness appconfig.StandAssignmentReadiness, dbpool *pgxpool.Pool, core coreRepositories, now func() time.Time) (satAssembly, error) {
	graph := satAssembly{
		failures: standdiagnostics.NewAllocationFailureLog(100),
	}
	if !readiness.Ready {
		return graph, nil
	}

	assignments := postgres.NewStandAssignmentRepository(dbpool)
	stands := appconfig.GetStandCapabilities()
	aircraft := appconfig.GetAircraftReference()
	engines := appconfig.GetAircraftEngineReference()
	borders := appconfig.GetAirportCountries()
	allocations, err := services.NewStandAllocationService(
		dbpool, core.strips, assignments, stands, appconfig.GetAirlineAssignment(),
		services.WithStandAllocationFailureLog(graph.failures),
		services.WithStandAllocationClock(now),
	)
	if err != nil {
		return satAssembly{}, fmt.Errorf("initialize stand allocation service: %w", err)
	}
	departures, err := services.NewDepartureLifecycleService(
		allocations, assignments, core.strips, core.sessions, stands, aircraft, engines, borders,
		services.WithDepartureHoldDuration(cfg.StandAssignmentHoldDuration),
		services.WithDepartureBlockExtension(cfg.StandAssignmentBlockExtension),
		services.WithDepartureSweepInterval(cfg.StandAssignmentSweepInterval),
		services.WithDepartureLifecycleClock(now),
	)
	if err != nil {
		return satAssembly{}, fmt.Errorf("initialize departure lifecycle service: %w", err)
	}
	arrivals, err := services.NewArrivalLifecycleService(
		allocations, assignments, core.strips, core.sessions, stands, aircraft, engines, borders,
		services.WithArrivalSweepInterval(cfg.StandAssignmentSweepInterval),
		services.WithArrivalLifecycleClock(now),
	)
	if err != nil {
		return satAssembly{}, fmt.Errorf("initialize arrival lifecycle service: %w", err)
	}

	graph.assignments = assignments
	graph.allocations = allocations
	graph.actions = services.NewStandActionService(allocations, assignments, core.strips, aircraft, engines, borders)
	graph.departures = departures
	graph.arrivals = arrivals
	return graph, nil
}

type transportAssembly struct {
	transceivers             *vatsim.TransceiverCache
	serverFrequencyProviders []server.TransceiverLookup
	pdcFrequencyProviders    []pdc.TransceiverLookup
}

func assembleTransports(cfg Config, deps Dependencies, refreshSectors func(context.Context) error) transportAssembly {
	if !cfg.EnableTransceivers || cfg.EnableTestTools {
		return transportAssembly{}
	}

	cache := vatsim.NewTransceiverCache(
		deps.TransceiversURL,
		deps.TransceiversInterval,
		nil,
		refreshSectors,
	)
	slog.Info("VATSIM transceiver cache enabled for sector ownership refresh")
	return transportAssembly{
		transceivers:             cache,
		serverFrequencyProviders: []server.TransceiverLookup{cache},
		pdcFrequencyProviders:    []pdc.TransceiverLookup{cache},
	}
}

type realtimeAssembly struct {
	frontend  *frontend.Hub
	euroscope *euroscope.Hub
}

func assembleRealtime(stripService shared.StripService, controllerService shared.ControllerService, authService shared.AuthenticationService, amanState frontend.AMANStateProvider, amanCommands aman.CommandService, amanFMPRoles []string, amanMutations bool) (realtimeAssembly, error) {
	frontendHub, err := frontend.NewHub(frontend.HubDependencies{
		Strips: stripService, Authentication: authService, AMANState: amanState, AMANCommands: amanCommands, AMANFMPRoles: amanFMPRoles, AMANMutations: amanMutations,
	})
	if err != nil {
		return realtimeAssembly{}, fmt.Errorf("initialize frontend hub: %w", err)
	}
	euroscopeHub, err := euroscope.NewHub(euroscope.HubDependencies{
		Strips: stripService, Controllers: controllerService, Authentication: authService,
	})
	if err != nil {
		return realtimeAssembly{}, fmt.Errorf("initialize EuroScope hub: %w", err)
	}
	return realtimeAssembly{frontend: frontendHub, euroscope: euroscopeHub}, nil
}

// StandAssignmentReadiness returns the SAT configuration state created with
// this application. Future SAT routes, workers, and contracts must use this
// state instead of independently reading environment configuration.
func (a *App) StandAssignmentReadiness() appconfig.StandAssignmentReadiness {
	return a.standAssignmentReadiness
}

// AMANRuntime returns the application's AMAN lifecycle and ownership state.
func (a *App) AMANRuntime() *aman.Runtime {
	if a == nil {
		return nil
	}
	return a.amanRuntime
}

func validateAMANTerminalGeometry(config aman.RuntimeConfig) error {
	path := strings.TrimSpace(config.TerminalGeometryPath)
	if (config.Mode == "" || config.Mode == aman.ModeDisabled) && path == "" {
		return nil
	}
	if path == "" {
		return fmt.Errorf("AMAN terminal geometry path is required when enabled")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("AMAN terminal geometry path %q: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("AMAN terminal geometry path %q must name a file", path)
	}
	return nil
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
	frontendHub shared.FrontendHub,
	euroscopeHub shared.EuroscopeHub,
	stripService shared.StripService,
	transceiverProviders []pdc.TransceiverLookup,
) (*pdc.Service, error) {
	if client == nil {
		if hoppieLogon := strings.TrimSpace(cfg.HoppieLogon); hoppieLogon != "" {
			client = pdc.NewClient(hoppieLogon)
			slog.Info("PDC Hoppie client initialized")
		} else {
			slog.Info("PDC Hoppie integration disabled; Web PDC remains available")
		}
	}

	service, err := pdc.NewPDCService(pdc.ServiceDependencies{
		Client:               client,
		Sessions:             sessionRepo,
		Strips:               stripRepo,
		Sectors:              sectorRepo,
		Controllers:          controllerRepo,
		Frontend:             frontendHub,
		Euroscope:            euroscopeHub,
		StripService:         stripService,
		TransceiverProviders: transceiverProviders,
		WebLookupLiveOnly:    cfg.PDCWebLookupLiveOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize PDC service: %w", err)
	}
	return service, nil
}

func buildVATSIMCache(cfg Config, deps Dependencies, requireLiveCIDVerification bool, enableReconciliation bool) *vatsim.Cache {
	if !cfg.EnableVATSIM || (!requireLiveCIDVerification && !enableReconciliation) {
		return nil
	}

	cache := vatsim.NewCache(deps.VATSIMStatusURL, deps.VATSIMPollInterval, nil)
	slog.Info("VATSIM cache enabled", slog.Bool("livePdcVerification", requireLiveCIDVerification), slog.Bool("reconciliationRequired", enableReconciliation))
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
) (*cdm.SequenceService, *cdm.CdmConfigStore, error) {
	if !cfg.EnableCDMConfigStore {
		return nil, nil, nil
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
	sequenceService, err := cdm.NewSequenceService(cdm.SequenceServiceDependencies{
		Strips: stripRepo, Sessions: sessionRepo, Config: configStore, Frontend: frontendHub, Euroscope: euroscopeHub,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("initialize CDM sequence service: %w", err)
	}
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

	return sequenceService, configStore, nil
}

type buildHandlerConfig struct {
	amanRuntime                *aman.Runtime
	authService                shared.AuthenticationService
	frontendHub                *frontend.Hub
	euroscopeHub               *euroscope.Hub
	albHub                     *alb.Hub
	pdcService                 *pdc.Service
	efbAPI                     *efb.WebAPI
	sessionRepo                repository.SessionRepository
	sequenceService            *cdm.SequenceService
	vatsimSource               vatsim.FlightSource
	standAssignmentRepo        repository.StandAssignmentRepository
	standAssignmentReadiness   appconfig.StandAssignmentReadiness
	standAssignmentDiagnostics standstatus.WebAPIDiagnostics
	standAssignmentFailures    *standdiagnostics.AllocationFailureLog
	standAssignmentStaleAfter  time.Duration
	requireLiveCIDVerification bool
	enableHTTPTracing          bool
	enableALB                  bool
	enableCDMAPI               bool
	enableECFMPAPI             bool
	enablePilotAPI             bool
	enableEFBAPI               bool
	enablePDCAPI               bool
	enableTestTools            bool
	ecfmpService               *ecfmp.Service
	testToolsAPI               *testtools.WebAPI
}

func buildHandler(cfg buildHandlerConfig) http.Handler {
	mux := http.NewServeMux()
	frontendUpgrader := websocket.NewConnectionUpgrader[pkgFrontend.EventType, *frontend.Client](cfg.frontendHub, cfg.authService)
	euroscopeUpgrader := websocket.NewConnectionUpgrader[pkgEuroscope.EventType, *euroscope.Client](cfg.euroscopeHub, cfg.authService)

	mux.HandleFunc("/healthz", healthz(cfg.amanRuntime, cfg.standAssignmentReadiness, cfg.vatsimSource, cfg.standAssignmentStaleAfter))
	mux.HandleFunc("/euroscopeEvents", euroscopeUpgrader.Upgrade)
	mux.HandleFunc("/frontEndEvents", frontendUpgrader.Upgrade)
	if cfg.enableALB {
		mux.HandleFunc("/albEvents", cfg.albHub.Upgrade)
	}

	apiMux := http.NewServeMux()
	standstatus.NewWebAPI(standstatus.WebAPIConfig{
		Auth: cfg.authService, Sessions: cfg.sessionRepo, Assignments: cfg.standAssignmentRepo,
		Feed: cfg.vatsimSource, Enabled: cfg.standAssignmentReadiness.Enabled,
		Ready: cfg.standAssignmentReadiness.Ready, Reason: cfg.standAssignmentReadiness.Reason,
		StaleAfter: cfg.standAssignmentStaleAfter, Diagnostics: cfg.standAssignmentDiagnostics,
		Failures: cfg.standAssignmentFailures,
	}).RegisterRoutes(apiMux)
	if cfg.enableCDMAPI {
		cdm.NewWebAPI(cfg.authService, cfg.sessionRepo, cfg.sequenceService).RegisterRoutes(apiMux)
	}
	if cfg.enableECFMPAPI {
		ecfmpWebAPI.NewWebAPI(cfg.ecfmpService).RegisterRoutes(apiMux)
	}
	if cfg.enablePilotAPI {
		flightLookup := pdc.NewFlightLookupAdapter(cfg.pdcService, cfg.sessionRepo)
		pilot.NewWebAPI(cfg.authService, cfg.vatsimSource, flightLookup, cfg.requireLiveCIDVerification).RegisterRoutes(apiMux)
	}
	if cfg.enableEFBAPI && cfg.efbAPI != nil {
		cfg.efbAPI.RegisterRoutes(apiMux)
	}
	if cfg.enablePDCAPI {
		pdc.NewWebAPI(cfg.authService, cfg.pdcService, cfg.vatsimSource, cfg.requireLiveCIDVerification).RegisterRoutes(apiMux)
	}
	if cfg.enableTestTools && cfg.testToolsAPI != nil {
		cfg.testToolsAPI.RegisterRoutes(apiMux)
	}
	mux.Handle("/api/", server.APIMiddleware(http.StripPrefix("/api", apiMux)))

	var handler http.Handler = mux
	if cfg.enableHTTPTracing {
		handler = otelhttp.NewHandler(handler, "http.server")
	}
	return handler
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

func healthz(amanRuntime *aman.Runtime, readiness appconfig.StandAssignmentReadiness, cache vatsim.SnapshotSource, staleAfter time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		result := healthResponse{Status: "ok", StandAssignment: satHealth{Enabled: readiness.Enabled, Ready: readiness.Ready, Status: "disabled"}, AMAN: evaluateAMANHealth(amanRuntime, cache, staleAfter)}
		sat := &result.StandAssignment
		switch {
		case !readiness.Enabled:
		case !readiness.Ready:
			result.Status, sat.Status, sat.Reason = "degraded", "invalid_config", readiness.Reason
		case cache == nil:
			result.Status, sat.Status, sat.Ready, sat.Reason = "degraded", "feed_unavailable", false, "VATSIM feed is unavailable"
		default:
			*sat = evaluateSATHealth(readiness, cache.Snapshot(), staleAfter)
			if !sat.Ready {
				result.Status = "degraded"
			}
		}
		if result.AMAN.Enabled && !result.AMAN.Ready {
			result.Status = "degraded"
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // SAT degradation must not take unrelated features down.
		_ = json.NewEncoder(w).Encode(result)
	}
}

func evaluateAMANHealth(runtime *aman.Runtime, cache vatsim.SnapshotSource, staleAfter time.Duration) aman.TechnicalHealth {
	report := runtime.Health(context.Background())
	if !report.Enabled {
		return report
	}
	report.VATSIM = amanVATSIMHealth(cache, staleAfter)
	return aman.EvaluateTechnicalHealth(report.Mode, report.VATSIM, report.Navigation, report.Weather, report.Repository, report.Predictor, report.ReplayValidation)
}

func amanVATSIMHealth(cache vatsim.SnapshotSource, staleAfter time.Duration) aman.ComponentHealth {
	if cache == nil {
		return aman.ComponentHealth{Status: aman.HealthUnavailable, Reason: "feed_unavailable"}
	}
	snapshot := cache.Snapshot()
	age := time.Since(snapshot.Timestamp)
	if age < 0 {
		age = 0
	}
	ageSeconds := age.Seconds()
	result := aman.ComponentHealth{Status: aman.HealthReady, UpdatedAt: optionalHealthTime(snapshot.Timestamp), AgeSeconds: &ageSeconds}
	switch {
	case snapshot.Timestamp.IsZero():
		result.Status, result.Reason = aman.HealthUnavailable, "snapshot_unavailable"
	case snapshot.LastRefreshError != nil:
		result.Status, result.Reason = aman.HealthUnavailable, "refresh_failed"
	case age > staleAfter:
		result.Status, result.Reason = aman.HealthDegraded, "snapshot_stale"
	}
	return result
}

func optionalHealthTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	value = value.UTC()
	return &value
}

func evaluateSATHealth(readiness appconfig.StandAssignmentReadiness, snapshot vatsim.Snapshot, staleAfter time.Duration) satHealth {
	result := satHealth{Enabled: readiness.Enabled, Ready: readiness.Ready, Status: "ready"}
	ageDuration := time.Since(snapshot.Timestamp)
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
	if cfg.AMAN.Mode == "" {
		cfg.AMAN.Mode = aman.ModeDisabled
	}
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
