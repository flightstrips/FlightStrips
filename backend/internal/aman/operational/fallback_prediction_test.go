package operational

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/routefact"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/aman/trajectory"
	"github.com/stretchr/testify/require"
)

func fallbackFixture(t *testing.T, now time.Time) (*Service, aman.AirportState, aman.FlightObservation) {
	t.Helper()
	service, state, observation, _ := arrivalChangeFixture(now)
	geometry := service.deps.Geometry.(arrivalChangeGeometry)
	geometry.path.Coverage = navdata.CoverageComplete
	geometry.path.Digest = "validated-terminal"
	service.deps.Geometry = geometry
	service.deps.Materializer = unavailableNavigation{}
	service.deps.Terminal.Paths = []terminal.Path{{Feeder: "TESPI", RunwayGroup: state.RunwayGroups[0].ID, STARSuffix: "1A"}}
	state.Revision = 7
	state.GeneratedAt = now
	state.Mode = aman.ModeAuthoritative
	state.PolicyVersion = policyVersion
	return service, state, observation
}

func TestProviderFailureProducesFreshFallbackAndFeederETA(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	service, state, observation := fallbackFixture(t, now)
	flight, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
	require.NoError(t, err)
	require.NotNil(t, flight.Prediction)
	require.True(t, flight.Prediction.Publishable)
	require.Equal(t, aman.PredictionBasisFallback, flight.Prediction.Basis)
	require.Equal(t, aman.DataFresh, flight.DataStatus)
	require.Contains(t, *flight.Prediction.DegradationReason, "offline")
	require.NotNil(t, flight.Prediction.HoldingFixETA)
	require.NotNil(t, flight.FeederETA)
	require.Equal(t, now, flight.Prediction.InputObservedAt)
	state.Flights = []aman.AMANFlight{flight}
	service.resequence(&state, now)
	require.NotNil(t, state.Flights[0].Slot)
	state.Flights[0].Slot.Time = flight.Prediction.RawTETA.Add(2 * time.Minute)
	service.refreshHoldingPlans(&state)
	require.NotNil(t, state.Flights[0].Prediction.HoldingPlan)
	require.NotNil(t, state.Flights[0].FeederETA)
	require.Equal(t, state.Flights[0].Prediction.HoldingPlan.ApproachReleaseTime.Add(*flight.Prediction.HoldingToFeederTransit), *state.Flights[0].FeederETA.ETA)
	// Reprocessing the same observation must not turn processing delay into travel.
	same, err := service.reconcileFlight(t.Context(), state, state.Flights[0], observation, now.Add(30*time.Second))
	require.NoError(t, err)
	require.Equal(t, flight.Prediction.RawTETA, same.Prediction.RawTETA)
	service.deps.Materializer = fixedNavigation{key: "new-route"}
	recovered, err := service.reconcileFlight(t.Context(), state, same, observation, now.Add(time.Minute))
	require.NoError(t, err)
	require.NotEqual(t, aman.PredictionBasisFallback, recovered.Prediction.Basis)
}

func TestFallbackDoesNotInventTimingForDisconnectedOrStaleAircraft(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	for _, scenario := range []string{"disconnected", "stale", "missing-speed"} {
		t.Run(scenario, func(t *testing.T) {
			service, state, o := fallbackFixture(t, now)
			switch scenario {
			case "disconnected":
				o.SourceStatus = aman.DataDisconnected
			case "stale":
				old := now.Add(-3 * time.Minute)
				o.Surveillance.ObservedAt = &old
			case "missing-speed":
				o.Surveillance.GroundspeedKnots = nil
			}
			got, _ := service.reconcileFlight(t.Context(), state, newFlight(o, now), o, now)
			require.True(t, got.Prediction == nil || got.Prediction.Basis != aman.PredictionBasisFallback)
		})
	}
}

func TestInsertionWithSTARSelectionPersistsAtomicallyAndRetries(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	service, state, o := fallbackFixture(t, now)
	anchor, err := service.reconcileFlight(t.Context(), state, newFlight(o, now), o, now)
	require.NoError(t, err)
	state.Flights = []aman.AMANFlight{anchor}
	service.resequence(&state, now)
	state.Flights[0].Slot.Time = now.Add(10 * time.Minute)
	state.Flights[0].Slot.Sequence = 1
	state.Flights[0].Slot.Revision = 7
	targetObservation := o
	targetObservation.Callsign = "TARGET"
	targetObservation.AssignedSTAR = stringPointer("BAD-STAR")
	targetObservation.FiledRoute = stringPointer("BROKEN")
	target := newFlight(targetObservation, now)
	target.State = aman.StateAirborne
	target.LatestObservation = &targetObservation
	state.Flights = append(state.Flights, target)
	repository := &memoryRepository{has: true, state: state}
	publisher := &recordingPublisher{}
	coordinator, err := sequence.NewCoordinator(sequence.CoordinatorDependencies{States: repository, Outcomes: repository, Committer: repository, Publisher: publisher, Now: func() time.Time { return now.Add(time.Second) }})
	require.NoError(t, err)
	actions, err := sequence.NewActionService(coordinator, service)
	require.NoError(t, err)
	auth := aman.CommandContext{Airport: "EKCH", Actor: "123", Role: "EKDK_FMP", ReceivedAt: now}
	command := aman.InsertFlightAfterCommand{Metadata: aman.CommandMetadata{CommandID: "insert-target", ExpectedRevision: 7}, Callsign: "TARGET", AfterCallsign: anchor.Callsign, STARFamily: stringPointer("TESPI")}
	execution, err := actions.InsertFlightAfter(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, execution.Changed)
	inserted := stateFlight(t, repository.state, "TARGET")
	require.NotNil(t, inserted.Slot)
	require.Equal(t, "TESPI", *inserted.ControllerSTARFamily)
	require.Equal(t, anchor.Callsign, inserted.ManualPosition.AfterCallsign)
	require.Len(t, repository.commits, 1)
	encoded, err := json.Marshal(repository.state)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, &repository.state))
	retry, err := actions.InsertFlightAfter(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, retry.Duplicate)
	require.Len(t, repository.commits, 1)
	updated, err := service.reconcileFlight(t.Context(), repository.state, stateFlight(t, repository.state, "TARGET"), targetObservation, now.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, "TESPI", *updated.ControllerSTARFamily)
	require.NotNil(t, updated.ManualPosition)
	// Rejected stale-source insertion cannot allocate a revision or modify state.
	old := now.Add(-3 * time.Minute)
	repository.state.Flights[1].LatestObservation.Surveillance.ObservedAt = &old
	command.Metadata = aman.CommandMetadata{CommandID: "stale-insert", ExpectedRevision: repository.state.Revision}
	_, err = actions.InsertFlightAfter(context.Background(), auth, command)
	require.Error(t, err)
	require.Len(t, repository.commits, 1)
}

