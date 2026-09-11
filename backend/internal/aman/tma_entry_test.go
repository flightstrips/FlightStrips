package aman

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestObserveTMAContainmentCapturesOnlyFirstOutsideToInsideEdge(t *testing.T) {
	base := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	var state *TMAEntryState

	for index, test := range []struct {
		contained, wantTrigger, wantSticky bool
	}{
		{contained: true}, // No accepted prior outside observation.
		{contained: false},
		{contained: true, wantTrigger: true, wantSticky: true},
		{contained: false, wantSticky: true},
		{contained: true, wantSticky: true},
	} {
		next, triggered, err := ObserveTMAContainment(state, test.contained, base.Add(time.Duration(index)*time.Second))
		require.NoError(t, err)
		require.Equal(t, test.wantTrigger, triggered)
		require.Equal(t, test.wantSticky, next.FreezeTriggered)
		state = &next
	}
}

func TestObserveTMAContainmentIsIdempotentAndOrdered(t *testing.T) {
	at := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	previous := &TMAEntryState{LastContainment: TMAOutside, LastObservedAt: at}

	same, triggered, err := ObserveTMAContainment(previous, false, at)
	require.NoError(t, err)
	require.False(t, triggered)
	require.Equal(t, *previous, same)

	_, _, err = ObserveTMAContainment(previous, true, at)
	assertInvalidArgument(t, err)
	_, _, err = ObserveTMAContainment(previous, true, at.Add(-time.Second))
	assertInvalidArgument(t, err)
}

func TestTMAEntryReplaySurvivesRestartAtEveryObservation(t *testing.T) {
	base := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	observations := []bool{false, false, true, false, true}
	replay := func(restartEveryStep bool) (TMAEntryState, int) {
		var state *TMAEntryState
		captures := 0
		for index, contained := range observations {
			next, triggered, err := ObserveTMAContainment(state, contained, base.Add(time.Duration(index)*time.Second))
			require.NoError(t, err)
			if triggered {
				captures++
			}
			state = &next
			if restartEveryStep {
				encoded, err := json.Marshal(state)
				require.NoError(t, err)
				state = nil
				require.NoError(t, json.Unmarshal(encoded, &state))
			}
		}
		return *state, captures
	}

	uninterrupted, uninterruptedCaptures := replay(false)
	restarted, restartedCaptures := replay(true)
	require.Equal(t, uninterrupted, restarted)
	require.Equal(t, 1, uninterruptedCaptures)
	require.Equal(t, uninterruptedCaptures, restartedCaptures)
}
