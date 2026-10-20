package operational

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
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
