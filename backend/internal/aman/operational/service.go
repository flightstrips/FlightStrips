// Package operational assembles the pure AMAN components into the durable,
// minute-driven AMAN-CPH reconciliation owner.
package operational

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/prediction"
	"FlightStrips/internal/aman/predictor"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/aman/trajectory"
	"FlightStrips/internal/sat"
)

const (
	policyVersion              = "aman-cph-v1"
	modelVersion               = "aman-cph-teta-v3"
	routeResolverVersion       = "airacnet-route-v4"
	defaultArrivalRate         = uint32(20)
	ekchDefaultArrivalRate     = uint32(40)
	weatherRefreshEvery        = 30 * time.Minute
	queueOfferValidity         = 2 * time.Minute
	euroScopeSurveillanceFresh = 30 * time.Second
)

type NavigationMaterializer interface {
	MaterializeRoute(context.Context, navdata.RouteQuery, string) (navdata.RouteKey, error)
}

type GeometryCache interface {
	navdata.GeometryReader
	navdata.GeometrySnapshotReader
}

type Repository interface {
	aman.AirportStateReader
	aman.StateCommitter
}

// ActiveArrivalRunwaySource supplies the runway selected by the active airport
// session. The value is a runway identifier (for example "22L"), not an AMAN
// group identifier; the terminal configuration is the sole mapping authority.
type ActiveArrivalRunwaySource interface {
	ActiveArrivalRunway(context.Context, string) (string, error)
}

// AircraftEngineReference is the read-only TopSky ICAO aircraft lookup used
// to identify light piston traffic without trusting pilot-entered WTC data.
type AircraftEngineReference interface {
	Lookup(string) (sat.EngineType, bool)
	LookupWTC(string) (string, bool)
}

type Dependencies struct {
	Repository      Repository
	Retirer         aman.VATSIMFlightIdentityRetirer
	Materializer    NavigationMaterializer
	Geometry        GeometryCache
	Wind            predictor.WindProfileReader
	Runways         ActiveArrivalRunwaySource
	AircraftEngines AircraftEngineReference
	Terminal        terminal.Configuration
	Airports        []string
	Mode            aman.RolloutMode
	Publisher       sequence.FullStatePublisher
	Now             func() time.Time
}

type Service struct {
	deps Dependencies

	mu                 sync.Mutex
	observed           map[string]map[aman.FlightID]aman.FlightObservation
	lastWeatherRefresh map[string]time.Time
	health             serviceHealth
}

type serviceHealth struct {
	vatsim, navigation, weather, repository, predictor, replay aman.ComponentHealth
}

func New(deps Dependencies) (*Service, error) {
	if deps.Repository == nil || deps.Materializer == nil || deps.Geometry == nil || deps.Wind == nil || deps.Publisher == nil {
		return nil, errors.New("AMAN operational service requires repository, navigation, wind, and publisher dependencies")
	}
	if deps.Terminal.Airport == "" || len(deps.Terminal.RunwayGroups) == 0 {
		return nil, errors.New("AMAN operational service requires terminal configuration")
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if len(deps.Airports) == 0 {
		return nil, errors.New("AMAN operational service requires enabled airports")
	}
	now := deps.Now().UTC()
	pending := func(reason string) aman.ComponentHealth {
		return componentHealth(aman.HealthUnavailable, reason, now)
	}
	return &Service{
		deps: deps, observed: map[string]map[aman.FlightID]aman.FlightObservation{}, lastWeatherRefresh: map[string]time.Time{},
		health: serviceHealth{
			vatsim: pending("source_not_observed"), navigation: pending("navigation_not_refreshed"),
			weather: pending("weather_not_observed"), repository: pending("repository_not_checked"),
			predictor: componentHealth(aman.HealthReady, "", now), replay: componentHealth(aman.HealthReady, "", now),
		},
	}, nil
}

func (*Service) Name() string { return "AMAN-CPH operational service" }

func (s *Service) TechnicalHealth(context.Context) aman.TechnicalHealth {
	s.mu.Lock()
	health := s.health
	s.mu.Unlock()
	return aman.EvaluateTechnicalHealth(s.deps.Mode, health.vatsim, health.navigation, health.weather, health.repository, health.predictor, health.replay)
}

func (s *Service) Observe(_ context.Context, observation aman.FlightObservation) error {
	if err := observation.Validate(); err != nil {
		return err
	}
	isEuroScopeSurveillance := observation.UsesEuroScopeSurveillance()
	airport := strings.ToUpper(strings.TrimSpace(observation.Destination))
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.observed[airport] == nil {
		s.observed[airport] = map[aman.FlightID]aman.FlightObservation{}
	}
	if previous, ok := s.observed[airport][observation.FlightID]; ok {
		observation = mergeSurveillanceObservation(previous, observation)
	}
	s.observed[airport][observation.FlightID] = observation
	if !isEuroScopeSurveillance {
		s.health.vatsim = sourceComponentHealth(observation.SourceStatus, observation.ReconciledAt)
	}
	return nil
}

// mergeSurveillanceObservation preserves a fresh EuroScope track while
// accepting every newer VATSIM flight-plan update. EuroScope observations are
// positional overlays only; they never replace route, aircraft, or planning
// facts received from VATSIM.
func mergeSurveillanceObservation(previous, incoming aman.FlightObservation) aman.FlightObservation {
	if incoming.UsesEuroScopeSurveillance() {
		merged := previous
		merged.FlightID, merged.VATSIMCID, merged.Callsign = incoming.FlightID, incoming.VATSIMCID, incoming.Callsign
		merged.Surveillance, merged.SurveillanceSource = incoming.Surveillance, aman.SurveillanceSourceEuroScope
		merged.ReconciledAt, merged.SourceStatus, merged.Missing = incoming.ReconciledAt, aman.DataFresh, false
		return merged
	}
	if freshEuroScopeSurveillance(previous, incoming.ReconciledAt) {
		incoming.Surveillance = previous.Surveillance
		incoming.SurveillanceSource = aman.SurveillanceSourceEuroScope
		incoming.SourceStatus, incoming.Missing = aman.DataFresh, false
	}
	return incoming
}

func freshEuroScopeSurveillance(observation aman.FlightObservation, at time.Time) bool {
	if !observation.UsesEuroScopeSurveillance() || observation.Surveillance == nil || observation.Surveillance.ObservedAt == nil {
		return false
	}
	return !at.After(observation.Surveillance.ObservedAt.Add(euroScopeSurveillanceFresh))
}

func (s *Service) ObserveSourceHealth(_ context.Context, status aman.DataStatus, observedAt time.Time) error {
	s.mu.Lock()
	s.health.vatsim = sourceComponentHealth(status, observedAt)
	s.mu.Unlock()
	return nil
}

func (s *Service) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	s.reconcileAll(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reconcileAll(ctx)
		}
	}
}

// Reconcile performs one complete AMAN reconciliation cycle. It is used by
// deterministic replay/validation tooling; the runtime worker calls the same
// internal operation on its configured interval.
func (s *Service) Reconcile(ctx context.Context) { s.reconcileAll(ctx) }

func (s *Service) reconcileAll(ctx context.Context) {
	for _, airport := range s.deps.Airports {
		airport = strings.ToUpper(strings.TrimSpace(airport))
		s.observeNavigationCache(ctx, airport)
		s.refreshWeather(ctx, airport, s.deps.Now().UTC().Truncate(time.Second))
		if err := s.reconcileAirport(ctx, airport); err != nil {
			slog.WarnContext(ctx, "AMAN reconciliation failed", "airport", airport, "error", err)
		}
	}
}

// refreshWeather observes the airport wind profile independently of the
// current arrival set. Waiting for an aircraft with resolvable geometry left
// weather permanently "not observed" during quiet or degraded operations.
func (s *Service) refreshWeather(ctx context.Context, airport string, now time.Time) {
	s.mu.Lock()
	last := s.lastWeatherRefresh[airport]
	if !last.IsZero() && now.Sub(last) < weatherRefreshEvery {
		s.mu.Unlock()
		return
	}
	s.lastWeatherRefresh[airport] = now
	s.mu.Unlock()

	request, err := weatherProbeRequest(s.deps.Terminal, airport, now)
	if err != nil {
		s.setHealthComponent("weather", aman.HealthUnavailable, "weather_probe_location_unconfigured", now)
		return
	}
	profile, err := s.deps.Wind.WindProfile(ctx, request)
	if err != nil {
		s.setHealthComponent("weather", aman.HealthUnavailable, "weather_refresh_failed", now)
		return
	}
	if !observedWeatherProfile(profile, request, now) {
		s.setHealthComponent("weather", aman.HealthUnavailable, "weather_profile_invalid", now)
		return
	}
	s.setHealthComponent("weather", aman.HealthReady, "", now)
}

