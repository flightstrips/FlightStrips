package sector

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"

	"FlightStrips/internal/aman/navdata"
)

var speedLevel = regexp.MustCompile(`^(?:[NK]\d{4}|M\d{3})(?:[FA]\d{3}|[SM]\d{4})$`)

func (a *Adapter) Resolve(ctx context.Context, q navdata.RouteQuery) (navdata.RouteGeometry, error) {
	if err := q.Validate(); err != nil {
		return navdata.RouteGeometry{}, err
	}
	if err := a.compatible(ctx, q.Version); err != nil {
		return navdata.RouteGeometry{}, err
	}
	bad := func(message string) (navdata.RouteGeometry, error) {
		return navdata.RouteGeometry{}, incomplete(message)
	}
	if q.Destination != a.terminal.Airport || q.TerminalEntry == nil {
		return bad("sector route requires a configured destination and terminal join")
	}
	// Only a join on the selected terminal path is acceptable. The canonical
	// cache composes the remainder, without copying the SCT's display STAR.
	joinAllowed := false
	for _, path := range a.terminal.Paths {
		if q.RunwayGroup != nil && path.RunwayGroup != *q.RunwayGroup {
			continue
		}
		if slices.Contains(path.Fixes, *q.TerminalEntry) {
			joinAllowed = true
		}
	}
	if !joinAllowed {
		return bad("sector route join is absent from selected terminal configuration")
	}
	var nodes []point
	airway := ""
	reached := false
	for _, token := range strings.Fields(strings.ToUpper(q.FiledRoute)) {
		if err := ctx.Err(); err != nil {
			return navdata.RouteGeometry{}, err
		}
		name, _, _ := strings.Cut(token, "/")
		if speedLevel.MatchString(name) {
			continue
		}
		if name == string(q.Origin) && len(nodes) == 0 {
			continue
		}
		if name == "DCT" {
			if airway != "" {
				return bad("airway has no exit fix before DCT")
			}
			continue
		}
		if _, ok := a.airways[name]; ok {
			if len(nodes) == 0 || airway != "" {
				return bad("airway requires an entry fix and an exit fix")
			}
			airway = name
			continue
		}
		id := navdata.FixID(name)
		position, ok := a.fixes[id]
		if !ok || a.ambiguous[id] {
			return bad("sector route fix missing or ambiguous: " + name)
		}
		next := point{id, position}
		if airway != "" {
			expanded, err := a.expandAirway(ctx, airway, nodes[len(nodes)-1], next)
			if err != nil {
				return navdata.RouteGeometry{}, err
			}
			nodes = append(nodes, expanded...)
			airway = ""
		} else if len(nodes) == 0 || nodes[len(nodes)-1].id != id {
			nodes = append(nodes, next)
		}
		if id == *q.TerminalEntry {
			reached = true
			break
		}
	}
	if !reached || airway != "" {
		return bad("sector route does not reach the selected terminal join")
	}
	if len(nodes) < 2 {
		return bad("sector route needs at least two resolved points before the terminal continuation")
	}
	g := navdata.RouteGeometry{Version: q.Version, Coverage: navdata.CoverageComplete, Provenance: a.provenance}
	for i := 1; i < len(nodes); i++ {
		from, to := nodes[i-1], nodes[i]
		distance := navdata.CoordinateDistanceNM(from.position, to.position)
		if distance < 0.000001 {
			return bad("sector route contains co-located distinct fixes")
		}
		lat1, lat2 := from.position.LatitudeDeg*math.Pi/180, to.position.LatitudeDeg*math.Pi/180
		dlon := (to.position.LongitudeDeg - from.position.LongitudeDeg) * math.Pi / 180
		course := math.Mod(math.Atan2(math.Sin(dlon)*math.Cos(lat2), math.Cos(lat1)*math.Sin(lat2)-math.Sin(lat1)*math.Cos(lat2)*math.Cos(dlon))*180/math.Pi+360, 360)
		g.Legs = append(g.Legs, navdata.ProcedureLeg{ID: fmt.Sprintf("SECTOR-%04d", i), PathTerminator: navdata.PathTF, FromFix: &from.id, ToFix: &to.id, FromPosition: &from.position, ToPosition: &to.position, DistanceNM: &distance, CourseTrueDeg: &course})
		g.TotalDistanceNM += distance
	}
	if len(navdata.SuspectRouteDetours(g.Legs)) > 0 {
		return bad("sector route contains an implausible detour")
	}
	var err error
	g.Digest, err = navdata.RouteGeometryDigest(q, g)
	return g, err
}

// expandAirway walks actual SCT segments. A missing segment is never replaced
// by a direct leg. Multiple paths are rejected rather than guessing a branch.
func (a *Adapter) expandAirway(ctx context.Context, name string, from, to point) ([]point, error) {
	var vertices []navdata.Coordinate
	graph := map[int]map[int]bool{}
	vertex := func(p navdata.Coordinate) int {
		for i, old := range vertices {
			if navdata.CoordinateDistanceNM(old, p) < 0.005 {
				return i
			}
		}
		vertices = append(vertices, p)
		return len(vertices) - 1
	}
	for _, segment := range a.airways[name] {
		i, j := vertex(segment.from), vertex(segment.to)
		if i == j {
			continue
		}
		if graph[i] == nil {
			graph[i] = map[int]bool{}
		}
		if graph[j] == nil {
			graph[j] = map[int]bool{}
		}
		graph[i][j], graph[j][i] = true, true
	}
	endpoint := func(p navdata.Coordinate) int {
		found := -1
		for i, v := range vertices {
			if navdata.CoordinateDistanceNM(v, p) < 0.05 {
				if found != -1 {
					return -1
				}
				found = i
			}
		}
		return found
	}
	start, end := endpoint(from.position), endpoint(to.position)
	if start < 0 || end < 0 || start == end {
		return nil, incomplete("sector airway " + name + " does not cover both route endpoints")
	}
	var chosen []int
	paths, visits := 0, 0
	seen := map[int]bool{}
	var walk func(int, []int) error
	walk = func(current int, path []int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		visits++
		if visits > 10000 {
			return incomplete("sector airway graph search exceeded limit")
		}
		if current == end {
			paths++
			chosen = slices.Clone(path)
			return nil
		}
		seen[current] = true
		neighbors := make([]int, 0, len(graph[current]))
		for next := range graph[current] {
			neighbors = append(neighbors, next)
		}
		slices.Sort(neighbors)
		for _, next := range neighbors {
			if !seen[next] && paths < 2 {
				if err := walk(next, append(path, next)); err != nil {
					return err
				}
			}
		}
		delete(seen, current)
		return nil
	}
	if err := walk(start, []int{start}); err != nil {
		return nil, err
	}
	if paths != 1 {
		return nil, incomplete("sector airway " + name + " is disconnected or ambiguous")
	}
	result := make([]point, 0, len(chosen)-1)
	for _, index := range chosen[1 : len(chosen)-1] {
		id := navdata.FixID(fmt.Sprintf("SCT-%s-%04d", name, index))
		// Keep uniquely identified airway waypoints available to EuroScope
		// direct-to facts and route-progress diagnostics.
		var named navdata.FixID
		for candidate, position := range a.fixes {
			if !a.ambiguous[candidate] && navdata.CoordinateDistanceNM(position, vertices[index]) < 0.005 {
				if named != "" {
					named = ""
					break
				}
				named = candidate
			}
		}
		if named != "" {
			id = named
		}
		result = append(result, point{id, vertices[index]})
	}
	return append(result, to), nil
}
