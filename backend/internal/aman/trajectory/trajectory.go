// Package trajectory projects observations onto canonical cache-backed AMAN
// geometry. It owns no source acquisition and has no EuroScope dependency.
package trajectory

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
)

const (
	defaultMaxCrossTrackNM = 12.0
	defaultJitterNM        = 0.25
	defaultForwardSearchNM = 50.0
)

type Completeness string

const (
	Complete      Completeness = "complete"
	Partial       Completeness = "partial"
	OffRoute      Completeness = "off_route"
	Unresolved    Completeness = "unresolved"
	StalePosition Completeness = "stale_position"
)

type Config struct {
	MaxCrossTrackNM, JitterToleranceNM, MaxForwardSearchNM float64
	ReferenceTime                                          time.Time
	MaxObservationAge                                      time.Duration
}

func (c Config) normalized() Config {
	if c.MaxCrossTrackNM <= 0 {
		c.MaxCrossTrackNM = defaultMaxCrossTrackNM
	}
	if c.JitterToleranceNM <= 0 {
		c.JitterToleranceNM = defaultJitterNM
	}
	if c.MaxForwardSearchNM <= 0 {
		c.MaxForwardSearchNM = defaultForwardSearchNM
	}
	return c
}

// Input selects already-materialized geometry. Approach and missed approach
// are explicit: projection never guesses them from a runway or route string.
type Input struct {
	Airport            navdata.AirportID
	RouteKey           navdata.RouteKey
	Feeder             navdata.FeederID
	RunwayGroup        aman.RunwayGroupID
	Approach           *navdata.ProcedureID
	MissedApproach     *navdata.ProcedureID
	FlightPlanRevision uint64
	Observation        aman.SurveillanceFact
	RouteFact          *aman.RouteFact
	Prior              *aman.RouteProgress
}

type RemainingLeg struct {
	ID         string
	From, To   navdata.FixID
	DistanceNM float64
}
type Result struct {
	Remaining       []RemainingLeg
	AlongTrackNM    float64
	CrossTrackNM    float64
	DistanceToGoNM  *float64
	Completeness    Completeness
	Reasons         []string
	GeometryDigest  string
	SelectedHolding *navdata.HoldingPattern
	Progress        *aman.RouteProgress
}

type Readers struct {
	Geometry navdata.GeometryReader
	Snapshot navdata.GeometrySnapshotReader
}

// Project performs only cache reads then delegates to the deterministic reducer.
func Project(ctx context.Context, readers Readers, input Input, config Config) (Result, error) {
	if readers.Geometry == nil || readers.Snapshot == nil {
		return Result{}, fmt.Errorf("trajectory requires cache-only geometry and snapshot readers")
	}
	route, err := readers.Geometry.Route(ctx, input.RouteKey)
	if err != nil {
		return Result{}, err
	}
	snapshot, err := readers.Snapshot.ActiveGeometrySnapshot(ctx, input.Airport)
	if err != nil {
		return Result{}, err
	}
	return Reduce(snapshot, route, input, config), nil
}

