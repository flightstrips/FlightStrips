package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/holdingclearance"
	"FlightStrips/internal/aman/navdata"
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
	"FlightStrips/internal/vatsim"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	frontendEvents "FlightStrips/pkg/events/frontend"

	"github.com/jackc/pgx/v5/pgxpool"
)

type operationalAMANAssembly struct {
	dependencies aman.Dependencies
	commands     aman.CommandService
	transport    *amanTransport
}

type holdingEATProjection struct {
	callsign string
	hold     string
	holdType string
	eat      string
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
	repository        aman.AirportStateReader
	geometry          navdata.GeometrySnapshotReader
	mode              aman.RolloutMode
	health            aman.TechnicalHealthReporter
	vatsimSource      vatsim.SnapshotSource
	vatsimStaleAfter  time.Duration
	now               func() time.Time
	gainLossEnabled   bool
	holdingEATEnabled bool

	mu           sync.RWMutex
	frontendHub  *internalFrontend.Hub
	euroscopeHub *internalEuroscope.Hub
	// lastGainLossAuthority tracks whether EuroScope has received the current
	// authoritative transport marker for each airport.
	lastGainLossAuthority map[string]bool
	// lastHoldingEAT tracks the last complete authoritative projection so a
	// later publication can explicitly withdraw values that disappeared.
	lastHoldingEAT map[string]map[string]holdingEATProjection
}

func (*amanTransport) Name() string { return "AMAN frontend state publisher" }

