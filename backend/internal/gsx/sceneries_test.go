package gsx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gsx_sceneries.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

const ekchConfig = `{
  "icao": "EKCH",
  "gates": {
    "A31": {
      "Simnord-Sonnich": {
        "stand": "Gate A31",
        "points": { "Z/L": ["Z2 Face E"], "Y/L": ["Z3 Face W"], "K/J": ["J1 Face S"] }
      },
      "FlyTampa": {
        "points": { "Z/L": ["Z2 EAST"] }
      }
    }
  }
}`

func loaded(t *testing.T, body string) Sceneries {
	t.Helper()
	cfg, err := LoadSceneryConfig(writeConfig(t, body))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected a config")
	}
	return Sceneries{cfg.ICAO: cfg}
}

func TestResolveTranslatesStandAndPoint(t *testing.T) {
	s := loaded(t, ekchConfig)

	stand, pushback := s.Resolve("EKCH", "A31", "Simnord-Sonnich", "Z/L")
	if stand != "Gate A31" {
		t.Errorf("stand: got %q want %q", stand, "Gate A31")
	}
	if strings.Join(pushback, "|") != "Z2 Face E" {
		t.Errorf("pushback: got %q want %q", pushback, "Z2 Face E")
	}
}

func TestResolveIsPerScenery(t *testing.T) {
	s := loaded(t, ekchConfig)

	// Same gate, same release point, different add-on -> different label, and
	// this scenery declares no stand override so the controller's name stands.
	stand, pushback := s.Resolve("EKCH", "A31", "FlyTampa", "Z/L")
	if stand != "A31" {
		t.Errorf("stand: got %q want %q", stand, "A31")
	}
	if strings.Join(pushback, "|") != "Z2 EAST" {
		t.Errorf("pushback: got %q want %q", pushback, "Z2 EAST")
	}
}

func TestResolveUnknownsFallBackQuietly(t *testing.T) {
	s := loaded(t, ekchConfig)

	cases := []struct{ name, icao, gate, scenery, point string }{
		{"unknown scenery", "EKCH", "A31", "SomeOtherAddon", "Z/L"},
		{"unmapped release point", "EKCH", "A31", "Simnord-Sonnich", "B/Y"},
		{"unknown gate", "EKCH", "C34", "Simnord-Sonnich", "Z/L"},
		{"unknown airport", "EKBI", "A31", "Simnord-Sonnich", "Z/L"},
		{"no scenery given", "EKCH", "A31", "", "Z/L"},
		{"no release point", "EKCH", "A31", "Simnord-Sonnich", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stand, pushback := s.Resolve(c.icao, c.gate, c.scenery, c.point)
			if strings.Join(pushback, "|") != "" {
				t.Errorf("expected no pushback, got %q", pushback)
			}
			if stand == "" {
				t.Error("stand must always fall back to the controller's value")
			}
		})
	}
}

func TestResolveIgnoresSpacingAndCase(t *testing.T) {
	// One real EKCH profile contains both "Y1 Face E" and "Y1  Face E".
	s := loaded(t, `{"icao":"ekch","gates":{"a31":{"simnord-sonnich":{"points":{"z/l":["Z2  Face  E"]}}}}}`)

	stand, pushback := s.Resolve("EKCH", "A31", "Simnord-Sonnich", "Z/L")
	if strings.Join(pushback, "|") != "Z2  Face  E" {
		t.Errorf("expected the label verbatim, got %q", pushback)
	}
	if stand != "A31" {
		t.Errorf("stand: got %q", stand)
	}
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	cfg, err := LoadSceneryConfig(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("a missing config must not fail startup: %v", err)
	}
	if cfg != nil {
		t.Fatal("expected nil config")
	}
}

func TestLoadRejectsBadConfig(t *testing.T) {
	if _, err := LoadSceneryConfig(writeConfig(t, `{"gates":{}}`)); err == nil {
		t.Error("expected an error when icao is missing")
	}
	if _, err := LoadSceneryConfig(writeConfig(t, `{"icao":`)); err == nil {
		t.Error("expected an error on malformed JSON")
	}
}

func TestResolveOnNilSceneries(t *testing.T) {
	var s Sceneries
	stand, pushback := s.Resolve("EKCH", "A31", "Simnord-Sonnich", "Z/L")
	if stand != "A31" || len(pushback) != 0 {
		t.Errorf("nil config must be inert: got %q / %v", stand, pushback)
	}
}

// TestResolveKeepsEveryRouteToAPoint covers the multi-route capability. The
// shipped EKCH file has no ambiguity left, but another scenery will, and a
// release point names a taxiway rather than a facing.
func TestResolveKeepsEveryRouteToAPoint(t *testing.T) {
	s := loaded(t, `{"icao":"EKZZ","gates":{"A1":{"Some-Addon":{
		"points":{"Y1":["Y1 Face W","Y1 Face E"],"Y0":["Y0 Face E"]}}}}}`)

	_, both := s.Resolve("EKZZ", "A1", "Some-Addon", "Y1")
	if strings.Join(both, "|") != "Y1 Face W|Y1 Face E" {
		t.Errorf("both routes must survive in order, got %q", both)
	}
	if _, one := s.Resolve("EKZZ", "A1", "Some-Addon", "Y0"); strings.Join(one, "|") != "Y0 Face E" {
		t.Errorf("single route: got %q", one)
	}
}
