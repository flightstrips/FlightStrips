package operational

import (
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/models"
	"github.com/stretchr/testify/require"
)

func TestDelayedStripRevisionDoesNotRollBackDestinationFactTime(t *testing.T) {
	for _, destination := range []string{"EKCH", "ESGG"} {
		t.Run(destination, func(t *testing.T) {
			at := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
			service := &Service{deps: Dependencies{Airports: []string{"EKCH"}}, observed: make(map[string]map[aman.Callsign]aman.FlightObservation)}
			ctx := aman.WithSession(t.Context(), 1)
			current := aman.FlightObservation{
				Callsign: "SAS123", Origin: "ESSA", Destination: destination,
				Provider: aman.ObservationProviderEuroScope, SourceStatus: aman.DataFresh,
				ReconciledAt:                 at.Add(3 * time.Second),
				FlightPlan:                   aman.FlightPlanFact{ObservedAt: timePointer(at.Add(3 * time.Second))},
				ClearedAltitudeStripRevision: aman.StripRevision{ID: 42, Version: 19},
			}
			require.NoError(t, service.Observe(ctx, current))
			delayed := current
			delayed.ReconciledAt, delayed.FlightPlan.ObservedAt = at.Add(time.Second), timePointer(at.Add(time.Second))
			delayed.Destination = "EKBI"
			delayed.ClearedAltitudeStripRevision.Version = 20
			require.NoError(t, service.Observe(ctx, delayed))
			fact, found := service.destinationObservation(ctx, current.Callsign, current.ReconciledAt)
			require.True(t, found)
			require.Equal(t, current.Destination, fact.Destination)
			require.Equal(t, observationFactTime(current), observationFactTime(fact))
			require.Equal(t, delayed.ClearedAltitudeStripRevision, fact.ClearedAltitudeStripRevision)

			stale := delayed
			stale.ReconciledAt, stale.FlightPlan.ObservedAt = at.Add(2*time.Second), timePointer(at.Add(2*time.Second))
			stale.Destination = "EKCH"
			stale.ClearedAltitudeStripRevision.Version = 18
			require.NoError(t, service.Observe(ctx, stale))
			after, _ := service.destinationObservation(ctx, current.Callsign, stale.ReconciledAt)
			require.Equal(t, fact, after, "an intermediate stale report must not pass after the delayed revision")
			if destination == "ESGG" {
				require.Empty(t, service.sessionObservations(ctx, "EKCH"), "stale arrivals cannot undo a diversion")
			}
		})
	}
}

func TestDelayedHoldingSnapshotPreservesNewerFlightPlan(t *testing.T) {
	for _, withHolding := range []bool{false, true} {
		name := "position snapshot"
		if withHolding {
			name = "holding snapshot"
		}
		t.Run(name, func(t *testing.T) {
			service, state, now := holdingClearanceRefreshFixture(t)
			previous := *state.Flights[0].LatestObservation
			previous.Provider = aman.ObservationProviderEuroScope
			previous.FlightPlan.ObservedAt = timePointer(now.Add(time.Second))
			previous.ReconciledAt = now.Add(time.Second)
			previous.ClearedAltitudeStripRevision = aman.StripRevision{ID: 42, Version: 19}
			previous.FiledRoute, previous.AssignedSTAR = stringPointer("DCT TESPI"), stringPointer("TESPI1A")
			previous.AircraftType, previous.WakeCategory = stringPointer("A320"), stringPointer("M")
			requested := 35000
			previous.RequestedLevel = &requested
			duration := time.Hour
			previous.PlannedTiming = &aman.PlannedTiming{EstimatedEnrouteTime: &duration}
			state.Flights[0].LatestObservation = &previous
			incoming := previous
			incoming.ReconciledAt, incoming.FlightPlan.ObservedAt = now, &now
			incoming.ClearedAltitudeStripRevision.Version = 20
			incoming.Origin, incoming.Destination = "EGLL", "ESGG"
			incoming.FiledRoute, incoming.AssignedSTAR = stringPointer("OLD ROUTE"), stringPointer("MONAK1A")
			incoming.AircraftType, incoming.WakeCategory = stringPointer("B744"), stringPointer("H")
			oldRequested, oldDuration := 31000, 2*time.Hour
			incoming.RequestedLevel = &oldRequested
			incoming.PlannedTiming = &aman.PlannedTiming{EstimatedEnrouteTime: &oldDuration}
			if withHolding {
				incoming.HoldingClearance = normalizedStripHoldingClearance(&models.Strip{ID: 42, Version: 20, Hold: "TESPI", HoldType: "enroute"}, now)
			}
			updated, err := service.reconcileFlight(t.Context(), state, state.Flights[0], incoming, previous.ReconciledAt)
			require.NoError(t, err)
			require.NotEqual(t, aman.StateRemoved, updated.State)
			actual := updated.LatestObservation
			require.Equal(t, previous.Origin, actual.Origin)
			require.Equal(t, previous.Destination, actual.Destination)
			require.Equal(t, previous.FlightPlan, actual.FlightPlan)
			require.Equal(t, previous.FiledRoute, actual.FiledRoute)
			require.Equal(t, previous.AssignedSTAR, actual.AssignedSTAR)
			require.Equal(t, previous.AircraftType, actual.AircraftType)
			require.Equal(t, previous.WakeCategory, actual.WakeCategory)
			require.Equal(t, previous.RequestedLevel, actual.RequestedLevel)
			require.Equal(t, previous.PlannedTiming, actual.PlannedTiming)
			require.Equal(t, previous.Surveillance, actual.Surveillance)
			require.Equal(t, incoming.ClearedAltitudeStripRevision, actual.ClearedAltitudeStripRevision)
			if withHolding {
				require.Equal(t, incoming.HoldingClearance, actual.HoldingClearance)
			}
		})
	}
}
