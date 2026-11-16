package operational

import (
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
)

func TestGroundReconnectDoesNotReusePreviousTakeoff(t *testing.T) {
	now := time.Date(2026, 11, 16, 19, 0, 24, 0, time.UTC)
	service, state, previous := fallbackFixture(t, now)
	ground := previous
	ground.TakeoffDetected = nil
	altitude, speed := 478, 0.0
	surveillance := *ground.Surveillance
	surveillance.AltitudeFeet, surveillance.GroundspeedKnots = &altitude, &speed
	ground.Surveillance = &surveillance
	eet := time.Hour
	ground.PlannedTiming = &aman.PlannedTiming{EstimatedOffBlockTime: &now, EstimatedEnrouteTime: &eet}
	// A retained provider observation can still carry the earlier flight's takeoff.
	ground = mergeSurveillanceObservation(previous, ground)
	require.NotNil(t, ground.TakeoffDetected)

	flight, err := service.reconcileFlight(t.Context(), state, newFlight(ground, now), ground, now)
	require.NoError(t, err)
	require.Equal(t, aman.StatePlanned, flight.State)
	require.Nil(t, flight.Slot)
	require.Nil(t, flight.LatestObservation.TakeoffDetected)
	require.NotNil(t, flight.Prediction)
	require.Equal(t, "aman-planned-eobt-exot-eet-v1", flight.Prediction.ModelVersion)

	now = now.Add(45 * time.Minute)
	previous.TakeoffDetected, previous.ReconciledAt = &now, now
	previous.Surveillance.ObservedAt = &now
	flight, err = service.reconcileFlight(t.Context(), state, flight, previous, now)
	require.NoError(t, err)
	state.Flights = []aman.AMANFlight{flight}
	service.resequence(&state, now)
	require.NotEqual(t, aman.StateLanded, state.Flights[0].State)
	require.NotNil(t, state.Flights[0].Slot)
}

func TestPersistedLandedFlightRecoversOnFreshAirborneObservation(t *testing.T) {
	for _, provider := range []aman.ObservationProvider{aman.ObservationProviderEuroScope, aman.ObservationProviderVATSIM} {
		t.Run(string(provider), func(t *testing.T) {
			landedAt := time.Date(2026, 11, 16, 19, 0, 24, 0, time.UTC)
			now := landedAt.Add(45 * time.Minute)
			service, state, observation := fallbackFixture(t, now)
			service.deps.Materializer = fixedNavigation{key: "new-route"}
			observation.Callsign, observation.Provider = "WZZ2105", provider
			observation.SurveillanceSource = aman.SurveillanceSource(provider)
			// A fresh radar report is sufficient even without a new takeoff marker.
			observation.TakeoffDetected = timePointer(landedAt.Add(-time.Hour))
			ground := observation
			ground.ReconciledAt = landedAt
			altitude, speed := 478, 0.0
			ground.Surveillance = &aman.SurveillanceFact{AltitudeFeet: &altitude, GroundspeedKnots: &speed, ObservedAt: &landedAt}
			flight := applyGroundedObservation(newFlight(observation, landedAt), ground, landedAt)
			flight.LatestObservation = &ground
			require.Equal(t, aman.StateLanded, flight.State)
			flight.ArrivalBaseline = &aman.BaselineState{ArrivalAt: now, AirborneSensedAt: *observation.TakeoffDetected}
			// Reproduce a restart with the already-landed aggregate on disk.
			state.Flights = []aman.AMANFlight{flight}
			flight = cloneGapState(t, state).Flights[0]
			flight, err := service.reconcileFlight(t.Context(), state, flight, observation, now)
			require.NoError(t, err)
			state.Flights = []aman.AMANFlight{flight}
			service.resequence(&state, now)
			require.Contains(t, []aman.FlightState{aman.StateAirborne, aman.StateUnstable, aman.StateStable}, state.Flights[0].State)
			require.NotNil(t, state.Flights[0].Prediction)
			require.NotNil(t, state.Flights[0].Slot)
			require.Nil(t, flight.ArrivalBaseline, "the previous flight's baseline must not survive recovery")
			require.Equal(t, now, *flight.LatestObservation.TakeoffDetected)
		})
	}
}

