package operational

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/terminal"
	"github.com/stretchr/testify/require"
)

type runwaySTARGeometry struct {
	arrivalChangeGeometry
	paths []navdata.TerminalPath
}

func (g runwaySTARGeometry) TerminalPath(_ context.Context, _ navdata.AirportID, feeder navdata.FeederID, group aman.RunwayGroupID) (navdata.TerminalPath, error) {
	for _, path := range g.paths {
		if path.Feeder == feeder && path.RunwayGroup == group {
			return path, nil
		}
	}
	return navdata.TerminalPath{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "terminal path unavailable"}
}

func (g runwaySTARGeometry) ActiveGeometrySnapshot(ctx context.Context, airport navdata.AirportID) (navdata.ActiveGeometrySnapshot, error) {
	snapshot, err := g.arrivalChangeGeometry.ActiveGeometrySnapshot(ctx, airport)
	snapshot.TerminalPaths = g.paths
	return snapshot, err
}

func runwaySTARFixture(now time.Time) (*Service, aman.AirportState, aman.FlightObservation, *arrivalChangeMaterializer) {
	service, state, observation, materializer := arrivalChangeFixture(now)
	oldGroup, newGroup := aman.RunwayGroupID("ARRIVAL-22L"), aman.RunwayGroupID("ARRIVAL-04L")
	geometry := service.deps.Geometry.(arrivalChangeGeometry)
	oldPath, newPath := geometry.path, geometry.path
	oldPath.RunwayGroup, newPath.RunwayGroup = oldGroup, newGroup
	// Distinct feeder fixes ensure timing from the old runway is rebuilt.
	oldPath.FeederFix = "HOLD"
	service.deps.Geometry = runwaySTARGeometry{arrivalChangeGeometry: geometry, paths: []navdata.TerminalPath{oldPath, newPath}}
	service.deps.Terminal.RunwayGroups = []terminal.RunwayGroup{{ID: oldGroup}, {ID: newGroup}}
	service.deps.Terminal.Paths = []terminal.Path{
		{Feeder: "TESPI", RunwayGroup: oldGroup, STARSuffix: "1C"},
		{Feeder: "TESPI", RunwayGroup: newGroup, STARSuffix: "2A"},
	}
	state.RunwayGroups[0].ID = oldGroup
	second := state.RunwayGroups[0]
	second.ID, second.Selected = newGroup, false
	state.RunwayGroups = append(state.RunwayGroups, second)
	state.ActiveRunwayGroups = []aman.RunwayGroupID{oldGroup}
	observation.AssignedSTAR = stringPointer("TESPI1C")
	return service, state, observation, materializer
}

func selectSTARTestRunway(state *aman.AirportState, group aman.RunwayGroupID) {
	for index := range state.RunwayGroups {
		state.RunwayGroups[index].Selected = state.RunwayGroups[index].ID == group
	}
	state.ActiveRunwayGroups = []aman.RunwayGroupID{group}
	reassignFlightsToGroup(state, group)
}

