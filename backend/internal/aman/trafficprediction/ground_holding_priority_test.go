package trafficprediction

import (
	"slices"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/predictor"
	"github.com/stretchr/testify/require"
)

func TestGroundForecastsFollowHeldSlotsAcrossHoldsAndKeepRunwaysSeparate(t *testing.T) {
	now := utc(2026, time.October, 4, 12, 0)
	state := baseState(now, 40)
	group, other := aman.RunwayGroupID("22"), aman.RunwayGroupID("04")
	a := forecastHold("HELD-A", "MONAK", group, now.Add(30*time.Minute), now)
	b := forecastHold("HELD-B", "TESPI", group, now.Add(33*time.Minute), now)
	g1 := planned("ground-a", "", now.Add(10*time.Minute), aman.DataFresh)
	g2 := planned("ground-b", "", now.Add(11*time.Minute), aman.DataFresh)
	later := planned("later", "", now.Add(time.Hour), aman.DataFresh)
	unrelated := planned("other-runway", "", now.Add(12*time.Minute), aman.DataFresh)
	unrelated.SelectedRunwayGroup = &other
	state.Flights = []aman.AMANFlight{g1, a, g2, b, later, unrelated}
	model := Build(state, readyHealth())
	entries := forecastEntries(model)
	require.Equal(t, a.Slot.Time, entries["HELD-A"].LandingAt, "holding forecasts must use the actual reserved target")
	require.Equal(t, b.Slot.Time, entries["HELD-B"].LandingAt)
	require.True(t, entries["GROUND-A"].LandingAt.After(b.Slot.Time))
	require.True(t, entries["GROUND-B"].LandingAt.After(entries["GROUND-A"].LandingAt), "ground targets cannot share a reserved opportunity")
	require.False(t, entries["GROUND-A"].Airborne)
	require.Equal(t, SourceVATSIMPlanned, entries["GROUND-A"].TimingSource)
	require.Equal(t, now.Add(time.Hour), entries["LATER"].LandingAt)
	require.Equal(t, now.Add(12*time.Minute), entries["OTHER-RUNWAY"].LandingAt)
	require.NotContains(t, callsigns(model.Buckets[0]), "GROUND-A", "the old ground timing must no longer contribute to load")
	require.Empty(t, model.Warnings, "successfully putting ground arrivals behind holds is normal scheduling")
	require.Equal(t, StatusReady, model.Status)
	slices.Reverse(state.Flights)
	require.Equal(t, model, Build(state, readyHealth()), "wire order cannot affect provisional priority")
	// Persisted provisional times must also remain behind holding without
	// turning the successful priority rule into a controller warning.
	persisted := state
	persisted.Flights = slices.Clone(state.Flights)
	for i := range persisted.Flights {
		flight := &persisted.Flights[i]
		if flight.Callsign != "GROUND-A" && flight.Callsign != "GROUND-B" {
			continue
		}
		at := entries[normalizedCallsign(*flight)].LandingAt
		flight.Prediction = &aman.Prediction{RawTETA: flight.LatestObservation.PlannedTiming.EstimatedOffBlockTime.Add(predictor.DefaultEXOT + *flight.LatestObservation.PlannedTiming.EstimatedEnrouteTime),
			OperationalTETA: at, OperationalReason: aman.OperationalReasonHoldingPriority, Publishable: true}
	}
	require.Equal(t, model, Build(persisted, readyHealth()))
	// Cancellation or expiry frees the soft ground forecasts again.
	for i := range state.Flights {
		if state.Flights[i].State == aman.StateStable {
			state.Flights[i].State = aman.StateRemoved
		}
	}
	released := forecastEntries(Build(state, readyHealth()))
	require.Equal(t, now.Add(10*time.Minute), released["GROUND-A"].LandingAt)
}

func TestGroundHoldingPriorityNeverOverridesAirborneEvidence(t *testing.T) {
	now := utc(2026, time.October, 4, 12, 0)
	state := baseState(now, 40)
	group := aman.RunwayGroupID("22")
	held := forecastHold("HELD", "MONAK", group, now.Add(time.Hour), now)
	for _, evidence := range []string{"takeoff", "moving-position"} {
		t.Run(evidence, func(t *testing.T) {
			flight := planned("departed", "", now.Add(10*time.Minute), aman.DataFresh)
			altitude, speed := 15000, 300.0
			flight.LatestObservation.Surveillance = &aman.SurveillanceFact{LatitudeDegrees: 55.4, LongitudeDegrees: 12.4, AltitudeFeet: &altitude, GroundspeedKnots: &speed, ObservedAt: &now}
			if evidence == "takeoff" {
				flight.LatestObservation.TakeoffDetected = &now
			}
			state.Flights = []aman.AMANFlight{flight, held}
			model := BuildWithAirportPosition(state, readyHealth(), AirportPosition{LatitudeDegrees: 55.618, LongitudeDegrees: 12.656})
			entries := forecastEntries(model)
			require.True(t, entries["DEPARTED"].Airborne)
			require.True(t, entries["DEPARTED"].LandingAt.Before(held.Slot.Time))
			for _, warning := range model.Warnings {
				require.NotEqual(t, "ground_arrival_behind_holding", warning.Code)
			}
		})
	}
}

func TestGroundPriorityExplainsUnavailableCapacityWithoutKeepingOldTime(t *testing.T) {
	now := utc(2026, time.October, 4, 12, 0)
	state := baseState(now, 40)
	group := aman.RunwayGroupID("22")
	state.Flights = []aman.AMANFlight{planned("ground", "", now.Add(10*time.Minute), aman.DataFresh), forecastHold("HELD", "MONAK", group, now.Add(30*time.Minute), now)}
	state.RunwayGroups[0].RateSchedule = nil
	state.RunwayGroups[0].ActiveRatePerHour = 0
	model := Build(state, readyHealth())
	require.NotContains(t, forecastEntries(model), "GROUND")
	require.Contains(t, model.DegradedReasons, "ground_arrival_behind_holding")
	warned := false
	for _, warning := range model.Warnings {
		if warning.Callsign != nil && *warning.Callsign == "GROUND" {
			warned = true
			require.Equal(t, "ground_arrival_behind_holding", warning.Code)
			require.Contains(t, warning.Message, "current runway capacity constraints")
		}
	}
	require.True(t, warned, "unavailable capacity still needs an actionable warning")
}

func forecastHold(callsign, hold string, group aman.RunwayGroupID, at, now time.Time) aman.AMANFlight {
	entered := now.Add(-time.Minute)
	return aman.AMANFlight{Callsign: callsign, State: aman.StateStable, DataStatus: aman.DataFresh, SelectedRunwayGroup: &group,
		Slot:         &aman.Slot{Time: at, RunwayGroupID: group, Sequence: 1},
		HoldingStack: &aman.HoldingStackState{HoldingID: hold, Confirmed: true, FirstObservedAt: entered},
		Prediction:   &aman.Prediction{OperationalTETA: at.Add(-5 * time.Minute), Publishable: true, HoldingFixETA: &entered}}
}

func forecastEntries(model ReadModel) map[string]Flight {
	entries := map[string]Flight{}
	for _, bucket := range model.Buckets {
		for _, flight := range bucket.Flights {
			entries[flight.Callsign] = flight
		}
	}
	return entries
}