// Reduce is pure: callers can persist Result.Progress with their aggregate
// commit, and a restart receives the same compatibility/reset behavior.
func Reduce(snapshot navdata.ActiveGeometrySnapshot, route navdata.RouteGeometry, input Input, config Config) Result {
	config = config.normalized()
	result := Result{GeometryDigest: route.Digest}
	if !route.Version.Equal(snapshot.Manifest.Version) {
		return unresolved(result, "DATASET_VERSION_MISMATCH")
	}
	if input.Observation.ObservedAt == nil || (config.MaxObservationAge > 0 && !config.ReferenceTime.IsZero() && input.Observation.ObservedAt.Before(config.ReferenceTime.Add(-config.MaxObservationAge))) {
		result.Completeness, result.Reasons = StalePosition, []string{"STALE_POSITION"}
		return result
	}
	if !finite(input.Observation.LatitudeDegrees) || !finite(input.Observation.LongitudeDegrees) || input.Observation.LatitudeDegrees < -90 || input.Observation.LatitudeDegrees > 90 || input.Observation.LongitudeDegrees < -180 || input.Observation.LongitudeDegrees > 180 {
		return unresolved(result, "INVALID_POSITION")
	}
	fixes := make(map[navdata.FixID]navdata.Fix, len(snapshot.Fixes))
	for _, fix := range snapshot.Fixes {
		fixes[fix.ID] = fix
	}
	baseCompatible := baseCompatible(input.Prior, route.Digest, snapshot, input)
	legs, reasons, holding, directRejoin := compose(snapshot, route, input, fixes, baseCompatible)
	if len(legs) == 0 {
		result.Reasons = reasons
		if len(reasons) == 0 {
			result.Reasons = []string{"NO_USABLE_GEOMETRY"}
		}
		result.Completeness = Unresolved
		return result
	}
	compatible := compatible(input.Prior, route.Digest, snapshot, input)
	start := 0
	if compatible {
		start = min(input.Prior.LegIndex, len(legs)-1)
	}
	obs := coordinate(input.Observation.LatitudeDegrees, input.Observation.LongitudeDegrees)
	best, ok, progressOutOfRange := projectForward(obs, legs, start, config.MaxCrossTrackNM, config.MaxForwardSearchNM, config.JitterToleranceNM, input.Prior, compatible)
	if !ok {
		if progressOutOfRange && best.cross <= config.MaxCrossTrackNM {
			result.Completeness, result.Reasons, result.CrossTrackNM = Partial, append(reasons, "FORWARD_PROGRESS_OUT_OF_RANGE"), best.cross
			return result
		}
		result.Completeness, result.Reasons, result.CrossTrackNM = OffRoute, append(reasons, "OFF_ROUTE"), best.cross
		return result
	}
	progress := best.before + best.along
	if compatible && progress < input.Prior.AlongTrackNM {
		observedCross := best.cross
		progress = input.Prior.AlongTrackNM
		best = projectionAt(legs, progress)
		best.cross = observedCross
	}
	result.AlongTrackNM, result.CrossTrackNM, result.SelectedHolding = progress, best.cross, holding
	dtg := remainingDistance(legs, progress)
	result.DistanceToGoNM = &dtg
	result.Remaining = remaining(legs, best.index, best.along)
	if len(reasons) == 0 && route.Coverage == navdata.CoverageComplete {
		result.Completeness = Complete
	} else {
		result.Completeness = Partial
	}
	result.Reasons = reasons
	routeFactID := ""
	if activeRouteFact(input.RouteFact) {
		routeFactID = input.RouteFact.ID
	}
	rejoin := best.index
	if activeRouteFact(input.RouteFact) && directRejoin >= 0 {
		rejoin = directRejoin
	}
	result.Progress = &aman.RouteProgress{GeometryDigest: route.Digest, ManifestRevision: snapshot.ManifestRevision, TerminalDigest: snapshot.Manifest.TerminalDigest, FlightPlanRevision: input.FlightPlanRevision, RouteFactID: routeFactID, RunwayGroupID: input.RunwayGroup, LegIndex: best.index, RejoinLegIndex: rejoin, AlongTrackNM: progress}
	return result
}

type leg struct {
	id       string
	from, to navdata.FixID
	a, b     navdata.Coordinate
	distance float64
}

