package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testAirportRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}
func selectedAirportFile(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "selected.yaml")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestResolveAirportResources(t *testing.T) {
	root := testAirportRoot(t)
	outside := t.TempDir()
	valid := filepath.Join(outside, "regions.json")
	data, err := os.ReadFile(filepath.Join(root, "backend", "config", "ekch_regions.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(valid, data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, override, fallback string
		wantError                bool
	}{
		{"unset", "", "backend/config/ekch_regions.json", false},
		{"blank", "  ", "backend/config/ekch_regions.json", false},
		{"external override", valid, "backend/config/missing.json", false},
		{"relative override", "backend/config/ekch_regions.json", "backend/config/missing.json", false},
		{"missing override", filepath.Join(outside, "missing.json"), "backend/config/ekch_regions.json", true},
		{"missing fallback", "", "backend/config/missing.json", true},
		{"escaping default with override", valid, "../outside.json", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			definition := mutateAirport(t, airportExample(t, "ekch"), func(c map[string]any) {
				c["geography"].(map[string]any)["regions"] = map[string]any{"environment": "TEST_REGIONS", "default_repo_path": test.fallback}
			})
			registry, err := LoadAirportFiles(ProductionAirportCatalog(), []string{selectedAirportFile(t, definition)}, AirportLoadOptions{RepositoryRoot: root, LookupEnv: func(string) (string, bool) { return test.override, test.override != "" }})
			if (err != nil) != test.wantError {
				t.Fatalf("load error = %v, want error %v", err, test.wantError)
			}
			if err != nil {
				if !strings.Contains(err.Error(), "airport EKCH") {
					t.Fatalf("missing context: %v", err)
				}
				return
			}
			airport, _ := registry.Lookup("EKCH")
			if _, ok := airport.Resource("geography.regions"); !ok {
				t.Fatal("region unresolved")
			}
		})
	}
}
func TestDisabledAirportServicesDoNotRequireAssets(t *testing.T) {
	data := mutateAirport(t, airportExample(t, "ekch"), func(c map[string]any) {
		services := c["services"].(map[string]any)
		for _, service := range services {
			s := service.(map[string]any)
			s["availability"] = "disabled"
		}
		services["aman"].(map[string]any)["terminal"] = map[string]any{"repo_path": "missing.json"}
		services["stand_assignment"].(map[string]any)["aircraft_icao"] = map[string]any{"repo_path": "also-missing.json"}
	})
	registry, err := LoadAirportFiles(ProductionAirportCatalog(), []string{selectedAirportFile(t, data)}, AirportLoadOptions{RepositoryRoot: testAirportRoot(t), DeploymentServices: map[string]bool{"aman": true, "pdc": true, "stand_assignment": true}})
	if err != nil {
		t.Fatal(err)
	}
	airport, _ := registry.Lookup("EKCH")
	for _, name := range []string{"aman", "pdc", "stand_assignment"} {
		if airport.ServiceEnabled(name) {
			t.Fatalf("disabled %s active", name)
		}
	}
	if _, ok := airport.Resource("services.aman.terminal"); ok {
		t.Fatal("disabled service resolved assets")
	}
}
func TestRequiredAirportAssetsFailAtomically(t *testing.T) {
	data := mutateAirport(t, airportExample(t, "ekch"), func(c map[string]any) {
		c["services"].(map[string]any)["stand_assignment"].(map[string]any)["aircraft_icao"] = map[string]any{"repo_path": "missing.json"}
	})
	for _, enabled := range []bool{false, true} {
		registry, err := LoadAirportFiles(ProductionAirportCatalog(), []string{selectedAirportFile(t, data)}, AirportLoadOptions{RepositoryRoot: testAirportRoot(t), DeploymentServices: map[string]bool{"stand_assignment": enabled}})
		if enabled {
			if err == nil || registry != nil || !strings.Contains(err.Error(), "services.stand_assignment.aircraft_icao") {
				t.Fatalf("required asset accepted: %v", err)
			}
		} else if err != nil {
			t.Fatal(err)
		}
	}
}
func TestAirportRegionAndRevisionValidation(t *testing.T) {
	root := testAirportRoot(t)
	file := selectedAirportFile(t, airportExample(t, "ekch"))
	options := AirportLoadOptions{RepositoryRoot: root, DeploymentServices: map[string]bool{"cdm": true, "aman": true}}
	first, err := LoadAirportFiles(ProductionAirportCatalog(), []string{file}, options)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := first.Lookup("EKCH")
	second, err := LoadAirportFiles(ProductionAirportCatalog(), []string{file}, options)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := second.Lookup("EKCH")
	if a.Revision() != b.Revision() {
		t.Fatal("revision is nondeterministic")
	}
	data := mutateAirport(t, airportExample(t, "ekch"), func(c map[string]any) {
		c["coverage"].(map[string]any)["responsibilities"].([]any)[0].(map[string]any)["region"] = []any{"MISSING"}
	})
	if _, err := LoadAirportFiles(ProductionAirportCatalog(), []string{selectedAirportFile(t, data)}, options); err == nil || !strings.Contains(err.Error(), "unknown \"MISSING\"") {
		t.Fatalf("accepted unknown region: %v", err)
	}
}
func TestRepositoryResourceRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if _, err := constrainedRepositoryPath(root, "escape/missing.json"); err == nil {
		t.Fatal("accepted outward symlink")
	}
}
