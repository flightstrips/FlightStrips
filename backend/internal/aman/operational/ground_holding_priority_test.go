package operational

import (
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/terminal"
	"github.com/stretchr/testify/require"
)

func TestGroundArrivalReleasesEarlyReservationAndFollowsAllRunwayHolds(t *testing.T) {
	now := time.Date(2026, time.October, 4, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("ARRIVAL-22")
	service := &Service{deps: Dependencies{Terminal: terminal.Configuration{RunwayGroups: []terminal.RunwayGroup{{ID: group}}}}}
	for _, freeze := range []aman.FreezeReason{aman.FreezeNone, aman.FreezeManual, aman.FreezeSuperstable, aman.FreezeTMA} {
		t.Run(string(freeze), func(t *testing.T) {
			ground := gapCommandFlight("GROUND", group, now.Add(15*time.Minute), 1, aman.StatePlanned, freeze)
			ground.Prediction.ModelVersion = "aman-planned-eobt-exot-eet-v1"
			ground.Prediction.GeneratedAt, ground.Prediction.InputObservedAt = now, now
			ground.LatestObservation = groundPriorityObservation(ground.Callsign, now)
			order := 1
			ground.ManualOrder = &order
			a := groundPriorityHold("HELD-A", "MONAK", group, now, now.Add(24*time.Minute), 2)
			b := groundPriorityHold("HELD-B", "TESPI", group, now, now.Add(27*time.Minute), 3)
			state := service.initialState("EKCH", now)
			state.Revision, state.Flights = 7, []aman.AMANFlight{ground, a, b}
			service.resequence(&state, now)
			got := stateFlight(t, state, "GROUND")
			require.Nil(t, got.Slot)
			require.Nil(t, got.FrozenSlot)
			require.Nil(t, got.ManualOrder)
			require.Equal(t, aman.FreezeNone, got.FreezeReason)
			require.Equal(t, now.Add(15*time.Minute), got.Prediction.RawTETA, "filed/physical timing is preserved")
			require.Equal(t, now.Add(28*time.Minute+30*time.Second), got.Prediction.OperationalTETA)
			require.Equal(t, aman.OperationalReasonHoldingPriority, got.Prediction.OperationalReason)
			require.Equal(t, a.Slot.Time, stateFlight(t, state, "HELD-A").Slot.Time)
			require.Equal(t, b.Slot.Time, stateFlight(t, state, "HELD-B").Slot.Time)
			require.Equal(t, a.FrozenSlot, stateFlight(t, state, "HELD-A").FrozenSlot)
			require.Equal(t, b.FrozenSlot, stateFlight(t, state, "HELD-B").FrozenSlot)
			// A repeated update or restart recomputes the same soft target.
			state = cloneGapState(t, state)
			service.resequence(&state, now.Add(time.Minute))
			require.Equal(t, got.Prediction.OperationalTETA, stateFlight(t, state, "GROUND").Prediction.OperationalTETA)
		})
	}
}

func TestGroundHoldingPriorityRespondsToRateGapsAndReleasedHolds(t *testing.T) {
	now := time.Date(2026, time.October, 4, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("ARRIVAL-22")
	service := &Service{deps: Dependencies{Terminal: terminal.Configuration{RunwayGroups: []terminal.RunwayGroup{{ID: group}}}}}
	ground := gapCommandFlight("GROUND", group, now.Add(15*time.Minute), 1, aman.StatePlanned, aman.FreezeNone)
	ground.Slot, ground.Order = nil, nil
	ground.LatestObservation = groundPriorityObservation(ground.Callsign, now)
	a := groundPriorityHold("HELD-A", "MONAK", group, now, now.Add(24*time.Minute), 2)
	b := groundPriorityHold("HELD-B", "TESPI", group, now, now.Add(27*time.Minute), 3)
	state := service.initialState("EKCH", now)
	state.Revision, state.Flights = 7, []aman.AMANFlight{ground, a, b}
	service.resequence(&state, now)
	require.Equal(t, now.Add(28*time.Minute+30*time.Second), state.Flights[0].Prediction.OperationalTETA)
	state.RunwayGroups[0].RateSchedule = []aman.RunwayGroupRatePoint{{EffectiveAt: now, ArrivalsPerHour: 60}}
	state.RunwayGroups[0].ActiveRatePerHour = 60
	service.resequence(&state, now)
	require.Equal(t, now.Add(28*time.Minute), state.Flights[0].Prediction.OperationalTETA)
	state.RunwayGroups[0].Gaps = []aman.RunwayGap{{ID: "stop", Start: now.Add(28 * time.Minute), End: now.Add(33 * time.Minute)}}
	service.resequence(&state, now)
	require.Equal(t, now.Add(33*time.Minute), state.Flights[0].Prediction.OperationalTETA)
	state.RunwayGroups[0].Gaps = nil
	state.Flights[2].State = aman.StateRemoved
	clearSequencingState(&state.Flights[2])
	service.resequence(&state, now)
	require.Equal(t, now.Add(25*time.Minute), state.Flights[0].Prediction.OperationalTETA)
	state.Flights[1].State = aman.StateRemoved
	clearSequencingState(&state.Flights[1])
	service.resequence(&state, now)
	require.Equal(t, ground.Prediction.RawTETA, state.Flights[0].Prediction.OperationalTETA)
	require.Equal(t, aman.OperationalReasonPredicted, state.Flights[0].Prediction.OperationalReason)
}

func TestGroundPriorityRecoversWhenCapacityBecomesAvailable(t *testing.T) {
	now := time.Date(2026, time.October, 4, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("ARRIVAL-22")
	service := &Service{deps: Dependencies{Terminal: terminal.Configuration{RunwayGroups: []terminal.RunwayGroup{{ID: group}}}}}
	ground := gapCommandFlight("GROUND", group, now.Add(15*time.Minute), 1, aman.StatePlanned, aman.FreezeNone)
	ground.Slot, ground.Order = nil, nil
	ground.LatestObservation = groundPriorityObservation(ground.Callsign, now)
	held := groundPriorityHold("HELD", "MONAK", group, now, now.Add(27*time.Minute), 2)
	state := service.initialState("EKCH", now)
	state.Revision, state.Flights = 7, []aman.AMANFlight{ground, held}
	state.RunwayGroups[0].RateSchedule, state.RunwayGroups[0].ActiveRatePerHour = nil, 0
	service.resequence(&state, now)
	require.False(t, state.Flights[0].Prediction.Publishable)
	state.RunwayGroups[0].RateSchedule = []aman.RunwayGroupRatePoint{{EffectiveAt: now, ArrivalsPerHour: 40}}
	state.RunwayGroups[0].ActiveRatePerHour = 40
	service.resequence(&state, now)
	require.True(t, state.Flights[0].Prediction.Publishable)
	require.Equal(t, now.Add(28*time.Minute+30*time.Second), state.Flights[0].Prediction.OperationalTETA)
	require.Nil(t, state.Flights[0].Prediction.DegradationReason)
}

func TestRateCommandImmediatelyRefreshesProvisionalGroundTiming(t *testing.T) {
	now := time.Date(2026, time.October, 4, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("ARRIVAL-22")
	service := &Service{deps: Dependencies{Terminal: terminal.Configuration{RunwayGroups: []terminal.RunwayGroup{{ID: group}}}}}
	ground := gapCommandFlight("GROUND", group, now.Add(15*time.Minute), 1, aman.StatePlanned, aman.FreezeManual)
	ground.Slot, ground.Order, ground.FrozenSlot = nil, nil, nil
	ground.Prediction.OperationalReason = aman.OperationalReasonManualOverride
	ground.LatestObservation = groundPriorityObservation(ground.Callsign, now)
	held := groundPriorityHold("HELD", "MONAK", group, now, now.Add(27*time.Minute), 2)
	state := service.initialState("EKCH", now)
	state.Revision, state.Flights = 7, []aman.AMANFlight{ground, held}
	service.resequence(&state, now)
	require.Equal(t, aman.FreezeNone, state.Flights[0].FreezeReason, "a pending ground manual ETA cannot bypass holding priority")
	mutation, err := service.SetRate(aman.CommandContext{ReceivedAt: now}, aman.SetRateCommand{
		Metadata: aman.CommandMetadata{CommandID: "ground-rate", ExpectedRevision: 7}, RunwayGroupID: group, ArrivalsPerHour: 60, EffectiveAt: now,
	})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	require.True(t, change.Changed)
	require.Equal(t, now.Add(28*time.Minute), stateFlight(t, change.State, "GROUND").Prediction.OperationalTETA)
	require.Equal(t, held.Slot.Time, stateFlight(t, change.State, "HELD").Slot.Time)
}

func TestReleasingGroundManualReservationDoesNotBeatItsFiledArrival(t *testing.T) {
	now := time.Date(2026, time.October, 4, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("ARRIVAL-22")
	service := &Service{deps: Dependencies{Terminal: terminal.Configuration{RunwayGroups: []terminal.RunwayGroup{{ID: group}}}}}
	ground := gapCommandFlight("GROUND", group, now.Add(15*time.Minute), 1, aman.StatePlanned, aman.FreezeManual)
	ground.Prediction.OperationalReason = aman.OperationalReasonManualOverride
	ground.Prediction.RawTETA = now.Add(50 * time.Minute)
	state := service.initialState("EKCH", now)
	state.Revision, state.Flights = 7, []aman.AMANFlight{ground, groundPriorityHold("HELD", "MONAK", group, now, now.Add(27*time.Minute), 2)}
	service.resequence(&state, now)
	require.Nil(t, state.Flights[0].Slot)
	require.Equal(t, aman.FreezeNone, state.Flights[0].FreezeReason)
	require.Equal(t, now.Add(50*time.Minute), state.Flights[0].Prediction.OperationalTETA)
}

func groundPriorityHold(callsign, holding string, group aman.RunwayGroupID, now, at time.Time, order int) aman.AMANFlight {
	flight := gapCommandFlight(callsign, group, at, order, aman.StateStable, aman.FreezeManual)
	flight.LatestObservation = groundPriorityObservation(callsign, now)
	flight.SelectedHolding = &holding
	entered := now.Add(-time.Minute)
	flight.HoldingStack = &aman.HoldingStackState{HoldingID: holding, Confirmed: true, FirstObservedAt: entered, CandidateObservedAt: now, ConsecutiveObservations: 2}
	flight.Prediction.HoldingFixETA = &entered
	flight.Prediction.GeneratedAt, flight.Prediction.InputObservedAt = now, now
	flight.Prediction.RawTETA = at.Add(-5 * time.Minute)
	return flight
}

func groundPriorityObservation(callsign string, now time.Time) *aman.FlightObservation {
	wake := "M"
	return &aman.FlightObservation{Callsign: callsign, Origin: "ENGM", Destination: "EKCH", WakeCategory: &wake, SourceStatus: aman.DataFresh, ReconciledAt: now}
}
