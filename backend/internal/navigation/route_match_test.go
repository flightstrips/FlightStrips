package navigation

import (
	"testing"

	"FlightStrips/internal/aman/navdata"
	"github.com/stretchr/testify/require"
)

func testRoutePath(points ...navdata.Coordinate) navdata.RouteGeometry {
	g := navdata.RouteGeometry{Coverage: navdata.CoverageComplete}
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		g.Legs = append(g.Legs, navdata.ProcedureLeg{FromPosition: &a, ToPosition: &b})
	}
	return g
}

func TestRouteMatchAcceptsProviderPrefixesAndCollinearExpansion(t *testing.T) {
	a := navdata.Coordinate{LatitudeDeg: 55, LongitudeDeg: 12}
	b := navdata.Coordinate{LatitudeDeg: 55.5, LongitudeDeg: 12}
	c := navdata.Coordinate{LatitudeDeg: 56, LongitudeDeg: 12}
	origin := navdata.Coordinate{LatitudeDeg: 54, LongitudeDeg: 11}
	for _, pair := range [][2]navdata.RouteGeometry{
		{testRoutePath(origin, a, b, c), testRoutePath(a, c)},
		{testRoutePath(origin, a, c), testRoutePath(a, b, c)},
		{testRoutePath(origin, a, c), testRoutePath(b, c)},
	} {
		require.True(t, routeMatchesFiledGeometry(pair[0], pair[1]))
	}
}

func TestRouteMatchRejectsChangedWaypointsDirectsAndOrder(t *testing.T) {
	a := navdata.Coordinate{LatitudeDeg: 55, LongitudeDeg: 12}
	b := navdata.Coordinate{LatitudeDeg: 55.5, LongitudeDeg: 13}
	c := navdata.Coordinate{LatitudeDeg: 56, LongitudeDeg: 12}
	for _, pair := range [][2]navdata.RouteGeometry{
		{testRoutePath(a, c), testRoutePath(a, b, c)},
		{testRoutePath(a, b, c), testRoutePath(a, c)},
		{testRoutePath(a, b, c), testRoutePath(b, a, c)},
		{testRoutePath(a, b), testRoutePath(a, c)},
	} {
		require.False(t, routeMatchesFiledGeometry(pair[0], pair[1]))
	}
}

func TestPartialSectorCoverageStillRejectsStaleNamedFixesAndDirects(t *testing.T) {
	a, b, c := navdata.FixID("GLOBAL"), navdata.FixID("CH626"), navdata.FixID("SOK")
	g := testRoutePath(navdata.Coordinate{LatitudeDeg: 56, LongitudeDeg: 12}, navdata.Coordinate{LatitudeDeg: 55.9, LongitudeDeg: 13}, navdata.Coordinate{LatitudeDeg: 55.7, LongitudeDeg: 12.2})
	g.Legs[0].FromFix, g.Legs[0].ToFix, g.Legs[1].FromFix, g.Legs[1].ToFix = &a, &b, &b, &c
	q := navdata.RouteQuery{Origin: "ENGM", Destination: "EKCH", TerminalEntry: &c}
	known := func(id navdata.FixID) bool { return id == "CH626" || id == "SOK" }
	for route, matches := range map[string]bool{
		"N0450F350 GLOBAL L1 CH626 DCT SOK": true,
		"GLOBAL DCT NEWFIX DCT SOK":         false,
		"GLOBAL DCT SOK":                    false,
		"GLOBAL L1 SOK":                     true,
		"GLOBAL L1 CH627 DCT SOK":           false,
	} {
		q.FiledRoute = route
		// A locally known CH waypoint must not be misclassified as an airway.
		checkKnown := func(id navdata.FixID) bool { return known(id) || id == "CH627" }
		require.Equal(t, matches, filedWaypointsMatch(q, g, checkKnown), route)
	}
}

func TestPartialSectorGeometryChecksEveryResolvedSection(t *testing.T) {
	a := navdata.Coordinate{LatitudeDeg: 54, LongitudeDeg: 12}
	b := navdata.Coordinate{LatitudeDeg: 55, LongitudeDeg: 12}
	c := navdata.Coordinate{LatitudeDeg: 56, LongitudeDeg: 12}
	d := navdata.Coordinate{LatitudeDeg: 57, LongitudeDeg: 12}
	detour := navdata.Coordinate{LatitudeDeg: 55.5, LongitudeDeg: 13}
	gap := navdata.ProcedureLeg{ID: navdata.RouteGapPrefix + "UNKNOWN", PathTerminator: navdata.PathUnsupported}
	partial := testRoutePath(a, b)
	partial.Coverage = navdata.CoveragePartial
	partial.Legs = append(partial.Legs, gap)
	partial.Legs = append(partial.Legs, testRoutePath(c, d).Legs...)
	require.True(t, routeMatchesFiledGeometry(testRoutePath(a, b, detour, c, d), partial), "online geometry inside the gap remains usable")
	require.False(t, routeMatchesFiledGeometry(testRoutePath(a, detour, b, c, d), partial), "verified prefix must match")
	require.False(t, routeMatchesFiledGeometry(testRoutePath(a, b, c, detour, d), partial), "verified suffix must match")
	require.False(t, routeMatchesFiledGeometry(testRoutePath(a, b, c), partial), "terminal join must match")
	partial.Legs = append(partial.Legs, gap)
	require.False(t, routeMatchesFiledGeometry(testRoutePath(a, b, c, d), partial), "a trailing gap has no verified join")
}
