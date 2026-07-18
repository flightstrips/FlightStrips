package trajectory

import (
	"math"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"github.com/stretchr/testify/require"
)

func TestReduceGoldenDTGMonotonicJitterAndCompatibilityReset(t *testing.T) {
	snapshot, route, input := fixtureInput(t)
	first := Reduce(snapshot, route, input, Config{})
	require.Equal(t, Complete, first.Completeness)
	require.InDelta(t, 90, *first.DistanceToGoNM, 1)
	input.Observation.LongitudeDegrees = .75
	input.Prior = first.Progress
	forward := Reduce(snapshot, route, input, Config{})
	require.Less(t, *forward.DistanceToGoNM, *first.DistanceToGoNM)
	input.Observation.LongitudeDegrees = .7465 // routine backward tracking jitter
	input.Observation.LatitudeDegrees = .01
	input.Prior = forward.Progress
	jitter := Reduce(snapshot, route, input, Config{})
	require.Equal(t, forward.AlongTrackNM, jitter.AlongTrackNM, "compatible progress never moves backward")
	require.Greater(t, jitter.CrossTrackNM, 0.5, "clamping retains observed lateral error")
	input.FlightPlanRevision++
	reset := Reduce(snapshot, route, input, Config{})
	require.Less(t, reset.AlongTrackNM, forward.AlongTrackNM)
}

func TestReduceReportsActualOffRouteCrossTrackAndThreshold(t *testing.T) {
	snapshot, route, input := fixtureInput(t)
	input.Observation.LatitudeDegrees, input.Observation.LongitudeDegrees = .21, .5
	off := Reduce(snapshot, route, input, Config{MaxCrossTrackNM: 12})
	_, want := geodesicProject(coordinate(.21, .5), coordinate(0, 0), coordinate(0, 1), wgs84NM(coordinate(0, 0), coordinate(0, 1)))
	require.Equal(t, OffRoute, off.Completeness)
	require.InDelta(t, want, off.CrossTrackNM, .01)
	require.Greater(t, off.CrossTrackNM, 12.0)
	input.Observation.LatitudeDegrees = .19
	on := Reduce(snapshot, route, input, Config{MaxCrossTrackNM: 12})
	require.NotEqual(t, OffRoute, on.Completeness)
	require.LessOrEqual(t, on.CrossTrackNM, 12.0)
}

func TestReduceDistinguishesForwardProgressJumpFromOffRoute(t *testing.T) {
	snapshot, route, input := fixtureInput(t)
	input.Prior = &aman.RouteProgress{GeometryDigest: "digest", ManifestRevision: 1, TerminalDigest: "term", FlightPlanRevision: 7, RunwayGroupID: "G", LegIndex: 0, RejoinLegIndex: 0, AlongTrackNM: 6}
	input.Observation.LongitudeDegrees = .9
	onRoute := Reduce(snapshot, route, input, Config{MaxForwardSearchNM: 10})
	require.Equal(t, Partial, onRoute.Completeness)
	require.Contains(t, onRoute.Reasons, "FORWARD_PROGRESS_OUT_OF_RANGE")
	require.NotContains(t, onRoute.Reasons, "OFF_ROUTE")
	require.InDelta(t, 0, onRoute.CrossTrackNM, .01)
	input.Observation.LatitudeDegrees = 1
	lateral := Reduce(snapshot, route, input, Config{MaxForwardSearchNM: 10})
	require.Equal(t, OffRoute, lateral.Completeness)
	require.Contains(t, lateral.Reasons, "OFF_ROUTE")
	require.Greater(t, lateral.CrossTrackNM, 12.0)
}

