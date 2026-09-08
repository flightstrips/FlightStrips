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