func TestInsertFlightAfterRetainsTMAFrozenTargetAssignment(t *testing.T) {
	for _, scenario := range []struct {
		name string
		star *string
	}{{"existing_star", nil}, {"selected_star", stringPointer("TESPI")}} {
		t.Run(scenario.name, func(t *testing.T) {
			now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
			service, state, observation := fallbackFixture(t, now)
			anchor, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
			require.NoError(t, err)
			state.Flights = []aman.AMANFlight{anchor}
			service.resequence(&state, now)
			state.Flights[0].Slot.Time = now.Add(10 * time.Minute)
			state.Flights[0].Slot.Revision = state.Revision
			target := state.Flights[0]
			target.Callsign, target.FreezeReason = "TARGET", aman.FreezeTMA
			updateLifecycle(&target, target.State, aman.StateStable, now)
			targetObservation := observation
			targetObservation.Callsign = target.Callsign
			target.LatestObservation = &targetObservation
			target.Slot = &aman.Slot{Time: now.Add(11 * time.Minute), RunwayGroupID: *target.SelectedRunwayGroup, Sequence: 2, Revision: state.Revision, Reason: "freeze_tma"}
			frozenSlot := *target.Slot
			frozenTETA := target.Prediction.OperationalTETA
			target.FrozenSlot, target.FrozenAt, target.FrozenOperationalTETA = &frozenSlot, &now, &frozenTETA
			state.Flights = append(state.Flights, target)
			repository := &memoryRepository{has: true, state: state}
			coordinator, err := sequence.NewCoordinator(sequence.CoordinatorDependencies{States: repository, Outcomes: repository, Committer: repository, Publisher: &recordingPublisher{}, Now: func() time.Time { return now.Add(time.Second) }})
			require.NoError(t, err)
			actions, err := sequence.NewActionService(coordinator, service)
			require.NoError(t, err)
			_, err = actions.InsertFlightAfter(t.Context(), aman.CommandContext{Airport: "EKCH", Actor: "123", Role: "EKDK_FMP", ReceivedAt: now}, aman.InsertFlightAfterCommand{Metadata: aman.CommandMetadata{CommandID: "insert-tma", ExpectedRevision: state.Revision}, Callsign: target.Callsign, AfterCallsign: anchor.Callsign, STARFamily: scenario.star})
			require.NoError(t, err)
			inserted := stateFlight(t, repository.state, target.Callsign)
			require.Equal(t, aman.FreezeTMA, inserted.FreezeReason)
			require.Equal(t, target.SelectedRunwayGroup, inserted.SelectedRunwayGroup)
			require.False(t, inserted.RunwayChangePending)
			require.Equal(t, anchor.Callsign, inserted.ManualPosition.AfterCallsign)
			require.True(t, inserted.Slot.Time.After(state.Flights[0].Slot.Time))
			require.Equal(t, inserted.Slot.Time, inserted.FrozenSlot.Time)
			require.Len(t, repository.commits, 1)
			require.Equal(t, now.Add(11*time.Minute), state.Flights[1].Slot.Time, "the command must not modify the source aggregate")
		})
	}
}

func TestFailedPrimaryAndFallbackRetainStaleGuidanceWithFreshSurveillance(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	service, state, observation := fallbackFixture(t, now)
	flight, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
	require.NoError(t, err)
	state.Flights = []aman.AMANFlight{flight}
	service.resequence(&state, now)
	flight = state.Flights[0]
	wantSlot, wantPrediction := flight.Slot.Time, flight.Prediction.RawTETA
	service.deps.Geometry = unavailableGeometry{}
	updated, err := service.reconcileFlight(t.Context(), state, flight, observation, now.Add(time.Second))
	require.Error(t, err)
	updated = applyUnavailablePrediction(updated, observation, now.Add(time.Second), err)
	require.Equal(t, aman.DataFresh, updated.LatestObservation.SourceStatus, "the aircraft observations remain current")
	require.Equal(t, aman.DataStale, updated.DataStatus, "clients must render retained guidance as stale when neither prediction path works")
	require.NotNil(t, updated.PredictionFailureReason)
	require.Equal(t, wantPrediction, updated.Prediction.RawTETA)
	require.Equal(t, wantSlot, updated.Slot.Time)
	_, available := aman.GainLossPrediction(updated)
	require.True(t, available, "last-known guidance remains available with an explicit stale status")
}