func TestComposeOrderedRouteTerminalApproachMissedAndUnsupportedGap(t *testing.T) {
	snapshot, route, input := fixtureInput(t)
	a, b, c, d, e := navdata.FixID("A"), navdata.FixID("B"), navdata.FixID("C"), navdata.FixID("D"), navdata.FixID("E")
	snapshot.Fixes = []navdata.Fix{{ID: a, Position: coordinate(0, 0)}, {ID: b, Position: coordinate(0, 1)}, {ID: c, Position: coordinate(0, 2)}, {ID: d, Position: coordinate(0, 3)}, {ID: e, Position: coordinate(0, 4)}}
	app, missed := navdata.ProcedureID("APP"), navdata.ProcedureID("MISSED")
	snapshot.Procedures = []navdata.Procedure{{ID: app, Legs: []navdata.ProcedureLeg{{ID: "APPLEG", PathTerminator: navdata.PathTF, FromFix: &c, ToFix: &d}}}, {ID: missed, Legs: []navdata.ProcedureLeg{{ID: "MISSEDLEG", PathTerminator: navdata.PathTF, FromFix: &d, ToFix: &e}}}}
	snapshot.TerminalPaths[0].Legs = []navdata.ProcedureLeg{{ID: "TERM", PathTerminator: navdata.PathTF, FromFix: &b, ToFix: &c}}
	route.Legs = []navdata.ProcedureLeg{{ID: "FILED", PathTerminator: navdata.PathTF, FromFix: &a, ToFix: &b}}
	input.Approach, input.MissedApproach = &app, &missed
	result := Reduce(snapshot, route, input, Config{})
	require.Equal(t, []string{"FILED", "TERM", "APPLEG", "MISSEDLEG"}, legIDs(result.Remaining))
	vectorTo := c
	route.Legs = []navdata.ProcedureLeg{{ID: "FILED", PathTerminator: navdata.PathTF, FromFix: &a, ToFix: &b}, {ID: "VECTOR", PathTerminator: navdata.PathUnsupported, ToFix: &vectorTo}, {ID: "AFTER", PathTerminator: navdata.PathTF, ToFix: &d}}
	input.Approach, input.MissedApproach = nil, nil
	gap := Reduce(snapshot, route, input, Config{})
	require.Equal(t, []string{"FILED", "TERM"}, legIDs(gap.Remaining))
	require.Contains(t, gap.Reasons, "UNRESOLVED_LEG:AFTER:MISSING_FIX")
}

func TestDirectToRepeatedTargetUsesOnlyCompatibleRejoinFloor(t *testing.T) {
	snapshot, route, input := fixtureInput(t)
	a, b, c := navdata.FixID("A"), navdata.FixID("B"), navdata.FixID("C")
	route.Legs = []navdata.ProcedureLeg{{ID: "L1", PathTerminator: navdata.PathTF, FromFix: &a, ToFix: &b}, {ID: "L2", PathTerminator: navdata.PathTF, FromFix: &b, ToFix: &a}, {ID: "L3", PathTerminator: navdata.PathTF, FromFix: &a, ToFix: &b}, {ID: "L4", PathTerminator: navdata.PathTF, FromFix: &b, ToFix: &a}, {ID: "L5", PathTerminator: navdata.PathTF, FromFix: &a, ToFix: &c}}
	input.RouteFact = &aman.RouteFact{ID: "dct", Fix: "A"}
	input.Prior = &aman.RouteProgress{GeometryDigest: "digest", ManifestRevision: 1, TerminalDigest: "term", FlightPlanRevision: 7, RouteFactID: "dct", RunwayGroupID: "G", LegIndex: 0, RejoinLegIndex: 3, AlongTrackNM: 0}
	stable := Reduce(snapshot, route, input, Config{})
	require.Equal(t, []string{"DIRECT_TO:A", "L5"}, legIDs(stable.Remaining))
	require.Equal(t, 3, stable.Progress.RejoinLegIndex)
	route.Digest = "amended"
	amended := Reduce(snapshot, route, input, Config{})
	require.Equal(t, []string{"DIRECT_TO:A", "L3", "L4", "L5"}, legIDs(amended.Remaining))
	require.Equal(t, 1, amended.Progress.RejoinLegIndex)
}

func TestReduceProjectsLongLegAndPositionsBeforeAndAfterPath(t *testing.T) {
	snapshot, route, input := fixtureInput(t)
	route.Legs = route.Legs[:1]
	snapshot.Fixes[1].Position.LongitudeDeg = 5
	input.Observation.LongitudeDegrees = .5
	middle := Reduce(snapshot, route, input, Config{})
	require.Equal(t, Complete, middle.Completeness)
	require.InDelta(t, 270, *middle.DistanceToGoNM, 3)
	input.Observation.LongitudeDegrees = -.1
	before := Reduce(snapshot, route, input, Config{})
	require.InDelta(t, routeDistance(snapshot, "A", "B"), *before.DistanceToGoNM, .1)
	input.Observation.LongitudeDegrees = 5.1
	after := Reduce(snapshot, route, input, Config{})
	require.InDelta(t, 0, *after.DistanceToGoNM, .1)
}

