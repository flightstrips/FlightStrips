package config

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// This descriptor deliberately exists only in tests. It supplies no production
// handler, frontend, service clients or activation defaults.
type paperSettings struct {
	InitialBay      string            `yaml:"initial_bay"`
	RunwayBay       string            `yaml:"runway_bay"`
	BayGroundStates map[string]string `yaml:"bay_ground_states"`
	GroundStateBays map[string]string `yaml:"ground_state_bays"`
}

func testAirportCatalog(t *testing.T) *AirportCatalog {
	t.Helper()
	catalog, err := NewAirportCatalog(EKCHDescriptor(), HandlerDescriptor{ID: "example-paper/v1", View: "example-paper", Layouts: []string{"example-paper"}, DecodeSettings: TypedAirportSettings[paperSettings], Validate: func(d *AirportDocument) error {
		settings := d.Workflow.Settings.(paperSettings)
		if d.Ownership.Strips != "none" || d.Ownership.Sectors != "none" {
			return fmt.Errorf("ownership: paper requires none/none")
		}
		bays := map[string]bool{}
		for _, b := range d.Workflow.Bays {
			bays[b.ID] = true
		}
		if !bays[settings.InitialBay] || !bays[settings.RunwayBay] || settings.InitialBay == settings.RunwayBay {
			return fmt.Errorf("workflow.settings: invalid initial/runway bay")
		}
		for b := range settings.BayGroundStates {
			if !bays[b] {
				return fmt.Errorf("workflow.settings: unknown bay %q", b)
			}
		}
		for _, b := range settings.GroundStateBays {
			if !bays[b] {
				return fmt.Errorf("workflow.settings: unknown bay %q", b)
			}
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}
func TestAirportRegistryIsolation(t *testing.T) {
	registry, err := NewAirportRegistry(testAirportCatalog(t), []AirportInput{{"ekch.yaml", bytes.NewReader(airportExample(t, "ekch"))}, {"fictional.yaml", bytes.NewReader(airportExample(t, "zzzz"))}})
	if err != nil {
		t.Fatal(err)
	}
	ekch, _ := registry.Lookup("EKCH")
	paper, _ := registry.Lookup("ZZZZ")
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 20 {
				doc := ekch.Document()
				doc.Positions[0].Name = "CHANGED"
				doc.View.Layouts["EKCH_A_TWR"][0].Layout = "CHANGED"
				doc.Geography.InitialCFLFT["22L"] = 0
				other := paper.Document()
				s := other.Workflow.Settings.(paperSettings)
				s.BayGroundStates["HOLD"] = "PARK"
				other.Workflow.Bays[0].ID = "CHANGED"
			}
		})
	}
	wg.Wait()
	if ekch.Document().Positions[0].Name == "CHANGED" || paper.Document().Workflow.Settings.(paperSettings).BayGroundStates["HOLD"] != "TAXI" {
		t.Fatal("mutated immutable definition")
	}
	if ekch.Revision() == "" || ekch.Revision() == paper.Revision() {
		t.Fatal("missing definition revision")
	}
	if _, ok := registry.Lookup("EKBI"); ok {
		t.Fatal("unknown airport fell back")
	}
	names := registry.Airports()
	names[0] = "CHANGED"
	if registry.Airports()[0] != "EKCH" {
		t.Fatal("airport list aliases registry")
	}
}
func TestAirportRegistryRejectsUnsupportedReferences(t *testing.T) {
	if _, err := NewAirportRegistry(ProductionAirportCatalog(), []AirportInput{{"fictional.yaml", bytes.NewReader(airportExample(t, "zzzz"))}}); err == nil || !strings.Contains(err.Error(), "no compiled descriptor") {
		t.Fatalf("production accepted fictional descriptor: %v", err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"view":   func(c map[string]any) { c["view"].(map[string]any)["id"] = "other" },
		"layout": func(c map[string]any) { c["view"].(map[string]any)["airborne_fallback_layout"] = "other" },
		"bay": func(c map[string]any) {
			w := c["workflow"].(map[string]any)
			b := w["bays"].([]any)
			w["bays"] = b[:len(b)-1]
		},
		"airport":   func(c map[string]any) { c["airport"] = "EKBI" },
		"geography": func(c map[string]any) { c["geography"].(map[string]any)["latitude"] = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeAirport("selected.yaml", bytes.NewReader(mutateAirport(t, airportExample(t, "ekch"), mutate)))
			if err == nil {
				t.Fatal("accepted unsupported metadata")
			}
		})
	}
	if _, err := NewAirportRegistry(ProductionAirportCatalog(), nil); err == nil {
		t.Fatal("accepted empty selection")
	}
	data := airportExample(t, "ekch")
	if _, err := NewAirportRegistry(ProductionAirportCatalog(), []AirportInput{{"one", bytes.NewReader(data)}, {"two", bytes.NewReader(data)}}); err == nil {
		t.Fatal("accepted duplicate airport")
	}
}
