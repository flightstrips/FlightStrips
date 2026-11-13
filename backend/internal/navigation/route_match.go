package navigation

import (
	"math"
	"regexp"
	"strings"

	"FlightStrips/internal/aman/navdata"
)

const routeMatchToleranceNM = 0.5

// Online paths can contain airport/departure prefixes and additional expanded
// airway points. Compare the shared continuous path in both directions, in
// order, rather than comparing provider point counts or route spellings.
func routeMatchesFiledGeometry(primary, current navdata.RouteGeometry) bool {
	online, ok := routeCoordinates(primary)
	if !ok {
		return false
	}
	if current.Coverage == navdata.CoveragePartial && navdata.HasRouteGaps(current) {
		return resolvedSectionsMatch(online, current)
	}
	filed, ok := routeCoordinates(current)
	if !ok {
		return false
	}
	start, _, found := pointOnPath(filed[0], online, 0)
	if !found {
		return false
	}
	// The online path must finish at the same selected terminal join.
	if navdata.CoordinateDistanceNM(online[len(online)-1], filed[len(filed)-1]) > routeMatchToleranceNM {
		return false
	}
	var offset float64
	trimmed := []navdata.Coordinate{filed[0]}
	for i := 1; i < len(online); i++ {
		offset += navdata.CoordinateDistanceNM(online[i-1], online[i])
		if offset > start+0.000001 {
			trimmed = append(trimmed, online[i])
		}
	}
	return len(trimmed) >= 2 && followsPath(trimmed, filed) && followsPath(filed, trimmed)
}

// Compare every verified section without treating a source gap as a direct
// segment. The caller separately checks filed waypoint order through gaps.
func resolvedSectionsMatch(online []navdata.Coordinate, current navdata.RouteGeometry) bool {
	var section []navdata.ProcedureLeg
	var progress float64
	check := func() bool {
		if len(section) == 0 {
			return true
		}
		points, ok := routeCoordinates(navdata.RouteGeometry{Coverage: navdata.CoverageComplete, Legs: section})
		if !ok {
			return false
		}
		start, _, found := pointOnPath(points[0], online, progress)
		if !found {
			return false
		}
		end, _, found := pointOnPath(points[len(points)-1], online, start)
		if !found || end <= start {
			return false
		}
		trimmed := []navdata.Coordinate{points[0]}
		var offset float64
		for i := 1; i < len(online); i++ {
			offset += navdata.CoordinateDistanceNM(online[i-1], online[i])
			if offset > start+0.000001 && offset < end-0.000001 {
				trimmed = append(trimmed, online[i])
			}
		}
		trimmed = append(trimmed, points[len(points)-1])
		progress = end
		return followsPath(trimmed, points) && followsPath(points, trimmed)
	}
	for _, leg := range current.Legs {
		if navdata.IsRouteGap(leg) {
			if !check() {
				return false
			}
			section = nil
		} else {
			section = append(section, leg)
		}
	}
	return len(section) > 0 && check() && navdata.CoordinateDistanceNM(online[len(online)-1], *section[len(section)-1].ToPosition) <= routeMatchToleranceNM
}

func routeCoordinates(route navdata.RouteGeometry) ([]navdata.Coordinate, bool) {
	var points []navdata.Coordinate
	if route.Coverage != navdata.CoverageComplete || len(route.Legs) == 0 {
		return nil, false
	}
	for _, leg := range route.Legs {
		if leg.FromPosition == nil || leg.ToPosition == nil || leg.FromPosition.Validate() != nil || leg.ToPosition.Validate() != nil {
			return nil, false
		}
		if len(points) == 0 {
			points = append(points, *leg.FromPosition)
		} else if navdata.CoordinateDistanceNM(points[len(points)-1], *leg.FromPosition) > routeMatchToleranceNM {
			return nil, false
		}
		points = append(points, *leg.ToPosition)
	}
	return points, true
}

func followsPath(points, path []navdata.Coordinate) bool {
	var progress float64
	for _, point := range points {
		along, _, found := pointOnPath(point, path, progress)
		if !found {
			return false
		}
		progress = along
	}
	return true
}

