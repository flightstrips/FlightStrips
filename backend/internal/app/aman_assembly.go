package app

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/operational"
	"FlightStrips/internal/aman/predictor/openmeteo"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/aman/terminal"
	appconfig "FlightStrips/internal/config"
	internalEuroscope "FlightStrips/internal/euroscope"
	internalFrontend "FlightStrips/internal/frontend"
	"FlightStrips/internal/models"
	"FlightStrips/internal/navigation"
	"FlightStrips/internal/repository/postgres"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	frontendEvents "FlightStrips/pkg/events/frontend"

	"github.com/jackc/pgx/v5/pgxpool"
)

type operationalAMANAssembly struct {
	dependencies aman.Dependencies
	commands     aman.CommandService
	transport    *amanTransport
}

type sessionLister interface {
	List(context.Context) ([]*models.Session, error)
}

type sessionArrivalRunwaySource struct{ sessions sessionLister }

func (s sessionArrivalRunwaySource) ActiveArrivalRunway(ctx context.Context, airport string) (string, error) {
	sessions, err := s.sessions.List(ctx)
	if err != nil {
		return "", err
	}
	active := make(map[string]struct{})
	for _, session := range sessions {
		if session == nil || !strings.EqualFold(strings.TrimSpace(session.Airport), airport) {
			continue
		}
		for _, runway := range session.ActiveRunways.ArrivalRunways {
			runway = strings.ToUpper(strings.TrimSpace(runway))
			if runway != "" {
				active[runway] = struct{}{}
			}
		}
	}
	if len(active) == 0 {
		return "", fmt.Errorf("no active arrival runway in session")
	}
	if len(active) > 1 {
		return "", fmt.Errorf("multiple active arrival runways in session")
	}
	for runway := range active {
		return runway, nil
	}
	return "", fmt.Errorf("no active arrival runway in session")
}

type amanTransport struct {
	repository      aman.AirportStateReader
	mode            aman.RolloutMode
	health          aman.TechnicalHealthReporter
	gainLossEnabled bool

	mu           sync.RWMutex
	frontendHub  *internalFrontend.Hub
	euroscopeHub *internalEuroscope.Hub
	// lastGainLossAuthority tracks the projected transport authority rather
	// than the persisted aggregate flag, which can outlive a health change.
	lastGainLossAuthority map[string]bool
}

func (*amanTransport) Name() string { return "AMAN frontend state publisher" }

func (p *amanTransport) setHubs(frontendHub *internalFrontend.Hub, euroscopeHub *internalEuroscope.Hub) {
	p.mu.Lock()
	p.frontendHub = frontendHub
	if p.gainLossEnabled {
		p.euroscopeHub = euroscopeHub
	} else {
		p.euroscopeHub = nil
	}
	p.mu.Unlock()
}

func (p *amanTransport) CurrentAMANState(ctx context.Context, airport string) (frontendEvents.AMANStateEvent, error) {
	state, err := p.repository.LoadAirportState(ctx, airport)
	if err != nil {
		return frontendEvents.AMANStateEvent{}, err
	}
	health := p.health.TechnicalHealth(ctx)
	return frontendEvents.NewAMANStateEvent(state, health.EffectiveMode, health)
}

func (p *amanTransport) CurrentAMANGainLoss(ctx context.Context, airport string) (euroscopeEvents.AMANGainLossEvent, error) {
	state, err := p.repository.LoadAirportState(ctx, airport)
	if err != nil {
		return euroscopeEvents.AMANGainLossEvent{}, err
	}
	event, err := p.newGainLossEvent(ctx, state)
	if err == nil {
		p.rememberGainLossAuthority(event)
	}
	return event, err
}

func (p *amanTransport) newGainLossEvent(ctx context.Context, state aman.AirportState) (euroscopeEvents.AMANGainLossEvent, error) {
	event, err := euroscopeEvents.NewAMANGainLossEvent(state)
	if err != nil {
		return euroscopeEvents.AMANGainLossEvent{}, err
	}
	// Persisted authority describes the state when it was committed. Transport
	// consumers must also observe the current technical authority gate.
	event.Authoritative = event.Authoritative && p.health.TechnicalHealth(ctx).AuthorityAllowed
	return event, nil
}