func TestReduceRepeatedFixAndCrossingChoosePlausibleForwardLeg(t *testing.T) {
	snapshot, route, input := fixtureInput(t)
	a, b, c, d := navdata.FixID("A"), navdata.FixID("B"), navdata.FixID("C"), navdata.FixID("D")
	snapshot.Fixes = []navdata.Fix{{ID: a, Position: coordinate(-1, -1)}, {ID: b, Position: coordinate(1, 1)}, {ID: c, Position: coordinate(-1, 1)}, {ID: d, Position: coordinate(1, -1)}}
	route.Legs = []navdata.ProcedureLeg{{ID: "EARLY", PathTerminator: navdata.PathTF, FromFix: &a, ToFix: &b}, {ID: "JOIN", PathTerminator: navdata.PathTF, FromFix: &b, ToFix: &c}, {ID: "LATE", PathTerminator: navdata.PathTF, FromFix: &c, ToFix: &d}}
	input.Observation.LatitudeDegrees, input.Observation.LongitudeDegrees = 0, 0
	first := Reduce(snapshot, route, input, Config{})
	require.NotEmpty(t, first.Remaining, first.Reasons)
	require.Equal(t, "EARLY", first.Remaining[0].ID, "without progress, earliest plausible crossing wins")
	input.Prior = &aman.RouteProgress{GeometryDigest: "digest", ManifestRevision: 1, TerminalDigest: "term", FlightPlanRevision: 7, RunwayGroupID: "G", LegIndex: 0, RejoinLegIndex: 0, AlongTrackNM: 80}
	input.Observation.LatitudeDegrees, input.Observation.LongitudeDegrees = .1, .1
	next := Reduce(snapshot, route, input, Config{MaxForwardSearchNM: 20})
	require.Equal(t, "EARLY", next.Remaining[0].ID, "a later crossing cannot steal projection")
}

func TestReduceDirectToEmptyStateRestartAndHoldingWithoutCircuit(t *testing.T) {
	snapshot, route, input := fixtureInput(t)
	holdID := navdata.HoldingID("HOLD")
	snapshot.Holdings = []navdata.HoldingPattern{{ID: holdID, Fix: "B"}}
	snapshot.TerminalPaths[0].HoldingIDs = []navdata.HoldingID{holdID}
	base := Reduce(snapshot, route, input, Config{})
	input.RouteFact = &aman.RouteFact{ID: "dct", Fix: "B"} // legacy empty state remains active
	input.Prior = base.Progress
	direct := Reduce(snapshot, route, input, Config{})
	require.Equal(t, "DIRECT_TO:B", direct.Remaining[0].ID)
	require.NotNil(t, direct.SelectedHolding)
	require.InDelta(t, wgs84NM(coordinate(0, .5), coordinate(0, 1))+wgs84NM(coordinate(0, 1), coordinate(0, 2)), *direct.DistanceToGoNM, .1, "published hold adds no circuit distance")
	input.Prior = direct.Progress
	restarted := Reduce(snapshot, route, input, Config{})
	require.Equal(t, direct.Progress.RejoinLegIndex, restarted.Progress.RejoinLegIndex)
	input.RouteFact.State = aman.RouteFactExpired
	expired := Reduce(snapshot, route, input, Config{})
	require.NotEqual(t, "DIRECT_TO:B", expired.Remaining[0].ID)
}

func TestReduceResetsForManifestRunwayRouteAmendmentAndReportsFragments(t *testing.T) {
	snapshot, route, input := fixtureInput(t)
	first := Reduce(snapshot, route, input, Config{})
	input.Prior = first.Progress
	input.Observation.LongitudeDegrees = .9
	snapshot.ManifestRevision++
	manifestReset := Reduce(snapshot, route, input, Config{})
	require.False(t, compatible(first.Progress, route.Digest, snapshot, input))
	require.Equal(t, snapshot.ManifestRevision, manifestReset.Progress.ManifestRevision)
	input.Prior = first.Progress
	input.RunwayGroup = "OTHER"
	runwayReset := Reduce(snapshot, route, input, Config{})
	require.False(t, compatible(first.Progress, route.Digest, snapshot, input))
	require.Contains(t, runwayReset.Reasons, "TERMINAL_PATH_UNRESOLVED")
	input.RunwayGroup = "G"
	input.Prior = first.Progress
	route.Digest = "amended"
	amended := Reduce(snapshot, route, input, Config{})
	require.NotEqual(t, first.Progress.GeometryDigest, amended.Progress.GeometryDigest)
	route.Legs[1].PathTerminator = navdata.PathUnsupported
	partial := Reduce(snapshot, route, input, Config{})
	require.Equal(t, Partial, partial.Completeness)
	require.Equal(t, "L1", partial.Remaining[0].ID)
	require.Contains(t, partial.Reasons, "UNSUPPORTED_LEG:L2:UNSUPPORTED")
	input.Observation.LatitudeDegrees = math.NaN()
	invalid := Reduce(snapshot, route, input, Config{})
	require.Equal(t, Unresolved, invalid.Completeness)
	require.Contains(t, invalid.Reasons, "INVALID_POSITION")
}

