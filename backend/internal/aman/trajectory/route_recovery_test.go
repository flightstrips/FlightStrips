package trajectory

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"github.com/stretchr/testify/require"
)

func gapFixture(t *testing.T) (navdata.ActiveGeometrySnapshot, navdata.RouteGeometry, Input) {
	snapshot, route, input := fixtureInput(t)
	for _, fix := range []struct {
		id  string
		lon float64
	}{{"D", 3}, {"E", 4}} {
		snapshot.Fixes = append(snapshot.Fixes, navdata.Fix{ID: navdata.FixID(fix.id), Position: coordinate(0, fix.lon)})
	}
	route.Digest, route.Coverage = "repaired", navdata.CoveragePartial
	route.Legs = []navdata.ProcedureLeg{
		{ID: "L1", PathTerminator: navdata.PathTF, FromFix: ptr(navdata.FixID("A")), ToFix: ptr(navdata.FixID("B"))},
		{ID: navdata.RouteGapPrefix + "SPY", PathTerminator: navdata.PathUnsupported},
		{ID: "L3", PathTerminator: navdata.PathTF, FromFix: ptr(navdata.FixID("C")), ToFix: ptr(navdata.FixID("D"))},
		{ID: "L4", PathTerminator: navdata.PathTF, FromFix: ptr(navdata.FixID("D")), ToFix: ptr(navdata.FixID("E"))},
	}
	route.Unresolved = []string{"AMBIGUOUS_ROUTE_FIX:SPY"}
	input.Observation.TrackTrueDegrees = ptr(90.0)
	return snapshot, route, input
}

func TestRouteGapRecoveryRequiresDistinctReportsAndSurvivesRestart(t *testing.T) {
	snapshot, route, input := gapFixture(t)
	input.Observation.LongitudeDegrees = 2.5
	first := Reduce(snapshot, route, input, Config{})
	require.Nil(t, first.DistanceToGoNM)
	require.NotNil(t, first.Recovery)
	require.EqualValues(t, 1, first.Recovery.Samples)
	input.Recovery = first.Recovery
	for range 4 {
		duplicate := Reduce(snapshot, route, input, Config{})
		require.Nil(t, duplicate.DistanceToGoNM)
		require.EqualValues(t, 1, duplicate.Recovery.Samples)
	}
	bytes, err := json.Marshal(input.Recovery)
	require.NoError(t, err)
	var restored aman.RouteRecovery
	require.NoError(t, json.Unmarshal(bytes, &restored))
	input.Recovery = &restored
	for sample := 2; sample <= 3; sample++ {
		input.Observation.ObservedAt = ptr(input.Observation.ObservedAt.Add(10 * time.Second))
		input.Observation.LongitudeDegrees += .01
		result := Reduce(snapshot, route, input, Config{})
		if sample < 3 {
			require.Nil(t, result.DistanceToGoNM)
			require.EqualValues(t, sample, result.Recovery.Samples)
			input.Recovery = result.Recovery
		} else {
			require.NotNil(t, result.DistanceToGoNM)
			require.InDelta(t, 89, *result.DistanceToGoNM, 1)
			require.True(t, result.ReacquiredProgress)
			require.Nil(t, result.Recovery)
			input.Prior, input.Recovery = result.Progress, nil
		}
	}
	input.Observation.LongitudeDegrees = 3.8
	input.Observation.ObservedAt = ptr(input.Observation.ObservedAt.Add(time.Second))
	jump := Reduce(snapshot, route, input, Config{})
	require.Nil(t, jump.DistanceToGoNM)
	require.Contains(t, jump.Reasons, "FORWARD_PROGRESS_OUT_OF_RANGE")
}

func TestRouteGapAheadBlocksHoldingAndClearedDirectFallbacks(t *testing.T) {
	for _, mode := range []string{"ordinary", "holding", "cleared direct", "active direct"} {
		t.Run(mode, func(t *testing.T) {
			snapshot, route, input := gapFixture(t)
			switch mode {
			case "holding":
				snapshot.Holdings = []navdata.HoldingPattern{{ID: "HOLD", Fix: "B"}}
				snapshot.TerminalPaths[0].HoldingIDs = []navdata.HoldingID{"HOLD"}
				input.HoldingClearanceFix = "B"
			case "cleared direct":
				input.RouteFact = &aman.RouteFact{ID: "dct", Fix: "B", State: aman.RouteFactCleared}
			case "active direct":
				input.RouteFact = &aman.RouteFact{ID: "dct", Fix: "B", State: aman.RouteFactActive}
			}
			result := Reduce(snapshot, route, input, Config{})
			require.Nil(t, result.DistanceToGoNM)
			require.Contains(t, result.Reasons, "ROUTE_GEOMETRY_GAP_AHEAD")
			require.False(t, DirectToTargetOnForwardPath(snapshot, route, input, "B"))
		})
	}
}

