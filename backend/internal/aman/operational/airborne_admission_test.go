package operational

import (
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/lifecycle"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/terminal"
	"github.com/stretchr/testify/require"
)

func TestGroundTrafficAbove1000FeetCannotAcquireOrRetainArrivalSlots(t *testing.T) {
	for _, initial := range []aman.FlightState{aman.StatePlanned, aman.StateUnstable, aman.StateStable} {
		t.Run(string(initial), func(t *testing.T) {
			now := time.Date(2026, 11, 16, 18, 18, 0, 0, time.UTC)
			service, state, observation, materializer := arrivalChangeFixture(now)
			altitude, speed, eobt, eet := 1284, 20.0, now, 30*time.Minute
			observation.TakeoffDetected = nil
			observation.SurveillanceSource = aman.SurveillanceSourceEuroScope
			observation.Surveillance.AltitudeFeet, observation.Surveillance.GroundspeedKnots = &altitude, &speed
			observation.PlannedTiming = &aman.PlannedTiming{EstimatedOffBlockTime: &eobt, EstimatedEnrouteTime: &eet}
			flight := newFlight(observation, now)
			if initial != aman.StatePlanned {
				// Recover a physical prediction and commitment written by the old
				// Planned -> Unstable path, including after persistence/restart.
				flight = gapCommandFlight(observation.Callsign, state.RunwayGroups[0].ID, now.Add(35*time.Minute), 1, initial, aman.FreezeSuperstable)
				flight.LatestObservation = &observation
				flight = cloneGapState(t, aman.AirportState{Flights: []aman.AMANFlight{flight}}).Flights[0]
			}
			updated, err := service.reconcileFlight(t.Context(), state, flight, observation, now)
			require.NoError(t, err)
			require.Equal(t, aman.StatePlanned, updated.State)
			require.Nil(t, updated.Slot)
			require.Nil(t, updated.FrozenSlot)
			require.Nil(t, updated.Order)
			require.Nil(t, updated.RouteProgress)
			require.Nil(t, updated.PredictionFilter)
			require.Empty(t, updated.RawTETASamples)
			require.Equal(t, aman.FreezeNone, updated.FreezeReason)
			require.NotNil(t, updated.Prediction, "filed timing remains advisory")
			require.True(t, isPreliminaryPrediction(updated.Prediction))
			require.Equal(t, 0, materializer.calls, "taxi reports must not enter physical prediction")
			state.Flights = []aman.AMANFlight{updated}
			service.resequence(&state, now)
			require.Nil(t, state.Flights[0].Slot)
			require.Empty(t, service.sequenceInput(state).Flights)
			_, guidance := aman.GainLossPrediction(state.Flights[0])
			require.False(t, guidance)

			// Genuine takeoff admits the same aircraft normally.
			at := now.Add(time.Minute)
			altitude, speed = 5000, 180
			observation.TakeoffDetected, observation.ReconciledAt = &at, at
			observation.Surveillance.ObservedAt = &at
			updated, err = service.reconcileFlight(t.Context(), state, state.Flights[0], observation, at)
			require.NoError(t, err)
			require.NotEqual(t, aman.StatePlanned, updated.State)
			state.Flights[0] = updated
			service.resequence(&state, at)
			require.NotNil(t, state.Flights[0].Slot)
		})
	}
}

func TestArrivalTimeAloneCannotPromotePlannedFlight(t *testing.T) {
	now := time.Date(2026, 11, 16, 18, 18, 0, 0, time.UTC)
	require.Equal(t, aman.StatePlanned, lifecycleState(lifecycle.DefaultConfig(), aman.AMANFlight{State: aman.StatePlanned}, now.Add(time.Minute), now))
}

func TestGroundTrafficCannotEnterFallbackPrediction(t *testing.T) {
	// Reproduces the admission regression introduced by #1151: the normal
	// route is unavailable, but a terminal fallback can calculate an ETA.
	// A successful calculation must not manufacture takeoff evidence.
	now := time.Date(2026, 11, 16, 18, 18, 0, 0, time.UTC)
	service, state, observation, _ := arrivalChangeFixture(now)
	geometry := service.deps.Geometry.(arrivalChangeGeometry)
	geometry.path.Coverage, geometry.path.Digest = navdata.CoverageComplete, "validated-terminal"
	service.deps.Geometry, service.deps.Materializer = geometry, unavailableNavigation{}
	service.deps.Terminal.Paths = []terminal.Path{{Feeder: "TESPI", RunwayGroup: state.RunwayGroups[0].ID, STARSuffix: "1A"}}
	altitude, speed := 1284, 20.0
	observation.TakeoffDetected = nil
	observation.SurveillanceSource = aman.SurveillanceSourceEuroScope
	observation.Surveillance.AltitudeFeet, observation.Surveillance.GroundspeedKnots = &altitude, &speed
	flight, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
	require.NoError(t, err)
	state.Flights = []aman.AMANFlight{flight}
	service.resequence(&state, now)
	require.Equal(t, aman.StatePlanned, state.Flights[0].State)
	require.Nil(t, state.Flights[0].Slot)
	require.Nil(t, state.Flights[0].FallbackRouteProgress)
	require.Nil(t, state.Flights[0].Prediction)
}

