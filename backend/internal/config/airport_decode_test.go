package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

func airportExample(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", ".github", "specs", "multi-airport-support", "examples", name+".airport.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func mutateAirport(t *testing.T, data []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	var doc map[string]any
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	normalizeAirportYAMLScalars(&root)
	if err := root.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	mutate(doc)
	result, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func TestDecodeCompleteAirport(t *testing.T) {
	before := GetTransitionAltitude()
	doc, err := DecodeAirport("selected.yaml", bytes.NewReader(airportExample(t, "ekch")))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Airport != "EKCH" || len(doc.Workflow.Bays) != 18 || len(doc.Positions) != 25 || len(doc.Routing.Ground) != 33 {
		t.Fatalf("incomplete decoded airport: %+v", doc)
	}
	if _, ok := doc.Workflow.Settings.(EKCHSettings); !ok {
		t.Fatalf("settings have no concrete type: %T", doc.Workflow.Settings)
	}
	if GetTransitionAltitude() != before {
		t.Fatal("decoding mutated installed configuration")
	}
}
func TestAirportSchemaMatchesAuthoringContract(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", ".github", "specs", "multi-airport-support", "schema", "airport-configuration.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll(airportSchema, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("production and authoring schema differ")
	}
}

func TestDecodeAirportNormalizesNumericRunwaySelectors(t *testing.T) {
	data := mutateAirport(t, airportExample(t, "ekch"), func(c map[string]any) {
		geography := c["geography"].(map[string]any)
		geography["runways"] = append(geography["runways"].([]any), "04")
		c["routing"].(map[string]any)["ground"].([]any)[0].(map[string]any)["forRunway"] = 4
		c["coverage"].(map[string]any)["responsibilities"].([]any)[0].(map[string]any)["active"] = []any{4}
	})
	doc, err := DecodeAirport("numeric.yaml", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Routing.Ground[0].ForRunway != "04" || doc.Coverage.Responsibilities[0].Active[0] != "04" {
		t.Fatal("runway selectors were not normalized")
	}
}

func TestDecodeAirportRejectsUnknownAirborneResponsibility(t *testing.T) {
	data := mutateAirport(t, airportExample(t, "ekch"), func(c map[string]any) {
		c["routing"].(map[string]any)["airborne"].([]any)[0].(map[string]any)["name"] = "UNKNOWN_ROLE"
	})
	if _, err := DecodeAirport("airborne.yaml", bytes.NewReader(data)); err == nil || !strings.Contains(err.Error(), "routing.airborne.name") {
		t.Fatalf("unknown airborne responsibility accepted: %v", err)
	}
}
func TestDecodeAirportRejectsInvalidDocuments(t *testing.T) {
	cases := map[string]func(map[string]any){
		"missing required block": func(c map[string]any) { delete(c, "coverage") },
		"null":                   func(c map[string]any) { c["coverage"] = nil },
		"unknown root":           func(c map[string]any) { c["current_airport"] = "EKCH" },
		"missing false":          func(c map[string]any) { delete(c["master"].(map[string]any), "observers_eligible") },
		"unknown settings":       func(c map[string]any) { c["workflow"].(map[string]any)["settings"].(map[string]any)["invented"] = true },
		"missing settings": func(c map[string]any) {
			delete(c["workflow"].(map[string]any)["settings"].(map[string]any), "relevant_distance_nm")
		},
		"numeric frequency": func(c map[string]any) { c["positions"].([]any)[0].(map[string]any)["frequency"] = 119.905 },
		"unknown provider":  func(c map[string]any) { c["coverage"].(map[string]any)["airborne_fallback"] = []any{"ZZZZ_APP"} },
		"unknown route": func(c map[string]any) {
			c["routing"].(map[string]any)["ground"].([]any)[0].(map[string]any)["path"] = []any{"MISSING"}
		},
		"unknown runway": func(c map[string]any) {
			c["routing"].(map[string]any)["ground"].([]any)[0].(map[string]any)["forRunway"] = "36"
		},
		"duplicate position": func(c map[string]any) { p := c["positions"].([]any); c["positions"] = append(p, p[0]) },
		"disabled unknown setting": func(c map[string]any) {
			c["services"].(map[string]any)["pdc"] = map[string]any{"availability": "disabled", "logon": "secret"}
		},
		"missing service config": func(c map[string]any) { delete(c["services"].(map[string]any)["cdm"].(map[string]any), "config") },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			data := mutateAirport(t, airportExample(t, "ekch"), mutate)
			_, err := DecodeAirport("selected.yaml", bytes.NewReader(data))
			if err == nil {
				t.Fatal("accepted invalid airport")
			}
			if !strings.Contains(err.Error(), "selected.yaml airport EKCH") {
				t.Fatalf("missing context: %v", err)
			}
		})
	}
	for name, suffix := range map[string]string{"duplicate": "\nairport: EKCH\n", "multiple": "\n---\n{}\n"} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeAirport("selected.yaml", bytes.NewReader(append(airportExample(t, "ekch"), suffix...)))
			if err == nil {
				t.Fatal("accepted invalid YAML")
			}
		})
	}
}
