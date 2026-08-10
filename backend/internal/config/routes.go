package config

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"
)

// Stand represents a single stand like "A15".
type Stand struct {
	Prefix string `yaml:"prefix"`
	Number int    `yaml:"number"`
}

// Equal compares two stands for equality.
func (s Stand) Equal(o Stand) bool {
	return strings.EqualFold(s.Prefix, o.Prefix) && s.Number == o.Number
}

// ParseStand parses strings like "A15" into a Stand.
func ParseStand(s string) (Stand, error) {
	str := strings.TrimSpace(s)
	if str == "" {
		return Stand{}, fmt.Errorf("empty stand")
	}
	// Split into leading letters and trailing digits (ASCII-oriented).
	i := 0
	for i < len(str) && (str[i] < '0' || str[i] > '9') {
		i++
	}
	if i == 0 || i == len(str) {
		return Stand{}, fmt.Errorf("invalid stand format: %q", s)
	}
	prefix := strings.ToUpper(str[:i])
	numStr := str[i:]
	n, err := strconv.Atoi(numStr)
	if err != nil {
		return Stand{}, fmt.Errorf("invalid stand number %q: %w", numStr, err)
	}
	return Stand{Prefix: prefix, Number: n}, nil
}

// StandRange represents a contiguous range of stands (e.g., A15-A30).
type StandRange struct {
	Prefix string `yaml:"prefix"`
	From   int    `yaml:"from"`
	To     int    `yaml:"to"`
}

// UnmarshalYAML allows StandRange to be specified as:
// - scalar "A15-A30" (range)
// - scalar "A15" (single stand)
// - mapping {prefix: "A", from: 15, to: 30}
func (r *StandRange) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		str := strings.TrimSpace(value.Value)
		if strings.Contains(str, "-") {
			sr, err := ParseStandRange(str)
			if err != nil {
				return err
			}
			*r = sr
			return nil
		}
		// treat single stand scalar as From==To
		st, err := ParseStand(str)
		if err != nil {
			return err
		}
		*r = SingleStandRange(st.Prefix, st.Number)
		return nil
	case yaml.MappingNode:
		var aux struct {
			Prefix string `yaml:"prefix"`
			From   int    `yaml:"from"`
			To     int    `yaml:"to"`
		}
		if err := value.Decode(&aux); err != nil {
			return err
		}
		if strings.TrimSpace(aux.Prefix) == "" {
			return fmt.Errorf("stand range requires non-empty prefix")
		}
		if aux.From == 0 && aux.To == 0 {
			return fmt.Errorf("stand range requires at least one bound")
		}
		from, to := aux.From, aux.To
		if to == 0 {
			to = from
		}
		if from == 0 {
			from = to
		}
		if from > to {
			from, to = to, from
		}
		r.Prefix = strings.ToUpper(aux.Prefix)
		r.From = from
		r.To = to
		return nil
	default:
		return fmt.Errorf("invalid YAML for StandRange")
	}
}

// Contains returns true if the stand falls within the range.
func (r *StandRange) Contains(s Stand) bool {
	if !strings.EqualFold(r.Prefix, s.Prefix) {
		return false
	}
	return s.Number >= r.From && s.Number <= r.To
}

// ParseStandRange parses strings like "A15-A30" into a StandRange.
func ParseStandRange(s string) (StandRange, error) {
	str := strings.TrimSpace(s)
	parts := strings.Split(str, "-")
	if len(parts) != 2 {
		return StandRange{}, fmt.Errorf("invalid stand range format: %q", s)
	}
	left, err := ParseStand(parts[0])
	if err != nil {
		return StandRange{}, fmt.Errorf("left bound: %w", err)
	}
	right, err := ParseStand(parts[1])
	if err != nil {
		return StandRange{}, fmt.Errorf("right bound: %w", err)
	}
	if !strings.EqualFold(left.Prefix, right.Prefix) {
		return StandRange{}, fmt.Errorf("stand range prefixes must match: %q vs %q", left.Prefix, right.Prefix)
	}
	from := left.Number
	to := right.Number
	if from > to {
		from, to = to, from
	}
	return StandRange{Prefix: strings.ToUpper(left.Prefix), From: from, To: to}, nil
}