func compose(snapshot navdata.ActiveGeometrySnapshot, route navdata.RouteGeometry, input Input, fixes map[navdata.FixID]navdata.Fix, baseCompatible bool) ([]leg, []string, *navdata.HoldingPattern, int) {
	var all []navdata.ProcedureLeg
	all = append(all, route.Legs...)
	var terminal *navdata.TerminalPath
	for i := range snapshot.TerminalPaths {
		p := &snapshot.TerminalPaths[i]
		if p.Feeder == input.Feeder && p.RunwayGroup == input.RunwayGroup {
			terminal = p
			all = append(all, p.Legs...)
			break
		}
	}
	reasons := slices.Clone(route.Unresolved)
	if terminal == nil {
		reasons = append(reasons, "TERMINAL_PATH_UNRESOLVED")
	} else {
		reasons = append(reasons, terminal.Unresolved...)
	}
	procedure := func(id *navdata.ProcedureID, label string) {
		if id == nil {
			if label == "APPROACH" {
				reasons = append(reasons, "APPROACH_NOT_SELECTED")
			}
			return
		}
		for _, p := range snapshot.Procedures {
			if p.ID == *id {
				all = append(all, p.Legs...)
				return
			}
		}
		reasons = append(reasons, label+"_UNRESOLVED:"+string(*id))
	}
	procedure(input.Approach, "APPROACH")
	procedure(input.MissedApproach, "MISSED_APPROACH")
	holdings := map[navdata.HoldingID]navdata.HoldingPattern{}
	for _, h := range snapshot.Holdings {
		holdings[h.ID] = h
	}
	var selected *navdata.HoldingPattern
	if terminal != nil {
		for _, id := range terminal.HoldingIDs {
			if h, ok := holdings[id]; ok {
				copy := h
				selected = &copy
				break
			}
		}
	}
	var out []leg
	var last navdata.FixID
	for _, value := range all {
		if value.PathTerminator.IsHolding() {
			if value.HoldingID != nil && selected == nil {
				if h, ok := holdings[*value.HoldingID]; ok {
					copy := h
					selected = &copy
				}
			}
			continue
		}
		if value.PathTerminator == navdata.PathVA || value.PathTerminator == navdata.PathVM || value.PathTerminator == navdata.PathVI || value.PathTerminator == navdata.PathUnsupported {
			reasons = append(reasons, "UNSUPPORTED_LEG:"+value.ID+":"+string(value.PathTerminator))
			last = "" // never bridge an unsupported/vector gap implicitly.
			continue
		}
		from := last
		if value.FromFix != nil {
			from = *value.FromFix
		}
		to := navdata.FixID("")
		if value.ToFix != nil {
			to = *value.ToFix
		}
		if from == "" || to == "" {
			reasons = append(reasons, "UNRESOLVED_LEG:"+value.ID+":MISSING_FIX")
			if to != "" {
				last = to
			}
			continue
		}
		f, fok := fixes[from]
		t, tok := fixes[to]
		if !fok || !tok {
			reasons = append(reasons, "UNRESOLVED_LEG:"+value.ID+":FIX_NOT_IN_MANIFEST")
			last = to
			continue
		}
		d := wgs84NM(f.Position, t.Position)
		out = append(out, leg{id: value.ID, from: from, to: to, a: f.Position, b: t.Position, distance: d})
		last = to
	}
	if activeRouteFact(input.RouteFact) {
		target := navdata.FixID(input.RouteFact.Fix)
		targetFix, ok := fixes[target]
		if !ok {
			return out, append(reasons, "DIRECT_TO_TARGET_UNRESOLVED:"+input.RouteFact.Fix), selected, -1
		}
		rejoin := -1
		floor := 0
		if baseCompatible && input.Prior != nil {
			floor = input.Prior.RejoinLegIndex
		}
		for i := floor; i < len(out); i++ {
			if out[i].to == target {
				rejoin = i
				break
			}
		}
		if rejoin < 0 {
			return out, append(reasons, "DIRECT_TO_TARGET_NOT_ON_FORWARD_PATH:"+input.RouteFact.Fix), selected, -1
		}
		current := coordinate(input.Observation.LatitudeDegrees, input.Observation.LongitudeDegrees)
		direct := leg{id: "DIRECT_TO:" + string(target), from: "", to: target, a: current, b: targetFix.Position, distance: wgs84NM(current, targetFix.Position)}
		out = append([]leg{direct}, out[rejoin+1:]...)
		return out, dedupe(reasons), selected, rejoin
	}
	return out, dedupe(reasons), selected, -1
}

func compatible(p *aman.RouteProgress, digest string, snapshot navdata.ActiveGeometrySnapshot, in Input) bool {
	if !baseCompatible(p, digest, snapshot, in) {
		return false
	}
	id := ""
	if activeRouteFact(in.RouteFact) {
		id = in.RouteFact.ID
	}
	return p.RouteFactID == id
}
func baseCompatible(p *aman.RouteProgress, digest string, snapshot navdata.ActiveGeometrySnapshot, in Input) bool {
	if p == nil {
		return false
	}
	return p.GeometryDigest == digest && p.ManifestRevision == snapshot.ManifestRevision && p.TerminalDigest == snapshot.Manifest.TerminalDigest && p.FlightPlanRevision == in.FlightPlanRevision && p.RunwayGroupID == in.RunwayGroup
}

type projected struct {
	index                int
	along, before, cross float64
}

