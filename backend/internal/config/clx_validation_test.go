package config

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

func TestNormalizeClxValidationConfig_DoesNotInjectDefaults(t *testing.T) {
	cfg := normalizeClxValidationConfig(ClxValidationConfig{})

	if !cfg.IsEmpty() {
		t.Fatalf("expected empty CLX validation config, got %+v", cfg)
	}
	if len(cfg.SidFirstWaypoints) != 0 {
		t.Fatalf("expected no inferred SID waypoints, got %+v", cfg.SidFirstWaypoints)
	}
}

func TestClxValidationConfigYamlShape(t *testing.T) {
	var cfg Config
	dec := yaml.NewDecoder(strings.NewReader(`
clx_validation:
  sid_first_waypoints:
    - betud
    - odon
  sid_engine_rules:
    - code: sid_aircraft_type
      sid_families: [kopex]
      disallowed_engine_types: [j]
      message: "Aircraft planned on SID not valid for ATYP. {recommendation}"
      recommendations:
        MICOS: "Reclear on NEXEN T503 MICOS... as filed"
      default_recommendation: "Reclear on LANGO DCT... as filed"
  aircraft_runway_rules:
    - code: category_f_runway
      aircraft_types: [a388]
      restricted_runways: ["22r"]
      restricted_sid_suffixes: [d]
      message: "Planned RWY not available for aircraft Category (CAT F). Only 04R/22L approved"
  rnav_rules:
    "nil":
      code: rnav_nil
      message: "Aircraft filed on SID without RNAV capability. Clear via RV or update RNAV capability to \"1\"."
    insufficient:
      code: rnav_insufficient
      capabilities: ["5", "10"]
      message: "Aircraft filed on SID with insufficient RNAV capability. Clear via RV or update RNAV capability to \"1\"."
  route_conflict_rules:
    - code: route_lango_egpx
      sid_families: [lango]
      route_tokens_any: [artex]
      remark_tokens_any: [egpx/]
      message: "LANGO not valid for flights to EGPX. Refile to ODDON."
      allow_override: true
`))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}

	normalized := normalizeClxValidationConfig(cfg.ClxValidation)

	if normalized.IsEmpty() {
		t.Fatal("expected CLX validation config to contain rules")
	}
	if got := normalized.SidFirstWaypoints; len(got) != 2 || got[0] != "BETUD" || got[1] != "ODON" {
		t.Fatalf("unexpected sid_first_waypoints: %+v", got)
	}
	if got := normalized.SidEngineRules[0].SidFamilies; len(got) != 1 || got[0] != "KOPEX" {
		t.Fatalf("unexpected sid engine rule families: %+v", got)
	}
	if got := normalized.SidEngineRules[0].DisallowedEngineTypes; len(got) != 1 || got[0] != "J" {
		t.Fatalf("unexpected sid engine rule engine types: %+v", got)
	}
	if normalized.SidEngineRules[0].Recommendations["MICOS"] != "Reclear on NEXEN T503 MICOS... as filed" {
		t.Fatalf("unexpected recommendations: %+v", normalized.SidEngineRules[0].Recommendations)
	}
	if got := normalized.AircraftRunwayRules[0].RestrictedRunways; len(got) != 1 || got[0] != "22R" {
		t.Fatalf("unexpected restricted runways: %+v", got)
	}
	if normalized.RnavRules.Nil.Code != "rnav_nil" {
		t.Fatalf("unexpected rnav nil code: %q", normalized.RnavRules.Nil.Code)
	}
	if got := normalized.RnavRules.Insufficient.Capabilities; len(got) != 2 || got[0] != "5" || got[1] != "10" {
		t.Fatalf("unexpected insufficient capabilities: %+v", got)
	}
	if !normalized.RouteConflictRules[0].AllowOverride {
		t.Fatal("expected route conflict rule to allow override")
	}
	if got := normalized.RouteConflictRules[0].RemarkTokensAny; len(got) != 1 || got[0] != "EGPX/" {
		t.Fatalf("unexpected remark tokens: %+v", got)
	}
}