func TestInsertUnslottedFlightUsesAnchorRunwayWithParallelRunwaysActive(t *testing.T) {
	for _, assigned := range []bool{false, true} {
		t.Run(map[bool]string{false: "unassigned", true: "assigned_to_primary"}[assigned], func(t *testing.T) {
			now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
			service, state, observation := fallbackFixture(t, now)
			primary, secondary := state.RunwayGroups[0].ID, aman.RunwayGroupID("ARRIVAL-22R")
			policy := state.RunwayGroups[0]
			policy.ID, policy.Selected = secondary, false
			state.RunwayGroups = append(state.RunwayGroups, policy)
			state.ActiveRunwayGroups = []aman.RunwayGroupID{primary, secondary}
			geometry := service.deps.Geometry.(arrivalChangeGeometry)
			secondaryPath := geometry.path
			secondaryPath.RunwayGroup, secondaryPath.Digest = secondary, "secondary-terminal"
			service.deps.Geometry = runwaySTARGeometry{arrivalChangeGeometry: geometry, paths: []navdata.TerminalPath{geometry.path, secondaryPath}}
			service.deps.Terminal.Paths = append(service.deps.Terminal.Paths, terminal.Path{Feeder: "TESPI", RunwayGroup: secondary, STARSuffix: "1A"})
			service.deps.Terminal.RunwayGroups = append(service.deps.Terminal.RunwayGroups, terminal.RunwayGroup{ID: secondary})
			anchor, err := service.fallbackPredictionForRunway(t.Context(), state, newFlight(observation, now), observation, nil, secondary, now, "provider outage")
			require.NoError(t, err)
			anchor.Slot = &aman.Slot{Time: now.Add(10 * time.Minute), RunwayGroupID: secondary, Sequence: 1, Revision: state.Revision}
			targetObservation := observation
			targetObservation.Callsign = "TARGET"
			target := newFlight(targetObservation, now)
			target.State, target.LatestObservation = aman.StateAirborne, &targetObservation
			if assigned {
				target.SelectedRunwayGroup = &primary
			}
			state.Flights = []aman.AMANFlight{anchor, target}
			repository := &memoryRepository{has: true, state: state}
			coordinator, err := sequence.NewCoordinator(sequence.CoordinatorDependencies{States: repository, Outcomes: repository, Committer: repository, Publisher: &recordingPublisher{}, Now: func() time.Time { return now.Add(time.Second) }})
			require.NoError(t, err)
			actions, err := sequence.NewActionService(coordinator, service)
			require.NoError(t, err)
			_, err = actions.InsertFlightAfter(t.Context(), aman.CommandContext{Airport: "EKCH", Actor: "123", Role: "EKDK_FMP", ReceivedAt: now}, aman.InsertFlightAfterCommand{Metadata: aman.CommandMetadata{CommandID: "insert-secondary", ExpectedRevision: state.Revision}, Callsign: target.Callsign, AfterCallsign: anchor.Callsign})
			require.NoError(t, err)
			inserted := stateFlight(t, repository.state, target.Callsign)
			require.Equal(t, secondary, *inserted.SelectedRunwayGroup)
			require.Equal(t, secondary, inserted.Slot.RunwayGroupID)
			require.Equal(t, secondary, inserted.FallbackRouteProgress.RunwayGroupID)
			require.True(t, inserted.Slot.Time.After(anchor.Slot.Time))
			require.Equal(t, anchor.Slot.Time, stateFlight(t, repository.state, anchor.Callsign).Slot.Time)
			require.Len(t, repository.commits, 1)
			// Controller order must survive persistence and a normal update,
			// even while another active runway remains the default selection.
			encoded, err := json.Marshal(repository.state)
			require.NoError(t, err)
			var restored aman.AirportState
			require.NoError(t, json.Unmarshal(encoded, &restored))
			for _, lifecycle := range []aman.FlightState{aman.StateAirborne, aman.StateUnstable} {
				t.Run(string(lifecycle), func(t *testing.T) {
					nextAt := now.Add(time.Second)
					current := stateFlight(t, restored, target.Callsign)
					updateLifecycle(&current, current.State, lifecycle, now)
					current.FreezeReason = aman.FreezeNone
					nextObservation := targetObservation
					position := *targetObservation.Surveillance
					position.ObservedAt = &nextAt
					nextObservation.Surveillance, nextObservation.ReconciledAt = &position, nextAt
					// Primary prediction may also recover during this update.
					service.deps.Materializer = fixedNavigation{key: "new-route"}
					reconciled, err := service.reconcileFlight(t.Context(), restored, current, nextObservation, nextAt)
					require.NoError(t, err)
					require.Equal(t, secondary, *reconciled.SelectedRunwayGroup)
					require.Equal(t, anchor.Callsign, reconciled.ManualPosition.AfterCallsign)
					require.Nil(t, reconciled.ManualOrder)
					require.False(t, reconciled.RunwayChangePending)
					nextState := restored
					nextState.Flights = append([]aman.AMANFlight(nil), restored.Flights...)
					nextState.Flights[1] = reconciled
					service.resequence(&nextState, nextAt)
					after := stateFlight(t, nextState, target.Callsign)
					require.Equal(t, secondary, after.Slot.RunwayGroupID)
					require.Greater(t, after.Slot.Sequence, stateFlight(t, nextState, anchor.Callsign).Slot.Sequence)
				})
			}
		})
	}
}

