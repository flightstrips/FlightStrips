package operational

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/prediction"
	"github.com/stretchr/testify/require"
)

func TestSwapPersistsBothPositionsAtomicallyAndDeduplicates(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	state := runwayGapDisplacementState(now)
	state.Flights = []aman.AMANFlight{
		gapCommandFlight("FIRST", "north", now, 1, aman.StateStable, aman.FreezeNone),
		gapCommandFlight("MIDDLE", "north", now.Add(3*time.Minute), 2, aman.StateStable, aman.FreezeNone),
		gapCommandFlight("LAST", "north", now.Add(6*time.Minute), 3, aman.StateStable, aman.FreezeNone),
	}
	family := "TESPI"
	for i := range state.Flights {
		state.Flights[i].SelectedSTARFamily = &family
		state.Flights[i].SelectedFeeder = &family
	}
	repository := &memoryRepository{has: true, state: state}
	actions := runwayGapActions(t, repository, &recordingPublisher{}, now.Add(time.Second))
	command := aman.SwapFlightsCommand{Metadata: aman.CommandMetadata{CommandID: "swap", ExpectedRevision: state.Revision}, Callsign: "FIRST", OtherCallsign: "LAST"}
	result, err := actions.SwapFlights(context.Background(), closureAuth(now), command)
	require.NoError(t, err)
	require.Equal(t, state.Revision+1, result.CurrentRevision)
	require.Equal(t, 3, stateFlight(t, repository.state, "FIRST").Slot.Sequence)
	require.Equal(t, 2, stateFlight(t, repository.state, "MIDDLE").Slot.Sequence)
	require.Equal(t, 1, stateFlight(t, repository.state, "LAST").Slot.Sequence)
	committed := cloneGapState(t, repository.state)
	result, err = actions.SwapFlights(context.Background(), closureAuth(now), command)
	require.NoError(t, err)
	require.True(t, result.Duplicate)
	require.Equal(t, committed, repository.state)
	(&Service{}).resequence(&repository.state, now.Add(2*time.Second))
	for _, f := range repository.state.Flights {
		require.Equal(t, stateFlight(t, committed, f.Callsign).Slot, f.Slot)
	}
}

func TestSuperstableSwapSurvivesRestartPredictionRefreshAndResequencing(t *testing.T) {
	for _, freezes := range [][2]aman.FreezeReason{
		{aman.FreezeSuperstable, aman.FreezeNone},
		{aman.FreezeNone, aman.FreezeSuperstable},
		{aman.FreezeSuperstable, aman.FreezeSuperstable},
	} {
		t.Run(string(freezes[0])+"/"+string(freezes[1]), func(t *testing.T) {
			now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
			state := runwayGapDisplacementState(now)
			state.Flights = []aman.AMANFlight{
				gapCommandFlight("CCA808", "north", now.Add(10*time.Minute), 1, aman.StateStable, freezes[0]),
				gapCommandFlight("SAS1778", "north", now.Add(13*time.Minute), 2, aman.StateStable, freezes[1]),
			}
			family := "TESPI"
			for i := range state.Flights {
				state.Flights[i].SelectedSTARFamily, state.Flights[i].SelectedFeeder = &family, &family
			}
			before := cloneGapState(t, state)
			repository := &memoryRepository{has: true, state: state}
			command := aman.SwapFlightsCommand{Metadata: aman.CommandMetadata{CommandID: "swap", ExpectedRevision: state.Revision}, Callsign: "CCA808", OtherCallsign: "SAS1778"}
			actions := runwayGapActions(t, repository, &recordingPublisher{}, now.Add(time.Second))
			result, err := actions.SwapFlights(context.Background(), closureAuth(now), command)
			require.NoError(t, err)
			require.Equal(t, state.Revision+1, result.CurrentRevision)
			require.Len(t, repository.commits, 1)
			require.Equal(t, 2, stateFlight(t, repository.state, "CCA808").Slot.Sequence)
			require.Equal(t, 1, stateFlight(t, repository.state, "SAS1778").Slot.Sequence)
			committed := cloneGapState(t, repository.state)
			for _, f := range committed.Flights {
				original := stateFlight(t, before, f.Callsign)
				require.Equal(t, original.FreezeReason, f.FreezeReason)
				require.Equal(t, original.FrozenAt, f.FrozenAt)
				require.Equal(t, original.FrozenOperationalTETA, f.FrozenOperationalTETA)
				if f.FreezeReason == aman.FreezeSuperstable {
					require.Equal(t, f.Slot.Time, f.FrozenSlot.Time)
					require.Equal(t, f.Slot.Sequence, f.FrozenSlot.Sequence)
					require.Equal(t, f.Slot.RunwayGroupID, f.FrozenSlot.RunwayGroupID)
					require.Equal(t, f.Slot.Reason, f.FrozenSlot.Reason)
				}
			}
			result, err = actions.SwapFlights(context.Background(), closureAuth(now), command)
			require.NoError(t, err)
			require.True(t, result.Duplicate)
			require.Equal(t, committed, repository.state)

			// Reload persisted captures, then exercise the same prediction reducer
			// and resequencing used by reconciliation with changed physical ETAs.
			restarted := cloneGapState(t, committed)
			for i, f := range restarted.Flights {
				reduced, err := prediction.Reduce(prediction.DefaultConfig(), f, prediction.Input{
					Raw: acceptedRawPrediction(now.Add(2*time.Second), now.Add(time.Duration(25-i*5)*time.Minute)), State: f.State, Slot: f.Slot,
				})
				require.NoError(t, err)
				restarted.Flights[i] = reduced.Flight
			}
			(&Service{}).resequence(&restarted, now.Add(2*time.Second))
			for _, f := range restarted.Flights {
				previous := stateFlight(t, committed, f.Callsign)
				require.Equal(t, previous.Slot, f.Slot)
				require.Equal(t, previous.FrozenSlot, f.FrozenSlot)
				require.Equal(t, previous.FrozenOperationalTETA, f.FrozenOperationalTETA)
			}
		})
	}
}

func TestRejectedSwapDoesNotPersistEitherPosition(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	state := runwayGapDisplacementState(now)
	state.Flights = []aman.AMANFlight{
		gapCommandFlight("FIRST", "north", now, 1, aman.StateStable, aman.FreezeNone),
		gapCommandFlight("LAST", "north", now.Add(3*time.Minute), 2, aman.StateStable, aman.FreezeManual),
	}
	family := "TESPI"
	for i := range state.Flights {
		state.Flights[i].SelectedSTARFamily, state.Flights[i].SelectedFeeder = &family, &family
	}
	repository := &memoryRepository{has: true, state: state}
	before := cloneGapState(t, state)
	actions := runwayGapActions(t, repository, &recordingPublisher{}, now.Add(time.Second))
	_, err := actions.SwapFlights(context.Background(), closureAuth(now), aman.SwapFlightsCommand{
		Metadata: aman.CommandMetadata{CommandID: "swap", ExpectedRevision: state.Revision}, Callsign: "FIRST", OtherCallsign: "LAST",
	})
	require.Error(t, err)
	require.Equal(t, before, repository.state)
}