func weatherProbeRequest(config terminal.Configuration, airport string, at time.Time) (predictor.WindProfileRequest, error) {
	if navdata.AirportID(strings.ToUpper(strings.TrimSpace(airport))) != config.Airport {
		return predictor.WindProfileRequest{}, errors.New("weather probe airport does not match terminal configuration")
	}
	for _, group := range config.RunwayGroups {
		for _, approach := range group.FinalApproaches {
			position := approach.Threshold.Position
			if position.LatitudeDeg < -90 || position.LatitudeDeg > 90 || position.LongitudeDeg < -180 || position.LongitudeDeg > 180 || (position.LatitudeDeg == 0 && position.LongitudeDeg == 0) {
				continue
			}
			return predictor.WindProfileRequest{Samples: []predictor.WindSampleRequest{{
				Position: predictor.WindCoordinate{LatitudeDegrees: position.LatitudeDeg, LongitudeDegrees: position.LongitudeDeg},
				At:       at.UTC(), AltitudeFeet: 10000,
			}}}, nil
		}
	}
	return predictor.WindProfileRequest{}, errors.New("terminal configuration has no weather probe coordinate")
}

func observedWeatherProfile(profile predictor.WindProfile, request predictor.WindProfileRequest, now time.Time) bool {
	if strings.TrimSpace(profile.SourceID) == "" || profile.ObservedAt.IsZero() || profile.ExpiresAt.IsZero() || profile.ObservedAt.After(now) || profile.ExpiresAt.Before(now) || len(profile.Samples) != len(request.Samples) {
		return false
	}
	for index, sample := range profile.Samples {
		want := request.Samples[index]
		if sample.At.UTC() != want.At.UTC() || sample.Position != want.Position || len(sample.Levels) == 0 {
			return false
		}
	}
	return true
}

func (s *Service) observeNavigationCache(ctx context.Context, airport string) {
	updatedAt := s.deps.Now().UTC()
	_, err := s.deps.Geometry.ActiveGeometrySnapshot(ctx, navdata.AirportID(airport))
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.health.navigation = componentHealth(aman.HealthUnavailable, "navigation_cache_unavailable", updatedAt)
		return
	}
	s.health.navigation = componentHealth(aman.HealthReady, "", updatedAt)
}

func (s *Service) reconcileAirport(ctx context.Context, airport string) error {
	now := s.deps.Now().UTC().Truncate(time.Second)
	initializing := false
	current, err := s.deps.Repository.LoadAirportState(ctx, airport)
	if err != nil {
		var domain *aman.DomainError
		if !errors.As(err, &domain) || domain.Class != aman.ErrorNotFound {
			s.setHealthComponent("repository", aman.HealthUnavailable, "repository_load_failed", now)
			return err
		}
		s.setHealthComponent("repository", aman.HealthReady, "", now)
		current = s.initialState(airport, now)
		initializing = true
	} else {
		s.setHealthComponent("repository", aman.HealthReady, "", now)
	}
	next := current
	next.Flights = slices.Clone(current.Flights)
	next.RunwayGroups = slices.Clone(current.RunwayGroups)
	if !runwayGroupsMatchTerminal(next.RunwayGroups, s.deps.Terminal.RunwayGroups) {
		next.RunwayGroups = s.initialState(airport, now).RunwayGroups
		resetFlightsForRunwayConfiguration(&next)
	}
	if s.deps.Runways != nil {
		selectedGroup, selectionChanged, selectionErr := s.selectSessionRunwayGroup(ctx, airport, next.RunwayGroups)
		if selectionErr != nil {
			return selectionErr
		}
		if selectionChanged {
			reassignFlightsToGroup(&next, selectedGroup)
		}
	} else {
		selectedGroup, selectionChanged := updateSelectedRunwayGroup(next.RunwayGroups, now)
		if selectionChanged {
			reassignFlightsToGroup(&next, selectedGroup)
		}
	}
	updateActiveRates(next.RunwayGroups, now)
	observations := s.observations(airport)
	indexes := make(map[aman.FlightID]int, len(next.Flights))
	for i := range next.Flights {
		indexes[next.Flights[i].ID] = i
	}

	for _, observation := range observations {
		index, found := indexes[observation.FlightID]
		if !found {
			next.Flights = append(next.Flights, newFlight(observation, now))
			index = len(next.Flights) - 1
			indexes[observation.FlightID] = index
		}
		flight := next.Flights[index]
		updated, updateErr := s.reconcileFlight(ctx, next, flight, observation, now)
		if updateErr != nil {
			slog.WarnContext(ctx, "AMAN flight prediction degraded", "airport", airport, "flight_id", observation.FlightID, "error", updateErr)
			updated = applyUnavailablePrediction(updated, observation, now, updateErr)
		}
		next.Flights[index] = updated
	}
	for i := range next.Flights {
		if next.Flights[i].State == aman.StateRemoved || next.Flights[i].LatestObservation == nil {
			continue
		}
		observation, seen := observations[next.Flights[i].ID]
		if seen && !observation.Missing {
			continue
		}
		markMissing(&next.Flights[i], now)
	}
	for i := range next.Flights {
		repairSuperstableFreeze(&next.Flights[i])
	}

	s.resequence(&next, now)
	if !initializing && statesEqual(current, next) {
		return nil
	}
	next.Revision = current.Revision + 1
	next.GeneratedAt = now
	for i := range next.Flights {
		if next.Flights[i].State == aman.StateLanded || next.Flights[i].State == aman.StateRemoved {
			next.Flights[i].Slot = nil
			next.Flights[i].Order = nil
			next.Flights[i].ManualOrder = nil
		}
		if next.Flights[i].Slot != nil {
			next.Flights[i].Slot.Revision = next.Revision
		}
	}
	refreshHoldingPlans(&next)
	queueInput := s.sequenceInput(next)
	next, err = sequence.ProjectQueueOffers(next, queueInput, sequence.QueueOfferConfig{Validity: queueOfferValidity}, now)
	if err != nil {
		return fmt.Errorf("project AMAN queue offers: %w", err)
	}
	committed, err := s.deps.Repository.Commit(ctx, aman.StateCommit{ExpectedRevision: current.Revision, State: next})
	if err != nil {
		s.setHealthComponent("repository", aman.HealthUnavailable, "repository_commit_failed", now)
		return err
	}
	s.setHealthComponent("repository", aman.HealthReady, "", now)
	if s.deps.Retirer != nil {
		previous := make(map[aman.FlightID]aman.FlightState, len(current.Flights))
		for _, flight := range current.Flights {
			previous[flight.ID] = flight.State
		}
		for _, flight := range committed.State.Flights {
			if flight.State == aman.StateRemoved && previous[flight.ID] != aman.StateRemoved {
				if retireErr := s.deps.Retirer.RetireVATSIMFlight(context.WithoutCancel(ctx), flight.ID); retireErr != nil {
					slog.WarnContext(ctx, "retire removed AMAN VATSIM identity failed", "flight_id", flight.ID, "error", retireErr)
				}
			}
		}
	}
	return s.deps.Publisher.PublishAMANState(context.WithoutCancel(ctx), committed.State)
}

func (s *Service) observations(airport string) map[aman.FlightID]aman.FlightObservation {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[aman.FlightID]aman.FlightObservation, len(s.observed[airport]))
	for id, observation := range s.observed[airport] {
		result[id] = observation
	}
	return result
}

func (s *Service) initialState(airport string, now time.Time) aman.AirportState {
	rate := defaultRateForAirport(airport)
	groups := make([]aman.RunwayGroupPolicy, 0, len(s.deps.Terminal.RunwayGroups))
	for index, configured := range s.deps.Terminal.RunwayGroups {
		effective := now
		group := aman.RunwayGroupPolicy{
			ID: configured.ID, Selected: index == 0, ActiveRatePerHour: rate, RateEffectiveAt: &effective,
			RateSchedule: []aman.RunwayGroupRatePoint{{EffectiveAt: effective, ArrivalsPerHour: rate}},
		}
		if index == 0 {
			group.SelectionSchedule = []aman.RunwayGroupSelectionPoint{{EffectiveAt: effective}}
		}
		if spacing := configured.SameSTARSpacing; spacing != nil {
			group.SameSTARSpacing = &aman.SameSTARSpacingPolicy{Enabled: spacing.Enabled, ActivationRatePerHour: spacing.ActivationRatePerHour, MinimumEmptySlots: spacing.MinimumEmptySlots}
		}
		groups = append(groups, group)
	}
	return aman.AirportState{
		Airport: airport, GeneratedAt: now, PolicyVersion: policyVersion, Mode: s.deps.Mode,
		Authoritative: s.deps.Mode == aman.ModeAuthoritative, Flights: []aman.AMANFlight{}, RunwayGroups: groups,
	}
}