func TestFallbackPreservesRejectedSurveillanceProgressAndRecovers(t *testing.T) {
	for _, scenario := range []string{"primary_rejection", "cached_route_during_provider_failure"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
			service, state, observation := fallbackFixture(t, now)
			geometry := service.deps.Geometry.(arrivalChangeGeometry)
			// A long continuous route exposes a jump while terminal geometry
			// remains usable without the accepted physical anchor.
			for i := range geometry.fixes {
				geometry.fixes[i].Position = navdata.Coordinate{LatitudeDeg: 55, LongitudeDeg: 12 + float64(i)}
			}
			service.deps.Geometry, service.deps.Materializer = geometry, fixedNavigation{key: "new-route"}
			position := *observation.Surveillance
			position.LatitudeDegrees, position.LongitudeDegrees = 55, 12.01
			observation.Surveillance = &position
			initial, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
			require.NoError(t, err)
			require.NotNil(t, initial.RouteProgress)
			require.Nil(t, initial.FallbackRouteProgress)
			state.Flights = []aman.AMANFlight{initial}
			service.resequence(&state, now)
			initial = state.Flights[0]
			oldProgress, oldPrediction, oldSlot := *initial.RouteProgress, *initial.Prediction, *initial.Slot
			at := now.Add(time.Second)
			rejectedObservation := observation
			jumped := position
			jumped.LongitudeDegrees, jumped.ObservedAt = 14.8, &at
			rejectedObservation.Surveillance, rejectedObservation.ReconciledAt = &jumped, at
			snapshot, err := geometry.ActiveGeometrySnapshot(t.Context(), "EKCH")
			require.NoError(t, err)
			unanchored, err := trajectory.ProjectTerminalFallback(snapshot, geometry.path, trajectory.Input{Airport: "EKCH", Feeder: "TESPI", FeederFix: geometry.path.FeederFix, RunwayGroup: state.RunwayGroups[0].ID, Observation: jumped}, trajectory.Config{ReferenceTime: at, MaxObservationAge: tmaSurveillanceFresh})
			require.NoError(t, err)
			require.NotNil(t, unanchored.DistanceToGoNM)
			var updated aman.AMANFlight
			if scenario == "primary_rejection" {
				updated, err = service.reconcileFlight(t.Context(), state, initial, rejectedObservation, at)
			} else {
				updated, err = service.fallbackPrediction(t.Context(), state, initial, rejectedObservation, initial.LatestObservation, at, "provider offline")
			}
			require.ErrorContains(t, err, "FORWARD_PROGRESS_OUT_OF_RANGE")
			updated = applyUnavailablePrediction(updated, rejectedObservation, at, err)
			require.Equal(t, aman.DataStale, updated.DataStatus)
			require.Equal(t, oldProgress, *updated.RouteProgress)
			require.Nil(t, updated.FallbackRouteProgress)
			require.Equal(t, oldPrediction.RawTETA, updated.Prediction.RawTETA)
			require.Equal(t, oldPrediction.InputObservedAt, updated.Prediction.InputObservedAt)
			require.Equal(t, oldSlot, *updated.Slot)
			require.NotEqual(t, aman.PredictionBasisFallback, updated.Prediction.Basis)
			// Corrected data recovers without resetting the physical anchor.
			recoveredAt := now.Add(time.Minute)
			if scenario == "cached_route_during_provider_failure" {
				recoveredAt = now.Add(20 * time.Minute)
			}
			recoveredObservation := observation
			corrected := position
			corrected.LongitudeDegrees, corrected.ObservedAt = 12.1, &recoveredAt
			recoveredObservation.Surveillance, recoveredObservation.ReconciledAt = &corrected, recoveredAt
			if scenario == "primary_rejection" {
				updated, err = service.reconcileFlight(t.Context(), state, updated, recoveredObservation, recoveredAt)
				require.NoError(t, err)
				require.NotEqual(t, aman.PredictionBasisFallback, updated.Prediction.Basis)
			} else {
				updated, err = service.fallbackPrediction(t.Context(), state, updated, recoveredObservation, updated.LatestObservation, recoveredAt, "provider offline")
				require.NoError(t, err)
				require.Equal(t, aman.PredictionBasisFallback, updated.Prediction.Basis)
				require.Equal(t, geometry.route.Digest, updated.FallbackRouteProgress.GeometryDigest)
			}
			require.Equal(t, aman.DataFresh, updated.DataStatus)
			require.Equal(t, recoveredAt, updated.Prediction.InputObservedAt)
			require.Equal(t, oldSlot, *updated.Slot)
			if scenario == "cached_route_during_provider_failure" {
				// A persisted cached-route fallback must use its newer physical
				// anchor, rather than the much older normal-prediction progress.
				stored, marshalErr := json.Marshal(updated)
				require.NoError(t, marshalErr)
				var restored aman.AMANFlight
				require.NoError(t, json.Unmarshal(stored, &restored))
				nextAt := recoveredAt.Add(time.Second)
				jumped.ObservedAt = &nextAt
				rejectedObservation.Surveillance, rejectedObservation.ReconciledAt = &jumped, nextAt
				rejected, rejectionErr := service.fallbackPrediction(t.Context(), state, restored, rejectedObservation, restored.LatestObservation, nextAt, "provider offline")
				require.ErrorContains(t, rejectionErr, "FORWARD_PROGRESS_OUT_OF_RANGE")
				require.Equal(t, restored.FallbackRouteProgress, rejected.FallbackRouteProgress)
				require.Equal(t, restored.Prediction.RawTETA, rejected.Prediction.RawTETA)
			}
		})
	}
}

type descentOutageGeometry struct{ arrivalChangeGeometry }

func (g descentOutageGeometry) ActiveVersion(context.Context, navdata.AirportID) (navdata.DatasetVersion, error) {
	return navdata.DatasetVersion{}, &aman.DomainError{Class: aman.ErrorDependencyUnavailable, Message: "route version provider unavailable"}
}

