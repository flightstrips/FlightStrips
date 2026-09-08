package gsx

import (
	"encoding/json"
	"fmt"
	"os"
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
}

type GateScenery struct {
	// Stand overrides the name handed to selectGate() for this scenery. Leave
	// empty when the scenery uses the same stand name controllers do.
	Stand string `json:"stand,omitempty"`
	// Points maps a FlightStrips release point to the GSX pushback label for
	// this gate in this scenery. GSX labels come from the profile's
	// pushbacklabels (the two defaults, left then right) and from the label of
	// each entry in pushbackaddpos.
	Points map[string]string `json:"points,omitempty"`
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
func (s Sceneries) Resolve(icao, gate, scenery, releasePoint string) (stand string, pushback string) {
	stand = gate

	cfg := s[normalizeKey(icao)]
	if cfg == nil || scenery == "" {
		return stand, ""
	}

	byScenery, ok := lookup(cfg.Gates, gate)
	if !ok {
		return stand, ""
	}
	entry, ok := lookup(byScenery, scenery)
	if !ok {
		return stand, ""
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
