package gsx

import (
	"slices"
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

// TestShippedEKCHKeepsBothFacings guards the rule that matters most: when a
// stand offers the same taxiway in two facings, both are published. A release
// point cannot say which facing, but narrowing the pilot's menu to the pair
// still guarantees they leave via the taxiway the controller named - and the
// facing stays their choice rather than a coin flip.
func TestShippedEKCHKeepsBothFacings(t *testing.T) {
	cfg, err := LoadSceneryConfig("../../config/ekch/gsx_sceneries.json")
	if err != nil || cfg == nil {
		t.Fatalf("load shipped config: %v", err)
	}
	sceneries := Sceneries{cfg.ICAO: cfg}

	// A34 offers both "Z1 Face E" and "Z1 Face W" and has no local rule, so
	// both are published and the pilot picks.
	_, both := sceneries.Resolve("EKCH", "A34", "Simnord-Sonnich", "Z1")
	if len(both) != 2 {
		t.Fatalf("Z1 must publish both facings, got %q", both)
	}
	for _, want := range []string{"Z1 Face E", "Z1 Face W"} {
		if !slices.Contains(both, want) {
			t.Errorf("Z1 is missing %q, got %q", want, both)
		}
	}

	// A point the stand cannot reach publishes nothing.
	if _, none := sceneries.Resolve("EKCH", "A34", "Simnord-Sonnich", "T5"); len(none) != 0 {
		t.Errorf("an unreachable point must publish nothing, got %q", none)
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
	} {
		if _, got := sceneries.Resolve("EKCH", c.gate, "Simnord-Sonnich", c.point); strings.Join(got, "|") != c.want {
			t.Errorf("%s %s: got %q want %q", c.gate, c.point, got, c.want)
		}
	}

	// A17 is always eastbound, but its only Y0 route faces west - so Y0
	// publishes nothing rather than pushing against the rule.
	if _, none := sceneries.Resolve("EKCH", "A17", "Simnord-Sonnich", "Y0"); len(none) != 0 {
		t.Errorf("A17 Y0 faces west and must publish nothing, got %q", none)
	}
}