func (p *amanTransport) setHubs(frontendHub *internalFrontend.Hub, euroscopeHub *internalEuroscope.Hub) {
	p.mu.Lock()
	p.frontendHub = frontendHub
	// EuroScope outputs are independently controlled by backend rollout gates.
	if p.gainLossEnabled || p.holdingEATEnabled {
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
	health := p.currentTechnicalHealth(ctx)
	return p.newStateEvent(ctx, state, health)
}

func (p *amanTransport) currentTechnicalHealth(ctx context.Context) aman.TechnicalHealth {
	health := p.health.TechnicalHealth(ctx)
	if p.vatsimSource == nil {
		return health
	}
	now := time.Now
	if p.now != nil {
		now = p.now
	}
	health.VATSIM = amanVATSIMHealth(p.vatsimSource, p.vatsimStaleAfter, now)
	return aman.EvaluateTechnicalHealth(
		health.Mode, health.VATSIM, health.Navigation, health.Weather,
		health.Repository, health.Predictor, health.ReplayValidation,
	)
}

func (p *amanTransport) newStateEvent(ctx context.Context, state aman.AirportState, health aman.TechnicalHealth) (frontendEvents.AMANStateEvent, error) {
	return p.newStateEventWithGeometry(state, health, func() (navdata.ActiveGeometrySnapshot, error) {
		return p.geometry.ActiveGeometrySnapshot(ctx, navdata.AirportID(state.Airport))
	})
}

func (p *amanTransport) newStateEventWithGeometry(state aman.AirportState, health aman.TechnicalHealth, geometry func() (navdata.ActiveGeometrySnapshot, error)) (frontendEvents.AMANStateEvent, error) {
	event, err := frontendEvents.NewAMANStateEvent(state, health.EffectiveMode, health)
	if err != nil || p.geometry == nil {
		return event, err
	}
	snapshot, snapshotErr := geometry()
	if snapshotErr == nil {
		event.Data.TimelineConfig = frontendEvents.ProjectAMANTimelineConfig(snapshot.TerminalVersion, snapshot.TimelineMappings)
	}
	return event, nil
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

func (p *amanTransport) CurrentAMANHoldingEAT(ctx context.Context, airport string) ([]euroscopeEvents.HoldEvent, error) {
	if !p.holdingEATEnabled {
		return nil, nil
	}
	state, err := p.repository.LoadAirportState(ctx, airport)
	if err != nil {
		return nil, err
	}
	// Reconnect repair must replay the authoritative value even when the
	// backend already stores it: TopSky receives EAT as a transient command.
	return p.holdingEATEvents(ctx, state, false), nil
}

func (p *amanTransport) newHoldingEATEvents(ctx context.Context, state aman.AirportState) []euroscopeEvents.HoldEvent {
	return p.holdingEATEvents(ctx, state, true)
}

func (p *amanTransport) newHoldingEATPublication(ctx context.Context, state aman.AirportState) []euroscopeEvents.HoldEvent {
	geometry := sync.OnceValues(func() (navdata.ActiveGeometrySnapshot, error) {
		return p.geometry.ActiveGeometrySnapshot(ctx, navdata.AirportID(state.Airport))
	})
	return p.holdingEATPublicationWithGeometry(ctx, state, geometry)
}

func (p *amanTransport) holdingEATPublicationWithGeometry(ctx context.Context, state aman.AirportState, geometry func() (navdata.ActiveGeometrySnapshot, error)) []euroscopeEvents.HoldEvent {
	currentEvents := p.holdingEATEventsWithGeometry(ctx, state, false, geometry)
	requiredWrites := p.holdingEATEventsWithGeometry(ctx, state, true, geometry)
	requiresWrite := make(map[string]struct{}, len(requiredWrites))
	for _, event := range requiredWrites {
		requiresWrite[holdingEATKey(event.Callsign)] = struct{}{}
	}

	current := make(map[string]holdingEATProjection, len(currentEvents))
	order := make([]string, 0, len(currentEvents))
	for _, event := range currentEvents {
		key := holdingEATKey(event.Callsign)
		if _, exists := current[key]; !exists {
			order = append(order, key)
		}
		current[key] = holdingEATProjection{
			callsign: event.Callsign, hold: event.Hold, holdType: event.HoldType, eat: event.HoldEat,
		}
	}

	airport := strings.ToUpper(strings.TrimSpace(state.Airport))
	p.mu.Lock()
	previous := p.lastHoldingEAT[airport]
	updates := make([]euroscopeEvents.HoldEvent, 0, len(current)+len(previous))
	for _, key := range order {
		projection := current[key]
		prior, existed := previous[key]
		_, writeRequired := requiresWrite[key]
		if writeRequired || !existed || prior != projection {
			updates = append(updates, projection.event())
		}
	}

	withdrawn := make([]string, 0)
	for key := range previous {
		if _, exists := current[key]; !exists {
			withdrawn = append(withdrawn, key)
		}
	}
	sort.Strings(withdrawn)
	for _, key := range withdrawn {
		withdrawal := previous[key]
		withdrawal.eat = ""
		updates = append(updates, withdrawal.event())
	}
	if p.lastHoldingEAT == nil {
		p.lastHoldingEAT = make(map[string]map[string]holdingEATProjection)
	}
	p.lastHoldingEAT[airport] = current
	p.mu.Unlock()
	return updates
}

func holdingEATKey(callsign string) string {
	return strings.ToUpper(strings.TrimSpace(callsign))
}

func (p holdingEATProjection) event() euroscopeEvents.HoldEvent {
	return euroscopeEvents.HoldEvent{Callsign: p.callsign, Hold: p.hold, HoldType: p.holdType, HoldEat: p.eat}
}

func (p *amanTransport) holdingEATEvents(ctx context.Context, state aman.AirportState, suppressCurrent bool) []euroscopeEvents.HoldEvent {
	return p.holdingEATEventsWithGeometry(ctx, state, suppressCurrent, func() (navdata.ActiveGeometrySnapshot, error) {
		return p.geometry.ActiveGeometrySnapshot(ctx, navdata.AirportID(state.Airport))
	})
}

func (p *amanTransport) holdingEATEventsWithGeometry(ctx context.Context, state aman.AirportState, suppressCurrent bool, geometry func() (navdata.ActiveGeometrySnapshot, error)) []euroscopeEvents.HoldEvent {
	if !p.holdingEATEnabled || !state.Authoritative || !p.currentTechnicalHealth(ctx).AuthorityAllowed || p.geometry == nil {
		return nil
	}
	snapshot, err := geometry()
	if err != nil {
		return nil
	}
	holdingFixes := make(map[navdata.HoldingID]navdata.FixID, len(snapshot.Holdings))
	for _, holding := range snapshot.Holdings {
		holdingFixes[holding.ID] = holding.Fix
	}

	events := make([]euroscopeEvents.HoldEvent, 0)
	for _, flight := range state.Flights {
		clearance := flight.HoldingClearance
		prediction := flight.Prediction
		stack := flight.HoldingStack
		if clearance == nil || clearance.Hold == "" || clearance.HoldType != aman.HoldingClearanceEnroute ||
			prediction == nil || prediction.HoldingPlan == nil || stack == nil || !stack.Confirmed ||
			flight.SelectedHolding == nil || stack.HoldingID != *flight.SelectedHolding {
			continue
		}
		selectedFix, found := holdingFixes[navdata.HoldingID(*flight.SelectedHolding)]
		if !found || !strings.EqualFold(strings.TrimSpace(clearance.Hold), string(selectedFix)) {
			continue
		}

		eat := prediction.HoldingPlan.ApproachReleaseTime.UTC().Format("1504")
		if suppressCurrent && clearance.HoldEAT == eat {
			continue
		}
		events = append(events, euroscopeEvents.HoldEvent{
			Callsign: flight.CurrentCallsign,
			Hold:     clearance.Hold,
			HoldType: string(clearance.HoldType),
			HoldEat:  eat,
		})
	}
	return events
}

func (p *amanTransport) newGainLossEvent(_ context.Context, state aman.AirportState) (euroscopeEvents.AMANGainLossEvent, error) {
	return euroscopeEvents.NewAMANGainLossEvent(state)
}

func (p *amanTransport) PublishAMANState(ctx context.Context, state aman.AirportState) error {
	health := p.currentTechnicalHealth(ctx)
	// Both projections must use the same manifest, without loading all geometry
	// twice. Keep the cache scoped to this publication so activation stays fresh.
	geometry := sync.OnceValues(func() (navdata.ActiveGeometrySnapshot, error) {
		return p.geometry.ActiveGeometrySnapshot(ctx, navdata.AirportID(state.Airport))
	})
	event, err := p.newStateEventWithGeometry(state, health, geometry)
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
	if euroscopeHub != nil && p.gainLossEnabled {
		p.rememberGainLossAuthority(gainLoss)
		euroscopeHub.PublishAMANGainLoss(gainLoss)
	}
	if euroscopeHub != nil && p.holdingEATEnabled {
		euroscopeHub.PublishAMANHoldingEAT(state.Airport, p.holdingEATPublicationWithGeometry(ctx, state, geometry))
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
// It ensures EuroScope has received the authoritative marker and reevaluates
// holding EAT writeback without changing the aggregate revision or payload.
func (p *amanTransport) PublishAMANAuthority(ctx context.Context, state aman.AirportState) error {
	if !p.gainLossEnabled && !p.holdingEATEnabled {
		return nil
	}
	p.mu.RLock()
	hub := p.euroscopeHub
	p.mu.RUnlock()
	if p.gainLossEnabled {
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
		p.mu.Unlock()
		if hub != nil && (!known || previous != event.Authoritative) {
			hub.PublishAMANGainLoss(event)
		}
	}
	if hub != nil && p.holdingEATEnabled {
		hub.PublishAMANHoldingEAT(state.Airport, p.newHoldingEATPublication(ctx, state))
	}
	return nil
}

func assembleOperationalAMAN(config aman.RuntimeConfig, source *navigation.Source, vatsimSource vatsim.SnapshotSource, vatsimStaleAfter time.Duration, pool *pgxpool.Pool, now func() time.Time) (operationalAMANAssembly, error) {
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
		repository:        amanRepository,
		geometry:          source.Geometry,
		mode:              config.Mode,
		vatsimSource:      vatsimSource,
		vatsimStaleAfter:  vatsimStaleAfter,
		now:               now,
		gainLossEnabled:   config.EnableEuroScopeGainLoseTags,
		holdingEATEnabled: config.EnableHoldingEATWriteback,
	}
	aircraftEngines, err := appconfig.LoadAMANAircraftEngineReference()
	if err != nil {
		return operationalAMANAssembly{}, err
	}
	service, err := operational.New(operational.Dependencies{
		Repository: amanRepository, Retirer: amanRepository, Materializer: source, Geometry: source.Geometry, Wind: openmeteo.New(openmeteo.Config{Cache: postgres.NewAMANWeatherCache(pool)}),
		Runways: sessionArrivalRunwaySource{sessions: postgres.NewSessionRepository(pool)}, AircraftEngines: aircraftEngines,
		Terminal: terminalConfig, TMAVolumePath: terminal.DefaultEKCHTMAVolumePath,
		Airports: config.EnabledAirports, Mode: config.Mode, Publisher: transport, Now: now,
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
	holdingClearances, err := holdingclearance.New(holdingclearance.Dependencies{Repository: amanRepository, Publisher: transport})
	if err != nil {
		return operationalAMANAssembly{}, fmt.Errorf("initialize AMAN holding clearances: %w", err)
	}
	return operationalAMANAssembly{
		commands: actions, transport: transport,
		dependencies: aman.Dependencies{
			Repositories: amanRepository, NavigationMaterializer: source, NavigationReader: source.Geometry,
			Predictor: service, StateEngine: service, SequenceService: actions, Publisher: transport,
			ValidationService: service, HealthService: service, ObservationSink: service,
			HoldingClearanceSink: holdingClearances, ReconciliationWorker: service,
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
var _ internalEuroscope.AMANHoldingEATProvider = (*amanTransport)(nil)
var _ aman.Component = (*amanTransport)(nil)
