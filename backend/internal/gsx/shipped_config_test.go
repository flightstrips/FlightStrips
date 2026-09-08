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

	// A15 offers both "Y1 Face W" and "Y1 Face E".
	_, both := sceneries.Resolve("EKCH", "A15", "Simnord-Sonnich", "Y1")
	if len(both) != 2 {
		t.Fatalf("Y1 must publish both facings, got %q", both)
	}
	for _, want := range []string{"Y1 Face W", "Y1 Face E"} {
		if !slices.Contains(both, want) {
			t.Errorf("Y1 is missing %q, got %q", want, both)
		}
	}

	// Y0 is unambiguous at the same stand and resolves to exactly one route.
	if _, one := sceneries.Resolve("EKCH", "A15", "Simnord-Sonnich", "Y0"); strings.Join(one, "|") != "Y0 Face E" {
		t.Errorf("Y0: got %q want Y0 Face E", one)
	}

	// A point the stand cannot reach still publishes nothing.
	if _, none := sceneries.Resolve("EKCH", "A15", "Simnord-Sonnich", "T5"); len(none) != 0 {
		t.Errorf("an unreachable point must publish nothing, got %q", none)
	}
}
