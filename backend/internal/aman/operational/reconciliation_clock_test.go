package operational

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
)

func TestReconciliationUsesClockAfterObservationSnapshotWithoutRounding(t *testing.T) {
	base := time.Date(2026, time.October, 4, 17, 59, 22, 0, time.UTC)
	for _, scenario := range []struct {
		name       string
		duringLoad bool
		future     bool
	}{
		{name: "fresh position within current second"},
		{name: "fresh position received while repository loads", duringLoad: true},
		{name: "genuinely future position remains rejected", future: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			now := base.Add(900 * time.Millisecond)
			observedAt := base.Add(500 * time.Millisecond)
			if scenario.future {
				observedAt = base.Add(5 * time.Second)
			}
			repository, publisher := &memoryRepository{}, &recordingPublisher{}
			deps := disconnectedExpiryDependencies(repository, publisher, &now)
			loader := &clockObservationRepository{memoryRepository: repository}
			deps.Repository = loader
			service, err := New(deps)
			require.NoError(t, err)
			service.tmaVolume = testTMAService(t).tmaVolume
			flight := gapCommandFlight("NJE634Y", "ARRIVAL-22", base.Add(20*time.Minute), 1, aman.StateStable, aman.FreezeNone)
			flight.State, flight.Slot, flight.Prediction = aman.StateAirborne, nil, nil
			repository.state, repository.has = service.initialState("EKCH", base), true
			repository.state.Flights = []aman.AMANFlight{flight}
			observation := tmaObservation(observedAt, 1, 3, 12_000)
			route, speed, eet, takeoff := "DCT NOTASTAR", 250.0, 30*time.Minute, base.Add(-10*time.Minute)
			observation.Callsign, observation.Origin, observation.Destination = flight.Callsign, "ENGM", "EKCH"
			observation.FiledRoute, observation.TakeoffDetected = &route, &takeoff
			observation.PlannedTiming = &aman.PlannedTiming{EstimatedEnrouteTime: &eet}
			observation.Surveillance.GroundspeedKnots = &speed
			observation.ReconciledAt = base.Add(600 * time.Millisecond)
			observe := func() {
				now = base.Add(900 * time.Millisecond)
				require.NoError(t, service.Observe(context.Background(), observation))
			}
			if scenario.duringLoad {
				now = base
				loader.onLoad = observe
			} else {
				observe()
			}

			require.NoError(t, service.reconcileAirport(context.Background(), "EKCH"))
			updated := stateFlight(t, repository.state, flight.Callsign)
			if scenario.future {
				require.Nil(t, updated.TMAEntry)
				require.False(t, updated.Prediction.Publishable)
				require.NotNil(t, updated.Prediction.DegradationReason)
				require.Equal(t, "tma_entry_surveillance_stale", *updated.Prediction.DegradationReason)
				return
			}
			require.Equal(t, aman.DataFresh, updated.DataStatus)
			require.NotNil(t, updated.TMAEntry)
			require.Equal(t, observedAt, updated.TMAEntry.LastObservedAt)
			require.Equal(t, aman.TMAOutside, updated.TMAEntry.LastContainment)
			require.Equal(t, now, repository.state.GeneratedAt)
		})
	}
}

func TestReconciliationDefersDestinationFactsReceivedAfterClockSample(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "new flight", true: "existing flight"}[existing], func(t *testing.T) {
			base := time.Date(2026, time.October, 19, 17, 12, 11, 0, time.UTC)
			now := base.Add(900 * time.Millisecond)
			repository := &memoryRepository{}
			service, err := New(disconnectedExpiryDependencies(repository, &recordingPublisher{}, &now))
			require.NoError(t, err)
			service.tmaVolume = testTMAService(t).tmaVolume
			repository.state, repository.has = service.initialState("EKCH", base), true
			observedAt := base.Add(500 * time.Millisecond)
			observation := tmaObservation(observedAt, 1, 3, 12_000)
			route, speed, eet, takeoff := "DCT NOTASTAR", 250.0, 30*time.Minute, base.Add(-10*time.Minute)
			observation.Callsign, observation.Origin, observation.Destination = "SAS247", "ENGM", "EKCH"
			observation.Provider = aman.ObservationProviderEuroScope
			observation.ReconciledAt, observation.FlightPlan.ObservedAt = observedAt, &observedAt
			observation.FiledRoute, observation.TakeoffDetected = &route, &takeoff
			observation.PlannedTiming = &aman.PlannedTiming{EstimatedEnrouteTime: &eet}
			observation.Surveillance.GroundspeedKnots = &speed
			if existing {
				repository.state.Flights = []aman.AMANFlight{newFlight(observation, base)}
			}
			require.NoError(t, service.Observe(t.Context(), observation))
			newer := observation
			newerAt := base.Add(950 * time.Millisecond)
			position := *observation.Surveillance
			position.ObservedAt = &newerAt
			newer.Surveillance, newer.ReconciledAt = &position, newerAt
			newer.FlightPlan.ObservedAt = &newerAt
			clockCalls := 0
			service.deps.Now = func() time.Time {
				sampled := now
				clockCalls++
				if clockCalls == 2 {
					// A report arrives just after the reconciliation clock is read.
					now = base.Add(990 * time.Millisecond)
					require.NoError(t, service.Observe(t.Context(), newer))
				}
				return sampled
			}

			require.NoError(t, service.reconcileAirport(t.Context(), "EKCH"))
			flight := stateFlight(t, repository.state, observation.Callsign)
			require.NotNil(t, flight.TMAEntry)
			require.Equal(t, observedAt, flight.TMAEntry.LastObservedAt)
			require.Equal(t, observedAt, *flight.LatestObservation.Surveillance.ObservedAt)
			require.Equal(t, base.Add(900*time.Millisecond), repository.state.GeneratedAt)

			require.NoError(t, service.reconcileAirport(t.Context(), "EKCH"))
			flight = stateFlight(t, repository.state, observation.Callsign)
			require.Equal(t, newerAt, flight.TMAEntry.LastObservedAt)
			require.Equal(t, newerAt, *flight.LatestObservation.Surveillance.ObservedAt)
		})
	}
}

type clockObservationRepository struct {
	*memoryRepository
	onLoad func()
}

func (r *clockObservationRepository) LoadAirportState(ctx context.Context, airport string) (aman.AirportState, error) {
	if r.onLoad != nil {
		callback := r.onLoad
		r.onLoad = nil
		callback()
	}
	return r.memoryRepository.LoadAirportState(ctx, airport)
}