func TestRunwaySwitchRefreshesExistingSTARForecastAndRecoversAfterOutage(t *testing.T) {
	for _, outage := range []bool{false, true} {
		t.Run(map[bool]string{false: "immediate", true: "navigation outage and restart"}[outage], func(t *testing.T) {
			now := time.Date(2026, 11, 2, 17, 43, 11, 0, time.UTC)
			service, state, observation, materializer := runwaySTARFixture(now)
			flight, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
			require.NoError(t, err)
			require.True(t, flight.Prediction.Publishable)
			oldPrediction := *flight.Prediction
			// Model an enroute aircraft that is eligible for automatic reassignment.
			flight.State, flight.FreezeReason = aman.StateAirborne, aman.FreezeNone
			flight.Slot = &aman.Slot{RunwayGroupID: "ARRIVAL-22L", Time: now.Add(31 * time.Minute)}
			state.Flights = []aman.AMANFlight{flight}
			selectSTARTestRunway(&state, "ARRIVAL-04L")
			flight = state.Flights[0]
			require.True(t, flight.RunwayChangePending)
			if outage {
				geometry := service.deps.Geometry
				service.deps.Geometry = unavailableGeometry{}
				pending, err := service.reconcileFlight(t.Context(), state, flight, observation, now.Add(time.Minute))
				require.Error(t, err)
				pending = applyUnavailablePrediction(pending, observation, now.Add(time.Minute), err)
				require.Equal(t, aman.DataStale, pending.DataStatus, "retained timing must be visibly stale when fallback is unavailable")
				require.Equal(t, aman.DataFresh, pending.LatestObservation.SourceStatus)
				require.NotNil(t, pending.PredictionFailureReason)
				require.True(t, pending.Prediction.Publishable, "keep the previous estimate visibly stale")
				require.Equal(t, oldPrediction.GeneratedAt, pending.Prediction.GeneratedAt)
				require.True(t, pending.RunwayChangePending)
				flight = cloneGapState(t, aman.AirportState{Flights: []aman.AMANFlight{pending}}).Flights[0]
				service = &Service{deps: service.deps}
				service.deps.Geometry = geometry
			}
			// The same radar observation and FPL revision must still finish the rebuild.
			updated, err := service.reconcileFlight(t.Context(), state, flight, observation, now.Add(time.Minute))
			require.NoError(t, err)
			require.Equal(t, aman.DataFresh, updated.DataStatus)
			require.True(t, updated.Prediction.Publishable)
			require.Equal(t, now.Add(time.Minute), updated.Prediction.GeneratedAt)
			require.Equal(t, "ARRIVAL-04L", string(*updated.SelectedRunwayGroup))
			require.Equal(t, aman.RunwayGroupID("ARRIVAL-04L"), updated.RouteProgress.RunwayGroupID)
			require.Equal(t, "END", *updated.SelectedFeederFix)
			require.Contains(t, *updated.Prediction.DegradationReason, aman.STARRunwayInferred)
			require.Equal(t, "TESPI1C", *updated.LatestObservation.AssignedSTAR)
			require.False(t, updated.ArrivalPathChanged)
			require.False(t, updated.RunwayChangePending)
			require.Len(t, updated.RawTETASamples, 1)
			require.Nil(t, updated.PredictionFilter.Pending)
			require.Equal(t, 2, materializer.calls)
			warnings := aman.CurrentWarningSnapshot(aman.TechnicalHealth{}, aman.AirportState{Flights: []aman.AMANFlight{updated}}).Warnings
			require.Len(t, warnings, 1)
			require.Equal(t, aman.STARRunwayInferred, warnings[0].Code)

			// A later radar report keeps refreshing the inferred forecast, including
			// when the aircraft has since become stable on the new runway.
			nextAt := now.Add(90 * time.Second)
			observation.Surveillance.ObservedAt = &nextAt
			observation.ReconciledAt = nextAt
			updated, err = service.reconcileFlight(t.Context(), state, updated, observation, nextAt)
			require.NoError(t, err)
			require.Equal(t, nextAt, updated.Prediction.GeneratedAt)
			require.Contains(t, *updated.Prediction.DegradationReason, aman.STARRunwayInferred)

			// A matching assignment clears the inference even at the same radar time.
			observation.AssignedSTAR = stringPointer("TESPI2A")
			updated, err = service.reconcileFlight(t.Context(), state, updated, observation, nextAt)
			require.NoError(t, err)
			require.NotContains(t, stringValue(updated.Prediction.DegradationReason), aman.STARRunwayInferred)
			require.Empty(t, aman.CurrentWarningSnapshot(aman.TechnicalHealth{}, aman.AirportState{Flights: []aman.AMANFlight{updated}}).Warnings)
		})
	}
}

