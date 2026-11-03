package trajectory

import (
	"math"
	"time"

	"FlightStrips/internal/aman"
)

// confirmRouteSection locates an aircraft without using the old, invalid
// route's along-track distances. Three distinct reports must agree on a usable
// suffix and plausible motion before this can replace physical progress.
func confirmRouteSection(input Input, legs []leg, digest string, config Config) (*aman.RouteRecovery, projected, bool, string) {
	factID := ""
	if activeRouteFact(input.RouteFact) {
		factID = input.RouteFact.ID
	}
	at := *input.Observation.ObservedAt
	if !config.ReferenceTime.IsZero() && at.After(config.ReferenceTime) {
		return input.Recovery, projected{}, false, "ROUTE_RECOVERY_POSITION_FUTURE"
	}
	prior := input.Recovery
	if prior != nil && (prior.GeometryDigest != digest || prior.FlightPlanRevision != input.FlightPlanRevision || prior.RunwayGroupID != input.RunwayGroup || prior.RouteFactID != factID) {
		prior = nil
	}
	if prior != nil && !at.After(prior.ObservedAt) {
		return prior, projected{}, false, "ROUTE_RECOVERY_CONFIRMATION_PENDING"
	}
	watermark := &aman.RouteRecovery{FlightPlanRevision: input.FlightPlanRevision, RunwayGroupID: input.RunwayGroup, RouteFactID: factID, GeometryDigest: digest, ObservedAt: at, LatitudeDegrees: input.Observation.LatitudeDegrees, LongitudeDegrees: input.Observation.LongitudeDegrees}
	position := coordinate(input.Observation.LatitudeDegrees, input.Observation.LongitudeDegrees)
	best := nearestPhysicalProjection(input, legs)
	if best.cross > config.MaxCrossTrackNM {
		return watermark, best, false, "ROUTE_RECOVERY_POSITION_UNMATCHED"
	}
	if legs[best.index].gapAhead {
		return watermark, best, false, "ROUTE_GEOMETRY_GAP_AHEAD"
	}
	// A crossing gives no unique physical route section. Adjacent legs sharing
	// an endpoint are normal and do not constitute a competing section.
	for i, leg := range legs {
		if i == best.index || i == best.index-1 && legs[i].b == legs[best.index].a && !legs[i].gapAhead || i == best.index+1 && legs[i].a == legs[best.index].b && !legs[best.index].gapAhead {
			continue
		}
		_, cross := geodesicProject(position, leg.a, leg.b, leg.distance)
		if cross <= best.cross+config.JitterToleranceNM {
			return watermark, best, false, "ROUTE_RECOVERY_POSITION_AMBIGUOUS"
		}
	}
	selected := legs[best.index]
	_, initialCourse := wgs84Inverse(selected.a, selected.b)
	projectedPosition := wgs84Direct(selected.a, initialCourse, best.along)
	_, course := wgs84Inverse(projectedPosition, selected.b)
	if input.Observation.TrackTrueDegrees == nil || !finite(*input.Observation.TrackTrueDegrees) || angularDifferenceDegrees(*input.Observation.TrackTrueDegrees, course*180/math.Pi) > config.RecoveryTrackToleranceDegrees {
		return watermark, best, false, "ROUTE_RECOVERY_TRACK_UNMATCHED"
	}
	progress := best.before + best.along
	next := &aman.RouteRecovery{FlightPlanRevision: input.FlightPlanRevision, RunwayGroupID: input.RunwayGroup, RouteFactID: factID, GeometryDigest: digest, LegIndex: best.index, AlongTrackNM: progress, LatitudeDegrees: position.LatitudeDeg, LongitudeDegrees: position.LongitudeDeg, ObservedAt: at, Samples: 1}
	if prior != nil && prior.Samples > 0 {
		elapsed := at.Sub(prior.ObservedAt)
		limit := elapsed.Hours()*maximumProgressGroundspeedKnots + config.JitterToleranceNM
		physical := wgs84NM(coordinate(prior.LatitudeDegrees, prior.LongitudeDegrees), position)
		if elapsed <= 2*time.Minute && progress >= prior.AlongTrackNM-config.JitterToleranceNM && progress-prior.AlongTrackNM <= limit && physical <= limit {
			next.Samples = uint8(min(int(prior.Samples)+1, config.RecoveryConfirmationSamples))
		}
	}
	return next, best, int(next.Samples) >= config.RecoveryConfirmationSamples, "ROUTE_RECOVERY_CONFIRMATION_PENDING"
}

func blockedRoute(result Result, reasons []string, reason string) Result {
	result.Completeness, result.Reasons = Partial, append(reasons, reason)
	return result
}

func nearestPhysicalProjection(input Input, legs []leg) projected {
	position := coordinate(input.Observation.LatitudeDegrees, input.Observation.LongitudeDegrees)
	best, before := projected{cross: math.Inf(1)}, 0.0
	for i, leg := range legs {
		fraction, cross := geodesicProject(position, leg.a, leg.b, leg.distance)
		if cross < best.cross {
			best = projected{i, fraction * leg.distance, before, cross}
		}
		before += leg.distance
	}
	return best
}
