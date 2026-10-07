package groundplanning

import (
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"github.com/stretchr/testify/require"
)

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
