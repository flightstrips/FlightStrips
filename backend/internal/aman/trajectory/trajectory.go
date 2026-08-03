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
	defaultMaxCrossTrackNM             = 12.0
	defaultJitterNM                    = 0.25
	defaultForwardSearchNM             = 50.0
	defaultRecoveryConfirmationSamples = 3
	defaultRecoveryLegLookahead        = 8
	defaultRecoveryTrackTolerance      = 20.0
	defaultRecoveryRunwayDistanceNM    = 30.0
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
	MaxCrossTrackNM, JitterToleranceNM, MaxForwardSearchNM  float64
	ReferenceTime                                           time.Time
	MaxObservationAge                                       time.Duration
	RecoveryConfirmationSamples, RecoveryLegLookahead       int
	RecoveryTrackToleranceDegrees, RecoveryRunwayDistanceNM float64
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
	if c.RecoveryConfirmationSamples <= 0 {
		c.RecoveryConfirmationSamples = defaultRecoveryConfirmationSamples
	}
	if c.RecoveryConfirmationSamples > int(^uint8(0)) {
		c.RecoveryConfirmationSamples = int(^uint8(0))
	}
	if c.RecoveryLegLookahead <= 0 {
		c.RecoveryLegLookahead = defaultRecoveryLegLookahead
	}
	if c.RecoveryTrackToleranceDegrees <= 0 {
		c.RecoveryTrackToleranceDegrees = defaultRecoveryTrackTolerance
	}
	if c.RecoveryRunwayDistanceNM <= 0 {
		c.RecoveryRunwayDistanceNM = defaultRecoveryRunwayDistanceNM
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
	ID                string
	From, To          navdata.FixID
	DistanceNM        float64
	CourseTrueDegrees float64
	Start             navdata.Coordinate
	End               navdata.Coordinate
}
type Result struct {
	Remaining        []RemainingLeg
	AlongTrackNM     float64
	CrossTrackNM     float64
	DistanceToGoNM   *float64
	Completeness     Completeness
	Reasons          []string
	GeometryDigest   string
	SelectedHolding  *navdata.HoldingPattern
	HoldingCandidate *HoldingCandidate
	Progress         *aman.RouteProgress
	InTMA            bool
}

// HoldingCandidate is a geometric observation inside the published holding
// footprint. The operational owner requires consecutive candidates before it
// treats a flight as an active member of a holding stack.
type HoldingCandidate struct {
	HoldingID  navdata.HoldingID
	DistanceNM float64
}

type Readers struct {
	Geometry navdata.GeometryReader
	Snapshot navdata.GeometrySnapshotReader
}