func TestExplicitDownstreamDirectCanBypassSourceGap(t *testing.T) {
	snapshot, route, input := gapFixture(t)
	input.RouteFact = &aman.RouteFact{ID: "dct", Fix: "D", State: aman.RouteFactActive}
	result := Reduce(snapshot, route, input, Config{})
	require.NotNil(t, result.DistanceToGoNM)
	require.Equal(t, "DIRECT_TO:D", result.Remaining[0].ID)
	require.True(t, DirectToTargetOnForwardPath(snapshot, route, input, "D"))
}

func TestGeometryReplacementCannotAcceptOneFalsePosition(t *testing.T) {
	snapshot, route, input := fixtureInput(t)
	initial := Reduce(snapshot, route, input, Config{})
	input.Prior = initial.Progress
	route.Digest = "corrected-source"
	input.Observation.LongitudeDegrees, input.Observation.TrackTrueDegrees = 1.5, ptr(90.0)
	first := Reduce(snapshot, route, input, Config{})
	require.Nil(t, first.DistanceToGoNM)
	require.Nil(t, first.Progress)
	input.Recovery = first.Recovery
	input.Observation.ObservedAt = ptr(input.Observation.ObservedAt.Add(time.Second))
	input.Observation.LongitudeDegrees = .6
	second := Reduce(snapshot, route, input, Config{})
	require.Nil(t, second.DistanceToGoNM)
	require.EqualValues(t, 1, second.Recovery.Samples, "backward discontinuity resets evidence")
	older := *input.Observation.ObservedAt
	input.Observation.ObservedAt = ptr(older.Add(-time.Second))
	input.Recovery = second.Recovery
	result := Reduce(snapshot, route, input, Config{})
	require.EqualValues(t, 1, result.Recovery.Samples)
	require.Equal(t, older, result.Recovery.ObservedAt)
}

func TestRecoveryUsesLocalCourseOnLongHighLatitudeLeg(t *testing.T) {
	start, end := coordinate(60, -60), coordinate(60, 0)
	distance, initialCourse := wgs84Inverse(start, end)
	initialTrack := initialCourse * 180 / math.Pi
	require.InDelta(t, 63.4, initialTrack, .1)
	for _, test := range []struct {
		name      string
		track     float64
		confirmed bool
	}{
		{name: "local course", track: 90, confirmed: true},
		{name: "outdated initial course", track: initialTrack},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot, route, input := fixtureInput(t)
			input.Prior = Reduce(snapshot, route, input, Config{}).Progress
			from, to := navdata.FixID("WEST"), navdata.FixID("EAST")
			snapshot.Fixes = []navdata.Fix{{ID: from, Position: start}, {ID: to, Position: end}}
			route.Digest = "repaired-high-latitude-leg"
			route.Legs = []navdata.ProcedureLeg{{ID: "HIGH-LATITUDE", PathTerminator: navdata.PathTF, FromFix: &from, ToFix: &to}}
			input.Observation.TrackTrueDegrees = &test.track
			at := *input.Observation.ObservedAt
			for sample := 0; sample < 3; sample++ {
				// Move eastward along the geodesic at 360 kt near its midpoint,
				// where the local course has changed from about 63.4 to 90 degrees.
				along := distance/2 - 1 + float64(sample)
				position := wgs84Direct(start, initialCourse, along)
				_, localCourse := wgs84Inverse(position, end)
				require.InDelta(t, 90, localCourse*180/math.Pi, .1)
				observedAt := at.Add(time.Duration(sample) * 10 * time.Second)
				input.Observation.LatitudeDegrees, input.Observation.LongitudeDegrees = position.LatitudeDeg, position.LongitudeDeg
				input.Observation.ObservedAt = &observedAt
				result := Reduce(snapshot, route, input, Config{ReferenceTime: observedAt, MaxObservationAge: 2 * time.Minute})
				if !test.confirmed {
					require.Nil(t, result.DistanceToGoNM)
					require.Contains(t, result.Reasons, "ROUTE_RECOVERY_TRACK_UNMATCHED")
				} else if sample < 2 {
					require.Nil(t, result.DistanceToGoNM)
					require.Contains(t, result.Reasons, "ROUTE_RECOVERY_CONFIRMATION_PENDING")
					require.EqualValues(t, sample+1, result.Recovery.Samples)
				} else {
					require.NotNil(t, result.DistanceToGoNM)
					require.InDelta(t, distance-along, *result.DistanceToGoNM, .01)
					require.True(t, result.ReacquiredProgress)
					require.InDelta(t, 90, result.Remaining[0].CourseTrueDegrees, .1)
				}
				input.Recovery = result.Recovery
			}
		})
	}
}

