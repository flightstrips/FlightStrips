package gsx

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// SceneryConfig maps what controllers say onto what a particular scenery calls
// it, one gate at a time.
//
// The same physical gate is not the same object in every add-on: developers
// number stands differently and each authors their own pushback route names, so
// "push onto Z2 facing east" is a different string in Simnord's EKCH than in
// anyone else's. Nothing can resolve that centrally, so it is declared here:
//
//	gate -> scenery -> { stand, points }
//
// A pilot's handler script says which scenery it is running (it is bound to one
// GSX profile), and the endpoint answers in that scenery's vocabulary.
type SceneryConfig struct {
	ICAO string `json:"icao"`
	// Gates is keyed by the stand as controllers know it - the same string that
	// goes in strips.stand - then by scenery name.
	Gates map[string]map[string]GateScenery `json:"gates"`

	// ReleasePoints places each FlightStrips release point on the ground. It is
	// what makes the feed work for any GSX profile: route names differ from one
	// profile to the next (and are sometimes wrong), but where a route ends does
	// not, so the handler picks the route ending nearest the point.
	ReleasePoints map[string]PointPosition `json:"release_points,omitempty"`
	// ReleasePointRadiusMeters is how far a route may end from its point and
	// still count, provided that point is the closest.
	ReleasePointRadiusMeters float64 `json:"release_point_radius_m,omitempty"`
	// Facings are the airport's own rules for a point a stand can leave in two
	// facings: gate -> point -> compass direction (N, NE, E ... NW). Operational
	// knowledge, so it is stated once here rather than in any profile's labels.
	Facings map[string]map[string]string `json:"facings,omitempty"`
}

// PointPosition is where a release point is, in degrees.
type PointPosition struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Target is where a departure should end up, for the handler to match against
// the routes its own profile offers at the stand. A named point carries a
// position and perhaps a facing; a bare direction carries a heading only.
type Target struct {
	Lat          *float64 `json:"lat"`
	Lon          *float64 `json:"lon"`
	Heading      *float64 `json:"heading"`
	RadiusMeters float64  `json:"radius_m"`
	// Avoid lists the other named points. For a position, a route closer to
	// any of them belongs to a different point. For a direction, the handler
	// prefers a route ending away from all named points.
	Avoid []PointPosition `json:"avoid,omitempty"`
}

const defaultReleasePointRadius = 40

var compassHeadings = map[string]float64{
	"N": 0, "NE": 45, "E": 90, "SE": 135, "S": 180, "SW": 225, "W": 270, "NW": 315,
	"NORTH": 0, "NORTHEAST": 45, "EAST": 90, "SOUTHEAST": 135,
	"SOUTH": 180, "SOUTHWEST": 225, "WEST": 270, "NORTHWEST": 315,
}

// Target resolves a release point at a gate to something a handler can match
// by geometry. nil when the point is neither a known position nor a direction.
func (s Sceneries) Target(icao, gate, releasePoint string) *Target {
	point := normalizeKey(releasePoint)
	if point == "" {
		return nil
	}

	radius := float64(defaultReleasePointRadius)
	var cfg *SceneryConfig
	if s != nil {
		cfg = s[normalizeKey(icao)]
	}
	if cfg != nil && cfg.ReleasePointRadiusMeters > 0 {
		radius = cfg.ReleasePointRadiusMeters
	}

	// The direction buttons on the pushback map: no point, only which way to
	// face. That is how a bare taxiway is assigned.
	if heading, ok := compassHeadings[point]; ok {
		target := &Target{Heading: &heading, RadiusMeters: radius}
		if cfg != nil {
			target.Avoid = cfg.otherReleasePoints("")
		}
		return target
	}

	if cfg == nil {
		return nil
	}
	position, ok := lookup(cfg.ReleasePoints, point)
	if !ok {
		return nil
	}
	target := &Target{
		Lat: &position.Lat, Lon: &position.Lon, RadiusMeters: radius,
		Avoid: cfg.otherReleasePoints(point),
	}
	if rules, ok := lookup(cfg.Facings, gate); ok {
		if facing, ok := lookup(rules, point); ok {
			if heading, ok := compassHeadings[normalizeKey(facing)]; ok {
				target.Heading = &heading
			}
		}
	}
	return target
}

func (cfg *SceneryConfig) otherReleasePoints(point string) []PointPosition {
	names := make([]string, 0, len(cfg.ReleasePoints))
	for name := range cfg.ReleasePoints {
		if point == "" || normalizeKey(name) != point {
			names = append(names, name)
		}
	}
	sort.Strings(names) // keep the target and its revision stable across polls
	positions := make([]PointPosition, 0, len(names))
	for _, name := range names {
		positions = append(positions, cfg.ReleasePoints[name])
	}
	return positions
}

type GateScenery struct {
	// Stand overrides the name handed to selectGate() for this scenery. Leave
	// empty when the scenery uses the same stand name controllers do.
	Stand string `json:"stand,omitempty"`
	// Points maps a FlightStrips release point to the GSX pushback label for
	// this gate in this scenery. GSX labels come from the profile's
	// pushbacklabels (the two defaults, left then right) and from the label of
	// each entry in pushbackaddpos.
	Points map[string][]string `json:"points,omitempty"`
}

// LoadSceneryConfig reads a per-airport scenery file. A missing file is not an
// error: the feed still publishes stands, just without scenery-specific naming
// or any pushback point.
func LoadSceneryConfig(path string) (*SceneryConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read GSX scenery config: %w", err)
	}

	var cfg SceneryConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse GSX scenery config %s: %w", path, err)
	}
	if strings.TrimSpace(cfg.ICAO) == "" {
		return nil, fmt.Errorf("parse GSX scenery config %s: icao is required", path)
	}
	cfg.ICAO = strings.ToUpper(strings.TrimSpace(cfg.ICAO))
	return &cfg, nil
}

// Sceneries indexes loaded configs by airport.
type Sceneries map[string]*SceneryConfig

// normalizeKey makes lookups tolerant of the spacing and casing differences that
// creep into hand-maintained config and into scenery labels alike - one EKCH
// profile has both "Y1 Face E" and "Y1  Face E".
func normalizeKey(value string) string {
	return strings.Join(strings.Fields(strings.ToUpper(value)), " ")
}

// Resolve returns the stand name and pushback label to publish for this gate in
// this scenery. stand falls back to the controller's own value, and pushback is
// empty whenever anything is unknown - an unmapped release point is a normal
// answer, not an error.
func (s Sceneries) Resolve(icao, gate, scenery, releasePoint string) (stand string, pushback []string) {
	stand = gate

	cfg := s[normalizeKey(icao)]
	if cfg == nil || scenery == "" {
		return stand, nil
	}

	byScenery, ok := lookup(cfg.Gates, gate)
	if !ok {
		return stand, nil
	}
	entry, ok := lookup(byScenery, scenery)
	if !ok {
		return stand, nil
	}

	if entry.Stand != "" {
		stand = entry.Stand
	}
	if releasePoint != "" {
		if label, found := lookup(entry.Points, releasePoint); found {
			pushback = label
		}
	}
	return stand, pushback
}

// lookup does a normalized-key search over a map that is keyed by whatever the
// config author typed.
func lookup[V any](m map[string]V, key string) (V, bool) {
	var zero V
	if m == nil {
		return zero, false
	}
	if v, ok := m[key]; ok {
		return v, true
	}
	wanted := normalizeKey(key)
	for k, v := range m {
		if normalizeKey(k) == wanted {
			return v, true
		}
	}
	return zero, false
}