func TestReduceOffRouteStaleDirectAndPartialReasons(t *testing.T) {
	snapshot, route, input := fixtureInput(t)
	input.Observation.LatitudeDegrees = 5
	off := Reduce(snapshot, route, input, Config{})
	require.Equal(t, OffRoute, off.Completeness)
	input.Observation.LatitudeDegrees, input.Observation.LongitudeDegrees = 0, .25
	input.RouteFact = &aman.RouteFact{ID: "dct-1", Fix: "B", State: aman.RouteFactActive}
	direct := Reduce(snapshot, route, input, Config{})
	require.Equal(t, "DIRECT_TO:B", direct.Remaining[0].ID)
	require.Equal(t, navdata.FixID("C"), direct.Remaining[len(direct.Remaining)-1].To)
	input.RouteFact.State = aman.RouteFactExpired
	expired := Reduce(snapshot, route, input, Config{})
	require.NotEqual(t, "DIRECT_TO:B", expired.Remaining[0].ID)
	now := time.Date(2026, 1, 1, 0, 2, 0, 0, time.UTC)
	input.Observation.ObservedAt = ptr(now.Add(-3 * time.Minute))
	stale := Reduce(snapshot, route, input, Config{ReferenceTime: now, MaxObservationAge: time.Minute})
	require.Equal(t, StalePosition, stale.Completeness)
	route.Legs[1].PathTerminator = navdata.PathUnsupported
	partial := Reduce(snapshot, route, input, Config{})
	require.Contains(t, partial.Reasons, "UNSUPPORTED_LEG:L2:UNSUPPORTED")
}

func TestRouteProgressRejectsNonFiniteValue(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	f := validFlight(now)
	f.RouteProgress = &aman.RouteProgress{GeometryDigest: "digest", ManifestRevision: 1, LegIndex: 0, RejoinLegIndex: 0, AlongTrackNM: math.NaN()}
	require.Error(t, f.Validate())
}

func fixtureInput(t *testing.T) (navdata.ActiveGeometrySnapshot, navdata.RouteGeometry, Input) {
	t.Helper()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	v := navdata.DatasetVersion{Cycle: "2601", SourceRevision: "r1", EffectiveFrom: now.Add(-time.Hour), EffectiveUntil: now.Add(time.Hour)}
	fix := func(id string, lon float64) navdata.Fix {
		return navdata.Fix{ID: navdata.FixID(id), Position: navdata.Coordinate{LatitudeDeg: 0, LongitudeDeg: lon}}
	}
	a, b, c := navdata.FixID("A"), navdata.FixID("B"), navdata.FixID("C")
	legs := []navdata.ProcedureLeg{{ID: "L1", PathTerminator: navdata.PathTF, FromFix: &a, ToFix: &b}, {ID: "L2", PathTerminator: navdata.PathTF, FromFix: &b, ToFix: &c}}
	approach := navdata.ProcedureID("APP")
	s := navdata.ActiveGeometrySnapshot{Manifest: navdata.ManifestCandidate{Airport: "EKCH", Version: v, TerminalDigest: "term"}, ManifestRevision: 1, Fixes: []navdata.Fix{fix("A", 0), fix("B", 1), fix("C", 2)}, Procedures: []navdata.Procedure{{ID: approach}}, TerminalPaths: []navdata.TerminalPath{{Airport: "EKCH", Feeder: "F", RunwayGroup: "G"}}}
	r := navdata.RouteGeometry{Version: v, Legs: legs, Coverage: navdata.CoverageComplete, Digest: "digest"}
	return s, r, Input{Airport: "EKCH", RouteKey: "route", Feeder: "F", RunwayGroup: "G", Approach: &approach, FlightPlanRevision: 7, Observation: aman.SurveillanceFact{LatitudeDegrees: 0, LongitudeDegrees: .5, ObservedAt: ptr(now)}}
}
func ptr[T any](v T) *T { return &v }
func legIDs(legs []RemainingLeg) []string {
	out := make([]string, len(legs))
	for i, leg := range legs {
		out[i] = leg.ID
	}
	return out
}
func routeDistance(snapshot navdata.ActiveGeometrySnapshot, from, to navdata.FixID) float64 {
	fixes := map[navdata.FixID]navdata.Coordinate{}
	for _, fix := range snapshot.Fixes {
		fixes[fix.ID] = fix.Position
	}
	return wgs84NM(fixes[from], fixes[to])
}
func validFlight(now time.Time) aman.AMANFlight {
	return aman.AMANFlight{ID: "f", VATSIMCID: "1", CurrentCallsign: "SAS1", State: aman.StateAirborne, DataStatus: aman.DataFresh, FreezeReason: aman.FreezeNone, UpdatedAt: now}
}
