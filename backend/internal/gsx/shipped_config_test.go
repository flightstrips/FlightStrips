package gsx

import (
	"encoding/json"
	"os"
	"testing"
)

const shippedEKCH = "../../config/ekch/gsx_sceneries.json"

func TestLoadsTheShippedEKCHConfig(t *testing.T) {
	cfg, err := LoadSceneryConfig(shippedEKCH)
	if err != nil {
		t.Fatalf("shipped config must load: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected the shipped config to be found")
	}
	if cfg.ICAO != "EKCH" {
		t.Fatalf("icao: %q", cfg.ICAO)
	}
}

// TestShippedEKCHIsTheSameForEveryProfile keeps the file free of anything that
// belongs to one GSX profile. Stand names and route labels differ between
// profiles; the handler matches those itself.
func TestShippedEKCHIsTheSameForEveryProfile(t *testing.T) {
	raw, err := os.ReadFile(shippedEKCH)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatalf("parse: %v", err)
	}
	allowed := map[string]bool{
		"icao": true, "release_point_radius_m": true, "release_points": true,
		"release_points_review": true, "facings": true,
	}
	for key := range keys {
		if !allowed[key] {
			t.Errorf("unexpected key %q: the file describes the airport, not a GSX profile", key)
		}
	}
}

// TestShippedEKCHAppliesLocalFacingRules pins the stands where aircraft always
// leave the same way. A15 offers Y1 in both facings, but only the eastbound
// route is correct there, so the target carries that heading.
func TestShippedEKCHAppliesLocalFacingRules(t *testing.T) {
	cfg, err := LoadSceneryConfig(shippedEKCH)
	if err != nil || cfg == nil {
		t.Fatalf("load shipped config: %v", err)
	}
	sceneries := Sceneries{cfg.ICAO: cfg}

	for _, c := range []struct {
		gate, point string
		heading     float64
	}{
		{"A12", "Y1", 90},
		{"A14", "Y1", 90},
		{"A15", "Y1", 90},
		{"A34", "Z1", 90},
		{"E20", "S1", 0},
	} {
		target := sceneries.Target("EKCH", c.gate, c.point)
		if target == nil || target.Heading == nil || *target.Heading != c.heading {
			t.Errorf("%s %s: want heading %v, got %+v", c.gate, c.point, c.heading, target)
		}
	}

	// Without a rule the target has a position and no facing.
	if target := sceneries.Target("EKCH", "D2", "S1"); target == nil || target.Lat == nil || target.Heading != nil {
		t.Errorf("D2 S1: want a position and no heading, got %+v", target)
	}
}