func TestValidDirectToSurvivesRunwaySwitchAndRebuildRecovery(t *testing.T) {
	for _, scenario := range []string{"pending assignment", "selected runway changed", "inferred rebuild", "navigation outage and restart"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Date(2026, 11, 2, 17, 43, 11, 0, time.UTC)
			service, state, observation, _ := runwaySTARFixture(now)
			// The direct skips an enroute dogleg shared by both runway paths.
			geometry := service.deps.Geometry.(runwaySTARGeometry)
			origin, bend, entry := navdata.FixID("ORIGIN"), navdata.FixID("BEND"), navdata.FixID("ENTRY")
			geometry.fixes = append(geometry.fixes, navdata.Fix{ID: bend, Position: navdata.Coordinate{LatitudeDeg: 55, LongitudeDeg: 12.5}})
			geometry.route.Legs = []navdata.ProcedureLeg{
				{ID: "TO-BEND", PathTerminator: navdata.PathTF, FromFix: &origin, ToFix: &bend},
				{ID: "TO-ENTRY", PathTerminator: navdata.PathTF, FromFix: &bend, ToFix: &entry},
			}
			service.deps.Geometry = geometry
			flight, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
			require.NoError(t, err)
			filedDistance := *flight.Prediction.DistanceToGoNM
			direct := aman.RouteFact{ID: "cleared-entry", Callsign: flight.Callsign, Fix: "ENTRY", State: aman.RouteFactActive,
				DatasetVersion: navigationDatasetID(geometry.version), ObservedAt: now, ReceivedAt: now}
			flight.ActiveRouteFact = &direct
			flight, err = service.reconcileFlight(t.Context(), state, flight, observation, now)
			require.NoError(t, err)
			directDistance := *flight.Prediction.DistanceToGoNM
			require.Less(t, directDistance, filedDistance)
			require.Equal(t, direct.ID, flight.RouteProgress.RouteFactID)
			flight.State, flight.FreezeReason = aman.StateAirborne, aman.FreezeNone
			state.Flights = []aman.AMANFlight{flight}
			if scenario == "pending assignment" || scenario == "navigation outage and restart" {
				selectSTARTestRunway(&state, "ARRIVAL-04L")
				flight = state.Flights[0]
			} else {
				// Exercise reconciliation without a previously queued assignment.
				for index := range state.RunwayGroups {
					state.RunwayGroups[index].Selected = state.RunwayGroups[index].ID == "ARRIVAL-04L"
				}
				if scenario == "inferred rebuild" {
					group := aman.RunwayGroupID("ARRIVAL-04L")
					flight.SelectedRunwayGroup, flight.RouteProgress = &group, nil
				}
			}
			if scenario == "navigation outage and restart" {
				service.deps.Geometry = unavailableGeometry{}
				pending, err := service.reconcileFlight(t.Context(), state, flight, observation, now.Add(time.Minute))
				require.Error(t, err)
				pending = applyUnavailablePrediction(pending, observation, now.Add(time.Minute), err)
				require.Equal(t, direct, *pending.ActiveRouteFact)
				flight = cloneGapState(t, aman.AirportState{Flights: []aman.AMANFlight{pending}}).Flights[0]
				service = &Service{deps: service.deps}
				service.deps.Geometry = geometry
			}
			updated, err := service.reconcileFlight(t.Context(), state, flight, observation, now.Add(time.Minute))
			require.NoError(t, err)
			require.Equal(t, direct, *updated.ActiveRouteFact)
			require.Equal(t, aman.RunwayGroupID("ARRIVAL-04L"), updated.RouteProgress.RunwayGroupID)
			require.Equal(t, direct.ID, updated.RouteProgress.RouteFactID)
			require.Equal(t, []string{"ENTRY", "HOLD", "END"}, updated.Prediction.RemainingRouteFixes)
			require.InDelta(t, directDistance, *updated.Prediction.DistanceToGoNM, 0.001)
			require.True(t, updated.Prediction.Publishable)
			require.Equal(t, now.Add(time.Minute), updated.Prediction.GeneratedAt)
			require.False(t, updated.RunwayChangePending)
		})
	}
}

func TestRunwaySwitchPreservesProtectedSTARAircraft(t *testing.T) {
	for _, protection := range []string{"stable", "frozen", "holding"} {
		t.Run(protection, func(t *testing.T) {
			now := time.Date(2026, 11, 2, 17, 43, 11, 0, time.UTC)
			service, state, observation, _ := runwaySTARFixture(now)
			flight := gapCommandFlight("SAS123", "ARRIVAL-22L", now.Add(20*time.Minute), 1, aman.StateAirborne, aman.FreezeNone)
			flight.LatestObservation = &observation
			switch protection {
			case "stable":
				flight.State = aman.StateStable
			case "frozen":
				flight.FreezeReason = aman.FreezeManual
				at, eta := now, flight.Prediction.OperationalTETA
				flight.FrozenAt, flight.FrozenOperationalTETA = &at, &eta
			case "holding":
				flight.HoldingClearance = &aman.HoldingClearance{Hold: "HOLD", HoldType: aman.HoldingClearanceEnroute, ObservedAt: now}
			}
			state.Flights = []aman.AMANFlight{flight}
			selectSTARTestRunway(&state, "ARRIVAL-04L")
			updated, err := service.reconcileFlight(t.Context(), state, state.Flights[0], observation, now)
			require.NoError(t, err)
			require.Equal(t, aman.RunwayGroupID("ARRIVAL-22L"), *updated.SelectedRunwayGroup)
			require.Equal(t, flight.Slot, updated.Slot)
			require.True(t, updated.Prediction.Publishable)
			require.NotContains(t, stringValue(updated.Prediction.DegradationReason), aman.STARRunwayInferred)
		})
	}
}

