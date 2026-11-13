package sector

import (
	"strings"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/trajectory"
	"github.com/stretchr/testify/require"
)

func TestSectorPreservesVerifiedSuffixAcrossGaps(t *testing.T) {
	for _, tc := range []struct{ name, route, extra string }{
		{name: "departure procedure", route: "ESSA UNKNOWN1A START/N0450F350 L1 JOIN"},
		{name: "missing fix", route: "NAV NDB UNKNOWN MID DCT JOIN"},
		{name: "missing airway", route: "START L99 MID JOIN"},
		{name: "disconnected airway", route: "NAV L1 MID JOIN"},
		{name: "ambiguous airway", route: "START L1 MID DCT JOIN", extra: "[LOW AIRWAY]\nL1 START START JOIN JOIN\n"},
		{name: "ambiguous fix", route: "START MID JOIN", extra: "[FIXES]\nSTART N051.00.00.000 E009.00.00.000\n"},
		{name: "missing airway entry", route: "UNKNOWN L1 MID JOIN"},
		{name: "malformed airway", route: "START L1 DCT MID JOIN"},
		{name: "consecutive airways", route: "START L1 L1 MID JOIN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, err := Read(strings.NewReader(sectorFixture+tc.extra), testConfig(), testNow)
			require.NoError(t, err)
			q := testQuery(a, tc.route)
			g, err := a.Resolve(t.Context(), q)
			require.NoError(t, err)
			require.NoError(t, g.Validate())
			require.Equal(t, navdata.CoveragePartial, g.Coverage)
			require.True(t, navdata.HasRouteGaps(g))
			require.NotEmpty(t, g.Unresolved)
			last := g.Legs[len(g.Legs)-1]
			require.Equal(t, navdata.FixID("JOIN"), *last.ToFix)
			require.Equal(t, navdata.FixID("MID"), *last.FromFix)
			var distance float64
			for _, leg := range g.Legs {
				if !navdata.IsRouteGap(leg) {
					// Missing spans cannot become straight lines to the next fix.
					require.NotEqual(t, navdata.FixID("START"), *leg.ToFix)
					if *leg.ToFix == "MID" {
						require.Equal(t, navdata.FixID("START"), *leg.FromFix)
					}
					distance += *leg.DistanceNM
				}
			}
			require.Equal(t, distance, g.TotalDistanceNM)
			again, err := a.Resolve(t.Context(), q)
			require.NoError(t, err)
			require.Equal(t, g.Digest, again.Digest)
		})
	}
}

func TestSectorDoesNotInventInboundLegToTerminalJoin(t *testing.T) {
	a, err := Read(strings.NewReader(sectorFixture), testConfig(), testNow)
	require.NoError(t, err)
	for _, route := range []string{"UNKNOWN JOIN", "START UNKNOWN JOIN", "START L99 JOIN", "NAV L1 JOIN", "START MID UNKNOWN JOIN", "START MID L1 DCT JOIN"} {
		_, err := a.Resolve(t.Context(), testQuery(a, route))
		require.Error(t, err, route)
	}
}

func TestSectorSuffixRequiresPhysicalConfirmation(t *testing.T) {
	a, err := Read(strings.NewReader(sectorFixture), testConfig(), testNow)
	require.NoError(t, err)
	route, err := a.Resolve(t.Context(), testQuery(a, "NAV NDB UNKNOWN MID JOIN"))
	require.NoError(t, err)
	join, final := navdata.FixID("JOIN"), navdata.FixID("FINAL")
	snapshot := navdata.ActiveGeometrySnapshot{
		Manifest:      navdata.ManifestCandidate{Version: a.version},
		Fixes:         []navdata.Fix{{ID: join, Position: a.fixes[join]}, {ID: final, Position: a.fixes[final]}},
		TerminalPaths: []navdata.TerminalPath{{Feeder: "JOIN", RunwayGroup: "22L", Legs: []navdata.ProcedureLeg{{ID: "TERMINAL", PathTerminator: navdata.PathTF, FromFix: &join, ToFix: &final}}}},
	}
	track := *route.Legs[len(route.Legs)-1].CourseTrueDeg
	input := trajectory.Input{Feeder: "JOIN", RunwayGroup: "22L", Observation: aman.SurveillanceFact{LatitudeDegrees: 55.7, LongitudeDegrees: 11.7, TrackTrueDegrees: &track}}
	for sample := 0; sample < 3; sample++ {
		at := testNow().Add(time.Duration(sample) * 10 * time.Second)
		input.Observation.ObservedAt = &at
		input.Observation.LatitudeDegrees += .001
		input.Observation.LongitudeDegrees += .001
		result := trajectory.Reduce(snapshot, route, input, trajectory.Config{ReferenceTime: at})
		if sample < 2 {
			require.Nil(t, result.DistanceToGoNM)
			require.Contains(t, result.Reasons, "ROUTE_RECOVERY_CONFIRMATION_PENDING")
			input.Recovery = result.Recovery
		} else {
			require.NotNil(t, result.DistanceToGoNM)
			require.True(t, result.ReacquiredProgress)
			require.Equal(t, navdata.FixID("JOIN"), result.Remaining[0].To)
			require.Equal(t, final, result.Remaining[len(result.Remaining)-1].To)
			for _, leg := range result.Remaining {
				require.NotContains(t, leg.ID, "DIRECT_TO")
			}
		}
	}
	at := testNow()
	input.Recovery = nil
	input.Observation.ObservedAt = &at
	input.Observation.LatitudeDegrees, input.Observation.LongitudeDegrees = 55.05, 10.05
	blocked := trajectory.Reduce(snapshot, route, input, trajectory.Config{ReferenceTime: at})
	require.Nil(t, blocked.DistanceToGoNM)
	require.Contains(t, blocked.Reasons, "ROUTE_GEOMETRY_GAP_AHEAD")
	input.Observation.LatitudeDegrees, input.Observation.LongitudeDegrees = 54, 10
	offRoute := trajectory.Reduce(snapshot, route, input, trajectory.Config{ReferenceTime: at})
	require.Nil(t, offRoute.DistanceToGoNM)
	require.Contains(t, offRoute.Reasons, "ROUTE_RECOVERY_POSITION_UNMATCHED")
}
