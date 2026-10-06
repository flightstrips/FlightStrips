package operational

import (
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/aman/terminal"
	"github.com/stretchr/testify/require"
)

func TestScheduledGainUsesLivePredictionInsteadOfRetainedOperationalETA(t *testing.T) {
	start := time.Date(2026, time.October, 4, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("ARRIVAL-22")
	for _, drift := range []time.Duration{2 * time.Minute, 2*time.Minute + time.Nanosecond, 10 * time.Minute} {
		t.Run(drift.String(), func(t *testing.T) {
			flight := operationalFlight("ARRIVAL", group, "MONAK", "M", start.Add(5*time.Minute))
			flight.DataStatus = aman.DataFresh
			flight.Prediction.RawTETA = flight.Prediction.OperationalTETA.Add(drift)
			state := aman.AirportState{RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, ActiveRatePerHour: 60, RateEffectiveAt: &start}}, Flights: []aman.AMANFlight{flight}}
			input := sequenceInput(state, terminal.Configuration{})
			result, err := sequence.Generate(input)
			require.NoError(t, err)
			require.False(t, result.HasConflicts())
			require.Len(t, result.Entries, 1)
			require.LessOrEqual(t, flight.Prediction.RawTETA.Sub(result.Entries[0].Time), aman.MaxScheduledGain)
			if drift == aman.MaxScheduledGain {
				require.Equal(t, flight.Prediction.OperationalTETA, result.Entries[0].Time)
			}
		})
	}
}

func TestGainLimitDoesNotMoveStableTargetsOrFollowingReservations(t *testing.T) {
	start := time.Date(2026, time.October, 4, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("ARRIVAL-22")
	state := aman.AirportState{Revision: 7, RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, ActiveRatePerHour: 60, RateEffectiveAt: &start}}}
	for index, id := range []string{"FIRST", "SECOND"} {
		at := start.Add(time.Duration(5+index) * time.Minute)
		flight := operationalFlight(id, group, "MONAK", "M", at)
		flight.State, flight.DataStatus = aman.StateStable, aman.DataFresh
		flight.Prediction.RawTETA = at
		flight.Slot = &aman.Slot{Time: at, RunwayGroupID: group, Sequence: index + 1, Revision: 7, Reason: "stable"}
		state.Flights = append(state.Flights, flight)
	}
	state.Flights[0].Prediction.RawTETA = start.Add(12 * time.Minute)
	service := &Service{}
	service.resequence(&state, start)
	require.Equal(t, start.Add(5*time.Minute), state.Flights[0].Slot.Time)
	require.Equal(t, start.Add(6*time.Minute), state.Flights[1].Slot.Time)
	require.Equal(t, 1, state.Flights[0].Slot.Sequence)
	require.Equal(t, 2, state.Flights[1].Slot.Sequence)
	service.resequence(&state, start)
	require.Equal(t, start.Add(5*time.Minute), state.Flights[0].Slot.Time)
	require.Equal(t, start.Add(6*time.Minute), state.Flights[1].Slot.Time)
}

func TestQueueOffersRejectTargetsRequiringMoreThanTwoMinutesGain(t *testing.T) {
	start := time.Date(2026, time.October, 4, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("ARRIVAL-22")
	for _, candidateTime := range []time.Time{start.Add(10*time.Minute - time.Second), start.Add(10 * time.Minute)} {
		t.Run(candidateTime.Format("150405"), func(t *testing.T) {
			candidate := operationalFlight("OCCUPANT", group, "MONAK", "M", candidateTime)
			candidate.Slot = &aman.Slot{Time: candidateTime, RunwayGroupID: group, Sequence: 1, Revision: 7, Reason: "rate_wtc"}
			target := operationalFlight("TARGET", group, "MONAK", "M", start.Add(5*time.Minute))
			target.DataStatus = aman.DataFresh
			target.Prediction.RawTETA = start.Add(12 * time.Minute)
			target.Slot = &aman.Slot{Time: start.Add(20 * time.Minute), RunwayGroupID: group, Sequence: 2, Revision: 7, Reason: "rate_wtc"}
			state := aman.AirportState{Revision: 7, RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, ActiveRatePerHour: 60, RateEffectiveAt: &start}}, Flights: []aman.AMANFlight{candidate, target}}
			offers, err := sequence.CalculateQueueOffers(sequenceInput(state, terminal.Configuration{}), sequence.QueueOfferConfig{Validity: time.Minute}, start)
			require.NoError(t, err)
			if candidateTime.Before(start.Add(10 * time.Minute)) {
				require.Empty(t, offers)
			} else {
				require.Len(t, offers, 1)
				require.Equal(t, target.Callsign, offers[0].Callsign)
			}
		})
	}
}