// SingleStandRange returns a StandRange representing a single stand (From == To).
func SingleStandRange(prefix string, number int) StandRange {
	return StandRange{Prefix: strings.ToUpper(prefix), From: number, To: number}
}

// Route is an ordered logical route qualified by runway, stand, or both.
type Route struct {
	Name           string            `yaml:"name"`
	ForRunway      string            `yaml:"forRunway"`       // e.g., "RWY27" or "27" (case-insensitive)
	ForStandRanges []StandRange      `yaml:"forStandRanges"`  // e.g., A10-A20, B5-B10, or single stands like W1 (From=To=1)
	Path           []string          `yaml:"path"`            // ordered logical sector identifiers
	Active         []string          `yaml:"active"`          // runways that must be active for this route to match
	RequireAll     bool              `yaml:"require_all"`     // if true, all Active runways must be present (default: any)
	OwnerOverrides map[string]string `yaml:"owner_overrides"` // matched sector -> sector whose owner should be used
}

type ResolvedRoute struct {
	Path           []string
	OwnerOverrides map[string]string
}

// RouteCandidateDiagnostics describes why a configured runway route was or
// was not eligible for a route selection. It is intended for operational
// diagnostics when a route cannot be resolved.
type RouteCandidateDiagnostics struct {
	Name          string
	Path          []string
	Active        []string
	RequireAll    bool
	StandRanges   []string
	StandSpecific bool
	StandMatched  bool
	ActiveScore   int
	Rejection     string
}

// RouteSelectionDiagnostics explains a failed route selection without
// changing the existing boolean-based route selection API.
type RouteSelectionDiagnostics struct {
	Runway           string
	NormalizedRunway string
	Stand            string
	StandParseError  string
	FailureReason    string
	CandidateCount   int
	Candidates       []RouteCandidateDiagnostics
}

// ComputeToRunway selects the best-matching route to the given runway under the
// current active flags and containing the aircraft's current Sector in its path.
// "Best" means the route whose active list has the most runways in common with
// the current active set (same scoring as sector selection). A route with an
// empty active list is a catch-all (score 0) and loses to any specific match.
// Returns the subsequence of path from the current Sector to the end together with
// any route-scoped owner overrides that should apply while constructing next owners.
func ComputeToRunway(active []string, currentSector string, runway string, stand ...string) (ResolvedRoute, bool) {
	return selectRunwayRoute(active, currentSector, runway, firstString(stand), true)
}

// ComputeDepartureRoute selects the complete outbound route for a stand and
// runway. The caller resolves the first currently carried logical stage.
func ComputeDepartureRoute(active []string, stand string, runway string) (ResolvedRoute, bool) {
	return selectRunwayRoute(active, "", runway, stand, false)
}

// ComputeDepartureRouteWithDiagnostics selects the complete outbound route
// and returns structured information when no configured route matches.
func ComputeDepartureRouteWithDiagnostics(active []string, stand string, runway string) (ResolvedRoute, bool, RouteSelectionDiagnostics) {
	return selectRunwayRouteWithDiagnostics(active, "", runway, stand, false)
}

func selectRunwayRoute(active []string, currentSector string, runway string, stand string, requireCurrentSector bool) (ResolvedRoute, bool) {
	route, ok, _ := selectRunwayRouteInternal(active, currentSector, runway, stand, requireCurrentSector, false)
	return route, ok
}

func selectRunwayRouteWithDiagnostics(active []string, currentSector string, runway string, stand string, requireCurrentSector bool) (ResolvedRoute, bool, RouteSelectionDiagnostics) {
	return selectRunwayRouteInternal(active, currentSector, runway, stand, requireCurrentSector, true)
}