func TestNewAircraftCanForecastRecognizedSTARForSelectedRunway(t *testing.T) {
	now := time.Date(2026, 11, 2, 18, 0, 0, 0, time.UTC)
	service, state, observation, _ := runwaySTARFixture(now)
	selectSTARTestRunway(&state, "ARRIVAL-04L")
	flight, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
	require.NoError(t, err)
	require.Equal(t, aman.DataFresh, flight.DataStatus)
	require.True(t, flight.Prediction.Publishable)
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-04L"), flight.RouteProgress.RunwayGroupID)
	require.Contains(t, *flight.Prediction.DegradationReason, aman.STARRunwayInferred)
}

func TestSTARResolutionRejectsAmbiguousFamilies(t *testing.T) {
	service := &Service{deps: Dependencies{Terminal: terminal.Configuration{
		Feeders: []terminal.Feeder{{ID: "ONE", Aliases: []navdata.FeederID{"SHARED"}}, {ID: "TWO", Aliases: []navdata.FeederID{"SHARED"}}},
		Paths:   []terminal.Path{{Feeder: "ONE", RunwayGroup: "OLD", STARSuffix: "1A"}, {Feeder: "TWO", RunwayGroup: "NEW", STARSuffix: "1A"}},
	}}}
	resolved := service.resolveObservationSTAR(aman.FlightObservation{AssignedSTAR: stringPointer("SHARED1A"), FiledRoute: stringPointer("DCT ONE")}, "NEW")
	require.Equal(t, aman.STARFamilyAmbiguous, resolved.reason)
	require.Empty(t, resolved.feeder)
}

func TestSTARForecastRecoversAircraftPersistedBeforeRunwayInference(t *testing.T) {
	now := time.Date(2026, 11, 2, 17, 43, 11, 0, time.UTC)
	service, state, observation, _ := runwaySTARFixture(now)
	flight, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
	require.NoError(t, err)
	newGroup := aman.RunwayGroupID("ARRIVAL-04L")
	selectSTARTestRunway(&state, newGroup)
	// Older versions persisted the selected runway without a pending rebuild.
	// A retained ETA far from the new calculation must not enter the spike filter.
	flight.SelectedRunwayGroup, flight.RouteProgress = &newGroup, nil
	flight.DataStatus, flight.ArrivalPathChanged = aman.DataStale, false
	flight.RunwayChangePending = false
	flight.Prediction.RawTETA = now.Add(time.Hour)
	flight.Prediction.OperationalTETA = flight.Prediction.RawTETA
	flight.Prediction.DegradationReason = stringPointer("unknown_star_family")
	updated, err := service.reconcileFlight(t.Context(), state, flight, observation, now.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, aman.DataFresh, updated.DataStatus)
	require.Equal(t, now.Add(time.Minute), updated.Prediction.GeneratedAt)
	require.True(t, updated.Prediction.RawTETA.Before(now.Add(10*time.Minute)))
	require.Contains(t, *updated.Prediction.DegradationReason, aman.STARRunwayInferred)
	require.Nil(t, updated.PredictionFilter.Pending)
	require.False(t, updated.ArrivalPathChanged)
	require.False(t, updated.RunwayChangePending)
}