// pointOnPath finds the nearest forward projection within the match corridor.
// All distances use the same spherical model as the source integrity checks.
func pointOnPath(point navdata.Coordinate, path []navdata.Coordinate, minimum float64) (float64, float64, bool) {
	const radius = 3440.065
	best, progress, offset := math.Inf(1), 0.0, 0.0
	for i := 1; i < len(path); i++ {
		a, b := path[i-1], path[i]
		length := navdata.CoordinateDistanceNM(a, b)
		if length <= 0 {
			continue
		}
		delta := navdata.CoordinateDistanceNM(a, point) / radius
		angle := routeBearing(a, point) - routeBearing(a, b)
		along := math.Atan2(math.Sin(delta)*math.Cos(angle), math.Cos(delta)) * radius
		cross := math.Abs(math.Asin(math.Max(-1, math.Min(1, math.Sin(delta)*math.Sin(angle))))) * radius
		if along < 0 {
			along, cross = 0, navdata.CoordinateDistanceNM(a, point)
		}
		if along > length {
			along, cross = length, navdata.CoordinateDistanceNM(b, point)
		}
		if offset+along+0.000001 >= minimum && cross < best {
			best, progress = cross, offset+along
		}
		offset += length
	}
	return progress, best, best <= routeMatchToleranceNM
}

func routeBearing(a, b navdata.Coordinate) float64 {
	lat1, lat2 := a.LatitudeDeg*math.Pi/180, b.LatitudeDeg*math.Pi/180
	dlon := (b.LongitudeDeg - a.LongitudeDeg) * math.Pi / 180
	return math.Atan2(math.Sin(dlon)*math.Cos(lat2), math.Cos(lat1)*math.Sin(lat2)-math.Sin(lat1)*math.Cos(lat2)*math.Cos(dlon))
}

var routeAirway = regexp.MustCompile(`^[A-Z]{1,2}\d{1,4}$`)
var routeProcedure = regexp.MustCompile(`^[A-Z]{2,}\d[A-Z]$`)
var routeSpeedLevel = regexp.MustCompile(`^(?:[NK]\d{4}|M\d{3})(?:[FA]\d{3}|[SM]\d{4})$`)

// When a regional SCT cannot resolve the entire flight, still reject positive
// evidence of stale vIFF data: explicit waypoints absent or in the wrong order.
// Airway labels and procedure names need not appear in an expanded path.
func filedWaypointsMatch(q navdata.RouteQuery, primary navdata.RouteGeometry, knownFix func(navdata.FixID) bool) bool {
	var names []string
	var positions []navdata.Coordinate
	for _, leg := range primary.Legs {
		if leg.FromFix == nil || leg.ToFix == nil || leg.FromPosition == nil || leg.ToPosition == nil {
			return false
		}
		if len(names) == 0 || names[len(names)-1] != string(*leg.FromFix) {
			names = append(names, string(*leg.FromFix))
			positions = append(positions, *leg.FromPosition)
		}
		names = append(names, string(*leg.ToFix))
		positions = append(positions, *leg.ToPosition)
	}
	next, previous := 0, -1
	direct := true
	for _, token := range strings.Fields(strings.ToUpper(q.FiledRoute)) {
		name, _, _ := strings.Cut(token, "/")
		if name == "DCT" || name == string(q.Origin) || name == string(q.Destination) || routeSpeedLevel.MatchString(name) {
			continue
		}
		if !knownFix(navdata.FixID(name)) && (routeAirway.MatchString(name) || routeProcedure.MatchString(name)) {
			direct = false
			continue
		}
		for next < len(names) && names[next] != name {
			next++
		}
		if next == len(names) {
			return false
		}
		if previous >= 0 && direct && !followsPath(positions[previous:next+1], []navdata.Coordinate{positions[previous], positions[next]}) {
			return false
		}
		previous, direct = next, true
		next++
		if q.TerminalEntry != nil && name == string(*q.TerminalEntry) {
			break
		}
	}
	return true
}