func defaultRateForAirport(airport string) uint32 {
	if strings.EqualFold(strings.TrimSpace(airport), "EKCH") {
		return ekchDefaultArrivalRate
	}
	return defaultArrivalRate
}

func newFlight(observation aman.FlightObservation, now time.Time) aman.AMANFlight {
	state := aman.StatePlanned
	if observation.TakeoffDetected != nil {
		state = aman.StateAirborne
	}
	copy := observation
	return aman.AMANFlight{
		ID: observation.FlightID, VATSIMCID: observation.VATSIMCID, CurrentCallsign: observation.Callsign,
		State: state, DataStatus: observation.SourceStatus, LatestObservation: &copy,
		FreezeReason: aman.FreezeNone, UpdatedAt: now,
	}
}

func (s *Service) reconcileFlight(ctx context.Context, state aman.AirportState, flight aman.AMANFlight, observation aman.FlightObservation, now time.Time) (aman.AMANFlight, error) {
	previousObservation := flight.LatestObservation
	copy := observation
	flight.LatestObservation = &copy
	flight.VATSIMCID, flight.CurrentCallsign, flight.DataStatus = observation.VATSIMCID, observation.Callsign, observation.SourceStatus
	flight.UpdatedAt = now
	if observation.Missing || observation.SourceStatus != aman.DataFresh {
		return flight, nil
	}
	flight.Lifecycle = clearAbsence(flight.Lifecycle)
	if groundedSurveillance(observation.Surveillance) {
		return applyGroundedObservation(flight, observation, now), nil
	}
	applyBaseline(&flight, observation, now)
	applyPreliminaryPrediction(&flight, observation, now)
	if observation.Surveillance == nil || observation.Surveillance.GroundspeedKnots == nil || observation.Surveillance.AltitudeFeet == nil || observation.FiledRoute == nil {
		if flight.State != aman.StatePlanned {
			markPredictionNonPublishable(&flight, missingEssentialReason(observation))
		}
		return flight, nil
	}
	if invalid := invalidEssentialReason(*observation.Surveillance); invalid != "" {
		if flight.State != aman.StatePlanned {
			markPredictionNonPublishable(&flight, invalid)
		}
		return flight, nil
	}
	group, ok := s.selectedGroup(flight, state.RunwayGroups)
	if !ok {
		return flight, fmt.Errorf("terminal has no configured runway group")
	}
	flight.SelectedRunwayGroup = &group
	feeder, ok := s.feeder(*observation.FiledRoute, group)
	if !ok {
		markUnknownSTARFamily(&flight, now)
		return flight, nil
	}
	flight.SelectedFeeder = stringPointer(string(feeder))
	version, err := s.deps.Geometry.ActiveVersion(ctx, navdata.AirportID(observation.Destination))
	if err != nil {
		return flight, err
	}
	query := navdata.RouteQuery{Version: version, Origin: navdata.AirportID(observation.Origin), Destination: navdata.AirportID(observation.Destination), FiledRoute: *observation.FiledRoute, RunwayGroup: &group}
	revision := revisionValue(observation.FlightPlan.Revision)
	projectionRevision := routeProjectionRevision(flight, revision)
	datasetID := navigationDatasetID(version)
	var key navdata.RouteKey
	if canReuseActiveRoute(flight, projectionRevision, group, datasetID) {
		key = navdata.RouteKey(*flight.ActiveRouteKey)
		if !hasResolvedRouteFixes(ctx, s.deps.Geometry, navdata.AirportID(observation.Destination), key) {
			key = ""
		}
	}
	if key == "" {
		key, err = s.deps.Materializer.MaterializeRoute(ctx, query, routeResolverVersion)
		if err != nil {
			return flight, err
		}
		activeKey := string(key)
		flight.ActiveRouteKey = &activeKey
		flight.ActiveRouteDatasetID = &datasetID
	}
	projection, err := trajectory.Project(ctx, trajectory.Readers{Geometry: s.deps.Geometry, Snapshot: s.deps.Geometry}, trajectory.Input{
		Airport: navdata.AirportID(observation.Destination), RouteKey: key, Feeder: feeder, RunwayGroup: group,
		FlightPlanRevision: projectionRevision, Observation: *observation.Surveillance, RouteFact: flight.ActiveRouteFact, Prior: flight.RouteProgress,
	}, trajectory.Config{ReferenceTime: now, MaxObservationAge: 2 * time.Minute})
	if err != nil {
		return flight, err
	}
	if projection.DistanceToGoNM == nil || len(projection.Remaining) == 0 || projection.Completeness == trajectory.Unresolved || projection.Completeness == trajectory.OffRoute {
		return flight, fmt.Errorf("route geometry is not publishable: %s", projection.Completeness)
	}
	descentConfirmed, descentEvidenceSamples := descentStateForRoute(flight.RouteProgress, previousObservation, observation, projection.InTMA)
	if projection.Progress != nil {
		projection.Progress.DescentConfirmed = descentConfirmed
		projection.Progress.DescentEvidenceSamples = descentEvidenceSamples
	}
	input := predictor.PerformanceWindInput{
		PredictionAt: now, AircraftICAO: stringValue(observation.AircraftType), WakeTurbulenceCategory: category(observation.WakeCategory),
		AltitudeFeet: float64(*observation.Surveillance.AltitudeFeet), CruiseAltitudeFeet: predictionCruiseAltitudeForRoute(observation, projection.InTMA, descentConfirmed), CurrentGroundspeedKnots: *observation.Surveillance.GroundspeedKnots,
		CurrentTrackTrueDegrees:         observation.Surveillance.TrackTrueDegrees,
		UseObservedGroundspeedBeforeTOD: useObservedGroundspeedForRoute(observation, projection.InTMA),
		DescentConfirmed:                descentConfirmed,
		Remaining:                       predictorLegs(projection.Remaining),
	}
	estimate, err := predictor.EstimatePerformanceWind(ctx, nil, s.deps.Wind, input, predictor.PerformanceWindConfig{})
	if err != nil {
		s.setHealthComponent("predictor", aman.HealthUnavailable, "prediction_failed", now)
		return flight, err
	}
	s.setHealthComponent("predictor", aman.HealthReady, "", now)
	confidence := estimate.Confidence
	degradations := slices.Clone(estimate.DegradationReasons)
	if fallback := offRouteFallbackReason(projection.Reasons); fallback != "" {
		confidence = aman.ConfidenceMedium
		degradations = append([]string{fallback}, degradations...)
	}
	rawRETA := estimate.RawRETA
	dtg := estimate.DistanceToGoNM
	raw := aman.Prediction{
		RawTETA: estimate.RawTETA, RawRETA: &rawRETA, GeneratedAt: now, InputObservedAt: observedAt(observation.Surveillance, now),
		Confidence: confidence, Publishable: true, DatasetVersion: version.Cycle, GeometryDigest: projection.GeometryDigest,
		DistanceToGoNM: &dtg, ModelVersion: estimate.ModelVersion, ConfigVersion: s.deps.Terminal.ConfigVersion,
		PerformanceProfileID: estimate.PerformanceProfileID, WeatherSource: estimate.WeatherSource,
		Sources: []string{"vatsim", "airacnet", "terminal-config:" + s.deps.Terminal.ConfigVersion},
		Calculation: &aman.PredictionCalculation{
			NoWindDuration: estimate.NoWindDuration,
			Duration:       estimate.Duration,
			Legs:           calculationLegs(projection.Remaining, estimate.NoWindLegDurations, estimate.LegDurations),
			Segments:       calculationSegments(estimate.Segments),
		},
	}
	if len(degradations) > 0 {
		reason := strings.Join(degradations, ",")
		raw.DegradationReason = &reason
	}
	if projection.SelectedHolding != nil {
		holding := string(projection.SelectedHolding.ID)
		flight.SelectedHolding = &holding
		raw.HoldingFixETA = holdingETA(now, estimate.LegDurations, projection.Remaining, projection.SelectedHolding.Fix)
	}
	flight.HoldingStack = updateHoldingStack(flight.HoldingStack, projection.HoldingCandidate, observedAt(observation.Surveillance, now))
	flight.RouteProgress = projection.Progress
	previousState := flight.State
	nextState := lifecycleState(flight, raw.RawTETA, now)
	reduced, err := prediction.Reduce(prediction.DefaultConfig(), flight, prediction.Input{
		Raw:                          raw,
		State:                        nextState,
		Slot:                         flight.Slot,
		FreezeForTMA:                 projection.InTMA,
		ReplacePreliminaryPrediction: isPreliminaryPrediction(flight.Prediction),
	})
	if err != nil {
		return flight, err
	}
	updated := reduced.Flight
	updateLifecycle(&updated, previousState, nextState, now)
	return updated, nil
}