func (p *amanTransport) PublishAMANState(ctx context.Context, state aman.AirportState) error {
	health := p.health.TechnicalHealth(ctx)
	event, err := frontendEvents.NewAMANStateEvent(state, health.EffectiveMode, health)
	if err != nil {
		return err
	}
	gainLoss, err := p.newGainLossEvent(ctx, state)
	if err != nil {
		return err
	}
	p.mu.RLock()
	frontendHub := p.frontendHub
	euroscopeHub := p.euroscopeHub
	p.mu.RUnlock()
	if frontendHub != nil {
		frontendHub.PublishAMANStateEvent(event)
	}
	if euroscopeHub != nil {
		p.rememberGainLossAuthority(gainLoss)
		euroscopeHub.PublishAMANGainLoss(gainLoss)
	}
	return nil
}

func (p *amanTransport) rememberGainLossAuthority(event euroscopeEvents.AMANGainLossEvent) {
	p.mu.Lock()
	if p.lastGainLossAuthority == nil {
		p.lastGainLossAuthority = map[string]bool{}
	}
	p.lastGainLossAuthority[event.Airport] = event.Authoritative
	p.mu.Unlock()
}

// PublishAMANAuthority is called on otherwise unchanged reconciliation ticks.
// It emits only when current technical health changes the authority projected
// to EuroScope, retaining the aggregate revision and payload.
func (p *amanTransport) PublishAMANAuthority(ctx context.Context, state aman.AirportState) error {
	if !p.gainLossEnabled {
		return nil
	}
	event, err := p.newGainLossEvent(ctx, state)
	if err != nil {
		return err
	}
	p.mu.Lock()
	if p.lastGainLossAuthority == nil {
		p.lastGainLossAuthority = map[string]bool{}
	}
	previous, known := p.lastGainLossAuthority[event.Airport]
	p.lastGainLossAuthority[event.Airport] = event.Authoritative
	hub := p.euroscopeHub
	p.mu.Unlock()
	if hub != nil && (!known || previous != event.Authoritative) {
		hub.PublishAMANGainLoss(event)
	}
	return nil
}

func assembleOperationalAMAN(config aman.RuntimeConfig, source *navigation.Source, pool *pgxpool.Pool) (operationalAMANAssembly, error) {
	if source == nil {
		return operationalAMANAssembly{}, fmt.Errorf("AMAN requires an enabled navigation source")
	}
	terminalConfig := source.Terminal
	if err := terminalConfig.ValidateOperationalSettings(); err != nil {
		return operationalAMANAssembly{}, fmt.Errorf("validate AMAN terminal operational settings: %w", err)
	}
	if err := validateTerminalAirportCoverage(terminalConfig, config.EnabledAirports); err != nil {
		return operationalAMANAssembly{}, err
	}
	amanRepository := postgres.NewAMANRepository(pool)
	transport := &amanTransport{
		repository:      amanRepository,
		mode:            config.Mode,
		gainLossEnabled: config.EnableEuroScopeGainLoseTags,
	}
	aircraftEngines, err := appconfig.LoadAMANAircraftEngineReference()
	if err != nil {
		return operationalAMANAssembly{}, err
	}
	service, err := operational.New(operational.Dependencies{
		Repository: amanRepository, Retirer: amanRepository, Materializer: source, Geometry: source.Geometry, Wind: openmeteo.New(openmeteo.Config{Cache: postgres.NewAMANWeatherCache(pool)}),
		Runways: sessionArrivalRunwaySource{sessions: postgres.NewSessionRepository(pool)}, AircraftEngines: aircraftEngines,
		Terminal: terminalConfig, Airports: config.EnabledAirports, Mode: config.Mode, Publisher: transport,
	})
	if err != nil {
		return operationalAMANAssembly{}, fmt.Errorf("initialize AMAN operational service: %w", err)
	}
	transport.health = service
	coordinator, err := sequence.NewCoordinator(sequence.CoordinatorDependencies{
		States: amanRepository, Outcomes: amanRepository, Committer: amanRepository, Publisher: transport,
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
			Repositories: amanRepository, NavigationMaterializer: source, NavigationReader: source.Geometry,
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
var _ internalEuroscope.AMANGainLossProvider = (*amanTransport)(nil)
var _ aman.Component = (*amanTransport)(nil)
