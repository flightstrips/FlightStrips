package gsx

import (
	"strings"
	"testing"
)

func TestLoadsTheShippedEKCHConfig(t *testing.T) {
	cfg, err := LoadSceneryConfig("../../config/ekch/gsx_sceneries.json")
	if err != nil {
		t.Fatalf("shipped config must load: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected the shipped config to be found")
	}
	if cfg.ICAO != "EKCH" {
		t.Fatalf("icao: %q", cfg.ICAO)
	}
	if len(cfg.Gates) != 119 {
		t.Fatalf("expected 119 gates, got %d", len(cfg.Gates))
	}
	entry, ok := cfg.Gates["A31"]["Simnord-Sonnich"]
	if !ok {
		t.Fatal("A31 / Simnord-Sonnich missing")
	}
	if entry.Stand != "Gate A31" {
		t.Fatalf("stand: %q", entry.Stand)
	}
}

// TestShippedEKCHResolvesAnInferredPushback pins one real inferred mapping end
// to end, so a regeneration that silently empties the points is caught.
func TestShippedEKCHResolvesAnInferredPushback(t *testing.T) {
	cfg, err := LoadSceneryConfig("../../config/ekch/gsx_sceneries.json")
	if err != nil || cfg == nil {
		t.Fatalf("load shipped config: %v", err)
	}
	sceneries := Sceneries{cfg.ICAO: cfg}

	stand, pushback := sceneries.Resolve("EKCH", "A31", "Simnord-Sonnich", "Z2")
	if stand != "Gate A31" {
		t.Errorf("stand: got %q want Gate A31", stand)
	}
	if strings.Join(pushback, "|") != "Z2 Face E" {
		t.Errorf("pushback: got %q want Z2 Face E", pushback)
	}
}

// TestShippedEKCHAppliesLocalFacingRules pins the stands where aircraft always
// leave the same way. A15 offers Y1 in both facings, but only the eastbound
// route is correct there, so only that one is published.
func TestShippedEKCHAppliesLocalFacingRules(t *testing.T) {
	cfg, err := LoadSceneryConfig("../../config/ekch/gsx_sceneries.json")
	if err != nil || cfg == nil {
		t.Fatalf("load shipped config: %v", err)
	}
	sceneries := Sceneries{cfg.ICAO: cfg}

	for _, c := range []struct{ gate, point, want string }{
		{"A12", "Y1", "Y1 Face E"},
		{"A14", "Y1", "Y1 Face E"},
		{"A15", "Y1", "Y1 Face E"},
		{"A17", "Z5", "Z5 Face E"},
		{"A17", "Y0", "Y0 Face W"},
		{"A34", "Z1", "Z1 Face E"},
		{"E20", "S1", "S1 Face N"},
	} {
		if _, got := sceneries.Resolve("EKCH", c.gate, "Simnord-Sonnich", c.point); strings.Join(got, "|") != c.want {
			t.Errorf("%s %s: got %q want %q", c.gate, c.point, got, c.want)
		}
	}

	// A point the stand cannot reach still publishes nothing.
	if _, none := sceneries.Resolve("EKCH", "A17", "Simnord-Sonnich", "T5"); len(none) != 0 {
		t.Errorf("an unreachable point must publish nothing, got %q", none)
	}
}

// TestShippedEKCHIsUnambiguous asserts the invariant the facing rules bought:
// every release point in the shipped file resolves to exactly one route, so no
// pilot is ever handed a choice the controller already made.
func TestShippedEKCHIsUnambiguous(t *testing.T) {
	cfg, err := LoadSceneryConfig("../../config/ekch/gsx_sceneries.json")
	if err != nil || cfg == nil {
		t.Fatalf("load shipped config: %v", err)
	}

	total := 0
	for gate, sceneries := range cfg.Gates {
		for scenery, entry := range sceneries {
			for point, routes := range entry.Points {
				total++
				if len(routes) != 1 {
					t.Errorf("%s/%s %s resolves to %d routes, want 1: %q", gate, scenery, point, len(routes), routes)
				}
			}
		}
	}
	if total == 0 {
		t.Fatal("expected the shipped config to map some points")
	}
}
