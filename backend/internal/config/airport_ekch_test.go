package config

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go.yaml.in/yaml/v4"
)

func TestProductionEKCHPreservesAllLegacySettings(t *testing.T) {
	root := testAirportRoot(t)
	registry, err := LoadAirportFiles(ProductionAirportCatalog(), []string{"backend/config/airports/ekch.airport.yaml"}, AirportLoadOptions{RepositoryRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	definition, _ := registry.Lookup("EKCH")
	data, err := os.ReadFile(filepath.Join(root, "backend", "config", "ekch.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var legacy Config
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&legacy); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(legacy, definition.DomainConfig()) {
		t.Fatal("production projection changed legacy configuration")
	}
	var keys map[string]any
	if err := yaml.Unmarshal(data, &keys); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 20 {
		t.Fatalf("legacy baseline has %d settings; review migration coverage", len(keys))
	}
	if len(definition.Document().Workflow.Bays) != 18 {
		t.Fatal("missing production bays")
	}
	// The backend image includes exactly the data used by its coded text view.
	backend, err := os.ReadFile(filepath.Join(root, "backend", "config", "ekch", "text-pilot-layouts.json"))
	if err != nil {
		t.Fatal(err)
	}
	frontend, err := os.ReadFile(filepath.Join(root, "frontend", "src", "components", "text-pilot", "storyboard-layouts.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.ReplaceAll(backend, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll(frontend, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("text-pilot data differs from coded frontend")
	}
}
func TestProductionMasterPriorities(t *testing.T) {
	registry, err := LoadAirportFiles(ProductionAirportCatalog(), []string{"backend/config/airports/ekch.airport.yaml"}, AirportLoadOptions{RepositoryRoot: testAirportRoot(t)})
	if err != nil {
		t.Fatal(err)
	}
	definition, _ := registry.Lookup("EKCH")
	for _, test := range []struct {
		callsign, frequency string
		want                int
	}{
		{"EKDK_FMP", "", 0}, {"OTHER", "131.040", 0}, {"EKDK_B_CTR", "", 1}, {"EKDK_D_CTR", "", 2}, {"EKDK_UC_CTR", "", 3}, {"EKCH_A_TWR", "", 4}, {"OTHER", "", 5},
		{"EKDK_B_CTR", "119.905", 5},
	} {
		if got := definition.MasterPriority(test.callsign, test.frequency); got != test.want {
			t.Errorf("priority(%q,%q)=%d,want %d", test.callsign, test.frequency, got, test.want)
		}
	}
}