func selectRunwayRouteInternal(active []string, currentSector string, runway string, stand string, requireCurrentSector bool, collectDiagnostics bool) (ResolvedRoute, bool, RouteSelectionDiagnostics) {
	normalizedRunway := normalizeRunway(runway)
	diagnostics := RouteSelectionDiagnostics{}
	if collectDiagnostics {
		diagnostics = RouteSelectionDiagnostics{
			Runway:           runway,
			NormalizedRunway: normalizedRunway,
			Stand:            stand,
		}
	}

	candidates := runwayRoutes[normalizedRunway]
	if collectDiagnostics {
		diagnostics.CandidateCount = len(candidates)
	}
	if len(candidates) == 0 {
		if collectDiagnostics {
			diagnostics.FailureReason = "no_routes_for_runway"
		}
		return ResolvedRoute{}, false, diagnostics
	}

	parsedStand, standErr := ParseStand(stand)
	if collectDiagnostics && standErr != nil {
		diagnostics.StandParseError = standErr.Error()
	}
	bestScore := -1
	bestStandSpecificity := -1
	var bestRoute ResolvedRoute
	standCandidates := 0
	standMatches := 0
	activeMatches := 0
	for _, r := range candidates {
		candidate := RouteCandidateDiagnostics{}
		if collectDiagnostics {
			candidate = RouteCandidateDiagnostics{
				Name:        r.Name,
				Path:        slices.Clone(r.Path),
				Active:      slices.Clone(r.Active),
				RequireAll:  r.RequireAll,
				StandRanges: formatStandRanges(r.ForStandRanges),
				ActiveScore: -1,
			}
		}
		standSpecificity := 0
		if len(r.ForStandRanges) > 0 {
			if collectDiagnostics {
				candidate.StandSpecific = true
			}
			standCandidates++
			if standErr != nil || !slices.ContainsFunc(r.ForStandRanges, func(standRange StandRange) bool {
				return standRange.Contains(parsedStand)
			}) {
				if collectDiagnostics {
					if standErr != nil {
						candidate.Rejection = "invalid_stand"
					} else {
						candidate.Rejection = "stand_not_in_range"
					}
					diagnostics.Candidates = append(diagnostics.Candidates, candidate)
				}
				continue
			}
			standSpecificity = 1
			if collectDiagnostics {
				candidate.StandMatched = true
			}
			standMatches++
		} else {
			if collectDiagnostics {
				candidate.StandMatched = true
			}
			standMatches++
		}

		score := scoreActive(r.Active, active, r.RequireAll)
		if collectDiagnostics {
			candidate.ActiveScore = score
		}
		if score < 0 || standSpecificity < bestStandSpecificity || (standSpecificity == bestStandSpecificity && score <= bestScore) {
			if collectDiagnostics {
				if score < 0 {
					candidate.Rejection = "active_runways_do_not_match"
				} else {
					candidate.Rejection = "lower_priority_than_selected_route"
				}
				diagnostics.Candidates = append(diagnostics.Candidates, candidate)
			}
			continue
		}
		activeMatches++

		startIdx := 0
		if requireCurrentSector {
			startIdx = indexOfSector(r.Path, currentSector)
			if startIdx < 0 {
				if collectDiagnostics {
					candidate.Rejection = "current_sector_not_in_path"
					diagnostics.Candidates = append(diagnostics.Candidates, candidate)
				}
				continue
			}
		}

		bestStandSpecificity = standSpecificity
		bestScore = score
		bestRoute = resolveRouteSelection(r, startIdx)
		if collectDiagnostics {
			diagnostics.Candidates = append(diagnostics.Candidates, candidate)
		}
	}
	if bestScore < 0 {
		if collectDiagnostics {
			switch {
			case standCandidates > 0 && standMatches == 0 && standErr != nil:
				diagnostics.FailureReason = "invalid_stand"
			case standCandidates > 0 && standMatches == 0:
				diagnostics.FailureReason = "stand_not_configured_for_runway"
			case activeMatches == 0:
				diagnostics.FailureReason = "no_active_runway_match"
			default:
				diagnostics.FailureReason = "no_matching_route"
			}
		}
		return ResolvedRoute{}, false, diagnostics
	}
	return bestRoute, true, diagnostics
}

