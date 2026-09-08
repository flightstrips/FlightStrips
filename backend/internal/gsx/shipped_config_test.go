package gsx

import "testing"

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
	if pushback != "Z2 Face E" {
		t.Errorf("pushback: got %q want Z2 Face E", pushback)
	}
}

// TestShippedEKCHOmitsAmbiguousPushback guards the rule that matters most: a
// stand offering the same taxiway in two facings publishes neither, because a
// release point cannot say which and pushing the wrong way is worse than
// leaving the menu alone.
func TestShippedEKCHOmitsAmbiguousPushback(t *testing.T) {
	cfg, err := LoadSceneryConfig("../../config/ekch/gsx_sceneries.json")
	if err != nil || cfg == nil {
		t.Fatalf("load shipped config: %v", err)
	}
	sceneries := Sceneries{cfg.ICAO: cfg}

	// A15 offers both "Y1 Face W" and "Y1 Face E".
	if _, pushback := sceneries.Resolve("EKCH", "A15", "Simnord-Sonnich", "Y1"); pushback != "" {
		t.Errorf("ambiguous Y1 must publish nothing, got %q", pushback)
	}
	// Y0 is unambiguous at the same stand and must still resolve.
	if _, pushback := sceneries.Resolve("EKCH", "A15", "Simnord-Sonnich", "Y0"); pushback != "Y0 Face E" {
		t.Errorf("Y0: got %q want Y0 Face E", pushback)
	}
}
