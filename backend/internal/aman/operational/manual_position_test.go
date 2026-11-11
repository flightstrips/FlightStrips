package operational

import (
	"encoding/json"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/aman/terminal"
	"github.com/stretchr/testify/require"
)

func TestManualMoveRetainsLocalOrderAcrossPersistenceAndPromotion(t *testing.T) {
	at := time.Date(2026, 11, 9, 20, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("A")
	service := &Service{deps: Dependencies{Terminal: terminal.Configuration{RunwayGroups: []terminal.RunwayGroup{{ID: group}}}}}
	state := aman.AirportState{Airport: "EKCH", Revision: 7,
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, ActiveRatePerHour: 40, RateEffectiveAt: &at}},
	}
	for i, id := range []string{"PREVIOUS", "NEXT", "MOVED"} {
		f := operationalFlight(id, group, "MONAK", "M", at.Add(time.Duration(10+4*i)*time.Minute))
		f.State = aman.StateStable
		f.Slot = &aman.Slot{Time: f.Prediction.OperationalTETA, Sequence: i + 1, RunwayGroupID: group, Revision: 7}
		state.Flights = append(state.Flights, f)
	}
	state.Flights[2].Prediction.OperationalTETA = at.Add(12 * time.Minute)
	anchor := aman.Callsign("NEXT")
	mutation, err := service.MoveFlight(aman.CommandContext{}, aman.MoveFlightCommand{
		Metadata: aman.CommandMetadata{CommandID: "move", ExpectedRevision: 7}, Callsign: "MOVED", RunwayGroupID: group, BeforeCallsign: &anchor,
	})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	encoded, err := json.Marshal(change.State)
	require.NoError(t, err)
	var restored aman.AirportState
	require.NoError(t, json.Unmarshal(encoded, &restored))
	require.Equal(t, &aman.ManualSequencePosition{AfterCallsign: "PREVIOUS", BeforeCallsign: "NEXT"}, restored.Flights[2].ManualPosition)
	for _, f := range restored.Flights {
		require.Nil(t, f.ManualOrder)
	}
	restored.Flights[2].Prediction.OperationalTETA = at.Add(9 * time.Minute)
	changes := service.resequence(&restored, at)
	require.Len(t, changes.Promotions, 3, "the placed flight and both neighbours may improve their targets")
	require.Equal(t, at.Add(11*time.Minute), restored.Flights[2].Slot.Time)
	require.Equal(t, 1, restored.Flights[0].Slot.Sequence)
	require.Equal(t, 2, restored.Flights[2].Slot.Sequence)
	require.Equal(t, 3, restored.Flights[1].Slot.Sequence)
}

func TestResumeClearsNeighbourReferencesToLateFlight(t *testing.T) {
	at := time.Date(2026, 11, 9, 20, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("A")
	service := &Service{deps: Dependencies{Terminal: terminal.Configuration{RunwayGroups: []terminal.RunwayGroup{{ID: group}}}}}
	state := aman.AirportState{Airport: "EKCH", Revision: 7, ActiveRunwayGroups: []aman.RunwayGroupID{group},
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, ActiveRatePerHour: 40, RateEffectiveAt: &at}},
		Flights: []aman.AMANFlight{
			dispositionFlight("A", group, at.Add(20*time.Minute), at.Add(10*time.Minute), 1),
			dispositionFlight("C", group, at.Add(12*time.Minute), at.Add(12*time.Minute), 2),
			dispositionFlight("B", group, at.Add(16*time.Minute), at.Add(16*time.Minute), 3),
		},
	}
	state.Flights[0].SequenceDisposition = aman.SequenceDispositionDesequenced
	state.Flights[1].ManualPosition = &aman.ManualSequencePosition{AfterCallsign: "A", BeforeCallsign: "B"}
	before, err := json.Marshal(state)
	require.NoError(t, err)
	mutation, err := service.ResumeFlight(aman.CommandContext{Role: "EKDK_FMP", ReceivedAt: at}, aman.ResumeFlightCommand{Callsign: "A"})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	require.Equal(t, aman.SequenceDispositionActive, change.State.Flights[0].SequenceDisposition)
	require.Equal(t, at.Add(20*time.Minute), change.State.Flights[0].Slot.Time)
	require.Equal(t, state.Flights[1].Slot.Time, change.State.Flights[1].Slot.Time)
	require.Equal(t, &aman.ManualSequencePosition{BeforeCallsign: "B"}, change.State.Flights[1].ManualPosition)
	after, err := json.Marshal(state)
	require.NoError(t, err)
	require.Equal(t, before, after, "resume must not mutate the original aggregate")
	encoded, err := json.Marshal(change.State)
	require.NoError(t, err)
	var restored aman.AirportState
	require.NoError(t, json.Unmarshal(encoded, &restored))
	service.resequence(&restored, at)
	require.Equal(t, change.State.Flights[0].Slot.Time, restored.Flights[0].Slot.Time)
	require.Equal(t, change.State.Flights[1].ManualPosition, restored.Flights[1].ManualPosition)
}

func TestHoldingRepairKeepsResolvedIndirectControllerOrder(t *testing.T) {
	at := time.Date(2026, 11, 9, 20, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("A")
	input := sequence.Input{Policies: []sequence.Policy{{RunwayGroupID: group,
		Rates:           []sequence.RatePoint{{EffectiveAt: at, ArrivalsPerHour: 60}},
		SeparationRules: sequence.CPHSeparationRules(), UnknownSeparation: 3 * time.Minute,
	}}}
	state := aman.AirportState{}
	for i, id := range []aman.Callsign{"A", "B", "C"} {
		entered := at.Add(time.Duration(-i-1) * time.Minute)
		input.Flights = append(input.Flights, sequence.Flight{Callsign: id, RunwayGroupID: group,
			State: aman.StateUnstable, FreezeReason: aman.FreezeNone, OperationalTETA: at.Add(10 * time.Minute), WakeCategory: "M",
			HoldingQueueID: "HOLD", HoldingQueueTime: &entered, HoldingTransit: 5 * time.Minute,
		})
		state.Flights = append(state.Flights, operationalFlight(id, group, "MONAK", "M", at.Add(10*time.Minute)))
	}
	input.Flights[0].ManualPosition = &aman.ManualSequencePosition{BeforeCallsign: "C"}
	result, err := sequence.Generate(input)
	require.NoError(t, err)
	require.Equal(t, []aman.Callsign{"A", "C", "B"}, []aman.Callsign{result.Entries[0].Callsign, result.Entries[1].Callsign, result.Entries[2].Callsign})
	repaired := map[aman.Callsign]bool{}
	require.False(t, enforceHoldingOrder(&input, result, state, at, repaired), "repair must not restore rejected FIFO edges")
	require.Empty(t, repaired)
	for _, flight := range input.Flights {
		require.Nil(t, flight.SlotNotBefore)
	}
}