func TestFallbackAccumulatesDescentEvidenceAcrossOutageAndRestart(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(map[bool]string{false: "basic_join", true: "cached_route"}[cached], func(t *testing.T) {
			now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
			service, state, observation := fallbackFixture(t, now)
			geometry := service.deps.Geometry.(arrivalChangeGeometry)
			geometry.fixes[0].Position = navdata.Coordinate{LatitudeDeg: 53, LongitudeDeg: 10}
			for i := 1; i < len(geometry.fixes); i++ {
				geometry.fixes[i].Position = navdata.Coordinate{LatitudeDeg: 56 + float64(i-1)*.1, LongitudeDeg: 13 + float64(i-1)*.1}
			}
			service.deps.Geometry = geometry
			observation.AircraftType, observation.WakeCategory = stringPointer("A320"), stringPointer("M")
			requested := 35000
			observation.RequestedLevel = &requested
			baselinePosition := *observation.Surveillance
			baselinePosition.LatitudeDegrees, baselinePosition.LongitudeDegrees = 54, 11
			observation.Surveillance = &baselinePosition
			baselineAltitude := 30000
			baselinePosition.AltitudeFeet = &baselineAltitude
			if cached {
				service.deps.Materializer = fixedNavigation{key: "new-route"}
			}
			flight, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
			require.NoError(t, err)
			if cached {
				require.NotNil(t, flight.RouteProgress)
				require.False(t, flight.RouteProgress.DescentConfirmed)
				service.deps.Geometry = descentOutageGeometry{arrivalChangeGeometry: geometry}
			} else {
				require.Equal(t, "BASIC_JOIN", flight.Prediction.Calculation.Legs[0].ID)
				require.Nil(t, flight.FallbackRouteProgress, "a joining estimate must not fabricate a route anchor")
			}
			for i, altitude := range []int{29800, 29600, 29700} {
				at := now.Add(time.Duration(i+1) * 15 * time.Second)
				next := observation
				position := baselinePosition
				position.ObservedAt, position.AltitudeFeet = &at, &altitude
				next.Surveillance, next.ReconciledAt = &position, at
				flight, err = service.reconcileFlight(t.Context(), state, flight, next, at)
				require.NoError(t, err)
				require.Equal(t, aman.PredictionBasisFallback, flight.Prediction.Basis)
				require.NotNil(t, flight.FallbackDescent)
				require.EqualValues(t, min(i+1, 2), flight.FallbackDescent.Samples)
				require.Equal(t, i >= 1, flight.FallbackDescent.Confirmed)
				if cached {
					require.Equal(t, flight.FallbackDescent.Confirmed, flight.FallbackRouteProgress.DescentConfirmed)
					require.Zero(t, flight.RouteProgress.DescentEvidenceSamples, "fallback must not mutate the original physical anchor")
				} else {
					require.Nil(t, flight.FallbackRouteProgress)
				}
				if i == 0 {
					// Persist partial evidence and replay the same observation;
					// a duplicate must neither confirm nor erase descent.
					stored, marshalErr := json.Marshal(flight)
					require.NoError(t, marshalErr)
					var restored aman.AMANFlight
					require.NoError(t, json.Unmarshal(stored, &restored))
					flight, err = service.reconcileFlight(t.Context(), state, restored, next, at)
					require.NoError(t, err)
					require.Equal(t, &aman.DescentEvidence{Samples: 1}, flight.FallbackDescent)
				} else {
					require.NotEmpty(t, flight.Prediction.Calculation.Segments)
					for _, segment := range flight.Prediction.Calculation.Segments {
						require.LessOrEqual(t, segment.EndAltitudeFeet, segment.StartAltitudeFeet, "confirmed descent must not model a return to filed cruise")
					}
				}
			}
			// Provider recovery transfers the latched evidence to ordinary
			// route progress, then clears fallback-specific state.
			service.deps.Geometry, service.deps.Materializer = geometry, fixedNavigation{key: "new-route"}
			at := now.Add(time.Minute)
			recoveredObservation := *flight.LatestObservation
			recoveredPosition := *recoveredObservation.Surveillance
			recoveredPosition.ObservedAt = &at
			recoveredObservation.Surveillance, recoveredObservation.ReconciledAt = &recoveredPosition, at
			recovered, err := service.reconcileFlight(t.Context(), state, flight, recoveredObservation, at)
			require.NoError(t, err)
			require.NotEqual(t, aman.PredictionBasisFallback, recovered.Prediction.Basis)
			require.True(t, recovered.RouteProgress.DescentConfirmed)
			require.Nil(t, recovered.FallbackDescent)
			require.Nil(t, recovered.FallbackRouteProgress)
			// An actual new arrival path starts with fresh descent evidence.
			invalidateRunwayPath(&flight)
			require.Nil(t, flight.FallbackDescent)
			require.Nil(t, flight.FallbackRouteProgress)
		})
	}
}

func TestFallbackDirectToImmediatelyReplacesTimingAndClearsBypassedHolding(t *testing.T) {
	for _, freeze := range []aman.FreezeReason{aman.FreezeNone, aman.FreezeManual, aman.FreezeSuperstable, aman.FreezeTMA} {
		t.Run(string(freeze), func(t *testing.T) {
			now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
			service, state, observation := fallbackFixture(t, now)
			geometry := service.deps.Geometry.(arrivalChangeGeometry)
			// The confirmed direct skips a long terminal dogleg and its hold.
			geometry.fixes[2].Position = navdata.Coordinate{LatitudeDeg: 55.8, LongitudeDeg: 11.2}
			service.deps.Geometry = geometry
			initial, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
			require.NoError(t, err)
			require.Equal(t, aman.PredictionBasisFallback, initial.Prediction.Basis)
			require.NotNil(t, initial.Prediction.HoldingFixETA)
			initial.State, initial.FreezeReason = aman.StateStable, freeze
			if freeze == aman.FreezeNone {
				initial.State = aman.StateAirborne
			}
			initial.Slot = &aman.Slot{Time: now.Add(35 * time.Minute), RunwayGroupID: *initial.SelectedRunwayGroup, Sequence: 1, Revision: state.Revision}
			if freeze != aman.FreezeNone {
				slot, eta := *initial.Slot, initial.Prediction.OperationalTETA
				initial.FrozenSlot, initial.FrozenOperationalTETA, initial.FrozenAt = &slot, &eta, &now
			}
			entered := now.Add(-5 * time.Minute)
			initial.HoldingClearance = &aman.HoldingClearance{Hold: "HOLD", HoldType: aman.HoldingClearanceEnroute, ObservedAt: entered}
			initial.HoldingStack = &aman.HoldingStackState{HoldingID: "TESPI-HOLD", FirstObservedAt: entered, CandidateObservedAt: now, ConsecutiveObservations: 2, Confirmed: true}
			initial.HoldingReleaseBasis = &aman.HoldingReleaseBasis{HoldingID: "TESPI-HOLD", Fix: "HOLD", RunwayGroupID: *initial.SelectedRunwayGroup, RouteDigest: "held-journey", HoldingEntryTime: entered, PostHoldingTransit: 5 * time.Minute}
			require.NotNil(t, activeHoldingReleaseBasis(initial))
			oldSlot, oldTETA, oldRaw := *initial.Slot, initial.Prediction.OperationalTETA, initial.Prediction.RawTETA
			directAt := now.Add(time.Second)
			initial.ActiveRouteFact = &aman.RouteFact{ID: "fallback-direct", Callsign: initial.Callsign, Fix: "END", State: aman.RouteFactActive, DatasetVersion: navigationDatasetID(geometry.version), ObservedAt: directAt, ReceivedAt: directAt}
			// The command reconciliation uses the existing radar report.
			updated, err := service.reconcileFlight(t.Context(), state, initial, observation, directAt)
			require.NoError(t, err)
			require.Equal(t, aman.PredictionBasisFallback, updated.Prediction.Basis)
			require.True(t, updated.Prediction.RawTETA.Before(oldRaw.Add(-3*time.Minute)))
			require.Equal(t, now, updated.Prediction.InputObservedAt)
			require.Equal(t, []string{"END"}, updated.Prediction.RemainingRouteFixes)
			require.Equal(t, initial.ActiveRouteFact.ID, updated.FallbackRouteProgress.RouteFactID)
			require.Nil(t, updated.RouteProgress)
			require.Nil(t, updated.HoldingReleaseBasis)
			require.Nil(t, updated.Prediction.HoldingFixETA)
			require.Nil(t, updated.Prediction.HoldingPlan)
			require.Len(t, updated.RawTETASamples, 1, "the confirmed route revision discards pre-direct smoothing")
			require.Nil(t, updated.PredictionFilter.Pending)
			require.False(t, updated.PredictionFilter.Unreliable)
			require.Equal(t, freeze, updated.FreezeReason)
			require.Equal(t, oldSlot, *updated.Slot)
			if freeze == aman.FreezeNone {
				require.Equal(t, updated.Prediction.RawTETA, updated.Prediction.OperationalTETA)
				require.Equal(t, aman.OperationalReasonRouteRevision, updated.Prediction.OperationalReason)
			} else {
				require.Equal(t, oldTETA, updated.Prediction.OperationalTETA)
				require.Equal(t, oldSlot, *updated.FrozenSlot)
			}
			// Replaying the same accepted direct after restart must not turn
			// each fallback update into another forced route revision.
			restored := cloneGapState(t, aman.AirportState{Flights: []aman.AMANFlight{updated}}).Flights[0]
			replayed, err := service.reconcileFlight(t.Context(), state, restored, observation, directAt.Add(time.Second))
			require.NoError(t, err)
			require.Equal(t, restored.Prediction.GeneratedAt, replayed.Prediction.GeneratedAt)
			require.Equal(t, restored.RawTETASamples, replayed.RawTETASamples)
		})
	}
}