func TestUnresolvedSTARAfterRunwaySwitchRetainsStaleEstimateUntilPathAvailable(t *testing.T) {
	for _, assignment := range []string{"TESPI1C", "UNKNOWN1C"} {
		t.Run(assignment, func(t *testing.T) {
			now := time.Date(2026, 11, 2, 17, 43, 11, 0, time.UTC)
			service, state, observation, materializer := runwaySTARFixture(now)
			flight, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
			require.NoError(t, err)
			flight.State = aman.StateAirborne
			oldETA, oldGeneratedAt := flight.Prediction.OperationalTETA, flight.Prediction.GeneratedAt
			state.Flights = []aman.AMANFlight{flight}
			selectSTARTestRunway(&state, "ARRIVAL-04L")
			paths := service.deps.Terminal.Paths
			service.deps.Terminal.Paths = paths[:1]
			observation.AssignedSTAR = &assignment
			pending, err := service.reconcileFlight(t.Context(), state, state.Flights[0], observation, now.Add(time.Minute))
			require.NoError(t, err)
			require.Equal(t, aman.DataStale, pending.DataStatus)
			require.True(t, pending.RunwayChangePending)
			require.Equal(t, oldETA, pending.Prediction.OperationalTETA)
			require.Equal(t, oldGeneratedAt, pending.Prediction.GeneratedAt)
			require.Equal(t, 1, materializer.calls, "unresolved procedures must not use a filed-route fallback")
			if assignment == "UNKNOWN1C" {
				require.Equal(t, "unknown_star_family", *pending.Prediction.DegradationReason)
				return
			}
			require.Equal(t, aman.STARRunwayUnavailable, *pending.Prediction.DegradationReason)
			service.deps.Terminal.Paths = paths
			updated, err := service.reconcileFlight(t.Context(), state, pending, observation, now.Add(time.Minute))
			require.NoError(t, err)
			require.Equal(t, aman.DataFresh, updated.DataStatus)
			require.False(t, updated.ArrivalPathChanged)
			require.False(t, updated.RunwayChangePending)
			require.Contains(t, *updated.Prediction.DegradationReason, aman.STARRunwayInferred)
		})
	}
}

func TestProtectedFlightKeepsHoldingReservationWhenAssignedSTARRunwayChanges(t *testing.T) {
	now := time.Date(2026, 11, 2, 17, 43, 11, 0, time.UTC)
	service, state, observation, _ := runwaySTARFixture(now)
	flight, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
	require.NoError(t, err)
	flight.Slot = &aman.Slot{RunwayGroupID: "ARRIVAL-22L", Time: now.Add(20 * time.Minute)}
	flight.FreezeReason = aman.FreezeManual
	eta, frozenSlot := flight.Prediction.OperationalTETA, *flight.Slot
	flight.FrozenAt, flight.FrozenOperationalTETA, flight.FrozenSlot = &now, &eta, &frozenSlot
	// A missing persisted projection requires a rebuild even though the
	// protected runway and STAR family remain the same.
	flight.RouteProgress = nil
	leader := gapCommandFlight("LEADER", "ARRIVAL-22L", now.Add(15*time.Minute), 1, aman.StateStable, aman.FreezeNone)
	leader.SelectedHolding = stringPointer("TESPI-HOLD")
	leader.HoldingClearance = &aman.HoldingClearance{Hold: "HOLD", HoldType: aman.HoldingClearanceEnroute, ObservedAt: now}
	state.Flights = []aman.AMANFlight{flight, leader}
	selectSTARTestRunway(&state, "ARRIVAL-04L")
	observation.AssignedSTAR = stringPointer("TESPI2A")
	updated, err := service.reconcileFlight(t.Context(), state, state.Flights[0], observation, now)
	require.NoError(t, err)
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-22L"), *updated.SelectedRunwayGroup)
	require.Equal(t, flight.Slot, updated.Slot)
	require.Equal(t, aman.FreezeManual, updated.FreezeReason)
	require.Equal(t, eta, updated.Prediction.OperationalTETA)
	require.Contains(t, *updated.Prediction.DegradationReason, aman.STARRunwayInferred)
	require.False(t, updated.RunwayChangePending)
}

