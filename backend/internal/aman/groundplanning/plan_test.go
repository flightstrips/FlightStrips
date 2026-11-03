package groundplanning

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"github.com/stretchr/testify/require"
)

func TestPlanRetainsForecastsAroundConflictingReservations(t *testing.T) {
	for _, sameSTAR := range []bool{false, true} {
		name := "wake_spacing"
		if sameSTAR {
			name = "same_star_spacing"
		}
		t.Run(name, func(t *testing.T) {
			now := time.Date(2026, 11, 2, 19, 45, 0, 0, time.UTC)
			group, family, wake := aman.RunwayGroupID("04L"), "TUDLO", "M"
			state := aman.AirportState{RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, Selected: true,
				ActiveRatePerHour: 40, RateEffectiveAt: &now}}}
			if sameSTAR {
				state.RunwayGroups[0].SameSTARSpacing = &aman.SameSTARSpacingPolicy{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}
			}
			for index, id := range []aman.Callsign{"HELD", "TRAIL"} {
				at := now.Add(30*time.Minute + time.Duration(index)*90*time.Second)
				entered := now.Add(-time.Minute)
				flight := aman.AMANFlight{Callsign: id, State: aman.StateStable, FreezeReason: aman.FreezeNone,
					SelectedRunwayGroup: &group, SelectedSTARFamily: &family, SelectedHolding: &family,
					Slot:              &aman.Slot{RunwayGroupID: group, Time: at, Sequence: index + 1},
					HoldingStack:      &aman.HoldingStackState{HoldingID: family, Confirmed: true, FirstObservedAt: entered},
					LatestObservation: &aman.FlightObservation{WakeCategory: &wake},
					Prediction:        &aman.Prediction{Publishable: true, RawTETA: at, OperationalTETA: at, HoldingFixETA: &entered}}
				if !sameSTAR && index == 0 {
					heavy := "H"
					flight.LatestObservation.WakeCategory = &heavy
				}
				state.Flights = append(state.Flights, flight)
			}
			for _, id := range []aman.Callsign{"GROUND-A", "GROUND-B"} {
				state.Flights = append(state.Flights, aman.AMANFlight{Callsign: id, State: aman.StatePlanned,
					SelectedRunwayGroup: &group, LatestObservation: &aman.FlightObservation{WakeCategory: &wake},
					Prediction: &aman.Prediction{Publishable: true, RawTETA: now.Add(10 * time.Minute)}})
			}
			state.RunwayGroups[0].CapacityReservations = []aman.RunwayCapacityReservation{{Start: now.Add(33 * time.Minute), End: now.Add(37 * time.Minute)}}
			input := ProjectionInput(state)
			for index := range input.Flights {
				input.Flights[index].ProtectCurrentSlot = true
			}
			generated, err := sequence.Generate(input)
			require.NoError(t, err)
			require.True(t, generated.HasConflicts(), "the existing reservations genuinely conflict")
			require.Len(t, generated.Entries, 2)
			for index, entry := range generated.Entries {
				require.Equal(t, state.Flights[index].Slot.Time, entry.Time)
			}
			before, err := json.Marshal(struct {
				State aman.AirportState
				Input sequence.Input
			}{state, input})
			require.NoError(t, err)
			plans := Plan(state, input, now)
			require.Len(t, plans, 2)
			require.Equal(t, now.Add(37*time.Minute), plans["GROUND-A"], "forecasts must respect the reservation after the occupied hold")
			require.Equal(t, now.Add(38*time.Minute+30*time.Second), plans["GROUND-B"], "new forecasts must keep runway separation")
			after, err := json.Marshal(struct {
				State aman.AirportState
				Input sequence.Input
			}{state, input})
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after), "forecast planning must not alter committed reservations or its input")
			slices.Reverse(state.Flights)
			slices.Reverse(input.Flights)
			require.Equal(t, plans, Plan(state, input, now), "forecast timing must not depend on input order")

			state.RunwayGroups[0].Closures = []aman.RunwayClosure{{Start: now.Add(40 * time.Minute)}}
			input = ProjectionInput(state)
			invalid := input.Flights[0]
			invalid.Callsign = "INVALID"
			invalid.CurrentSlot = &aman.Slot{RunwayGroupID: group, Time: now.Add(41 * time.Minute), Sequence: 3}
			input.Flights = append(input.Flights, invalid)
			require.Empty(t, Plan(state, input, now), "an invalid reservation must still block forecasts")
		})
	}
}

func TestProjectionCarriesDemandAndActiveHolding(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	group, family, wake := aman.RunwayGroupID("A"), "MONAK", "M"
	state := aman.AirportState{RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, Selected: true, ActiveRatePerHour: 40, RateEffectiveAt: &at, SameSTARSpacing: &aman.SameSTARSpacingPolicy{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}}}}
	for _, id := range []aman.Callsign{"LEAD", "TRAIL"} {
		state.Flights = append(state.Flights, aman.AMANFlight{Callsign: id, State: aman.StateUnstable, SelectedRunwayGroup: &group, SelectedSTARFamily: &family, FreezeReason: aman.FreezeNone, LatestObservation: &aman.FlightObservation{WakeCategory: &wake}, Prediction: &aman.Prediction{Publishable: true, RawTETA: at, OperationalTETA: at, GeneratedAt: at}})
	}
	state.Flights[0].SelectedHolding = &family
	state.Flights[0].HoldingStack = &aman.HoldingStackState{HoldingID: family, Confirmed: true, FirstObservedAt: at}
	state.Flights[0].Prediction.HoldingFixETA = &at
	input := ProjectionInput(state)
	require.Equal(t, at, *input.Flights[0].DemandArrivalAt)
	require.Equal(t, at, *input.Flights[0].ActiveHoldingSince)
	result, err := sequence.Generate(input)
	require.NoError(t, err)
	require.Equal(t, 3*time.Minute, result.Entries[1].Time.Sub(result.Entries[0].Time))
	state.Flights[0].HoldingClearanceCanceledAt = &at
	result, err = sequence.Generate(ProjectionInput(state))
	require.NoError(t, err)
	require.Equal(t, 90*time.Second, result.Entries[1].Time.Sub(result.Entries[0].Time))
}