func TestRouteRecoveryRejectsCrossingsAndTrailingGaps(t *testing.T) {
	snapshot, route, input := gapFixture(t)
	route.Legs = append(route.Legs, navdata.ProcedureLeg{ID: navdata.RouteGapPrefix + "LAST", PathTerminator: navdata.PathUnsupported})
	input.Observation.LongitudeDegrees = 2.5
	result := Reduce(snapshot, route, input, Config{})
	require.Nil(t, result.DistanceToGoNM)
	require.Contains(t, result.Reasons, "ROUTE_GEOMETRY_GAP_AHEAD")
	input.Observation.LatitudeDegrees, input.Observation.LongitudeDegrees = 0, 0
	legs := []leg{
		{id: "cross1", a: coordinate(-1, -1), b: coordinate(1, 1)},
		{id: "connector", a: coordinate(1, 1), b: coordinate(-1, 1)},
		{id: "cross2", a: coordinate(-1, 1), b: coordinate(1, -1)},
	}
	for i := range legs {
		legs[i].distance = wgs84NM(legs[i].a, legs[i].b)
	}
	_, _, confirmed, reason := confirmRouteSection(input, legs, "crossing", Config{}.normalized())
	require.False(t, confirmed)
	require.Equal(t, "ROUTE_RECOVERY_POSITION_AMBIGUOUS", reason)
}

func TestConfirmedSuffixCannotFallbackAcrossGapOnLaterReport(t *testing.T) {
	for _, mode := range []string{"ordinary", "cleared direct", "holding", "off route"} {
		t.Run(mode, func(t *testing.T) {
			snapshot, route, input := gapFixture(t)
			input.Observation.LongitudeDegrees = 2.5
			for sample := 0; sample < 3; sample++ {
				input.Observation.ObservedAt = ptr(input.Observation.ObservedAt.Add(10 * time.Second))
				result := Reduce(snapshot, route, input, Config{})
				input.Recovery = result.Recovery
				if sample == 2 {
					require.NotNil(t, result.Progress)
					input.Prior = result.Progress
				}
			}
			switch mode {
			case "holding":
				snapshot.Holdings = []navdata.HoldingPattern{{ID: "HOLD", Fix: "D"}}
				snapshot.TerminalPaths[0].HoldingIDs = []navdata.HoldingID{"HOLD"}
				input.HoldingClearanceFix = "D"
			case "cleared direct":
				input.RouteFact = &aman.RouteFact{ID: "cleared", State: aman.RouteFactCleared, Fix: "D"}
			}
			input.Observation.ObservedAt = ptr(input.Observation.ObservedAt.Add(time.Second))
			if mode == "off route" {
				input.Observation.LatitudeDegrees = 1
			} else {
				input.Observation.LongitudeDegrees = .5
			}
			result := Reduce(snapshot, route, input, Config{})
			require.Nil(t, result.DistanceToGoNM, "a target on the suffix cannot justify a new segment across the gap")
		})
	}
}

func TestGapSeparatedAdjacentLegsAreCompetingSections(t *testing.T) {
	_, _, input := fixtureInput(t)
	input.Observation.LatitudeDegrees, input.Observation.LongitudeDegrees = .001, 0
	legs := []leg{
		{id: "PREFIX", a: coordinate(0, -1), b: coordinate(0, 1), gapAhead: true},
		{id: "SUFFIX", a: coordinate(.001, -1), b: coordinate(.001, 1)},
	}
	for i := range legs {
		legs[i].distance = wgs84NM(legs[i].a, legs[i].b)
	}
	_, _, confirmed, reason := confirmRouteSection(input, legs, "gap-crossing", Config{}.normalized())
	require.False(t, confirmed)
	require.Equal(t, "ROUTE_RECOVERY_POSITION_AMBIGUOUS", reason)
}

