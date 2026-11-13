package trajectory

import (
	"fmt"
	"math"
	"strings"

	"FlightStrips/internal/aman/navdata"
)

// ValidateTerminalFallbackGeometry applies equally to cached-route and
// terminal-only estimates: neither may invent an unvalidated terminal journey.
func ValidateTerminalFallbackGeometry(snapshot navdata.ActiveGeometrySnapshot, path navdata.TerminalPath) error {
	if path.Coverage != navdata.CoverageComplete || len(path.Legs) == 0 || !path.Version.Equal(snapshot.Manifest.Version) {
		return fmt.Errorf("validated terminal geometry is unavailable")
	}
	return nil
}

// TerminalFallbackRoute exposes the same validated terminal-only geometry for
// live clearance validation without acquiring or inventing a filed route.
func TerminalFallbackRoute(snapshot navdata.ActiveGeometrySnapshot, input Input) (navdata.RouteGeometry, error) {
	path := selectedTerminalPath(snapshot.TerminalPaths, input)
	if path == nil || input.Airport != "" && path.Airport != input.Airport {
		return navdata.RouteGeometry{}, fmt.Errorf("selected terminal path is unavailable")
	}
	if err := ValidateTerminalFallbackGeometry(snapshot, *path); err != nil {
		return navdata.RouteGeometry{}, err
	}
	return terminalFallbackRoute(*path), nil
}

func terminalFallbackRoute(path navdata.TerminalPath) navdata.RouteGeometry {
	return navdata.RouteGeometry{Version: path.Version, Coverage: navdata.CoverageComplete, Digest: "basic-terminal:" + path.Digest}
}

// ProjectTerminalFallback uses the active, validated terminal snapshot without
// acquiring a filed route. A coarse joining leg is used only before the STAR;
// established terminal progress continues through the ordinary reducer.
func ProjectTerminalFallback(snapshot navdata.ActiveGeometrySnapshot, path navdata.TerminalPath, input Input, config Config) (Result, error) {
	if err := ValidateTerminalFallbackGeometry(snapshot, path); err != nil {
		return Result{}, err
	}
	route := terminalFallbackRoute(path)
	// Progress from an unrelated filed-route digest must not prevent a terminal
	// fallback from locating the observed aircraft on the cached STAR.
	if input.Prior != nil && input.Prior.GeometryDigest != route.Digest {
		input.Prior = nil
	}
	input.Recovery = nil
	result := Reduce(snapshot, route, input, config)
	approximateRecovery := len(result.Remaining) > 0 && strings.HasPrefix(result.Remaining[0].ID, "OFF_ROUTE")
	if !approximateRecovery && len(result.Remaining) > 0 && result.DistanceToGoNM != nil && result.Completeness != OffRoute && result.Completeness != Unresolved && result.Completeness != StalePosition {
		return result, nil
	}
	if result.Completeness != OffRoute && !approximateRecovery || input.Prior != nil || activeRouteFact(input.RouteFact) {
		return Result{}, fmt.Errorf("terminal projection unavailable: %v", result.Reasons)
	}
	fixes := make(map[navdata.FixID]navdata.Fix, len(snapshot.Fixes))
	for _, fix := range snapshot.Fixes {
		fixes[fix.ID] = fix
	}
	legs, reasons, holding, _, _, bypassed := compose(snapshot, route, input, fixes, false)
	if len(legs) == 0 {
		return Result{}, fmt.Errorf("terminal path has no usable legs")
	}
	for _, leg := range legs {
		if leg.gapAhead {
			return Result{}, fmt.Errorf("terminal path has unresolved geometry")
		}
	}
	obs := coordinate(input.Observation.LatitudeDegrees, input.Observation.LongitudeDegrees)
	distance, bearing := wgs84Inverse(obs, legs[0].a)
	remaining := []RemainingLeg{{ID: "BASIC_JOIN", To: legs[0].from, DistanceNM: distance, CourseTrueDegrees: bearing * 180 / math.Pi, Start: obs, End: legs[0].a}}
	for _, leg := range legs {
		_, course := wgs84Inverse(leg.a, leg.b)
		remaining = append(remaining, RemainingLeg{ID: leg.id, From: leg.from, To: leg.to, DistanceNM: leg.distance, CourseTrueDegrees: course * 180 / math.Pi, Start: leg.a, End: leg.b})
	}
	total := remainingLegDistance(remaining)
	result = Result{Remaining: remaining, DistanceToGoNM: &total, GeometryDigest: route.Digest, Completeness: Partial, Reasons: append(reasons, "BASIC_STAR_JOIN"), SelectedHolding: holding, FeederProgress: feederProgress(legs, input.FeederFix, 0, bypassed)}
	if holding != nil {
		if fix, ok := fixes[holding.Fix]; ok {
			position := fix.Position
			result.HoldingFixPosition = &position
		}
	}
	return result, nil
}