func (s *Service) feeder(route string, runwayGroup aman.RunwayGroupID) (navdata.FeederID, bool) {
	tokens := strings.FieldsFunc(strings.ToUpper(route), func(r rune) bool {
		return !((r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
	})
	// Prefer an explicitly filed feeder or configured alias.
	for _, configured := range s.deps.Terminal.Feeders {
		for _, token := range tokens {
			if token == string(configured.ID) || slices.Contains(configured.Aliases, navdata.FeederID(token)) {
				return configured.ID, true
			}
		}
	}
	// A flight may join a STAR downstream of its named feeder. Walk the filed
	// route backwards and use the first waypoint that belongs uniquely to one
	// configured path for the active runway group. Shared merge fixes are
	// deliberately ignored instead of guessing a family.
	for tokenIndex := len(tokens) - 1; tokenIndex >= 0; tokenIndex-- {
		token := navdata.FixID(tokens[tokenIndex])
		matches := map[navdata.FeederID]struct{}{}
		for _, path := range s.deps.Terminal.Paths {
			if path.RunwayGroup == runwayGroup && slices.Contains(path.Fixes, token) {
				matches[path.Feeder] = struct{}{}
			}
		}
		if len(matches) == 1 {
			for feeder := range matches {
				return feeder, true
			}
		}
	}
	return "", false
}

func (s *Service) resequence(state *aman.AirportState, now time.Time) {
	defer refreshHoldingPlans(state)
	targets := releaseGainResequenceTargets(state)
	input := s.sequenceInput(*state)
	for index := range input.Flights {
		if _, target := targets[input.Flights[index].ID]; target {
			input.Flights[index].ProtectCurrentSlot = false
			input.Flights[index].ManualOrder = nil
			input.Flights[index].State = aman.StateUnstable
		}
	}
	if len(input.Flights) == 0 || len(input.Policies) == 0 {
		return
	}
	result, err := sequence.Generate(input)
	if err != nil || result.HasConflicts() {
		return
	}
	entries := make(map[aman.FlightID]sequence.CandidateEntry, len(result.Entries))
	for _, entry := range result.Entries {
		entries[entry.FlightID] = entry
	}
	for i := range state.Flights {
		entry, ok := entries[state.Flights[i].ID]
		if !ok {
			continue
		}
		state.Flights[i].Slot = &aman.Slot{Time: entry.Time, RunwayGroupID: entry.RunwayGroupID, Sequence: entry.Sequence, Revision: state.Revision, Reason: string(entry.Reason)}
		order := entry.Sequence
		state.Flights[i].Order = &order
		state.Flights[i].UpdatedAt = now
	}
}

// refreshHoldingPlans derives a holding/release plan from the immutable slot
// and the latest physical prediction. The physical prediction incorporates
// current surveillance altitude; a later raw ETA therefore reduces the hold
// rather than moving the committed slot. If the slot is no longer feasible,
// no holding plan is emitted and the existing slot remains for controller
// review or an authorized action.
func refreshHoldingPlans(state *aman.AirportState) {
	for index := range state.Flights {
		flight := &state.Flights[index]
		if flight.Prediction == nil {
			continue
		}
		prediction := *flight.Prediction
		prediction.HoldingPlan = holdingPlan(prediction, flight.Slot)
		flight.Prediction = &prediction
	}
}

func holdingPlan(prediction aman.Prediction, slot *aman.Slot) *aman.HoldingPlan {
	if !prediction.Publishable || slot == nil || prediction.HoldingFixETA == nil || !slot.Time.After(prediction.RawTETA) {
		return nil
	}
	postHoldingTransit := prediction.RawTETA.Sub(*prediction.HoldingFixETA)
	if postHoldingTransit <= 0 {
		return nil
	}
	release := slot.Time.Add(-postHoldingTransit)
	if !release.After(*prediction.HoldingFixETA) {
		return nil
	}
	return &aman.HoldingPlan{
		HoldingEntryTime:        *prediction.HoldingFixETA,
		ApproachReleaseTime:     release,
		ExpectedHoldingDuration: release.Sub(*prediction.HoldingFixETA),
		PostHoldingTransit:      postHoldingTransit,
	}
}

func (s *Service) sequenceInput(state aman.AirportState) sequence.Input {
	return sequenceInputWithAircraft(state, s.deps.Terminal, s.deps.AircraftEngines)
}

// sequenceInput is retained for package tests that do not need the optional
// TopSky aircraft classification source.
func sequenceInput(state aman.AirportState, config terminal.Configuration) sequence.Input {
	return sequenceInputWithAircraft(state, config, nil)
}

func sequenceInputWithAircraft(state aman.AirportState, config terminal.Configuration, aircraft AircraftEngineReference) sequence.Input {
	input := sequence.Input{Revision: state.Revision}
	configured := map[aman.RunwayGroupID]terminal.RunwayGroup{}
	for _, group := range config.RunwayGroups {
		configured[group.ID] = group
	}
	for _, group := range state.RunwayGroups {
		rates := sequenceRates(group)
		if len(rates) == 0 {
			continue
		}
		policy := sequence.Policy{
			RunwayGroupID: group.ID, Rates: rates,
			EarlyTolerance: 30 * time.Second, SeparationRules: amanCPHSeparations(), UnknownSeparation: 3 * time.Minute,
		}
		if spacing := group.SameSTARSpacing; spacing != nil {
			policy.SameSTARSpacing = sequence.SameSTARSpacing{Enabled: spacing.Enabled, ActivationRatePerHour: spacing.ActivationRatePerHour, MinimumEmptySlots: spacing.MinimumEmptySlots}
		} else if spacing := configured[group.ID].SameSTARSpacing; spacing != nil {
			policy.SameSTARSpacing = sequence.SameSTARSpacing{Enabled: spacing.Enabled, ActivationRatePerHour: spacing.ActivationRatePerHour, MinimumEmptySlots: spacing.MinimumEmptySlots}
		}
		input.Policies = append(input.Policies, policy)
	}
	for _, flight := range state.Flights {
		if flight.Prediction == nil || flight.SelectedRunwayGroup == nil || flight.State == aman.StatePlanned || flight.State == aman.StateLanded || flight.State == aman.StateRemoved ||
			(!flight.Prediction.Publishable && flight.FreezeReason == aman.FreezeNone) {
			continue
		}
		if flight.FreezeReason == aman.FreezeTMA && flight.Slot == nil {
			continue
		}
		if isLightPiston(flight, aircraft) && !flight.ManualSequenceIncluded {
			continue
		}
		wakeCategory := ""
		if flight.LatestObservation != nil {
			wakeCategory = strings.ToUpper(stringValue(flight.LatestObservation.WakeCategory))
		}
		input.Flights = append(input.Flights, sequence.Flight{
			ID: flight.ID, RunwayGroupID: *flight.SelectedRunwayGroup, State: flight.State, OperationalTETA: flight.Prediction.OperationalTETA,
			WakeCategory: sequence.WakeCategory(wakeCategory), STARFamily: stringValue(flight.SelectedFeeder),
			ManualOrder:  flight.ManualOrder,
			FreezeReason: flight.FreezeReason, FrozenAt: flight.FrozenAt, FrozenOperationalTETA: flight.FrozenOperationalTETA,
			CapturedSlot: flight.FrozenSlot, CurrentSlot: flight.Slot,
			ProtectCurrentSlot: flight.State == aman.StateStable && flight.ManualOrder == nil && flight.Slot != nil && flight.FreezeReason == aman.FreezeNone,
			HoldingStackID:     holdingStackID(flight), HoldingAltitudeFeet: holdingStackAltitude(flight),
		})
	}
	return input
}

const holdingConfirmationObservations = uint32(2)

func updateHoldingStack(previous *aman.HoldingStackState, candidate *trajectory.HoldingCandidate, at time.Time) *aman.HoldingStackState {
	if candidate == nil {
		return nil
	}
	id := string(candidate.HoldingID)
	if previous != nil && previous.HoldingID == id && !at.After(previous.CandidateObservedAt) {
		copy := *previous
		return &copy
	}
	next := aman.HoldingStackState{HoldingID: id, CandidateObservedAt: at, ConsecutiveObservations: 1}
	if previous != nil && previous.HoldingID == id && at.Sub(previous.CandidateObservedAt) <= 3*time.Minute {
		next.ConsecutiveObservations = previous.ConsecutiveObservations + 1
	}
	next.Confirmed = next.ConsecutiveObservations >= holdingConfirmationObservations
	return &next
}

func holdingStackID(flight aman.AMANFlight) string {
	if flight.HoldingStack == nil || !flight.HoldingStack.Confirmed {
		return ""
	}
	return flight.HoldingStack.HoldingID
}

func holdingStackAltitude(flight aman.AMANFlight) *int {
	if holdingStackID(flight) == "" || flight.LatestObservation == nil || flight.LatestObservation.Surveillance == nil || flight.LatestObservation.Surveillance.AltitudeFeet == nil {
		return nil
	}
	altitude := *flight.LatestObservation.Surveillance.AltitudeFeet
	return &altitude
}

func isLightPiston(flight aman.AMANFlight, aircraft AircraftEngineReference) bool {
	if aircraft == nil || flight.LatestObservation == nil || flight.LatestObservation.AircraftType == nil {
		return false
	}
	aircraftType := stringValue(flight.LatestObservation.AircraftType)
	engine, engineKnown := aircraft.Lookup(aircraftType)
	wtc, wtcKnown := aircraft.LookupWTC(aircraftType)
	return engineKnown && wtcKnown && engine == sat.EnginePiston && strings.EqualFold(wtc, "L")
}

const gainResequenceThreshold = 4 * time.Minute

// releaseGainResequenceTargets authorizes the narrow automatic exception to
// committed-slot immutability. Stable slots around the target stay protected;
// a late target is reinserted at the earliest policy-valid open grid slot.
func releaseGainResequenceTargets(state *aman.AirportState) map[aman.FlightID]struct{} {
	targets := map[aman.FlightID]struct{}{}
	for index := range state.Flights {
		flight := &state.Flights[index]
		if flight.State != aman.StateStable || flight.Slot == nil || flight.Prediction == nil || flight.FreezeReason == aman.FreezeTMA {
			continue
		}
		reference := flight.Prediction.OperationalTETA
		if flight.FreezeReason == aman.FreezeSuperstable {
			reference = flight.Prediction.RawTETA
		}
		if reference.Sub(flight.Slot.Time) <= gainResequenceThreshold {
			continue
		}
		if flight.FreezeReason == aman.FreezeSuperstable {
			flight.FreezeReason, flight.FrozenAt, flight.FrozenOperationalTETA, flight.FrozenSlot = aman.FreezeNone, nil, nil, nil
			prediction := *flight.Prediction
			prediction.OperationalTETA = prediction.RawTETA
			prediction.OperationalReason = aman.OperationalReasonPredicted
			flight.Prediction = &prediction
		}
		flight.ManualOrder = nil
		targets[flight.ID] = struct{}{}
	}
	return targets
}

func sequenceRates(group aman.RunwayGroupPolicy) []sequence.RatePoint {
	if len(group.RateSchedule) > 0 {
		rates := make([]sequence.RatePoint, len(group.RateSchedule))
		for i, rate := range group.RateSchedule {
			rates[i] = sequence.RatePoint{EffectiveAt: rate.EffectiveAt, ArrivalsPerHour: rate.ArrivalsPerHour}
		}
		return rates
	}
	if group.ActiveRatePerHour == 0 || group.RateEffectiveAt == nil {
		return nil
	}
	return []sequence.RatePoint{{EffectiveAt: *group.RateEffectiveAt, ArrivalsPerHour: group.ActiveRatePerHour}}
}

func updateActiveRates(groups []aman.RunwayGroupPolicy, now time.Time) {
	for i := range groups {
		for _, rate := range groups[i].RateSchedule {
			if rate.EffectiveAt.After(now) {
				break
			}
			effective := rate.EffectiveAt
			groups[i].ActiveRatePerHour = rate.ArrivalsPerHour
			groups[i].RateEffectiveAt = &effective
		}
	}
}

func updateSelectedRunwayGroup(groups []aman.RunwayGroupPolicy, now time.Time) (aman.RunwayGroupID, bool) {
	if len(groups) == 0 {
		return "", false
	}
	selectedIndex := -1
	for index := range groups {
		if groups[index].Selected {
			selectedIndex = index
			break
		}
	}
	candidateIndex, candidatePoint := -1, aman.RunwayGroupSelectionPoint{}
	for index := range groups {
		for _, point := range groups[index].SelectionSchedule {
			if point.EffectiveAt.After(now) {
				break
			}
			if candidateIndex < 0 || point.EffectiveAt.After(candidatePoint.EffectiveAt) ||
				(point.EffectiveAt.Equal(candidatePoint.EffectiveAt) && point.CommandRevision > candidatePoint.CommandRevision) {
				candidateIndex, candidatePoint = index, point
			}
		}
	}
	if candidateIndex < 0 {
		if selectedIndex >= 0 {
			return groups[selectedIndex].ID, false
		}
		candidateIndex = 0
	}
	changed := selectedIndex != candidateIndex
	for index := range groups {
		groups[index].Selected = index == candidateIndex
	}
	return groups[candidateIndex].ID, changed
}

// selectSessionRunwayGroup makes the airport session's selected arrival
// runway authoritative for AMAN. It deliberately does not infer a runway from
// surveillance or a STAR: the terminal configuration maps the session runway
// to its exact, one-runway AMAN group.
func (s *Service) selectSessionRunwayGroup(ctx context.Context, airport string, groups []aman.RunwayGroupPolicy) (aman.RunwayGroupID, bool, error) {
	runway, err := s.deps.Runways.ActiveArrivalRunway(ctx, airport)
	if err != nil {
		return "", false, fmt.Errorf("read active arrival runway: %w", err)
	}
	runway = strings.ToUpper(strings.TrimSpace(runway))
	if runway == "" {
		return "", false, fmt.Errorf("active session has no selected arrival runway")
	}
	requested := aman.RunwayGroupID(runway)
	selection := s.deps.Terminal.ResolveRunwayGroup(terminal.SelectionInput{SessionRunwayGroup: &requested})
	if selection.RunwayGroup == nil {
		return "", false, fmt.Errorf("active arrival runway %q is not enabled for AMAN", runway)
	}
	for index := range groups {
		if groups[index].ID != *selection.RunwayGroup {
			continue
		}
		changed := !groups[index].Selected
		for groupIndex := range groups {
			groups[groupIndex].Selected = groupIndex == index
		}
		return groups[index].ID, changed, nil
	}
	return "", false, fmt.Errorf("active arrival runway %q maps to unavailable AMAN group %q", runway, *selection.RunwayGroup)
}

func amanCPHSeparations() []sequence.SeparationRule {
	categories := []sequence.WakeCategory{"L", "M", "H", "J"}
	rules := make([]sequence.SeparationRule, 0, len(categories)*len(categories))
	for _, leading := range categories {
		for _, trailing := range categories {
			gap := time.Duration(0)
			if leading == "H" {
				gap = 120 * time.Second
			}
			if leading == "J" || trailing == "L" {
				gap = 180 * time.Second
			}
			rules = append(rules, sequence.SeparationRule{Leading: leading, Trailing: trailing, Minimum: gap})
		}
	}
	return rules
}

func offRouteFallbackReason(reasons []string) string {
	for _, reason := range reasons {
		if strings.HasPrefix(reason, "OFF_ROUTE_NEXT_WAYPOINT:") {
			return strings.ToLower(reason)
		}
	}
	return ""
}

func (s *Service) selectedGroup(flight aman.AMANFlight, groups []aman.RunwayGroupPolicy) (aman.RunwayGroupID, bool) {
	if flight.SelectedRunwayGroup != nil && (flight.State == aman.StateStable || flight.FreezeReason != aman.FreezeNone) && containsRunwayGroup(groups, *flight.SelectedRunwayGroup) {
		return *flight.SelectedRunwayGroup, true
	}
	for _, group := range groups {
		if group.Selected {
			return group.ID, true
		}
	}
	if flight.SelectedRunwayGroup != nil {
		return *flight.SelectedRunwayGroup, true
	}
	if len(groups) > 0 {
		return groups[0].ID, true
	}
	if len(s.deps.Terminal.RunwayGroups) > 0 {
		return s.deps.Terminal.RunwayGroups[0].ID, true
	}
	return "", false
}

func runwayGroupsMatchTerminal(current []aman.RunwayGroupPolicy, configured []terminal.RunwayGroup) bool {
	if len(current) != len(configured) {
		return false
	}
	for index := range current {
		if current[index].ID != configured[index].ID {
			return false
		}
	}
	return true
}

func resetFlightsForRunwayConfiguration(state *aman.AirportState) {
	for index := range state.Flights {
		flight := &state.Flights[index]
		flight.SelectedRunwayGroup = nil
		flight.SelectedHolding = nil
		flight.HoldingStack = nil
		flight.ActiveRouteKey = nil
		flight.ActiveRouteDatasetID = nil
		flight.RouteProgress = nil
		clearSequencingState(flight)
	}
}

func containsRunwayGroup(groups []aman.RunwayGroupPolicy, want aman.RunwayGroupID) bool {
	return slices.ContainsFunc(groups, func(group aman.RunwayGroupPolicy) bool { return group.ID == want })
}

func applyBaseline(flight *aman.AMANFlight, observation aman.FlightObservation, now time.Time) {
	if observation.TakeoffDetected == nil || observation.PlannedTiming == nil || observation.PlannedTiming.EstimatedEnrouteTime == nil || flight.ArrivalBaseline != nil {
		return
	}
	arrival := observation.TakeoffDetected.Add(*observation.PlannedTiming.EstimatedEnrouteTime)
	observed := now
	if observation.FlightPlan.ObservedAt != nil {
		observed = *observation.FlightPlan.ObservedAt
	}
	flight.ArrivalBaseline = &aman.BaselineState{
		ArrivalAt: arrival, AirborneSensedAt: *observation.TakeoffDetected, Source: aman.BaselineSourceAirborneFiledEET,
		Confidence: aman.ConfidenceMedium, FlightPlanRevision: observation.FlightPlan.Revision, FlightPlanObservedAt: observed,
		ModelVersion: "aman-baseline-v1", ConfigVersion: "aman-baseline-defaults-v1",
	}
	if flight.State == aman.StatePlanned {
		flight.State = aman.StateAirborne
	}
}

func applyPreliminaryPrediction(flight *aman.AMANFlight, observation aman.FlightObservation, now time.Time) {
	if observation.PlannedTiming == nil || observation.PlannedTiming.EstimatedEnrouteTime == nil {
		return
	}
	if flight.Prediction != nil && flight.Prediction.ModelVersion == modelVersion {
		return
	}
	var arrival time.Time
	model := "aman-planned-eobt-exot-eet-v1"
	if observation.TakeoffDetected != nil {
		takeoff := *observation.TakeoffDetected
		if flight.ArrivalBaseline != nil {
			takeoff = flight.ArrivalBaseline.AirborneSensedAt
		}
		arrival = takeoff.Add(*observation.PlannedTiming.EstimatedEnrouteTime)
		model = "aman-airborne-takeoff-eet-v1"
	} else if observation.PlannedTiming.EstimatedOffBlockTime != nil {
		arrival = observation.PlannedTiming.EstimatedOffBlockTime.Add(predictor.DefaultEXOT).Add(*observation.PlannedTiming.EstimatedEnrouteTime)
	}
	if arrival.IsZero() || !arrival.After(now) {
		return
	}
	flight.Prediction = &aman.Prediction{
		RawTETA: arrival, OperationalTETA: arrival, OperationalReason: aman.OperationalReasonPredicted,
		GeneratedAt: now, InputObservedAt: now, Confidence: aman.ConfidenceMedium, Publishable: true,
		DatasetVersion: "flight-plan", GeometryDigest: "flight-plan", ModelVersion: model,
		ConfigVersion: policyVersion, Sources: []string{"vatsim"},
	}
}

func isPreliminaryPrediction(value *aman.Prediction) bool {
	if value == nil {
		return false
	}
	return strings.HasPrefix(value.ModelVersion, "aman-planned-") ||
		strings.HasPrefix(value.ModelVersion, "aman-airborne-")
}

func lifecycleState(flight aman.AMANFlight, teta, now time.Time) aman.FlightState {
	until := teta.Sub(now)
	switch flight.State {
	case aman.StatePlanned, aman.StateAirborne, aman.StateGoAround:
		if until <= 45*time.Minute {
			return aman.StateUnstable
		}
	case aman.StateUnstable:
		entered := flight.UpdatedAt
		if flight.Lifecycle != nil {
			entered = flight.Lifecycle.EnteredAt
		}
		if until <= 20*time.Minute && now.Sub(entered) >= 2*time.Minute {
			return aman.StateStable
		}
	}
	return flight.State
}

func updateLifecycle(flight *aman.AMANFlight, previousState, state aman.FlightState, now time.Time) {
	reason := aman.LifecycleReasonInitial
	entered := now
	if flight.Lifecycle != nil {
		reason, entered = flight.Lifecycle.Reason, flight.Lifecycle.EnteredAt
	}
	if previousState != state {
		entered = now
	}
	switch state {
	case aman.StateAirborne:
		reason = aman.LifecycleReasonAirborneDetected
	case aman.StateUnstable:
		reason = aman.LifecycleReasonUnstableHorizon
	case aman.StateStable:
		reason = aman.LifecycleReasonStableHorizon
	}
	flight.State = state
	flight.Lifecycle = &aman.LifecycleState{EnteredAt: entered, Reason: reason, LastEventID: fmt.Sprintf("tick-%d", now.UnixNano()), LastEventFingerprint: modelVersion, LastEventAt: now}
}

func markMissing(flight *aman.AMANFlight, now time.Time) {
	// A missing source record is not a valid arrival candidate. Release its
	// capacity immediately; the lifecycle timeout only controls when its
	// identity can be retired.
	clearSequencingState(flight)
	if flight.Lifecycle == nil {
		flight.Lifecycle = &aman.LifecycleState{EnteredAt: flight.UpdatedAt, Reason: aman.LifecycleReasonInitial, LastEventID: "missing", LastEventFingerprint: modelVersion, LastEventAt: now}
	}
	if flight.Lifecycle.Absence == nil {
		due := now.Add(time.Minute)
		flight.Lifecycle.Absence = &aman.AbsenceState{MissingSince: now, RemovalDueAt: &due}
	} else if flight.Lifecycle.Absence.RemovalDueAt != nil && !now.Before(*flight.Lifecycle.Absence.RemovalDueAt) {
		flight.State = aman.StateRemoved
		flight.Lifecycle.Reason = aman.LifecycleReasonSourceDisappearance
		flight.Lifecycle.EnteredAt = now
	}
	flight.Lifecycle.LastEventAt = now
	flight.UpdatedAt = now
}

func clearAbsence(value *aman.LifecycleState) *aman.LifecycleState {
	if value == nil {
		return nil
	}
	copy := *value
	copy.Absence = nil
	copy.ReconciliationPending = false
	return &copy
}

func applyUnavailablePrediction(flight aman.AMANFlight, observation aman.FlightObservation, now time.Time, cause error) aman.AMANFlight {
	copy := observation
	flight.LatestObservation, flight.DataStatus, flight.UpdatedAt = &copy, observation.SourceStatus, now
	markPredictionNonPublishable(&flight, cause.Error())
	return flight
}

func missingEssentialReason(observation aman.FlightObservation) string {
	missing := make([]string, 0, 3)
	if observation.Surveillance == nil {
		missing = append(missing, "surveillance")
	} else {
		if observation.Surveillance.GroundspeedKnots == nil {
			missing = append(missing, "groundspeed")
		}
		if observation.Surveillance.AltitudeFeet == nil {
			missing = append(missing, "altitude")
		}
	}
	if observation.FiledRoute == nil {
		missing = append(missing, "filed_route")
	}
	return "missing_essential_data:" + strings.Join(missing, ",")
}

const (
	groundedMaximumAltitudeFeet     = 1000
	groundedMaximumGroundspeedKnots = 50.0
)

func groundedSurveillance(surveillance *aman.SurveillanceFact) bool {
	return surveillance != nil && surveillance.AltitudeFeet != nil && surveillance.GroundspeedKnots != nil &&
		*surveillance.AltitudeFeet < groundedMaximumAltitudeFeet && *surveillance.GroundspeedKnots <= groundedMaximumGroundspeedKnots
}

// applyGroundedObservation treats low-and-slow surveillance as a lifecycle
// fact. Before the VATSIM movement classifier has observed takeoff, the
// flight remains planned; after that observation it has completed its AMAN
// arrival and must no longer participate in route prediction or sequencing.
func applyGroundedObservation(flight aman.AMANFlight, observation aman.FlightObservation, now time.Time) aman.AMANFlight {
	clearGroundedOperationalState(&flight)
	if observation.TakeoffDetected == nil {
		flight.State = aman.StatePlanned
		// A previous airborne prediction cannot be reused while the aircraft is
		// still on the ground. Rebuild the planned baseline from filed times.
		flight.Prediction = nil
		flight.ArrivalBaseline = nil
		applyPreliminaryPrediction(&flight, observation, now)
		setGroundedLifecycle(&flight, aman.LifecycleReasonInitial, now)
		return flight
	}
	flight.State = aman.StateLanded
	if flight.Prediction != nil {
		prediction := *flight.Prediction
		prediction.Publishable = false
		reason := "landed"
		prediction.DegradationReason = &reason
		flight.Prediction = &prediction
	}
	setGroundedLifecycle(&flight, aman.LifecycleReasonLandingConfirmed, now)
	return flight
}

func clearGroundedOperationalState(flight *aman.AMANFlight) {
	flight.SelectedFeeder, flight.SelectedHolding, flight.HoldingStack = nil, nil, nil
	flight.ActiveRouteKey, flight.ActiveRouteDatasetID, flight.RouteProgress = nil, nil, nil
	flight.Slot, flight.Order, flight.ManualOrder, flight.QueueOffers = nil, nil, nil, nil
	flight.FreezeReason, flight.FrozenAt, flight.FrozenOperationalTETA, flight.FrozenSlot = aman.FreezeNone, nil, nil, nil
}

// repairSuperstableFreeze recovers aggregates written before a captured slot
// was persisted atomically with a superstable freeze. A current slot is the
// only safe captured-slot value; without one, release the impossible freeze
// and let normal sequencing allocate a new slot.
func repairSuperstableFreeze(flight *aman.AMANFlight) {
	if flight.FreezeReason != aman.FreezeSuperstable {
		return
	}
	if flight.State == aman.StatePlanned || flight.State == aman.StateLanded || flight.State == aman.StateRemoved || flight.Slot == nil {
		flight.FreezeReason, flight.FrozenAt, flight.FrozenOperationalTETA, flight.FrozenSlot = aman.FreezeNone, nil, nil, nil
		flight.QueueOffers = nil
		return
	}
	if flight.FrozenSlot == nil {
		captured := *flight.Slot
		flight.FrozenSlot = &captured
	}
}

func setGroundedLifecycle(flight *aman.AMANFlight, reason aman.LifecycleReason, now time.Time) {
	enteredAt := now
	if flight.Lifecycle != nil && flight.Lifecycle.Reason == reason {
		enteredAt = flight.Lifecycle.EnteredAt
	}
	flight.Lifecycle = &aman.LifecycleState{
		EnteredAt: enteredAt, Reason: reason, LastEventID: fmt.Sprintf("grounded-%d", now.UnixNano()),
		LastEventFingerprint: "grounded-surveillance", LastEventAt: now,
	}
}

func invalidEssentialReason(surveillance aman.SurveillanceFact) string {
	if surveillance.GroundspeedKnots != nil && *surveillance.GroundspeedKnots <= 0 {
		return "invalid_essential_data:groundspeed"
	}
	if surveillance.AltitudeFeet != nil && *surveillance.AltitudeFeet < 0 {
		return "invalid_essential_data:altitude"
	}
	return ""
}

func markPredictionNonPublishable(flight *aman.AMANFlight, reason string) {
	if flight.Prediction == nil {
		return
	}
	prediction := *flight.Prediction
	prediction.Publishable = false
	prediction.DegradationReason = &reason
	flight.Prediction = &prediction
	clearSequencingState(flight)
}

func clearSequencingState(flight *aman.AMANFlight) {
	flight.Slot, flight.Order, flight.ManualOrder, flight.QueueOffers = nil, nil, nil, nil
	flight.FreezeReason, flight.FrozenAt, flight.FrozenOperationalTETA, flight.FrozenSlot = aman.FreezeNone, nil, nil, nil
	flight.HoldingStack = nil
}

func predictionCruiseAltitude(observation aman.FlightObservation) float64 {
	altitude := float64(*observation.Surveillance.AltitudeFeet)
	if observation.RequestedLevel == nil || *observation.RequestedLevel < *observation.Surveillance.AltitudeFeet+3_000 {
		return altitude
	}
	return float64(*observation.RequestedLevel)
}

func predictionCruiseAltitudeForRoute(observation aman.FlightObservation, inTMA, descentConfirmed bool) float64 {
	if inTMA || descentConfirmed {
		return float64(*observation.Surveillance.AltitudeFeet)
	}
	return predictionCruiseAltitude(observation)
}

const (
	descentEvidenceMinimumFeet = 100
	descentEvidenceRequired    = 2
	descentEvidenceMaximumAge  = 2 * time.Minute
)

// descentStateForRoute distinguishes an aircraft climbing toward filed cruise
// from one already descending toward the terminal. Once confirmed, descent is
// deliberately latched for the route so level-offs and speed-control climbs do
// not make the performance model climb back to the filed level.
func descentStateForRoute(progress *aman.RouteProgress, previous *aman.FlightObservation, current aman.FlightObservation, inTMA bool) (bool, uint8) {
	if inTMA {
		return true, descentEvidenceRequired
	}
	if progress != nil && progress.DescentConfirmed {
		return true, max(progress.DescentEvidenceSamples, uint8(descentEvidenceRequired))
	}
	samples := uint8(0)
	if progress != nil {
		samples = progress.DescentEvidenceSamples
	}
	if consecutiveDescentObservation(previous, current) {
		if samples < descentEvidenceRequired {
			samples++
		}
	} else {
		samples = 0
	}
	return samples >= descentEvidenceRequired, samples
}

func consecutiveDescentObservation(previous *aman.FlightObservation, current aman.FlightObservation) bool {
	if previous == nil || previous.Surveillance == nil || current.Surveillance == nil ||
		previous.Surveillance.AltitudeFeet == nil || current.Surveillance.AltitudeFeet == nil ||
		previous.Surveillance.ObservedAt == nil || current.Surveillance.ObservedAt == nil {
		return false
	}
	interval := current.Surveillance.ObservedAt.Sub(*previous.Surveillance.ObservedAt)
	if interval <= 0 || interval > descentEvidenceMaximumAge {
		return false
	}
	return *previous.Surveillance.AltitudeFeet-*current.Surveillance.AltitudeFeet >= descentEvidenceMinimumFeet
}

func useObservedGroundspeedBeforeTOD(observation aman.FlightObservation) bool {
	if observation.RequestedLevel == nil {
		return true
	}
	return *observation.Surveillance.AltitudeFeet >= *observation.RequestedLevel-3_000
}

func useObservedGroundspeedForRoute(observation aman.FlightObservation, inTMA bool) bool {
	return inTMA || useObservedGroundspeedBeforeTOD(observation)
}

func markUnknownSTARFamily(flight *aman.AMANFlight, now time.Time) {
	flight.SelectedFeeder = nil
	flight.SelectedHolding = nil
	flight.HoldingStack = nil
	flight.ActiveRouteKey = nil
	flight.ActiveRouteDatasetID = nil
	flight.RouteProgress = nil
	if flight.Prediction == nil {
		return
	}
	prediction := *flight.Prediction
	reason := string(sequence.WarningUnknownSTARFamily)
	prediction.DegradationReason = &reason
	if strings.HasPrefix(prediction.ModelVersion, "aman-planned-") || strings.HasPrefix(prediction.ModelVersion, "aman-airborne-") {
		prediction.Publishable = true
	}
	if prediction.Confidence == aman.ConfidenceHigh {
		prediction.Confidence = aman.ConfidenceMedium
	}
	flight.Prediction = &prediction
	previousState := flight.State
	nextState := lifecycleState(*flight, prediction.OperationalTETA, now)
	updateLifecycle(flight, previousState, nextState, now)
}

func predictorLegs(legs []trajectory.RemainingLeg) []predictor.RouteLeg {
	result := make([]predictor.RouteLeg, len(legs))
	for i, leg := range legs {
		result[i] = predictor.RouteLeg{ID: leg.ID, DistanceNM: leg.DistanceNM, CourseTrueDegrees: leg.CourseTrueDegrees, Start: predictor.WindCoordinate{LatitudeDegrees: leg.Start.LatitudeDeg, LongitudeDegrees: leg.Start.LongitudeDeg}, End: predictor.WindCoordinate{LatitudeDegrees: leg.End.LatitudeDeg, LongitudeDegrees: leg.End.LongitudeDeg}}
	}
	return result
}

func calculationLegs(legs []trajectory.RemainingLeg, noWindDurations, durations []time.Duration) []aman.PredictionLeg {
	if len(legs) != len(durations) || len(legs) != len(noWindDurations) {
		return nil
	}
	result := make([]aman.PredictionLeg, len(legs))
	for i, leg := range legs {
		from := string(leg.From)
		if from == "" {
			from = "CURRENT_POSITION"
		}
		result[i] = aman.PredictionLeg{
			ID: leg.ID, From: from, To: string(leg.To),
			StartLatitude: leg.Start.LatitudeDeg, StartLongitude: leg.Start.LongitudeDeg,
			EndLatitude: leg.End.LatitudeDeg, EndLongitude: leg.End.LongitudeDeg,
			DistanceNM: leg.DistanceNM, CourseTrueDegrees: leg.CourseTrueDegrees,
			NoWindDuration: noWindDurations[i], Duration: durations[i],
		}
	}
	return result
}

func calculationSegments(segments []predictor.DescentSegmentCalculation) []aman.PredictionSegment {
	result := make([]aman.PredictionSegment, len(segments))
	for i, segment := range segments {
		result[i] = aman.PredictionSegment{
			RouteLegIndex: segment.RouteLegIndex, PreTOD: segment.PreTOD,
			PhaseID: segment.PhaseID, PhaseName: segment.PhaseName, PhaseFormula: segment.PhaseFormula,
			DistanceNM: segment.DistanceNM, CourseTrueDegrees: segment.CourseTrueDegrees,
			StartAltitudeFeet: segment.StartAltitudeFeet, EndAltitudeFeet: segment.EndAltitudeFeet, AltitudeFeet: segment.AltitudeFeet,
			IndicatedAirspeedKnots: cloneFloat(segment.IndicatedAirspeedKnots),
			NoWindGroundspeedKnots: segment.NoWindGroundspeedKnots, GroundspeedKnots: segment.GroundspeedKnots,
			TailwindKnots: cloneFloat(segment.TailwindKnots), NoWindDuration: segment.NoWindDuration, Duration: segment.Duration,
		}
	}
	return result
}

func holdingETA(now time.Time, durations []time.Duration, legs []trajectory.RemainingLeg, fix navdata.FixID) *time.Time {
	if len(durations) != len(legs) {
		return nil
	}
	elapsed := time.Duration(0)
	for index, leg := range legs {
		elapsed += durations[index]
		if leg.To == fix {
			at := now.Add(elapsed)
			return &at
		}
	}
	return nil
}

func category(value *string) predictor.AircraftCategory {
	switch strings.ToUpper(stringValue(value)) {
	case "L":
		return predictor.CategoryLight
	case "H":
		return predictor.CategoryHeavy
	case "J":
		return predictor.CategorySuper
	default:
		return predictor.CategoryMedium
	}
}

func observedAt(fact *aman.SurveillanceFact, fallback time.Time) time.Time {
	if fact != nil && fact.ObservedAt != nil {
		return fact.ObservedAt.UTC().Truncate(time.Second)
	}
	return fallback.UTC().Truncate(time.Second)
}
func revisionValue(value *uint64) uint64 {
	if value == nil {
		return 0
	}
	return *value
}
func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func stringPointer(value string) *string { return &value }

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func navigationDatasetID(version navdata.DatasetVersion) string {
	return strings.Join([]string{
		version.Cycle,
		version.SourceRevision,
		version.EffectiveFrom.UTC().Format(time.RFC3339Nano),
		version.EffectiveUntil.UTC().Format(time.RFC3339Nano),
	}, "|")
}

func canReuseActiveRoute(flight aman.AMANFlight, revision uint64, group aman.RunwayGroupID, datasetID string) bool {
	return flight.ActiveRouteKey != nil &&
		flight.ActiveRouteDatasetID != nil &&
		*flight.ActiveRouteDatasetID == datasetID &&
		flight.RouteProgress != nil &&
		flight.RouteProgress.FlightPlanRevision == revision &&
		flight.RouteProgress.RunwayGroupID == group
}

// routeProjectionRevision protects terminal route progress from administrative
// flight-plan revisions. Once APP/TMA has frozen the arrival, only a confirmed
// go-around may release that state; a late VATSIM FPL update must not send the
// aircraft back to an earlier STAR leg.
func routeProjectionRevision(flight aman.AMANFlight, observed uint64) uint64 {
	if flight.FreezeReason == aman.FreezeTMA && flight.RouteProgress != nil {
		return flight.RouteProgress.FlightPlanRevision
	}
	return observed
}

func hasResolvedRouteFixes(ctx context.Context, geometry GeometryCache, airport navdata.AirportID, key navdata.RouteKey) bool {
	route, err := geometry.Route(ctx, key)
	if err != nil || len(route.Legs) == 0 {
		return false
	}
	if hasSyntheticDestinationClosingLeg(route, airport) {
		return false
	}
	snapshot, err := geometry.ActiveGeometrySnapshot(ctx, airport)
	if err != nil || !route.Version.Equal(snapshot.Manifest.Version) {
		return false
	}
	fixes := make(map[navdata.FixID]struct{}, len(snapshot.Fixes))
	for _, fix := range snapshot.Fixes {
		fixes[fix.ID] = struct{}{}
	}
	for _, leg := range route.Legs {
		if leg.FromFix != nil && leg.FromPosition == nil {
			if _, ok := fixes[*leg.FromFix]; !ok {
				return false
			}
		}
		if leg.ToFix != nil && leg.ToPosition == nil {
			if _, ok := fixes[*leg.ToFix]; !ok {
				return false
			}
		}
	}
	return true
}

// AIRAC.NET route parsing closes its output with a direct leg to the airport.
// AMAN replaces that leg with a configured feeder-to-runway terminal path, so
// a cached route ending at the destination predates that normalization and
// must be materialized again.
func hasSyntheticDestinationClosingLeg(route navdata.RouteGeometry, airport navdata.AirportID) bool {
	if len(route.Legs) == 0 {
		return false
	}
	last := route.Legs[len(route.Legs)-1]
	return last.ToFix != nil && *last.ToFix == navdata.FixID(airport) && (last.FromFix == nil || *last.FromFix != *last.ToFix)
}

func componentHealth(status aman.HealthStatus, reason string, at time.Time) aman.ComponentHealth {
	at = at.UTC()
	return aman.ComponentHealth{Status: status, Reason: reason, UpdatedAt: &at}
}

func sourceComponentHealth(status aman.DataStatus, at time.Time) aman.ComponentHealth {
	switch status {
	case aman.DataFresh:
		return componentHealth(aman.HealthReady, "", at)
	case aman.DataStale:
		return componentHealth(aman.HealthDegraded, "source_stale", at)
	default:
		return componentHealth(aman.HealthUnavailable, "source_disconnected", at)
	}
}

func (s *Service) setHealthComponent(name string, status aman.HealthStatus, reason string, at time.Time) {
	value := componentHealth(status, reason, at)
	s.mu.Lock()
	defer s.mu.Unlock()
	switch name {
	case "navigation":
		s.health.navigation = value
	case "weather":
		s.health.weather = value
	case "repository":
		s.health.repository = value
	case "predictor":
		s.health.predictor = value
	}
}

func statesEqual(left, right aman.AirportState) bool {
	left.Revision, right.Revision = 0, 0
	left.GeneratedAt, right.GeneratedAt = time.Time{}, time.Time{}
	return reflect.DeepEqual(left, right)
}

var _ aman.ObservationSink = (*Service)(nil)
var _ aman.ObservationSourceHealthSink = (*Service)(nil)
var _ aman.Worker = (*Service)(nil)
var _ aman.Component = (*Service)(nil)
var _ aman.TechnicalHealthReporter = (*Service)(nil)