func TestConfirmedAirborneFlightRetainsHistoryAcrossStripRefreshAndRestart(t *testing.T) {
	now := time.Date(2026, 11, 16, 18, 18, 0, 0, time.UTC)
	service, state, observation, _ := arrivalChangeFixture(now)
	flight, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
	require.NoError(t, err)
	state.Flights = []aman.AMANFlight{flight}
	service.resequence(&state, now)
	flight = cloneGapState(t, state).Flights[0]
	takeoff := *flight.LatestObservation.TakeoffDetected
	slot := *flight.Slot
	// A strip-only sync after a restart must not turn missing surveillance
	// into evidence that a confirmed airborne flight is still at departure.
	observation.TakeoffDetected, observation.Surveillance = nil, nil
	observation.ReconciledAt = now.Add(time.Second)
	flight, err = service.reconcileFlight(t.Context(), state, flight, observation, observation.ReconciledAt)
	require.NoError(t, err)
	require.NotEqual(t, aman.StatePlanned, flight.State)
	require.Equal(t, takeoff, *flight.LatestObservation.TakeoffDetected)
	require.Equal(t, slot, *flight.Slot)
}

func TestConfirmedAirborneHistorySurvivesUnavailableReportsAndReconnect(t *testing.T) {
	for _, outage := range []struct {
		name    string
		missing bool
		status  aman.DataStatus
	}{
		{"missing", true, aman.DataFresh},
		{"stale", false, aman.DataStale},
		{"disconnected", false, aman.DataDisconnected},
	} {
		for _, positionWithoutSpeed := range []bool{false, true} {
			name := outage.name + "/strip-only"
			if positionWithoutSpeed {
				name = outage.name + "/initial-position"
			}
			t.Run(name, func(t *testing.T) {
				now := time.Date(2026, 11, 16, 18, 18, 0, 0, time.UTC)
				service, state, observation, _ := arrivalChangeFixture(now)
				observation.PlannedTiming = nil // No EET means no baseline backup.
				flight, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
				require.NoError(t, err)
				require.Nil(t, flight.ArrivalBaseline)
				state.Flights = []aman.AMANFlight{flight}
				service.resequence(&state, now)
				flight = state.Flights[0]
				takeoff, slot, order := *flight.LatestObservation.TakeoffDetected, *flight.Slot, *flight.Order
				flight.State, flight.FreezeReason = aman.StateStable, aman.FreezeSuperstable
				flight.FrozenSlot = &slot
				flight.FrozenAt, flight.FrozenOperationalTETA = &now, &flight.Prediction.OperationalTETA
				prediction := *flight.Prediction

				report := observation
				report.TakeoffDetected, report.Surveillance = nil, nil
				report.Missing, report.SourceStatus = outage.missing, outage.status
				report.ReconciledAt = now.Add(time.Minute)
				flight, err = service.reconcileFlight(t.Context(), state, flight, report, report.ReconciledAt)
				require.NoError(t, err)
				require.NotNil(t, flight.LatestObservation.TakeoffDetected)
				require.Equal(t, takeoff, *flight.LatestObservation.TakeoffDetected)
				require.Equal(t, outage.status, flight.LatestObservation.SourceStatus)
				require.Equal(t, outage.missing, flight.LatestObservation.Missing)
				if outage.missing {
					markMissing(&flight, report.ReconciledAt)
				}
				flight = cloneGapState(t, aman.AirportState{Flights: []aman.AMANFlight{flight}}).Flights[0]

				at := now.Add(2 * time.Minute)
				report.Missing, report.SourceStatus, report.ReconciledAt = false, aman.DataFresh, at
				if positionWithoutSpeed {
					position := *observation.Surveillance
					position.GroundspeedKnots, position.ObservedAt = nil, &at
					report.Surveillance = &position
				}
				flight, err = service.reconcileFlight(t.Context(), state, flight, report, at)
				require.NoError(t, err)
				require.Equal(t, aman.StateStable, flight.State)
				require.NotNil(t, flight.LatestObservation.TakeoffDetected)
				require.Equal(t, takeoff, *flight.LatestObservation.TakeoffDetected)
				require.Equal(t, slot, *flight.Slot)
				require.Equal(t, order, *flight.Order)
				require.Equal(t, aman.FreezeSuperstable, flight.FreezeReason)
				require.Equal(t, slot, *flight.FrozenSlot)
				require.Equal(t, prediction.RawTETA, flight.Prediction.RawTETA)
				require.Equal(t, prediction.GeneratedAt, flight.Prediction.GeneratedAt)
			})
		}
	}
}