func TestUnresolvedFallbackDirectDoesNotBypassPredictionFilter(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	service, state, observation := fallbackFixture(t, now)
	flight, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
	require.NoError(t, err)
	priorPrediction := *flight.Prediction
	flight.ActiveRouteFact = &aman.RouteFact{ID: "unknown-fallback-direct", Fix: "UNKNOWN", State: aman.RouteFactActive, ObservedAt: now, ReceivedAt: now}
	updated, err := service.reconcileFlight(t.Context(), state, flight, observation, now.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, priorPrediction.GeneratedAt, updated.Prediction.GeneratedAt)
	require.Equal(t, priorPrediction.RawTETA, updated.Prediction.RawTETA)
	require.Equal(t, flight.RawTETASamples, updated.RawTETASamples)
}

func TestCachedFallbackSupportsDirectToBeforeSTARDuringProviderOutage(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	service, state, observation := fallbackFixture(t, now)
	geometry := service.deps.Geometry.(arrivalChangeGeometry)
	geometry.fixes[0].Position = navdata.Coordinate{LatitudeDeg: 53, LongitudeDeg: 10}
	for i := 1; i < len(geometry.fixes); i++ {
		geometry.fixes[i].Position = navdata.Coordinate{LatitudeDeg: 56 + float64(i-1)*.1, LongitudeDeg: 13 + float64(i-1)*.1}
	}
	enroute, origin, entry := navdata.FixID("ENROUTE"), navdata.FixID("ORIGIN"), navdata.FixID("ENTRY")
	geometry.fixes = append(geometry.fixes, navdata.Fix{ID: enroute, Position: navdata.Coordinate{LatitudeDeg: 54.5, LongitudeDeg: 11.5}})
	geometry.route.Legs = []navdata.ProcedureLeg{
		{ID: "ORIGIN-ENROUTE", PathTerminator: navdata.PathTF, FromFix: &origin, ToFix: &enroute},
		{ID: "ENROUTE-ENTRY", PathTerminator: navdata.PathTF, FromFix: &enroute, ToFix: &entry},
	}
	for i := range geometry.fixes {
		geometry.fixes[i].Provenance = navdata.Provenance{SourceID: "fixture", SourceRevision: "test", ImportedAt: now.Add(-time.Hour), EffectiveFrom: geometry.version.EffectiveFrom, EffectiveUntil: geometry.version.EffectiveUntil}
	}
	service.deps.Geometry, service.deps.Materializer = geometry, fixedNavigation{key: "new-route"}
	position := *observation.Surveillance
	position.LatitudeDegrees, position.LongitudeDegrees = 54, 11
	observation.Surveillance = &position
	direct := &aman.RouteFact{ID: "before-star-direct", Callsign: observation.Callsign, Fix: string(enroute), State: aman.RouteFactActive, DatasetVersion: navigationDatasetID(geometry.version), ObservedAt: now, ReceivedAt: now}
	initial := newFlight(observation, now)
	initial.ActiveRouteFact = direct
	initial, err := service.reconcileFlight(t.Context(), state, initial, observation, now)
	require.NoError(t, err)
	require.NotNil(t, initial.ActiveRouteKey)
	require.Equal(t, direct.ID, initial.RouteProgress.RouteFactID)
	state.Flights = []aman.AMANFlight{initial}
	service.resequence(&state, now)
	initial = state.Flights[0]
	require.NotNil(t, initial.Slot)
	// Terminal-only geometry cannot honor this enroute direct. That failure
	// must not prevent the complete cached route from supplying guidance.
	snapshot, err := geometry.ActiveGeometrySnapshot(t.Context(), "EKCH")
	require.NoError(t, err)
	_, err = trajectory.ProjectTerminalFallback(snapshot, geometry.path, trajectory.Input{Airport: "EKCH", Feeder: "TESPI", FeederFix: geometry.path.FeederFix, RunwayGroup: *initial.SelectedRunwayGroup, RouteFact: direct, Observation: position}, trajectory.Config{ReferenceTime: now, MaxObservationAge: tmaSurveillanceFresh})
	require.ErrorContains(t, err, "DIRECT_TO_TARGET_NOT_ON_FORWARD_PATH")
	service.deps.Geometry = descentOutageGeometry{arrivalChangeGeometry: geometry}
	at := now.Add(15 * time.Second)
	next := observation
	position.ObservedAt = &at
	next.Surveillance, next.ReconciledAt = &position, at
	// Without a usable cached route, retain the last accepted timing instead
	// of inventing a STAR join that ignores the controller's direct clearance.
	missing := initial
	missing.ActiveRouteKey = nil
	rejected, err := service.reconcileFlight(t.Context(), state, missing, next, at)
	require.Error(t, err)
	require.Equal(t, initial.Prediction.RawTETA, rejected.Prediction.RawTETA)
	require.Equal(t, initial.Slot, rejected.Slot)
	amended := next
	amended.FiledRoute = stringPointer("DCT AMENDED DCT TESPI")
	rejectedAmendment, err := service.reconcileFlight(t.Context(), state, initial, amended, at)
	require.Error(t, err, "a valid old cache must not substitute for the amended filed route")
	require.Equal(t, initial.Prediction.RawTETA, rejectedAmendment.Prediction.RawTETA)
	require.Equal(t, initial.ActiveRouteQueryKey, rejectedAmendment.ActiveRouteQueryKey)
	fallback, err := service.reconcileFlight(t.Context(), state, initial, next, at)
	require.NoError(t, err)
	require.Equal(t, aman.DataFresh, fallback.DataStatus)
	require.Equal(t, aman.PredictionBasisFallback, fallback.Prediction.Basis)
	require.Equal(t, at, fallback.Prediction.InputObservedAt)
	require.Equal(t, []string{"ENROUTE", "ENTRY", "HOLD", "END"}, fallback.Prediction.RemainingRouteFixes)
	require.Equal(t, geometry.route.Digest, fallback.FallbackRouteProgress.GeometryDigest)
	require.Equal(t, direct.ID, fallback.FallbackRouteProgress.RouteFactID)
	require.Equal(t, initial.RouteProgress, fallback.RouteProgress)
	require.Equal(t, initial.Slot, fallback.Slot)
	require.NotNil(t, fallback.Prediction.HoldingFixETA)
	require.NotNil(t, fallback.FeederETA)
	// A cached-route fallback must still validate its existing enroute direct,
	// even though that target is absent from terminal-only geometry.
	liveState := state
	liveState.SessionID, liveState.Flights = 42, []aman.AMANFlight{fallback}
	liveRepo := &memoryRepository{has: true, state: liveState}
	live, err := routefact.New(routefact.Dependencies{Repository: liveRepo, Strips: fallbackTrackingReader{}, Geometry: geometry, Publisher: &recordingPublisher{}, Reconciler: fallbackFactReconciler(func(context.Context) { t.Fatal("an unchanged direct must not trigger reconciliation") }), Now: func() time.Time { return at }})
	require.NoError(t, err)
	enrouteTarget := string(enroute)
	require.NoError(t, live.ReportDirectTo(t.Context(), 42, "EKCH", fallback.Callsign, "EKCH_A_APP", &enrouteTarget, at))
	require.Empty(t, liveRepo.commits)
	// Recovery after restart preserves the direct and its committed slot.
	restored := cloneGapState(t, aman.AirportState{Flights: []aman.AMANFlight{fallback}}).Flights[0]
	service.deps.Geometry = geometry
	recoveredAt := at.Add(15 * time.Second)
	position.ObservedAt = &recoveredAt
	next.Surveillance, next.ReconciledAt = &position, recoveredAt
	recovered, err := service.reconcileFlight(t.Context(), state, restored, next, recoveredAt)
	require.NoError(t, err)
	require.NotEqual(t, aman.PredictionBasisFallback, recovered.Prediction.Basis)
	require.Equal(t, direct.ID, recovered.RouteProgress.RouteFactID)
	require.Equal(t, initial.Slot, recovered.Slot)
	require.Nil(t, recovered.FallbackRouteProgress)
}

