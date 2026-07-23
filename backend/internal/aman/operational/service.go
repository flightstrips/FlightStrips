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
	"FlightStrips/internal/aman/materializer"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/prediction"
	"FlightStrips/internal/aman/predictor"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/aman/trajectory"
)

const (
	policyVersion          = "aman-cph-v1"
	modelVersion           = "aman-cph-teta-v1"
	defaultArrivalRate     = uint32(20)
	navigationRefreshEvery = 6 * time.Hour
	queueOfferValidity     = 2 * time.Minute
)

type NavigationMaterializer interface {
	Refresh(context.Context, materializer.Request) error
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

type Dependencies struct {
	Repository   Repository
	Retirer      aman.VATSIMFlightIdentityRetirer
	Materializer NavigationMaterializer
	Geometry     GeometryCache
	Wind         predictor.WindProfileReader
	Terminal     terminal.Configuration
	Airports     []string
	Mode         aman.RolloutMode
	Publisher    sequence.FullStatePublisher
	Now          func() time.Time
}

type Service struct {
	deps Dependencies

	mu          sync.Mutex
	observed    map[string]map[aman.FlightID]aman.FlightObservation
	lastRefresh map[string]time.Time
	health      serviceHealth
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
		deps: deps, observed: map[string]map[aman.FlightID]aman.FlightObservation{}, lastRefresh: map[string]time.Time{},
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
	airport := strings.ToUpper(strings.TrimSpace(observation.Destination))
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.observed[airport] == nil {
		s.observed[airport] = map[aman.FlightID]aman.FlightObservation{}
	}
	s.observed[airport][observation.FlightID] = observation
	s.health.vatsim = sourceComponentHealth(observation.SourceStatus, observation.ReconciledAt)
	return nil
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

func (s *Service) reconcileAll(ctx context.Context) {
	for _, airport := range s.deps.Airports {
		airport = strings.ToUpper(strings.TrimSpace(airport))
		if err := s.refreshNavigation(ctx, airport); err != nil {
			slog.WarnContext(ctx, "AMAN navigation refresh failed; cached geometry remains eligible", "airport", airport, "error", err)
		}
		if err := s.reconcileAirport(ctx, airport); err != nil {
			slog.WarnContext(ctx, "AMAN reconciliation failed", "airport", airport, "error", err)
		}
	}
}

func (s *Service) refreshNavigation(ctx context.Context, airport string) error {
	now := s.deps.Now().UTC()
	s.mu.Lock()
	last := s.lastRefresh[airport]
	if !last.IsZero() && now.Sub(last) < navigationRefreshEvery {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	err := s.deps.Materializer.Refresh(ctx, materializer.Request{Airport: navdata.AirportID(airport)})
	completedAt := s.deps.Now().UTC()
	s.mu.Lock()
	if err != nil {
		s.health.navigation = componentHealth(aman.HealthUnavailable, "navigation_refresh_failed", completedAt)
	} else {
		s.lastRefresh[airport] = completedAt
		s.health.navigation = componentHealth(aman.HealthReady, "", completedAt)
	}
	s.mu.Unlock()
	return err
}

func (s *Service) reconcileAirport(ctx context.Context, airport string) error {
	now := s.deps.Now().UTC()
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
	if len(next.RunwayGroups) == 0 {
		next.RunwayGroups = s.initialState(airport, now).RunwayGroups
	}
	selectedGroup, selectionChanged := updateSelectedRunwayGroup(next.RunwayGroups, now)
	if selectionChanged {
		reassignFlightsToGroup(&next, selectedGroup)
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
	queueInput := sequenceInput(next, s.deps.Terminal)
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
	groups := make([]aman.RunwayGroupPolicy, 0, len(s.deps.Terminal.RunwayGroups))
	for index, configured := range s.deps.Terminal.RunwayGroups {
		effective := now
		group := aman.RunwayGroupPolicy{
			ID: configured.ID, Selected: index == 0, ActiveRatePerHour: defaultArrivalRate, RateEffectiveAt: &effective,
			RateSchedule: []aman.RunwayGroupRatePoint{{EffectiveAt: effective, ArrivalsPerHour: defaultArrivalRate}},
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
	copy := observation
	flight.LatestObservation = &copy
	flight.VATSIMCID, flight.CurrentCallsign, flight.DataStatus = observation.VATSIMCID, observation.Callsign, observation.SourceStatus
	flight.UpdatedAt = now
	if observation.Missing || observation.SourceStatus != aman.DataFresh {
		return flight, nil
	}
	flight.Lifecycle = clearAbsence(flight.Lifecycle)
	applyBaseline(&flight, observation, now)
	applyPreliminaryPrediction(&flight, observation, now)
	if observation.Surveillance == nil || observation.Surveillance.GroundspeedKnots == nil || observation.Surveillance.AltitudeFeet == nil || observation.FiledRoute == nil {
		if flight.State != aman.StatePlanned {
			markPredictionNonPublishable(&flight, missingEssentialReason(observation))
		}
		return flight, nil
	}
	group, ok := s.selectedGroup(flight, state.RunwayGroups)
	if !ok {
		return flight, fmt.Errorf("terminal has no configured runway group")
	}
	flight.SelectedRunwayGroup = &group
	feeder, ok := s.feeder(*observation.FiledRoute)
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
	datasetID := navigationDatasetID(version)
	var key navdata.RouteKey
	if canReuseActiveRoute(flight, revision, group, datasetID) {
		key = navdata.RouteKey(*flight.ActiveRouteKey)
	} else {
		key, err = s.deps.Materializer.MaterializeRoute(ctx, query, modelVersion)
		if err != nil {
			return flight, err
		}
		activeKey := string(key)
		flight.ActiveRouteKey = &activeKey
		flight.ActiveRouteDatasetID = &datasetID
	}
	projection, err := trajectory.Project(ctx, trajectory.Readers{Geometry: s.deps.Geometry, Snapshot: s.deps.Geometry}, trajectory.Input{
		Airport: navdata.AirportID(observation.Destination), RouteKey: key, Feeder: feeder, RunwayGroup: group,
		FlightPlanRevision: revision, Observation: *observation.Surveillance, Prior: flight.RouteProgress,
	}, trajectory.Config{ReferenceTime: now, MaxObservationAge: 2 * time.Minute})
	if err != nil {
		return flight, err
	}
	if projection.DistanceToGoNM == nil || len(projection.Remaining) == 0 || projection.Completeness == trajectory.Unresolved || projection.Completeness == trajectory.OffRoute {
		return flight, fmt.Errorf("route geometry is not publishable: %s", projection.Completeness)
	}
	input := predictor.PerformanceWindInput{
		PredictionAt: now, AircraftICAO: stringValue(observation.AircraftType), WakeTurbulenceCategory: category(observation.WakeCategory),
		AltitudeFeet: float64(*observation.Surveillance.AltitudeFeet), CurrentGroundspeedKnots: *observation.Surveillance.GroundspeedKnots,
		Remaining: predictorLegs(projection.Remaining),
	}
	estimate, err := predictor.EstimatePerformanceWind(ctx, nil, s.deps.Wind, input, predictor.PerformanceWindConfig{})
	if err != nil {
		s.setHealthComponent("predictor", aman.HealthUnavailable, "prediction_failed", now)
		return flight, err
	}
	s.setHealthComponent("predictor", aman.HealthReady, "", now)
	weatherStatus, weatherReason := aman.HealthReady, ""
	for _, degradation := range estimate.DegradationReasons {
		if degradation == "WEATHER_UNAVAILABLE" || degradation == "WEATHER_INCOMPLETE" {
			weatherStatus, weatherReason = aman.HealthDegraded, strings.ToLower(degradation)
			break
		}
	}
	s.setHealthComponent("weather", weatherStatus, weatherReason, now)
	rawRETA := estimate.RawRETA
	dtg := estimate.DistanceToGoNM
	raw := aman.Prediction{
		RawTETA: estimate.RawTETA, RawRETA: &rawRETA, GeneratedAt: now, InputObservedAt: observedAt(observation.Surveillance, now),
		Confidence: estimate.Confidence, Publishable: true, DatasetVersion: version.Cycle, GeometryDigest: projection.GeometryDigest,
		DistanceToGoNM: &dtg, ModelVersion: estimate.ModelVersion, ConfigVersion: s.deps.Terminal.ConfigVersion,
		PerformanceProfileID: estimate.PerformanceProfileID, WeatherSource: estimate.WeatherSource,
		Sources: []string{"vatsim", "airacnet", "terminal-config:" + s.deps.Terminal.ConfigVersion},
	}
	if len(estimate.DegradationReasons) > 0 {
		reason := strings.Join(estimate.DegradationReasons, ",")
		raw.DegradationReason = &reason
	}
	if projection.SelectedHolding != nil {
		holding := string(projection.SelectedHolding.ID)
		flight.SelectedHolding = &holding
		raw.HoldingFixETA = holdingETA(now, estimate.LegDurations, projection.Remaining, projection.SelectedHolding.Fix)
	}
	flight.RouteProgress = projection.Progress
	previousState := flight.State
	nextState := lifecycleState(flight, raw.RawTETA, now)
	reduced, err := prediction.Reduce(prediction.DefaultConfig(), flight, prediction.Input{Raw: raw, State: nextState, Slot: flight.Slot})
	if err != nil {
		return flight, err
	}
	updated := reduced.Flight
	updateLifecycle(&updated, previousState, nextState, now)
	return updated, nil
}

func (s *Service) feeder(route string) (navdata.FeederID, bool) {
	tokens := strings.FieldsFunc(strings.ToUpper(route), func(r rune) bool {
		return !((r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
	})
	for _, configured := range s.deps.Terminal.Feeders {
		for _, token := range tokens {
			if token == string(configured.ID) || slices.Contains(configured.Aliases, navdata.FeederID(token)) {
				return configured.ID, true
			}
		}
	}
	return "", false
}

func (s *Service) resequence(state *aman.AirportState, now time.Time) {
	input := sequenceInput(*state, s.deps.Terminal)
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

func sequenceInput(state aman.AirportState, config terminal.Configuration) sequence.Input {
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
		})
	}
	return input
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

func (s *Service) selectedGroup(flight aman.AMANFlight, groups []aman.RunwayGroupPolicy) (aman.RunwayGroupID, bool) {
	if flight.SelectedRunwayGroup != nil && (flight.State == aman.StateStable || flight.FreezeReason != aman.FreezeNone) {
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

func markPredictionNonPublishable(flight *aman.AMANFlight, reason string) {
	if flight.Prediction == nil {
		return
	}
	prediction := *flight.Prediction
	prediction.Publishable = false
	prediction.DegradationReason = &reason
	flight.Prediction = &prediction
	if flight.FreezeReason == aman.FreezeNone {
		flight.Slot = nil
		flight.Order = nil
		flight.ManualOrder = nil
		flight.QueueOffers = nil
	}
}

func markUnknownSTARFamily(flight *aman.AMANFlight, now time.Time) {
	flight.SelectedFeeder = nil
	flight.SelectedHolding = nil
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
		return *fact.ObservedAt
	}
	return fallback
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