func TestInferredSTARFallbackPreservesEvidenceAndRetryBackoffAfterRunwaySwitch(t *testing.T) {
	for _, join := range []bool{true, false} {
		t.Run(map[bool]string{true: "basic_join", false: "on_star"}[join], func(t *testing.T) {
			now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
			service, state, observation, _ := runwaySTARFixture(now)
			geometry := service.deps.Geometry.(runwaySTARGeometry)
			for i := range geometry.paths {
				geometry.paths[i].Coverage, geometry.paths[i].Digest = navdata.CoverageComplete, "terminal-"+string(geometry.paths[i].RunwayGroup)
			}
			if join {
				geometry.fixes[0].Position = navdata.Coordinate{LatitudeDeg: 53, LongitudeDeg: 10}
				for i := 1; i < len(geometry.fixes); i++ {
					geometry.fixes[i].Position = navdata.Coordinate{LatitudeDeg: 56 + float64(i-1)*.1, LongitudeDeg: 13 + float64(i-1)*.1}
				}
			}
			service.deps.Geometry = geometry
			observation.AircraftType, observation.WakeCategory = stringPointer("A320"), stringPointer("M")
			requested, altitude := 35000, 30000
			observation.RequestedLevel = &requested
			initialPosition := *observation.Surveillance
			initialPosition.AltitudeFeet = &altitude
			if join {
				initialPosition.LatitudeDegrees, initialPosition.LongitudeDegrees = 54, 11
			} else {
				initialPosition.LatitudeDegrees, initialPosition.LongitudeDegrees = 55.15, 12.15
			}
			observation.Surveillance = &initialPosition
			initial, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
			require.NoError(t, err)
			initial.State, initial.FreezeReason = aman.StateAirborne, aman.FreezeNone
			state.Flights = []aman.AMANFlight{initial}
			selectSTARTestRunway(&state, "ARRIVAL-04L")
			flight := state.Flights[0]
			provider := &failingRepairMaterializer{}
			service.deps.Materializer = provider
			var retryAt time.Time
			for sample := 1; sample <= 3; sample++ {
				at := now.Add(time.Duration(sample) * 15 * time.Second)
				next := observation
				position := initialPosition
				nextAltitude := altitude - sample*200
				position.AltitudeFeet, position.ObservedAt = &nextAltitude, &at
				next.Surveillance, next.ReconciledAt = &position, at
				flight, err = service.reconcileFlight(t.Context(), state, flight, next, at)
				require.NoError(t, err)
				require.Equal(t, aman.PredictionBasisFallback, flight.Prediction.Basis)
				require.Nil(t, flight.RouteProgress)
				require.False(t, flight.RunwayChangePending)
				require.Equal(t, "TESPI", flight.STARFamilyIdentity())
				require.Equal(t, aman.RunwayGroupID("ARRIVAL-04L"), *flight.SelectedRunwayGroup)
				require.Equal(t, "TESPI1C", *flight.LatestObservation.AssignedSTAR)
				require.NotNil(t, flight.RouteRefreshAfter)
				if sample == 1 {
					retryAt = *flight.RouteRefreshAfter
				}
				require.Equal(t, retryAt, *flight.RouteRefreshAfter)
				require.Equal(t, 1, provider.calls, "the accepted path must retain the failed provider's retry backoff")
				if join {
					require.Nil(t, flight.FallbackRouteProgress)
					require.EqualValues(t, min(sample, 2), flight.FallbackDescent.Samples)
					require.Equal(t, sample >= 2, flight.FallbackDescent.Confirmed)
				} else {
					require.NotNil(t, flight.FallbackRouteProgress)
					require.Equal(t, aman.RunwayGroupID("ARRIVAL-04L"), flight.FallbackRouteProgress.RunwayGroupID)
				}
				if sample == 1 {
					// Persist the successful fallback rather than completing a
					// primary route first, then resume with the same old STAR.
					flight = cloneGapState(t, aman.AirportState{Flights: []aman.AMANFlight{flight}}).Flights[0]
					service = &Service{deps: service.deps}
				}
			}
			// Retry only when due, then recover normal routing on this runway.
			service.deps.Materializer = fixedNavigation{key: "new-route"}
			next := *flight.LatestObservation
			position := *next.Surveillance
			position.ObservedAt = &retryAt
			next.Surveillance, next.ReconciledAt = &position, retryAt
			recovered, err := service.reconcileFlight(t.Context(), state, flight, next, retryAt)
			require.NoError(t, err)
			require.NotEqual(t, aman.PredictionBasisFallback, recovered.Prediction.Basis)
			require.Equal(t, aman.RunwayGroupID("ARRIVAL-04L"), recovered.RouteProgress.RunwayGroupID)
			require.True(t, recovered.RouteProgress.DescentConfirmed)
			require.Nil(t, recovered.FallbackDescent)
			require.Nil(t, recovered.FallbackRouteProgress)
		})
	}
}