// FiledRouteResult is the complete cache-backed geometry derived from the
// filed route and terminal configuration. It deliberately excludes active
// direct-to facts and observation projection, so it remains distinct from the
// route the aircraft is currently flying.
type FiledRouteResult struct {
	Legs    []RemainingLeg
	Reasons []string
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

// ReadFiledRoute loads the full filed-route geometry for an on-demand display.
// It reads only the active cache and never materializes or acquires navigation
// data on the request path.
func ReadFiledRoute(ctx context.Context, readers Readers, airport navdata.AirportID, routeKey navdata.RouteKey, feeder navdata.FeederID, runwayGroup aman.RunwayGroupID) (FiledRouteResult, error) {
	if readers.Geometry == nil || readers.Snapshot == nil {
		return FiledRouteResult{}, fmt.Errorf("filed route requires cache-only geometry and snapshot readers")
	}
	route, err := readers.Geometry.Route(ctx, routeKey)
	if err != nil {
		return FiledRouteResult{}, err
	}
	snapshot, err := readers.Snapshot.ActiveGeometrySnapshot(ctx, airport)
	if err != nil {
		return FiledRouteResult{}, err
	}
	if !route.Version.Equal(snapshot.Manifest.Version) {
		return FiledRouteResult{}, fmt.Errorf("filed route geometry dataset does not match active manifest")
	}
	fixes := make(map[navdata.FixID]navdata.Fix, len(snapshot.Fixes))
	for _, fix := range snapshot.Fixes {
		fixes[fix.ID] = fix
	}
	legs, reasons, _, _, _ := compose(snapshot, route, Input{Feeder: feeder, RunwayGroup: runwayGroup}, fixes, false)
	result := FiledRouteResult{Legs: make([]RemainingLeg, len(legs)), Reasons: displayReasons(reasons)}
	for i, leg := range legs {
		_, bearing := wgs84Inverse(leg.a, leg.b)
		result.Legs[i] = RemainingLeg{ID: leg.id, From: leg.from, To: leg.to, DistanceNM: leg.distance, CourseTrueDegrees: bearing * 180 / math.Pi, Start: leg.a, End: leg.b}
	}
	return result, nil
}

func displayReasons(reasons []string) []string {
	result := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		if reason != "APPROACH_NOT_SELECTED" {
			result = append(result, reason)
		}
	}
	return result
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
	legs, reasons, holding, directRejoin, terminalStart := compose(snapshot, route, input, fixes, baseCompatible)
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
		recovery := nextWaypointRecovery(obs, legs, start, input.Observation.TrackTrueDegrees, input.Prior, config)
		if recovery.next >= 0 {
			next, progressLeg, routePrefix := recovery.next, recovery.next, "OFF_ROUTE_RECOVERY_TO:"
			candidateFix, candidateSamples := "", uint8(0)
			if recovery.trackAligned {
				candidateFix = string(legs[next].to)
				candidateSamples = recoveryCandidateSamples(input.Prior, candidateFix, config.RecoveryConfirmationSamples)
				if int(candidateSamples) >= config.RecoveryConfirmationSamples {
					routePrefix = "OFF_ROUTE_TO:"
				} else {
					routePrefix = "OFF_ROUTE_CANDIDATE_TO:"
					progressLeg = start
				}
			}
			remaining := remainingFromNextWaypointWithPrefix(obs, legs, next, routePrefix)
			if len(remaining) > 0 {
				progress := progressAtLegEnd(legs, progressLeg)
				if recovery.trackAligned && int(candidateSamples) < config.RecoveryConfirmationSamples {
					progress = progressAtLegStart(legs, progressLeg)
					if compatible && input.Prior != nil {
						progress = input.Prior.AlongTrackNM
					}
				}
				dtg := remainingLegDistance(remaining)
				routeFactID := ""
				if activeRouteFact(input.RouteFact) {
					routeFactID = input.RouteFact.ID
				}
				rejoin := next
				if activeRouteFact(input.RouteFact) && directRejoin >= 0 {
					rejoin = directRejoin
				}
				result.Completeness = Partial
				result.Reasons = append(reasons, "OFF_ROUTE", "OFF_ROUTE_NEXT_WAYPOINT:"+string(legs[next].to))
				if recovery.trackAligned && int(candidateSamples) < config.RecoveryConfirmationSamples {
					result.Reasons = append(result.Reasons, fmt.Sprintf("OFF_ROUTE_DIRECT_CANDIDATE:%s:%d/%d", candidateFix, candidateSamples, config.RecoveryConfirmationSamples))
				}
				result.CrossTrackNM, result.AlongTrackNM = best.cross, progress
				result.SelectedHolding, result.HoldingCandidate, result.DistanceToGoNM, result.Remaining = holding, holdingCandidate(holding, fixes, input.Observation), &dtg, remaining
				result.InTMA = terminalStart >= 0 && progressLeg >= terminalStart
				result.Progress = &aman.RouteProgress{
					GeometryDigest: route.Digest, ManifestRevision: snapshot.ManifestRevision, TerminalDigest: snapshot.Manifest.TerminalDigest,
					FlightPlanRevision: input.FlightPlanRevision, RouteFactID: routeFactID, RunwayGroupID: input.RunwayGroup,
					LegIndex: progressLeg, RejoinLegIndex: rejoin, AlongTrackNM: progress,
					RecoveryCandidateFix: candidateFix, RecoveryCandidateSamples: candidateSamples,
				}
				return result
			}
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
	result.AlongTrackNM, result.CrossTrackNM, result.SelectedHolding, result.HoldingCandidate = progress, best.cross, holding, holdingCandidate(holding, fixes, input.Observation)
	result.InTMA = terminalStart >= 0 && best.index >= terminalStart
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

func compose(snapshot navdata.ActiveGeometrySnapshot, route navdata.RouteGeometry, input Input, fixes map[navdata.FixID]navdata.Fix, baseCompatible bool) ([]leg, []string, *navdata.HoldingPattern, int, int) {
	// A route parser can disambiguate a globally duplicated fix identifier
	// using the filed route context. Preserve those coordinates when the same
	// fix joins the configured terminal path or owns its selected holding.
	// Otherwise a manifest lookup for a homonymous fix on another continent
	// can create a physically impossible route discontinuity.
	routeFixes := make(map[navdata.FixID]navdata.Fix, len(fixes))
	for id, fix := range fixes {
		routeFixes[id] = fix
	}
	for _, value := range route.Legs {
		if value.FromFix != nil && value.FromPosition != nil {
			routeFixes[*value.FromFix] = navdata.Fix{ID: *value.FromFix, Position: *value.FromPosition}
		}
		if value.ToFix != nil && value.ToPosition != nil {
			routeFixes[*value.ToFix] = navdata.Fix{ID: *value.ToFix, Position: *value.ToPosition}
		}
	}
	fixes = routeFixes

	var all []navdata.ProcedureLeg
	all = append(all, route.Legs...)
	terminalSourceStart, terminalSourceEnd := -1, -1
	var terminal *navdata.TerminalPath
	for i := range snapshot.TerminalPaths {
		p := &snapshot.TerminalPaths[i]
		if p.Feeder == input.Feeder && p.RunwayGroup == input.RunwayGroup {
			terminal = p
			terminalSourceStart = len(all)
			all = append(all, terminalContinuation(route.Legs, p.Legs)...)
			terminalSourceEnd = len(all)
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
	terminalStart := -1
	var last navdata.FixID
	for sourceIndex, value := range all {
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
		fromPosition, fromResolved := coordinateForLeg(value.FromPosition, from, fixes)
		toPosition, toResolved := coordinateForLeg(value.ToPosition, to, fixes)
		if len(out) > 0 && out[len(out)-1].to == from {
			// The shared fix is one physical point. Prefer the already resolved
			// inbound endpoint over a conflicting coordinate retained on the
			// next source fragment.
			fromPosition, fromResolved = out[len(out)-1].b, true
		}
		if !fromResolved || !toResolved {
			reasons = append(reasons, "UNRESOLVED_LEG:"+value.ID+":FIX_NOT_IN_MANIFEST")
			last = to
			continue
		}
		d := wgs84NM(fromPosition, toPosition)
		out = append(out, leg{id: value.ID, from: from, to: to, a: fromPosition, b: toPosition, distance: d})
		if terminalStart < 0 && terminalSourceStart >= 0 && sourceIndex >= terminalSourceStart && sourceIndex < terminalSourceEnd {
			terminalStart = len(out) - 1
		}
		last = to
	}
	if selected != nil && !legsContainFix(out, selected.Fix) {
		// A downstream STAR join can bypass the path's configured holding fix.
		// Do not offer a hold at a waypoint that is no longer on the route.
		selected = nil
	}
	if activeRouteFact(input.RouteFact) {
		target := navdata.FixID(input.RouteFact.Fix)
		targetFix, ok := fixes[target]
		if !ok {
			return out, append(reasons, "DIRECT_TO_TARGET_UNRESOLVED:"+input.RouteFact.Fix), selected, -1, terminalStart
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
			return out, append(reasons, "DIRECT_TO_TARGET_NOT_ON_FORWARD_PATH:"+input.RouteFact.Fix), selected, -1, terminalStart
		}
		current := coordinate(input.Observation.LatitudeDegrees, input.Observation.LongitudeDegrees)
		direct := leg{id: "DIRECT_TO:" + string(target), from: "", to: target, a: current, b: targetFix.Position, distance: wgs84NM(current, targetFix.Position)}
		out = append([]leg{direct}, out[rejoin+1:]...)
		if terminalStart >= 0 {
			if rejoin < terminalStart {
				terminalStart -= rejoin
			} else {
				terminalStart = 0
			}
		}
		return out, dedupe(reasons), selected, rejoin, terminalStart
	}
	return out, dedupe(reasons), selected, -1, terminalStart
}

func terminalContinuation(route, terminal []navdata.ProcedureLeg) []navdata.ProcedureLeg {
	join, ok := lastUsableRouteFix(route)
	if !ok {
		return terminal
	}
	for terminalIndex := range terminal {
		if terminal[terminalIndex].FromFix != nil && *terminal[terminalIndex].FromFix == join {
			return terminal[terminalIndex:]
		}
	}
	for terminalIndex := len(terminal) - 1; terminalIndex >= 0; terminalIndex-- {
		if terminal[terminalIndex].ToFix != nil && *terminal[terminalIndex].ToFix == join {
			return terminal[terminalIndex+1:]
		}
	}
	return terminal
}

func lastUsableRouteFix(route []navdata.ProcedureLeg) (navdata.FixID, bool) {
	var last navdata.FixID
	usable := false
	for _, value := range route {
		if value.PathTerminator.IsHolding() {
			continue
		}
		if value.PathTerminator == navdata.PathVA || value.PathTerminator == navdata.PathVM || value.PathTerminator == navdata.PathVI || value.PathTerminator == navdata.PathUnsupported {
			last, usable = "", false
			continue
		}
		from := last
		if value.FromFix != nil {
			from = *value.FromFix
		}
		if value.ToFix == nil || from == "" {
			if value.ToFix != nil {
				last = *value.ToFix
			}
			usable = false
			continue
		}
		last, usable = *value.ToFix, true
	}
	return last, usable
}

func legsContainFix(legs []leg, fix navdata.FixID) bool {
	for _, value := range legs {
		if value.from == fix || value.to == fix {
			return true
		}
	}
	return false
}

func coordinateForLeg(position *navdata.Coordinate, fixID navdata.FixID, fixes map[navdata.FixID]navdata.Fix) (navdata.Coordinate, bool) {
	if position != nil {
		return *position, true
	}
	fix, ok := fixes[fixID]
	return fix.Position, ok
}

func holdingCandidate(holding *navdata.HoldingPattern, fixes map[navdata.FixID]navdata.Fix, observation aman.SurveillanceFact) *HoldingCandidate {
	if holding == nil {
		return nil
	}
	fix, ok := fixes[holding.Fix]
	if !ok {
		return nil
	}
	distance := wgs84NM(coordinate(observation.LatitudeDegrees, observation.LongitudeDegrees), fix.Position)
	radius := 2.5 // turn allowance around the holding fix.
	if holding.LegLengthNM != nil {
		radius += *holding.LegLengthNM
	} else if holding.LegTimeSeconds != nil {
		speed := 180.0
		if observation.GroundspeedKnots != nil {
			speed = math.Min(250, math.Max(120, *observation.GroundspeedKnots))
		}
		radius += speed * float64(*holding.LegTimeSeconds) / 3600
	}
	radius = math.Min(8, math.Max(3, radius))
	if distance > radius {
		return nil
	}
	return &HoldingCandidate{HoldingID: holding.ID, DistanceNM: distance}
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

type waypointRecovery struct {
	next         int
	trackAligned bool
}

// nextWaypointRecovery keeps an arrival useful when its observed position is
// outside the narrow route corridor. Track-derived candidates are bounded to a
// small forward leg window and require repeated agreement in Reduce before
// becoming an inferred direct. A distant runway is never inferred as a direct
// target merely because its bearing resembles the aircraft's current track.
func nextWaypointRecovery(observation navdata.Coordinate, legs []leg, start int, track *float64, prior *aman.RouteProgress, config Config) waypointRecovery {
	aligned, nearest := -1, -1
	alignedDifference, alignedDistance, nearestDistance := math.Inf(1), math.Inf(1), math.Inf(1)
	const trackDifferenceTieDegrees = 1.0
	searchEnd := min(len(legs), start+config.RecoveryLegLookahead)
	confirmedFix := ""
	if prior != nil && int(prior.RecoveryCandidateSamples) >= config.RecoveryConfirmationSamples {
		confirmedFix = prior.RecoveryCandidateFix
	}
	for index := start; index < len(legs); index++ {
		distance, bearing := wgs84Inverse(observation, legs[index].b)
		if distance < nearestDistance {
			nearest, nearestDistance = index, distance
		}
		if index >= searchEnd || track == nil || !finite(*track) {
			continue
		}
		difference := angularDifferenceDegrees(*track, bearing*180/math.Pi)
		if difference > config.RecoveryTrackToleranceDegrees {
			continue
		}
		if runwayLikeFix(legs[index].to) && distance > config.RecoveryRunwayDistanceNM {
			continue
		}
		if confirmedFix != "" && string(legs[index].to) == confirmedFix {
			return waypointRecovery{next: index, trackAligned: true}
		}
		if difference < alignedDifference-trackDifferenceTieDegrees ||
			(math.Abs(difference-alignedDifference) <= trackDifferenceTieDegrees && distance < alignedDistance) {
			aligned, alignedDifference, alignedDistance = index, difference, distance
		}
	}
	if aligned >= 0 {
		return waypointRecovery{next: aligned, trackAligned: true}
	}
	return waypointRecovery{next: nearest}
}

func recoveryCandidateSamples(prior *aman.RouteProgress, fix string, required int) uint8 {
	if prior == nil || prior.RecoveryCandidateFix != fix {
		return 1
	}
	samples := int(prior.RecoveryCandidateSamples) + 1
	if samples > required {
		samples = required
	}
	return uint8(samples)
}

func runwayLikeFix(fix navdata.FixID) bool {
	value := strings.ToUpper(string(fix))
	return strings.HasPrefix(value, "RWY-") || strings.Contains(value, "-RWY-")
}

func remainingFromNextWaypoint(observation navdata.Coordinate, legs []leg, next int) []RemainingLeg {
	return remainingFromNextWaypointWithPrefix(observation, legs, next, "OFF_ROUTE_TO:")
}

func remainingFromNextWaypointWithPrefix(observation navdata.Coordinate, legs []leg, next int, prefix string) []RemainingLeg {
	if next < 0 || next >= len(legs) {
		return nil
	}
	distance, bearing := wgs84Inverse(observation, legs[next].b)
	result := make([]RemainingLeg, 0, len(legs)-next)
	if distance > 0 {
		result = append(result, RemainingLeg{
			ID: prefix + string(legs[next].to), From: "", To: legs[next].to,
			DistanceNM: distance, CourseTrueDegrees: bearing * 180 / math.Pi, Start: observation, End: legs[next].b,
		})
	}
	result = append(result, remaining(legs, next+1, 0)...)
	return usableRecoveryLegs(result)
}

func usableRecoveryLegs(legs []RemainingLeg) []RemainingLeg {
	result := make([]RemainingLeg, 0, len(legs))
	for _, leg := range legs {
		if leg.DistanceNM <= 0 || !finite(leg.DistanceNM) || !finite(leg.CourseTrueDegrees) || leg.CourseTrueDegrees < 0 || leg.CourseTrueDegrees >= 360 || !validCoordinate(leg.Start) || !validCoordinate(leg.End) {
			continue
		}
		result = append(result, leg)
	}
	return result
}

func validCoordinate(value navdata.Coordinate) bool {
	return finite(value.LatitudeDeg) && finite(value.LongitudeDeg) && value.LatitudeDeg >= -90 && value.LatitudeDeg <= 90 && value.LongitudeDeg >= -180 && value.LongitudeDeg <= 180
}

func progressAtLegEnd(legs []leg, index int) float64 {
	progress := 0.0
	for i := 0; i <= index && i < len(legs); i++ {
		progress += legs[i].distance
	}
	return progress
}

func progressAtLegStart(legs []leg, index int) float64 {
	if index <= 0 {
		return 0
	}
	return progressAtLegEnd(legs, index-1)
}

func remainingLegDistance(legs []RemainingLeg) float64 {
	total := 0.0
	for _, leg := range legs {
		total += leg.DistanceNM
	}
	return total
}

func angularDifferenceDegrees(left, right float64) float64 {
	difference := math.Mod(math.Abs(left-right), 360)
	if difference > 180 {
		return 360 - difference
	}
	return difference
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
		start := legs[i].a
		if i == index {
			d -= along
			_, bearing := wgs84Inverse(legs[i].a, legs[i].b)
			start = wgs84Direct(legs[i].a, bearing, along)
		}
		_, course := wgs84Inverse(start, legs[i].b)
		out = append(out, RemainingLeg{ID: legs[i].id, From: legs[i].from, To: legs[i].to, DistanceNM: math.Max(0, d), CourseTrueDegrees: course * 180 / math.Pi, Start: start, End: legs[i].b})
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
