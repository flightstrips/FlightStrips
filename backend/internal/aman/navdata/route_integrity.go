package navdata

import (
	"math"
	"strings"
)

// RouteGapPrefix identifies a source-resolution discontinuity. Unlike a
// published vector leg, it cannot be filled by an inferred direct-to.
const RouteGapPrefix = "ROUTE-GAP-"

func IsRouteGap(leg ProcedureLeg) bool {
	return leg.PathTerminator == PathUnsupported && strings.HasPrefix(leg.ID, RouteGapPrefix)
}

func HasRouteGaps(route RouteGeometry) bool {
	for _, leg := range route.Legs {
		if IsRouteGap(leg) {
			return true
		}
	}
	return false
}

// SuspectRouteDetours returns inbound leg indices for extreme isolated
// excursions. This is a request for contextual resolution, not permission to
// replace a waypoint with a straight line between its neighbors.
func SuspectRouteDetours(legs []ProcedureLeg) []int {
	var result []int
	for i := 0; i+1 < len(legs); i++ {
		a, b := legs[i], legs[i+1]
		if a.ToFix == nil || b.FromFix == nil || *a.ToFix != *b.FromFix || a.FromPosition == nil || a.ToPosition == nil || b.FromPosition == nil || b.ToPosition == nil {
			continue
		}
		if *a.ToPosition != *b.FromPosition {
			continue
		}
		inbound := CoordinateDistanceNM(*a.FromPosition, *a.ToPosition)
		outbound := CoordinateDistanceNM(*b.FromPosition, *b.ToPosition)
		shortcut := CoordinateDistanceNM(*a.FromPosition, *b.ToPosition)
		if inbound > 1000 && outbound > 1000 && inbound+outbound > 10*math.Max(100, shortcut) {
			result = append(result, i)
		}
	}
	return result
}

// CoordinateDistanceNM is used for source plausibility checks; trajectory
// prediction retains its more precise WGS84 distance calculation.
func CoordinateDistanceNM(a, b Coordinate) float64 {
	lat1, lat2 := a.LatitudeDeg*math.Pi/180, b.LatitudeDeg*math.Pi/180
	dlat, dlon := lat2-lat1, (b.LongitudeDeg-a.LongitudeDeg)*math.Pi/180
	h := math.Pow(math.Sin(dlat/2), 2) + math.Cos(lat1)*math.Cos(lat2)*math.Pow(math.Sin(dlon/2), 2)
	return 3440.065 * 2 * math.Asin(math.Sqrt(math.Max(0, math.Min(1, h))))
}