func TestPrimaryRecoveryUsesLatestCompatibleCachedFallbackAnchor(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	service, state, observation := fallbackFixture(t, now)
	geometry := service.deps.Geometry.(arrivalChangeGeometry)
	for i, longitude := range []float64{12, 16, 17, 18} {
		geometry.fixes[i].Position = navdata.Coordinate{LatitudeDeg: 55, LongitudeDeg: longitude}
	}
	service.deps.Geometry, service.deps.Materializer = geometry, fixedNavigation{key: "new-route"}
	position := *observation.Surveillance
	position.LatitudeDegrees, position.LongitudeDegrees = 55, 12.01
	observation.Surveillance = &position
	initial, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
	require.NoError(t, err)
	state.Flights = []aman.AMANFlight{initial}
	service.resequence(&state, now)
	initial = state.Flights[0]
	oldAnchor, oldSlot := *initial.RouteProgress, *initial.Slot
	service.deps.Geometry = descentOutageGeometry{arrivalChangeGeometry: geometry}
	outageAt := now.Add(10 * time.Minute)
	outageObservation := observation
	outagePosition := position
	outagePosition.LongitudeDegrees, outagePosition.ObservedAt = 13, &outageAt
	outageObservation.Surveillance, outageObservation.ReconciledAt = &outagePosition, outageAt
	fallback, err := service.reconcileFlight(t.Context(), state, initial, outageObservation, outageAt)
	require.NoError(t, err)
	require.Equal(t, aman.PredictionBasisFallback, fallback.Prediction.Basis)
	require.Equal(t, oldAnchor, *fallback.RouteProgress)
	require.Equal(t, outageAt, *fallback.FallbackRouteProgress.ObservedAt)
	// Provider recovery must not erase the immediately preceding physical
	// anchor, including when the persisted outage state has been restarted.
	restored := cloneGapState(t, aman.AirportState{Flights: []aman.AMANFlight{fallback}}).Flights[0]
	service.deps.Geometry = geometry
	jumpAt := outageAt.Add(time.Second)
	jumped := observation
	jumpPosition := position
	jumpPosition.LongitudeDegrees, jumpPosition.ObservedAt = 15, &jumpAt
	jumped.Surveillance, jumped.ReconciledAt = &jumpPosition, jumpAt
	rejected, err := service.reconcileFlight(t.Context(), state, restored, jumped, jumpAt)
	require.ErrorContains(t, err, "FORWARD_PROGRESS_OUT_OF_RANGE")
	rejected = applyUnavailablePrediction(rejected, jumped, jumpAt, err)
	require.Equal(t, aman.DataStale, rejected.DataStatus)
	require.Equal(t, restored.FallbackRouteProgress, rejected.FallbackRouteProgress)
	require.Equal(t, restored.Prediction.RawTETA, rejected.Prediction.RawTETA)
	require.Equal(t, oldSlot, *rejected.Slot)
	legacy := restored
	legacyPrior := *legacy.RouteProgress
	legacyPrior.ObservedAt = nil
	legacy.RouteProgress = &legacyPrior
	_, err = service.reconcileFlight(t.Context(), state, legacy, jumped, jumpAt)
	require.ErrorContains(t, err, "FORWARD_PROGRESS_OUT_OF_RANGE", "legacy progress timestamps must not hide the newer fallback anchor")
	correctedAt := jumpAt.Add(time.Second)
	corrected := observation
	correctedPosition := position
	correctedPosition.LongitudeDegrees, correctedPosition.ObservedAt = 13.02, &correctedAt
	corrected.Surveillance, corrected.ReconciledAt = &correctedPosition, correctedAt
	recovered, err := service.reconcileFlight(t.Context(), state, rejected, corrected, correctedAt)
	require.NoError(t, err)
	require.NotEqual(t, aman.PredictionBasisFallback, recovered.Prediction.Basis)
	require.Equal(t, correctedAt, *recovered.RouteProgress.ObservedAt)
	require.Nil(t, recovered.FallbackRouteProgress)
	require.Equal(t, oldSlot, *recovered.Slot)
	jumpPosition.ObservedAt = timePointer(correctedAt.Add(time.Second))
	jumped.Surveillance, jumped.ReconciledAt = &jumpPosition, *jumpPosition.ObservedAt
	_, err = service.reconcileFlight(t.Context(), state, recovered, jumped, jumped.ReconciledAt)
	require.ErrorContains(t, err, "FORWARD_PROGRESS_OUT_OF_RANGE", "normal recovery must keep the routine jump guard")
}