func TestLandedRecoveryRejectsUnusableSurveillanceAndManualRemoval(t *testing.T) {
	for _, scenario := range []string{"missing", "stale-source", "disconnected", "stale-radar", "future-radar", "replayed", "older-than-last-ground-report", "grounded", "slow", "missing-speed", "missing-altitude", "invalid-position", "no-radar-time", "manual-removal"} {
		t.Run(scenario, func(t *testing.T) {
			landedAt := time.Date(2026, 11, 16, 19, 0, 24, 0, time.UTC)
			now := landedAt.Add(time.Minute)
			service, state, observation := fallbackFixture(t, now)
			flight := aman.AMANFlight{Callsign: observation.Callsign, State: aman.StateLanded, UpdatedAt: landedAt,
				Lifecycle: &aman.LifecycleState{EnteredAt: landedAt, Reason: aman.LifecycleReasonLandingConfirmed}}
			switch scenario {
			case "missing":
				observation.Missing = true
			case "stale-source":
				observation.SourceStatus = aman.DataStale
			case "disconnected":
				observation.SourceStatus = aman.DataDisconnected
			case "stale-radar":
				observation.Surveillance.ObservedAt = timePointer(now.Add(-3 * time.Minute))
			case "future-radar":
				observation.Surveillance.ObservedAt = timePointer(now.Add(time.Second))
			case "replayed":
				observation.Surveillance.ObservedAt = &landedAt
			case "older-than-last-ground-report":
				altitude, speed := 478, 0.0
				flight.LatestObservation = &aman.FlightObservation{SourceStatus: aman.DataFresh, ReconciledAt: now,
					Surveillance: &aman.SurveillanceFact{ObservedAt: timePointer(now.Add(-10 * time.Second)), AltitudeFeet: &altitude, GroundspeedKnots: &speed}}
				observation.Surveillance.ObservedAt = timePointer(now.Add(-20 * time.Second))
			case "grounded":
				*observation.Surveillance.AltitudeFeet, *observation.Surveillance.GroundspeedKnots = 478, 0
			case "slow":
				*observation.Surveillance.GroundspeedKnots = 40
			case "missing-speed":
				observation.Surveillance.GroundspeedKnots = nil
			case "missing-altitude":
				observation.Surveillance.AltitudeFeet = nil
			case "invalid-position":
				observation.Surveillance.LatitudeDegrees = 91
			case "no-radar-time":
				observation.Surveillance.ObservedAt = nil
			case "manual-removal":
				flight.State, flight.Lifecycle.Reason = aman.StateRemoved, aman.LifecycleReasonManualRemoval
			}
			updated, _ := service.reconcileFlight(t.Context(), state, flight, observation, now)
			require.Equal(t, flight.State, updated.State)
			require.Nil(t, updated.Slot)
		})
	}
}

func TestRejectedLandedReportsPreserveRadarRecoveryWatermark(t *testing.T) {
	for _, rejected := range []string{"missing-speed", "newer-missing-speed", "missing-altitude", "older-radar", "no-radar", "no-radar-time", "future-radar", "invalid-position", "stale-source", "missing"} {
		t.Run(rejected, func(t *testing.T) {
			landedAt := time.Date(2026, 11, 16, 19, 0, 0, 0, time.UTC)
			now := landedAt.Add(2 * time.Minute)
			service, state, airborne := fallbackFixture(t, now)
			service.deps.Materializer = fixedNavigation{key: "new-route"}
			groundAt := landedAt.Add(110 * time.Second)
			ground := airborne
			altitude, speed := 478, 0.0
			ground.ReconciledAt = groundAt
			ground.Surveillance = &aman.SurveillanceFact{AltitudeFeet: &altitude, GroundspeedKnots: &speed, ObservedAt: &groundAt}
			flight := aman.AMANFlight{Callsign: airborne.Callsign, State: aman.StateLanded, UpdatedAt: landedAt,
				Lifecycle: &aman.LifecycleState{EnteredAt: landedAt, Reason: aman.LifecycleReasonLandingConfirmed}}
			flight, err := service.reconcileFlight(t.Context(), state, flight, ground, now)
			require.NoError(t, err)

			bad := airborne
			radar := *airborne.Surveillance
			bad.Surveillance = &radar
			bad.Surveillance.ObservedAt = timePointer(landedAt.Add(100 * time.Second))
			switch rejected {
			case "missing-speed":
				bad.Surveillance.GroundspeedKnots = nil
			case "newer-missing-speed":
				bad.Surveillance.ObservedAt, bad.Surveillance.GroundspeedKnots = &now, nil
			case "missing-altitude":
				bad.Surveillance.ObservedAt, bad.Surveillance.AltitudeFeet = &now, nil
			case "no-radar":
				bad.Surveillance = nil
			case "no-radar-time":
				bad.Surveillance.ObservedAt = nil
			case "future-radar":
				bad.Surveillance.ObservedAt = timePointer(now.Add(time.Minute))
			case "invalid-position":
				bad.Surveillance.ObservedAt = &now
				bad.Surveillance.LatitudeDegrees = 91
			case "stale-source":
				bad.SourceStatus = aman.DataStale
			case "missing":
				bad.Missing = true
			}
			flight, err = service.reconcileFlight(t.Context(), state, flight, bad, now)
			require.NoError(t, err)
			require.Equal(t, aman.StateLanded, flight.State)
			require.NotNil(t, flight.LatestObservation.Surveillance)
			require.Equal(t, groundAt, *flight.LatestObservation.Surveillance.ObservedAt)
			// The guard must survive persistence, not just an in-memory retry.
			flight = cloneGapState(t, aman.AirportState{Flights: []aman.AMANFlight{flight}}).Flights[0]
			airborne.Surveillance.ObservedAt = timePointer(landedAt.Add(105 * time.Second))
			flight, err = service.reconcileFlight(t.Context(), state, flight, airborne, now)
			require.NoError(t, err)
			require.Equal(t, groundAt, *flight.LatestObservation.Surveillance.ObservedAt)
			state.Flights = []aman.AMANFlight{flight}
			service.resequence(&state, now)
			require.Equal(t, aman.StateLanded, state.Flights[0].State)
			require.Nil(t, state.Flights[0].Slot)

			airborne.Surveillance.ObservedAt = timePointer(landedAt.Add(115 * time.Second))
			flight, err = service.reconcileFlight(t.Context(), state, flight, airborne, now)
			require.NoError(t, err)
			state.Flights = []aman.AMANFlight{flight}
			service.resequence(&state, now)
			require.NotEqual(t, aman.StateLanded, state.Flights[0].State)
			require.NotNil(t, state.Flights[0].Slot)
		})
	}
}