func TestUnmatchedAndOutOfOrderReportsCannotEraseRecoveryWatermark(t *testing.T) {
	snapshot, route, input := gapFixture(t)
	input.Observation.LongitudeDegrees = 2.5
	at := input.Observation.ObservedAt.Add(20 * time.Second)
	input.Observation.ObservedAt = &at
	first := Reduce(snapshot, route, input, Config{})
	input.Recovery = first.Recovery
	for _, position := range []struct{ lat, lon float64 }{{2, 2.5}, {0, .5}, {0, 2.5}} {
		input.Observation.ObservedAt = ptr(at.Add(-time.Second))
		input.Observation.LatitudeDegrees, input.Observation.LongitudeDegrees = position.lat, position.lon
		older := Reduce(snapshot, route, input, Config{})
		require.NotNil(t, older.Recovery)
		require.EqualValues(t, 1, older.Recovery.Samples)
		require.Equal(t, at, older.Recovery.ObservedAt)
		input.Recovery = older.Recovery
	}
	newerAt := at.Add(10 * time.Second)
	input.Observation.ObservedAt = &newerAt
	input.Observation.LatitudeDegrees = 2
	unmatched := Reduce(snapshot, route, input, Config{})
	require.NotNil(t, unmatched.Recovery)
	require.Zero(t, unmatched.Recovery.Samples)
	require.Equal(t, newerAt, unmatched.Recovery.ObservedAt)
	input.Recovery = unmatched.Recovery
	input.Observation.LatitudeDegrees = 0
	for second := 1; second <= 3; second++ {
		input.Observation.ObservedAt = ptr(at.Add(time.Duration(second) * time.Second))
		older := Reduce(snapshot, route, input, Config{})
		require.Nil(t, older.DistanceToGoNM)
		require.Zero(t, older.Recovery.Samples)
		require.Equal(t, newerAt, older.Recovery.ObservedAt)
		input.Recovery = older.Recovery
	}
}

func TestConfirmedSuffixCanRecoverAfterObservationOutage(t *testing.T) {
	snapshot, route, input := gapFixture(t)
	input.Observation.LongitudeDegrees = 2.5
	for sample := 0; sample < 3; sample++ {
		input.Observation.ObservedAt = ptr(input.Observation.ObservedAt.Add(10 * time.Second))
		result := Reduce(snapshot, route, input, Config{})
		input.Recovery = result.Recovery
		if sample == 2 {
			require.NotNil(t, result.Progress)
			input.Prior = result.Progress
		}
	}
	input.Observation.ObservedAt = ptr(input.Observation.ObservedAt.Add(3 * time.Minute))
	input.Observation.LongitudeDegrees = 2.7
	for sample := 0; sample < 3; sample++ {
		input.Observation.ObservedAt = ptr(input.Observation.ObservedAt.Add(10 * time.Second))
		input.Observation.LongitudeDegrees += .01
		result := Reduce(snapshot, route, input, Config{})
		if sample < 2 {
			require.Nil(t, result.DistanceToGoNM)
			require.NotNil(t, result.Recovery)
			input.Recovery = result.Recovery
		} else {
			require.NotNil(t, result.DistanceToGoNM)
			require.True(t, result.ReacquiredProgress)
			require.Equal(t, *input.Observation.ObservedAt, result.Progress.PhysicalPosition.ObservedAt)
		}
	}
}

func TestInvalidNewerPositionPreservesZeroSampleRecoveryWatermark(t *testing.T) {
	snapshot, route, input := gapFixture(t)
	input.Observation.LongitudeDegrees = 2.5
	start := *input.Observation.ObservedAt
	for _, seconds := range []int{10, 20} {
		input.Observation.ObservedAt = ptr(start.Add(time.Duration(seconds) * time.Second))
		result := Reduce(snapshot, route, input, Config{})
		require.Nil(t, result.DistanceToGoNM)
		input.Recovery = result.Recovery
	}
	require.EqualValues(t, 2, input.Recovery.Samples)
	invalidAt := start.Add(30 * time.Second)
	input.Observation.ObservedAt = &invalidAt
	input.Observation.LatitudeDegrees = math.NaN()
	invalid := Reduce(snapshot, route, input, Config{})
	require.Contains(t, invalid.Reasons, "INVALID_POSITION")
	require.NotNil(t, invalid.Recovery)
	require.Zero(t, invalid.Recovery.Samples)
	require.Equal(t, invalidAt, invalid.Recovery.ObservedAt)
	require.False(t, math.IsNaN(invalid.Recovery.LatitudeDegrees))
	input.Recovery = invalid.Recovery
	input.Observation.LatitudeDegrees = 0
	for _, seconds := range []int{21, 22, 23} {
		input.Observation.ObservedAt = ptr(start.Add(time.Duration(seconds) * time.Second))
		older := Reduce(snapshot, route, input, Config{})
		require.Nil(t, older.DistanceToGoNM)
		require.Zero(t, older.Recovery.Samples)
		require.Equal(t, invalidAt, older.Recovery.ObservedAt)
		input.Recovery = older.Recovery
	}
}
