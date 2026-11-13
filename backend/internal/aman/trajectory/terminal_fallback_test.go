package trajectory

import (
	"FlightStrips/internal/aman/navdata"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestTerminalFallbackJoinsEntryThenUsesRemainingSTAR(t *testing.T) {
	snapshot, route, input := fixtureInput(t)
	now := *input.Observation.ObservedAt
	path := snapshot.TerminalPaths[0]
	path.Version = route.Version
	path.Legs = route.Legs
	path.Coverage = navdata.CoverageComplete
	path.Digest = "terminal"
	path.FeederFix = "B"
	snapshot.TerminalPaths[0] = path
	input.FeederFix = "B"
	input.Approach = nil
	input.Observation.LatitudeDegrees = 2
	input.Observation.LongitudeDegrees = -1
	joined, err := ProjectTerminalFallback(snapshot, path, input, Config{ReferenceTime: now, MaxObservationAge: 2 * time.Minute})
	require.NoError(t, err)
	require.Equal(t, "BASIC_JOIN", joined.Remaining[0].ID)
	require.Equal(t, navdata.FixID("A"), joined.Remaining[0].To)
	input.Observation.LatitudeDegrees = 0
	input.Observation.LongitudeDegrees = 1.5
	terminal, err := ProjectTerminalFallback(snapshot, path, input, Config{ReferenceTime: now, MaxObservationAge: 2 * time.Minute})
	require.NoError(t, err)
	require.NotEqual(t, "BASIC_JOIN", terminal.Remaining[0].ID)
	require.Equal(t, navdata.FixID("C"), terminal.Remaining[0].To)
	require.Less(t, *terminal.DistanceToGoNM, *joined.DistanceToGoNM)
	old := now.Add(-3 * time.Minute)
	input.Observation.ObservedAt = &old
	_, err = ProjectTerminalFallback(snapshot, path, input, Config{ReferenceTime: now, MaxObservationAge: 2 * time.Minute})
	require.Error(t, err)
	path.Coverage = navdata.CoveragePartial
	input.Observation.ObservedAt = &now
	_, err = ProjectTerminalFallback(snapshot, path, input, Config{ReferenceTime: now, MaxObservationAge: 2 * time.Minute})
	require.Error(t, err)
}