func projectForward(p navdata.Coordinate, legs []leg, start int, maxCross, maxForward, jitter float64, prior *aman.RouteProgress, compatible bool) (projected, bool, bool) {
	best, nearest, geometryNearest := projected{}, projected{}, projected{}
	found, nearestFound, geometryFound := false, false, false
	before := 0.0
	minProgress, maxProgress := 0.0, math.Inf(1)
	if compatible {
		minProgress = math.Max(0, prior.AlongTrackNM-jitter)
		maxProgress = prior.AlongTrackNM + maxForward
	}
	for i, value := range legs {
		if i < start {
			before += value.distance
			continue
		}
		frac, cross := geodesicProject(p, value.a, value.b, value.distance)
		candidate := before + frac*value.distance
		candidateProjection := projected{i, frac * value.distance, before, cross}
		if !geometryFound || cross < geometryNearest.cross || (cross == geometryNearest.cross && i < geometryNearest.index) {
			geometryNearest, geometryFound = candidateProjection, true
		}
		if candidate >= minProgress && candidate <= maxProgress {
			if !nearestFound || cross < nearest.cross || (cross == nearest.cross && i < nearest.index) {
				nearest, nearestFound = candidateProjection, true
			}
			if cross <= maxCross && (!found || (!compatible && i < best.index) || (compatible && (cross < best.cross || (cross == best.cross && i < best.index)))) {
				best, found = candidateProjection, true
			}
		}
		before += value.distance
	}
	if found {
		return best, true, false
	}
	if nearestFound {
		return nearest, false, false
	}
	return geometryNearest, false, geometryFound
}
func activeRouteFact(value *aman.RouteFact) bool {
	return value != nil && (value.State == "" || value.State == aman.RouteFactActive)
}
func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
func projectionAt(legs []leg, distance float64) projected {
	before := 0.0
	for i, v := range legs {
		if distance <= before+v.distance {
			return projected{i, distance - before, before, 0}
		}
		before += v.distance
	}
	return projected{len(legs) - 1, legs[len(legs)-1].distance, before - legs[len(legs)-1].distance, 0}
}
func remainingDistance(legs []leg, progress float64) float64 {
	total := 0.0
	for _, l := range legs {
		total += l.distance
	}
	return math.Max(0, total-progress)
}
func remaining(legs []leg, index int, along float64) []RemainingLeg {
	out := make([]RemainingLeg, 0, len(legs)-index)
	for i := index; i < len(legs); i++ {
		d := legs[i].distance
		if i == index {
			d -= along
		}
		out = append(out, RemainingLeg{legs[i].id, legs[i].from, legs[i].to, math.Max(0, d)})
	}
	return out
}
func unresolved(r Result, reason string) Result {
	r.Completeness = rUnresolved(reason)
	r.Reasons = []string{reason}
	return r
}
func rUnresolved(_ string) Completeness { return Unresolved }
func coordinate(lat, lon float64) navdata.Coordinate {
	return navdata.Coordinate{LatitudeDeg: lat, LongitudeDeg: lon}
}
func dedupe(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// wgs84Inverse is Vincenty's inverse solution on WGS84. Accumulated DTG and
// every partial leg use this geodesic distance rather than a planar estimate.
func wgs84Inverse(a, b navdata.Coordinate) (float64, float64) {
	const aa = 6378137.0
	const f = 1 / 298.257223563
	const nm = 1852.0
	phi1, phi2 := a.LatitudeDeg*math.Pi/180, b.LatitudeDeg*math.Pi/180
	l := (b.LongitudeDeg - a.LongitudeDeg) * math.Pi / 180
	u1, u2 := math.Atan((1-f)*math.Tan(phi1)), math.Atan((1-f)*math.Tan(phi2))
	su1, cu1, su2, cu2 := math.Sin(u1), math.Cos(u1), math.Sin(u2), math.Cos(u2)
	lambda := l
	var sigma, ss, cs, sa, c2a, cm float64
	for i := 0; i < 100; i++ {
		sl, cl := math.Sin(lambda), math.Cos(lambda)
		ss = math.Hypot(cu2*sl, cu1*su2-su1*cu2*cl)
		if ss == 0 {
			return 0, 0
		}
		cs = su1*su2 + cu1*cu2*cl
		sigma = math.Atan2(ss, cs)
		sa = cu1 * cu2 * sl / ss
		c2a = 1 - sa*sa
		if c2a == 0 {
			cm = 0
		} else {
			cm = cs - 2*su1*su2/c2a
		}
		c := f / 16 * c2a * (4 + f*(4-3*c2a))
		next := l + (1-c)*f*sa*(sigma+c*ss*(cm+c*cs*(-1+2*cm*cm)))
		if math.Abs(next-lambda) < 1e-12 {
			lambda = next
			break
		}
		lambda = next
	}
	u2v := c2a * (aa*aa - (aa*(1-f))*(aa*(1-f))) / ((aa * (1 - f)) * (aa * (1 - f)))
	A := 1 + u2v/16384*(4096+u2v*(-768+u2v*(320-175*u2v)))
	B := u2v / 1024 * (256 + u2v*(-128+u2v*(74-47*u2v)))
	delta := B * ss * (cm + B/4*(cs*(-1+2*cm*cm)-B/6*cm*(-3+4*ss*ss)*(-3+4*cm*cm)))
	distance := (aa * (1 - f) * A * (sigma - delta)) / nm
	bearing := math.Atan2(cu2*math.Sin(lambda), cu1*su2-su1*cu2*math.Cos(lambda))
	return distance, math.Mod(bearing+2*math.Pi, 2*math.Pi)
}
func wgs84NM(a, b navdata.Coordinate) float64 { distance, _ := wgs84Inverse(a, b); return distance }

// geodesicProject finds the closest point on the WGS84 geodesic with a bounded
// ternary search. The result is deterministic and handles long route legs;
// no longitude/latitude planar fraction is used.
func geodesicProject(p, a, b navdata.Coordinate, totalNM float64) (float64, float64) {
	if totalNM == 0 {
		return 0, wgs84NM(p, a)
	}
	_, initialBearing := wgs84Inverse(a, b)
	lo, hi := 0.0, 1.0
	for range 48 {
		left, right := (2*lo+hi)/3, (lo+2*hi)/3
		if wgs84NM(p, wgs84Direct(a, initialBearing, totalNM*left)) <= wgs84NM(p, wgs84Direct(a, initialBearing, totalNM*right)) {
			hi = right
		} else {
			lo = left
		}
	}
	fraction := (lo + hi) / 2
	return fraction, wgs84NM(p, wgs84Direct(a, initialBearing, totalNM*fraction))
}

func wgs84Direct(start navdata.Coordinate, bearing, distanceNM float64) navdata.Coordinate {
	const aa = 6378137.0
	const f = 1 / 298.257223563
	const nm = 1852.0
	b := aa * (1 - f)
	alpha1 := bearing
	sinAlpha1, cosAlpha1 := math.Sin(alpha1), math.Cos(alpha1)
	phi1 := start.LatitudeDeg * math.Pi / 180
	tanU1 := (1 - f) * math.Tan(phi1)
	cosU1 := 1 / math.Sqrt(1+tanU1*tanU1)
	sinU1 := tanU1 * cosU1
	sigma1 := math.Atan2(tanU1, cosAlpha1)
	sinAlpha := cosU1 * sinAlpha1
	cosSqAlpha := 1 - sinAlpha*sinAlpha
	uSq := cosSqAlpha * (aa*aa - b*b) / (b * b)
	A := 1 + uSq/16384*(4096+uSq*(-768+uSq*(320-175*uSq)))
	B := uSq / 1024 * (256 + uSq*(-128+uSq*(74-47*uSq)))
	sigma := distanceNM * nm / (b * A)
	var cos2SigmaM, sinSigma, cosSigma float64
	for range 100 {
		cos2SigmaM = math.Cos(2*sigma1 + sigma)
		sinSigma = math.Sin(sigma)
		cosSigma = math.Cos(sigma)
		delta := B * sinSigma * (cos2SigmaM + B/4*(cosSigma*(-1+2*cos2SigmaM*cos2SigmaM)-B/6*cos2SigmaM*(-3+4*sinSigma*sinSigma)*(-3+4*cos2SigmaM*cos2SigmaM)))
		next := distanceNM*nm/(b*A) + delta
		if math.Abs(next-sigma) < 1e-12 {
			sigma = next
			break
		}
		sigma = next
	}
	tmp := sinU1*sinSigma - cosU1*cosSigma*cosAlpha1
	phi2 := math.Atan2(sinU1*cosSigma+cosU1*sinSigma*cosAlpha1, (1-f)*math.Sqrt(sinAlpha*sinAlpha+tmp*tmp))
	lambda := math.Atan2(sinSigma*sinAlpha1, cosU1*cosSigma-sinU1*sinSigma*cosAlpha1)
	C := f / 16 * cosSqAlpha * (4 + f*(4-3*cosSqAlpha))
	L := lambda - (1-C)*f*sinAlpha*(sigma+C*sinSigma*(cos2SigmaM+C*cosSigma*(-1+2*cos2SigmaM*cos2SigmaM)))
	return navdata.Coordinate{LatitudeDeg: phi2 * 180 / math.Pi, LongitudeDeg: math.Mod(start.LongitudeDeg+L*180/math.Pi+540, 360) - 180}
}