func TestUnavailableSurveillanceCannotCreateTakeoffEvidence(t *testing.T) {
	now := time.Date(2026, 11, 16, 18, 18, 0, 0, time.UTC)
	service, state, observation, _ := arrivalChangeFixture(now)
	observation.TakeoffDetected = nil
	observation.SourceStatus = aman.DataStale
	flight, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
	require.NoError(t, err)
	require.Nil(t, flight.LatestObservation.TakeoffDetected)

	observation.SourceStatus, observation.Surveillance = aman.DataFresh, nil
	observation.ReconciledAt = now.Add(time.Second)
	flight, err = service.reconcileFlight(t.Context(), state, flight, observation, observation.ReconciledAt)
	require.NoError(t, err)
	require.Equal(t, aman.StatePlanned, flight.State)
	require.Nil(t, flight.LatestObservation.TakeoffDetected)
	require.Nil(t, flight.Slot)
}

func TestAirportPredictionErrorRetainsAirborneHistoryThroughReconnect(t *testing.T) {
	now := time.Date(2026, 11, 16, 18, 18, 0, 0, time.UTC)
	fixture, state, observation, _ := arrivalChangeFixture(now)
	observation.PlannedTiming = nil // Takeoff history has no EET/baseline backup.
	flight, err := fixture.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
	require.NoError(t, err)
	state.Flights = []aman.AMANFlight{flight}
	fixture.resequence(&state, now)
	flight = state.Flights[0]
	takeoff, slot, order := *flight.LatestObservation.TakeoffDetected, *flight.Slot, *flight.Order
	updateLifecycle(&flight, flight.State, aman.StateStable, now)
	flight.FreezeReason = aman.FreezeSuperstable
	flight.FrozenSlot = &slot
	flight.FrozenAt, flight.FrozenOperationalTETA = &now, &flight.Prediction.OperationalTETA
	require.Nil(t, flight.ArrivalBaseline)

	repository, publisher := &memoryRepository{}, &recordingPublisher{}
	deps := disconnectedExpiryDependencies(repository, publisher, &now)
	deps.Terminal = fixture.deps.Terminal
	service, err := New(deps) // Unavailable navigation forces the prediction-error path.
	require.NoError(t, err)
	repository.state, repository.has = service.initialState("EKCH", now), true
	repository.state.Revision, repository.state.Flights = 7, []aman.AMANFlight{flight}

	// The position adapter supplies no takeoff flag below 1,000 ft. An
	// approach at 140 kt is still airborne and must retain its prior evidence.
	now = now.Add(10 * time.Second)
	altitude, speed := 800, 140.0
	report := observation
	position := *observation.Surveillance
	position.AltitudeFeet, position.GroundspeedKnots, position.ObservedAt = &altitude, &speed, &now
	report.Surveillance, report.TakeoffDetected, report.ReconciledAt = &position, nil, now
	require.NoError(t, service.Observe(t.Context(), report))
	require.NoError(t, service.reconcileAirport(t.Context(), "EKCH"))
	failed := stateFlight(t, repository.state, flight.Callsign)
	require.NotNil(t, failed.PredictionFailureReason, "exercise the airport prediction-error handler")
	require.NotNil(t, failed.LatestObservation.TakeoffDetected)
	require.Equal(t, takeoff, *failed.LatestObservation.TakeoffDetected)
	require.Equal(t, altitude, *failed.LatestObservation.Surveillance.AltitudeFeet)
	require.Equal(t, now, *failed.LatestObservation.Surveillance.ObservedAt)
	require.Equal(t, aman.DataStale, failed.DataStatus)
	require.Equal(t, slot.Time, failed.Slot.Time)

	// Persist and restart before a strip-only resync so the in-memory source
	// cache cannot hide loss of the confirmed takeoff history.
	repository.state = cloneGapState(t, repository.state)
	service, err = New(deps)
	require.NoError(t, err)
	now = now.Add(10 * time.Second)
	report.Surveillance, report.ReconciledAt = nil, now
	require.NoError(t, service.Observe(t.Context(), report))
	require.NoError(t, service.reconcileAirport(t.Context(), "EKCH"))
	reconnected := stateFlight(t, repository.state, flight.Callsign)
	require.Equal(t, aman.StateStable, reconnected.State)
	require.Equal(t, takeoff, *reconnected.LatestObservation.TakeoffDetected)
	require.Equal(t, slot.Time, reconnected.Slot.Time)
	require.Equal(t, order, *reconnected.Order)
	require.Equal(t, aman.FreezeSuperstable, reconnected.FreezeReason)
	require.Equal(t, slot.Time, reconnected.FrozenSlot.Time)
}