func TestTMAFrozenFallbackRetainsRevisionAndPhysicalAnchorAfterAdministrativeAmendment(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	service, state, observation := fallbackFixture(t, now)
	geometry := service.deps.Geometry.(arrivalChangeGeometry)
	for i, longitude := range []float64{12, 16, 17, 18} {
		geometry.fixes[i].Position = navdata.Coordinate{LatitudeDeg: 55, LongitudeDeg: longitude}
	}
	service.deps.Geometry = geometry
	position := *observation.Surveillance
	position.LatitudeDegrees, position.LongitudeDegrees = 55, 16.01
	observation.Surveillance = &position
	flight, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
	require.NoError(t, err)
	require.Nil(t, flight.ActiveRouteKey)
	require.Nil(t, flight.RouteProgress)
	require.NotNil(t, flight.FallbackRouteProgress)
	state.Flights = []aman.AMANFlight{flight}
	service.resequence(&state, now)
	flight = state.Flights[0]
	updateLifecycle(&flight, flight.State, aman.StateStable, now)
	captured, frozenETA := *flight.Slot, flight.Prediction.OperationalTETA
	flight.FreezeReason, flight.FrozenSlot, flight.FrozenOperationalTETA, flight.FrozenAt = aman.FreezeTMA, &captured, &frozenETA, &now
	restored := cloneGapState(t, aman.AirportState{Flights: []aman.AMANFlight{flight}}).Flights[0]
	revision := *observation.FlightPlan.Revision + 1
	at := now.Add(time.Second)
	amended := observation
	amended.FlightPlan.Revision = &revision
	jumped := position
	jumped.LongitudeDegrees, jumped.ObservedAt = 17.6, &at
	amended.Surveillance, amended.ReconciledAt = &jumped, at
	rejected, err := service.reconcileFlight(t.Context(), state, restored, amended, at)
	require.Error(t, err)
	require.Contains(t, stringValue(rejected.PredictionFailureReason), "FORWARD_PROGRESS_OUT_OF_RANGE")
	require.Equal(t, restored.FallbackRouteProgress, rejected.FallbackRouteProgress)
	require.Equal(t, restored.Prediction.RawTETA, rejected.Prediction.RawTETA)
	validAt := at.Add(time.Second)
	corrected := position
	corrected.LongitudeDegrees, corrected.ObservedAt = 16.02, &validAt
	amended.Surveillance, amended.ReconciledAt = &corrected, validAt
	accepted, err := service.reconcileFlight(t.Context(), state, rejected, amended, validAt)
	require.NoError(t, err)
	require.Equal(t, aman.DataFresh, accepted.DataStatus)
	require.Equal(t, *observation.FlightPlan.Revision, accepted.FallbackRouteProgress.FlightPlanRevision)
	require.Equal(t, validAt, *accepted.FallbackRouteProgress.ObservedAt)
	require.Equal(t, frozenETA, accepted.Prediction.OperationalTETA)
	require.Equal(t, captured, *accepted.Slot)
	require.Equal(t, captured, *accepted.FrozenSlot)
}
