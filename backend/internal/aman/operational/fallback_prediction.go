package operational

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/trajectory"
)

func freshArrivalObservation(o *aman.FlightObservation, now time.Time) bool {
	return aman.HasFreshArrivalSurveillance(o, now)
}

func (s *Service) resolveFlightSTAR(f aman.AMANFlight, o aman.FlightObservation, group aman.RunwayGroupID) starResolution {
	if f.ControllerSTARFamily != nil {
		family := navdata.FeederID(*f.ControllerSTARFamily)
		for _, path := range s.deps.Terminal.Paths {
			if path.Feeder == family && path.RunwayGroup == group {
				return starResolution{feeder: family}
			}
		}
		return starResolution{reason: aman.STARRunwayUnavailable}
	}
	return s.resolveObservationSTAR(o, group)
}

// A route/provider failure may use terminal fallback, but a rejected aircraft
// position must retain its physical-progress guard regardless of prediction basis.
type surveillanceProjectionError struct{ cause error }

func (e *surveillanceProjectionError) Error() string { return e.cause.Error() }
func (e *surveillanceProjectionError) Unwrap() error { return e.cause }

func projectionFailure(projection trajectory.Result) error {
	err := fmt.Errorf("route geometry is not publishable: %s (%s)", projection.Completeness, strings.Join(projection.Reasons, ", "))
	for _, reason := range projection.Reasons {
		switch reason {
		case "FORWARD_PROGRESS_OUT_OF_RANGE", "STALE_POSITION", "INVALID_POSITION", "ROUTE_RECOVERY_POSITION_FUTURE", "ROUTE_RECOVERY_POSITION_OUT_OF_ORDER":
			return &surveillanceProjectionError{cause: err}
		}
	}
	return err
}

func (s *Service) reconcileFlight(ctx context.Context, state aman.AirportState, flight aman.AMANFlight, observation aman.FlightObservation, now time.Time) (aman.AMANFlight, error) {
	updated, primaryErr := s.reconcileFlightPrimary(ctx, state, flight, observation, now)
	if primaryErr == nil && updated.PredictionFailureReason == nil {
		if updated.Prediction != nil && updated.Prediction.Basis != aman.PredictionBasisFallback {
			clearFallbackPredictionProgress(&updated)
		}
		return updated, nil
	}
	var rejected *surveillanceProjectionError
	if errors.As(primaryErr, &rejected) {
		return updated, primaryErr
	}
	if updated.LatestObservation != nil {
		observation = *updated.LatestObservation
	}
	if updated.State == aman.StateLanded || updated.State == aman.StateRemoved || !updated.SequenceDisposition.Participates() || !freshArrivalObservation(&observation, now) {
		return updated, primaryErr
	}
	reason := stringValue(updated.PredictionFailureReason)
	if primaryErr != nil {
		reason = primaryErr.Error()
	}
	fallback, err := s.fallbackPrediction(ctx, state, updated, observation, flight.LatestObservation, now, reason)
	if err == nil {
		fallback.PredictionFailureReason = nil
		return fallback, nil
	}
	diagnostic := reason + "; fallback: " + err.Error()
	updated.PredictionFailureReason = &diagnostic
	return updated, primaryErr
}

func (s *Service) fallbackPrediction(ctx context.Context, state aman.AirportState, flight aman.AMANFlight, observation aman.FlightObservation, previous *aman.FlightObservation, now time.Time, reason string) (aman.AMANFlight, error) {
	if !freshArrivalObservation(&observation, now) {
		return flight, fmt.Errorf("current aircraft surveillance is unavailable")
	}
	group, ok := s.selectedGroup(flight, state.RunwayGroups)
	if !ok {
		return flight, fmt.Errorf("active arrival runway is unavailable")
	}
	return s.fallbackPredictionForRunway(ctx, state, flight, observation, previous, group, now, reason)
}