func formatStandRanges(ranges []StandRange) []string {
	if len(ranges) == 0 {
		return nil
	}

	formatted := make([]string, 0, len(ranges))
	for _, standRange := range ranges {
		from := fmt.Sprintf("%s%d", standRange.Prefix, standRange.From)
		to := fmt.Sprintf("%s%d", standRange.Prefix, standRange.To)
		if from == to {
			formatted = append(formatted, from)
			continue
		}
		formatted = append(formatted, from+"-"+to)
	}
	return formatted
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// ComputeToStand selects a route to the given destination stand that is valid under
// current active flags and contains the aircraft's current Sector within its path.
// Returns the subsequence of config from the current Sector to the end of the route
// together with any route-scoped owner overrides that should apply while constructing
// next owners.
// Note: Single stands are represented as ranges with From == To.
func ComputeToStand(active []string, currentSector string, standStr string) (ResolvedRoute, bool) {
	st, err := ParseStand(standStr)
	if err != nil {
		return ResolvedRoute{}, false
	}
	bestScore := -1
	var bestRoute ResolvedRoute
	for _, r := range standRoutes {
		if len(r.ForStandRanges) == 0 {
			continue
		}
		matched := false
		for _, sr := range r.ForStandRanges {
			if sr.Contains(st) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		score := scoreActive(r.Active, active, r.RequireAll)
		if score < 0 || score <= bestScore {
			continue
		}
		startIdx := indexOfSector(r.Path, currentSector)
		if startIdx < 0 {
			continue
		}
		bestScore = score
		bestRoute = resolveRouteSelection(r, startIdx)
	}
	if bestScore < 0 {
		return ResolvedRoute{}, false
	}
	return bestRoute, true
}

func normalizeRunway(rwy string) string {
	return strings.ToUpper(strings.TrimSpace(rwy))
}

func indexOfSector(path []string, sector string) int {
	for i, s := range path {
		if strings.EqualFold(s, sector) ||
			strings.EqualFold(GetSectorDisplayName(s), GetSectorDisplayName(sector)) {
			return i
		}
	}
	return -1
}

func resolveRouteSelection(route Route, startIdx int) ResolvedRoute {
	return ResolvedRoute{
		Path:           slices.Clone(route.Path[startIdx:]),
		OwnerOverrides: cloneStringMap(route.OwnerOverrides),
	}
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(src))
	for key, value := range src {
		cloned[key] = value
	}
	return cloned
}

type AirborneRoutes struct {
	Name         string   `yaml:"name"`
	UseAsDefault bool     `yaml:"default"`
	Sids         []string `yaml:"sids"`
}

var ErrUnknownAirborneRoute = errors.New("unknown airborne route")
var ErrNoDefaultAirborneRoute = errors.New("no default airborne route configured")

func GetAirborneSector(sid string) (string, error) {
	for _, ar := range airborneRoutes {
		if slices.Contains(ar.Sids, sid) {
			return ar.Name, nil
		}
	}
	return "", fmt.Errorf("%w for SID %q", ErrUnknownAirborneRoute, sid)
}

// GetArrivalTowerSector returns the first sector in any arrival route that is
// valid under the given active runways. For EKCH this is always TE or TW —
// the receiving tower sector — and is used to ensure arrival strips have at
// least the tower controller as a next owner even before a stand is assigned.
func GetArrivalTowerSector(active []string) (string, bool) {
	bestScore := -1
	bestSector := ""
	for _, r := range standRoutes {
		if len(r.ForStandRanges) == 0 || len(r.Path) == 0 {
			continue
		}
		score := scoreActive(r.Active, active, r.RequireAll)
		if score < 0 || score <= bestScore {
			continue
		}
		bestScore = score
		bestSector = r.Path[0]
	}
	if bestScore < 0 {
		return "", false
	}
	return bestSector, true
}

func GetAirborneControllerPriority(sid string) ([]string, error) {
	sectorName, err := GetAirborneSector(sid)
	if err != nil {
		return nil, err
	}

	return getAirborneControllerPriorityForSector(sectorName)
}

func GetDefaultAirborneControllerPriority() ([]string, error) {
	for _, ar := range airborneRoutes {
		if ar.UseAsDefault {
			return getAirborneControllerPriorityForSector(ar.Name)
		}
	}

	return nil, ErrNoDefaultAirborneRoute
}

func getAirborneControllerPriorityForSector(sectorName string) ([]string, error) {
	for _, sector := range sectors {
		if sectorMatchesIdentifier(sector, sectorName) {
			slog.Debug("Found airborne sector, returning owner priority list", slog.String("sector", sector.Name), slog.Any("owners", sector.Owner))
			return slices.Clone(sector.Owner), nil
		}
	}

	return nil, fmt.Errorf("no sector owners configured for airborne sector %q", sectorName)
}