func TestLegacyAirborneSurveillanceAdmitsFlightWithoutAdapterTakeoffField(t *testing.T) {
	now := time.Date(2026, 11, 16, 18, 18, 0, 0, time.UTC)
	service, state, observation, _ := arrivalChangeFixture(now)
	observation.TakeoffDetected = nil
	flight, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
	require.NoError(t, err)
	require.NotEqual(t, aman.StatePlanned, flight.State)
	require.NotNil(t, flight.LatestObservation.TakeoffDetected)
	state.Flights = []aman.AMANFlight{flight}
	service.resequence(&state, now)
	require.NotNil(t, state.Flights[0].Slot)
}

func TestRecoveringGroundReservationFreesCapacityForAirborneArrival(t *testing.T) {
	now := time.Date(2026, 11, 16, 18, 18, 0, 0, time.UTC)
	service, state, observation, _ := arrivalChangeFixture(now)
	group, arrival := state.RunwayGroups[0].ID, now.Add(20*time.Minute)
	altitude, speed := 1284, 20.0
	observation.TakeoffDetected = nil
	observation.SurveillanceSource = aman.SurveillanceSourceEuroScope
	observation.Surveillance.AltitudeFeet, observation.Surveillance.GroundspeedKnots = &altitude, &speed
	ground := gapCommandFlight(observation.Callsign, group, arrival, 1, aman.StateStable, aman.FreezeManual)
	ground.LatestObservation = &observation
	airborne := operationalFlight("AIRBORNE", group, "MONAK", "M", arrival)
	state.Revision, state.GeneratedAt = 7, now
	state.Flights = []aman.AMANFlight{ground, airborne}
	service.resequence(&state, now)
	require.True(t, state.Flights[1].Slot.Time.After(arrival), "the erroneous protected reservation consumes capacity")

	updated, err := service.reconcileFlight(t.Context(), state, state.Flights[0], observation, now)
	require.NoError(t, err)
	state.Flights[0] = updated
	service.resequence(&state, now)
	require.Nil(t, state.Flights[0].Slot)
	require.Equal(t, arrival, state.Flights[1].Slot.Time, "the airborne arrival can use the freed opportunity immediately")
	require.Equal(t, 1, state.Flights[1].Slot.Sequence)
	require.Equal(t, arrival, state.Flights[1].Prediction.OperationalTETA)
}

func TestResequenceDiscardsEveryPlannedSlotIncludingManualPhysicalSlots(t *testing.T) {
	now := time.Date(2026, 11, 16, 18, 18, 0, 0, time.UTC)
	for _, freeze := range []aman.FreezeReason{aman.FreezeNone, aman.FreezeManual, aman.FreezeSuperstable, aman.FreezeTMA} {
		t.Run(string(freeze), func(t *testing.T) {
			flight := gapCommandFlight("GROUND", "ARRIVAL-22", now.Add(20*time.Minute), 1, aman.StatePlanned, freeze)
			state := aman.AirportState{Flights: []aman.AMANFlight{flight}}
			(&Service{}).resequence(&state, now)
			require.Nil(t, state.Flights[0].Slot)
			require.Nil(t, state.Flights[0].FrozenSlot)
			require.Nil(t, state.Flights[0].Order)
			require.Empty(t, state.Flights[0].QueueOffers)
			require.Equal(t, aman.FreezeNone, state.Flights[0].FreezeReason)
		})
	}
}
