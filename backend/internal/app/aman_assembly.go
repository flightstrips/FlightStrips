package app

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/materializer"
	"FlightStrips/internal/aman/navdata/airacnet"
	"FlightStrips/internal/aman/operational"
	"FlightStrips/internal/aman/predictor/openmeteo"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/aman/terminal"
	internalFrontend "FlightStrips/internal/frontend"
	"FlightStrips/internal/repository/postgres"
	events "FlightStrips/pkg/events/frontend"

	"github.com/jackc/pgx/v5/pgxpool"
)

type operationalAMANAssembly struct {
	dependencies aman.Dependencies
	commands     aman.CommandService
	transport    *amanTransport
}

type amanTransport struct {
	repository aman.AirportStateReader
	mode       aman.RolloutMode
	health     aman.TechnicalHealthReporter

	mu  sync.RWMutex
	hub *internalFrontend.Hub
}

func (*amanTransport) Name() string { return "AMAN frontend state publisher" }

func (p *amanTransport) setHub(hub *internalFrontend.Hub) {
	p.mu.Lock()
	p.hub = hub
	p.mu.Unlock()
}

func (p *amanTransport) CurrentAMANState(ctx context.Context, airport string) (events.AMANStateEvent, error) {
	state, err := p.repository.LoadAirportState(ctx, airport)
	if err != nil {
		return events.AMANStateEvent{}, err
	}
	health := p.health.TechnicalHealth(ctx)
	return events.NewAMANStateEvent(state, health.EffectiveMode, health)
}

func (p *amanTransport) PublishAMANState(ctx context.Context, state aman.AirportState) error {
	health := p.health.TechnicalHealth(ctx)
	event, err := events.NewAMANStateEvent(state, health.EffectiveMode, health)
	if err != nil {
		return err
	}
	p.mu.RLock()
	hub := p.hub
	p.mu.RUnlock()
	if hub != nil {
		hub.PublishAMANStateEvent(event)
	}
	return nil
}

func assembleOperationalAMAN(config aman.RuntimeConfig, pool *pgxpool.Pool) (operationalAMANAssembly, error) {
	terminalConfig, err := terminal.LoadFile(config.TerminalGeometryPath)
	if err != nil {
		return operationalAMANAssembly{}, fmt.Errorf("load AMAN terminal configuration: %w", err)
	}
	if err := terminalConfig.ValidateOperationalSettings(); err != nil {
		return operationalAMANAssembly{}, fmt.Errorf("validate AMAN terminal operational settings: %w", err)
	}
	if err := validateTerminalAirportCoverage(terminalConfig, config.EnabledAirports); err != nil {
		return operationalAMANAssembly{}, err
	}
	repository := postgres.NewAMANRepository(pool)
	cache := postgres.NewNavigationCache(pool)
	source, err := airacnet.New(airacnet.Config{Checkpoints: airacnet.NewPostgresCheckpoints(pool)})
	if err != nil {
		return operationalAMANAssembly{}, fmt.Errorf("initialize AIRAC.NET adapter: %w", err)
	}
	navigation, err := materializer.New(materializer.Dependencies{
		Cycles: source, Airports: source, Runways: terminalConfig, Procedures: source, Fixes: source, Routes: source,
		Cache: cache, Terminal: terminalConfig,
	})
	if err != nil {
		return operationalAMANAssembly{}, fmt.Errorf("initialize AMAN navigation materializer: %w", err)
	}
	transport := &amanTransport{repository: repository, mode: config.Mode}
	service, err := operational.New(operational.Dependencies{
		Repository: repository, Retirer: repository, Materializer: navigation, Geometry: cache, Wind: openmeteo.New(openmeteo.Config{}),
		Terminal: terminalConfig, Airports: config.EnabledAirports, Mode: config.Mode, Publisher: transport,
	})
	if err != nil {
		return operationalAMANAssembly{}, fmt.Errorf("initialize AMAN operational service: %w", err)
	}
	transport.health = service
	coordinator, err := sequence.NewCoordinator(sequence.CoordinatorDependencies{
		States: repository, Outcomes: repository, Committer: repository, Publisher: transport,
	})
	if err != nil {
		return operationalAMANAssembly{}, fmt.Errorf("initialize AMAN sequence coordinator: %w", err)
	}
	actions, err := sequence.NewActionService(coordinator, service)
	if err != nil {
		return operationalAMANAssembly{}, fmt.Errorf("initialize AMAN action service: %w", err)
	}
	return operationalAMANAssembly{
		commands: actions, transport: transport,
		dependencies: aman.Dependencies{
			Repositories: repository, NavigationMaterializer: navigation, NavigationReader: cache,
			Predictor: service, StateEngine: service, SequenceService: actions, Publisher: transport,
			ValidationService: service, HealthService: service, ObservationSink: service, ReconciliationWorker: service,
		},
	}, nil
}

func validateTerminalAirportCoverage(terminalConfig terminal.Configuration, enabledAirports []string) error {
	if len(enabledAirports) != 1 || strings.ToUpper(strings.TrimSpace(enabledAirports[0])) != string(terminalConfig.Airport) {
		return fmt.Errorf("AMAN terminal configuration for %q requires exactly that enabled airport", terminalConfig.Airport)
	}
	return nil
}

var _ sequence.FullStatePublisher = (*amanTransport)(nil)
var _ internalFrontend.AMANStateProvider = (*amanTransport)(nil)
var _ aman.Component = (*amanTransport)(nil)