// Explicit controller insertion derives its runway from the authoritative anchor.
func (s *Service) fallbackPredictionForRunway(ctx context.Context, state aman.AirportState, flight aman.AMANFlight, observation aman.FlightObservation, previous *aman.FlightObservation, group aman.RunwayGroupID, now time.Time, reason string) (aman.AMANFlight, error) {
	if !freshArrivalObservation(&observation, now) {
		return flight, fmt.Errorf("current aircraft surveillance is unavailable")
	}
	resolved := s.resolveFlightSTAR(flight, observation, group)
	if resolved.reason != "" {
		return flight, fmt.Errorf("STAR selection required: %s", resolved.reason)
	}
	path, err := s.deps.Geometry.TerminalPath(ctx, navdata.AirportID(state.Airport), resolved.feeder, group)
	if err != nil {
		return flight, err
	}
	snapshot, err := s.deps.Geometry.ActiveGeometrySnapshot(ctx, navdata.AirportID(state.Airport))
	if err != nil {
		return flight, err
	}
	if err := trajectory.ValidateTerminalFallbackGeometry(snapshot, path); err != nil {
		return flight, err
	}
	input := trajectory.Input{Airport: navdata.AirportID(state.Airport), Feeder: resolved.feeder, FeederFix: path.FeederFix, RunwayGroup: group, Observation: *observation.Surveillance, Prior: flight.FallbackRouteProgress, RouteFact: flight.ActiveRouteFact, HoldingClearanceFix: operationalHoldingClearanceFix(flight), FlightPlanRevision: routeProjectionRevision(flight, revisionValue(observation.FlightPlan.Revision))}
	config := trajectory.Config{ReferenceTime: now, MaxObservationAge: tmaSurveillanceFresh}
	var projection trajectory.Result
	// Prefer the cached filed route: it can honor an enroute direct that the
	// terminal-only path cannot resolve. Surveillance rejections remain binding.
	if flight.ActiveRouteKey != nil && fallbackRouteMatchesObservation(flight, observation, previous, path, snapshot.Manifest.Version, group) {
		cachedInput := input
		cachedInput.RouteKey = navdata.RouteKey(*flight.ActiveRouteKey)
		cachedInput.Prior, cachedInput.Recovery = flight.RouteProgress, flight.RouteRecovery
		cachedRoute, cacheErr := s.deps.Geometry.Route(ctx, cachedInput.RouteKey)
		if cacheErr == nil && hasResolvedRouteFixes(ctx, s.deps.Geometry, navdata.AirportID(state.Airport), cachedInput.RouteKey) {
			if prior := flight.FallbackRouteProgress; prior != nil && prior.GeometryDigest == cachedRoute.Digest {
				cachedInput.Prior = prior
			}
			cached := trajectory.Reduce(snapshot, cachedRoute, cachedInput, config)
			var rejected *surveillanceProjectionError
			if failure := projectionFailure(cached); errors.As(failure, &rejected) {
				return flight, failure
			}
			if cached.DistanceToGoNM != nil && len(cached.Remaining) > 0 && (cached.Completeness == trajectory.Complete || cached.Completeness == trajectory.Partial) {
				projection = cached
			}
		}
	}
	if projection.DistanceToGoNM == nil {
		projection, err = trajectory.ProjectTerminalFallback(snapshot, path, input, config)
		if err != nil {
			return flight, err
		}
	}
	observationCopy := observation
	flight.LatestObservation, flight.DataStatus, flight.SelectedRunwayGroup = &observationCopy, observation.SourceStatus, &group
	applyResolvedTerminalIdentity(&flight, path)
	if flight.State == aman.StatePlanned {
		updateLifecycle(&flight, aman.StatePlanned, aman.StateAirborne, now)
	}
	prior := flight.FallbackRouteProgress
	if prior == nil {
		prior = flight.RouteProgress
	}
	updated, err := s.applyProjectedPrediction(ctx, state, flight, observation, now, projectedPrediction{projection: projection, version: snapshot.Manifest.Version, resolved: resolved, prior: prior, previousObservation: previous, directChanged: confirmedDirectChanged(flight.ActiveRouteFact, prior, projection), fallbackReason: strings.TrimSpace(reason)})
	if err != nil {
		return flight, err
	}
	updated.FallbackRouteProgress, updated.RouteProgress = updated.RouteProgress, flight.RouteProgress
	updated.PredictionFailureReason = nil
	return updated, nil
}

// A cached filed route must describe the current request, except for the
// established TMA assignment that normal prediction deliberately preserves.
func fallbackRouteMatchesObservation(flight aman.AMANFlight, observation aman.FlightObservation, previous *aman.FlightObservation, path navdata.TerminalPath, version navdata.DatasetVersion, group aman.RunwayGroupID) bool {
	reusable := canReuseActiveRoute(flight, routeProjectionRevision(flight, revisionValue(observation.FlightPlan.Revision)), group, navigationDatasetID(version))
	if flight.FreezeReason == aman.FreezeTMA && reusable {
		return true
	}
	if observation.FiledRoute == nil {
		return false
	}
	if flight.ActiveRouteQueryKey == "" {
		// Legacy rows may lack a semantic key. Require their accepted route
		// identity and unchanged flight-plan inputs before reusing the cache.
		return reusable && previous != nil && previous.Origin == observation.Origin && previous.Destination == observation.Destination && stringValue(previous.FiledRoute) == *observation.FiledRoute
	}
	query := navdata.RouteQuery{Version: version, Origin: navdata.AirportID(observation.Origin), Destination: navdata.AirportID(observation.Destination), FiledRoute: *observation.FiledRoute, RunwayGroup: &group, Callsign: observation.Callsign, TerminalEntry: filedTerminalEntry(*observation.FiledRoute, path)}
	key, err := query.Key()
	return err == nil && flight.ActiveRouteQueryKey == string(key)
}

func (s *Service) populateInsertionOptions(state *aman.AirportState) {
	state.InsertionSTARFamilies = nil
	if len(s.deps.Terminal.Paths) == 0 {
		return
	}
	state.InsertionSTARFamilies = make(map[aman.RunwayGroupID][]string)
	for _, path := range s.deps.Terminal.Paths {
		family := string(path.Feeder)
		duplicate := false
		for _, existing := range state.InsertionSTARFamilies[path.RunwayGroup] {
			if existing == family {
				duplicate = true
			}
		}
		if !duplicate {
			state.InsertionSTARFamilies[path.RunwayGroup] = append(state.InsertionSTARFamilies[path.RunwayGroup], family)
		}
	}
	for group := range state.InsertionSTARFamilies {
		slices.Sort(state.InsertionSTARFamilies[group])
	}
}

func clearFallbackPredictionProgress(flight *aman.AMANFlight) {
	flight.FallbackRouteProgress, flight.FallbackDescent = nil, nil
}
