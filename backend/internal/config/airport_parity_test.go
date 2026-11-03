package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

type airportParityCase struct {
	Name        string                   `json:"name"`
	Base        string                   `json:"base"`
	Operations  []airportParityOperation `json:"operations"`
	Environment map[string]string        `json:"environment"`
	Valid       bool                     `json:"valid"`
}

func TestProductionLoaderPreservesZeroStandBounds(t *testing.T) {
	for _, upper := range []int{0, 2} {
		data := mutateAirport(t, airportExample(t, "zzzz"), func(c map[string]any) {
			c["routing"].(map[string]any)["ground"] = []any{map[string]any{
				"id": "range", "name": "range", "path": []any{"TWR"},
				"forStandRanges": []any{map[string]any{"prefix": "A", "from": 0, "to": upper}},
			}}
		})
		doc, err := decodeAirport("zero-stand.yaml", bytes.NewReader(data), testAirportCatalog(t))
		if err != nil {
			t.Fatal(err)
		}
		stand := doc.Routing.Ground[0].ForStandRanges[0]
		if stand.From != 0 || stand.To != upper {
			t.Fatalf("stand bounds changed: %+v", stand)
		}
	}
}

type airportParityOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

// This corpus is generated and checked by the Python JSON Schema/semantic
// validator. Run it through the actual selected-file loader with typed test-only
// paper metadata, rather than the optional proof's YAML-node projection.
func TestProductionLoaderAuthoringParity(t *testing.T) {
	root := testAirportRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "specs", "multi-airport-support", "schema", "loader-parity-cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []airportParityCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 300 {
		t.Fatalf("incomplete structural/domain parity corpus: %d", len(cases))
	}
	catalog := testAirportCatalog(t)
	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			var rootNode yaml.Node
			if err := yaml.Unmarshal(airportExample(t, test.Base), &rootNode); err != nil {
				t.Fatal(err)
			}
			normalizeAirportYAMLScalars(&rootNode)
			var value any
			if err := rootNode.Decode(&value); err != nil {
				t.Fatal(err)
			}
			for _, operation := range test.Operations {
				parts := strings.Split(strings.TrimPrefix(operation.Path, "/"), "/")
				for i := range parts {
					parts[i] = strings.ReplaceAll(strings.ReplaceAll(parts[i], "~1", "/"), "~0", "~")
				}
				value, err = applyAirportParityOperation(value, parts, operation)
				if err != nil {
					t.Fatal(err)
				}
			}
			yamlData, err := yaml.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			registry, err := LoadAirportFiles(catalog, []string{selectedAirportFile(t, yamlData)}, AirportLoadOptions{
				RepositoryRoot: root, DeploymentServices: map[string]bool{"cdm": true},
				LookupEnv: func(key string) (string, bool) { value, ok := test.Environment[key]; return value, ok },
			})
			if test.Valid {
				if err != nil {
					t.Fatal(err)
				}
				if registry == nil {
					t.Fatal("missing registry")
				}
			} else if err == nil || registry != nil {
				t.Fatal("production accepted invalid authoring case")
			}
		})
	}
}

func applyAirportParityOperation(value any, path []string, operation airportParityOperation) (any, error) {
	if len(path) == 0 {
		return operation.Value, nil
	}
	key := path[0]
	switch target := value.(type) {
	case map[string]any:
		if len(path) == 1 {
			if operation.Op == "remove" {
				delete(target, key)
			} else {
				target[key] = operation.Value
			}
			return target, nil
		}
		child, err := applyAirportParityOperation(target[key], path[1:], operation)
		if err != nil {
			return nil, err
		}
		target[key] = child
		return target, nil
	case []any:
		if key == "-" && operation.Op == "add" && len(path) == 1 {
			return append(target, operation.Value), nil
		}
		index, err := strconv.Atoi(key)
		if err != nil || index < 0 || index >= len(target) {
			return nil, fmt.Errorf("invalid parity array path %s", operation.Path)
		}
		if len(path) == 1 {
			switch operation.Op {
			case "remove":
				return append(target[:index], target[index+1:]...), nil
			case "add":
				target = append(target, nil)
				copy(target[index+1:], target[index:])
				target[index] = operation.Value
			default:
				target[index] = operation.Value
			}
			return target, nil
		}
		child, err := applyAirportParityOperation(target[index], path[1:], operation)
		if err != nil {
			return nil, err
		}
		target[index] = child
		return target, nil
	default:
		return nil, fmt.Errorf("invalid parity path %s through %T", operation.Path, value)
	}
}

func TestSelectedAirportDefinitionsCoexistWithoutRebindingEKCH(t *testing.T) {
	latitude, longitude := GetAirportCoordinates()
	registry, err := LoadAirportFiles(testAirportCatalog(t), []string{
		selectedAirportFile(t, airportExample(t, "ekch")), selectedAirportFile(t, airportExample(t, "zzzz")),
	}, AirportLoadOptions{RepositoryRoot: testAirportRoot(t)})
	if err != nil {
		t.Fatal(err)
	}
	ekch, _ := registry.Lookup("EKCH")
	paper, _ := registry.Lookup("ZZZZ")
	doc := paper.Document()
	doc.Workflow.Settings.(paperSettings).BayGroundStates["HOLD"] = "PARK"
	doc.Workflow.Bays[0].ID = "CHANGED"
	if paper.Document().Workflow.Settings.(paperSettings).BayGroundStates["HOLD"] != "TAXI" || ekch.Document().Workflow.Bays[0].ID == "CHANGED" {
		t.Fatal("selected definitions alias one another")
	}
	afterLat, afterLon := GetAirportCoordinates()
	if latitude != afterLat || longitude != afterLon {
		t.Fatal("loading selected definitions rebound existing EKCH consumers")
	}
}

func TestSelectedRegionSnapshotAndRevision(t *testing.T) {
	root := testAirportRoot(t)
	regions, err := os.ReadFile(filepath.Join(root, "backend", "config", "ekch_regions.json"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "regions.json")
	if err := os.WriteFile(path, regions, 0600); err != nil {
		t.Fatal(err)
	}
	data := mutateAirport(t, airportExample(t, "ekch"), func(c map[string]any) {
		c["geography"].(map[string]any)["regions"] = map[string]any{"environment": "PARITY_REGIONS", "default_repo_path": "backend/config/ekch_regions.json"}
	})
	selected := []string{selectedAirportFile(t, data)}
	options := AirportLoadOptions{RepositoryRoot: root, LookupEnv: func(string) (string, bool) { return path, true }}
	first, err := LoadAirportFiles(ProductionAirportCatalog(), selected, options)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := first.Lookup("EKCH")
	revision := before.Revision()
	if err := os.WriteFile(path, append(regions, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := LoadAirportFiles(ProductionAirportCatalog(), selected, options)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := second.Lookup("EKCH")
	if after.Revision() == revision {
		t.Fatal("selected asset change did not affect definition revision")
	}
	if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if len(before.regionGroups[0]) == 0 || before.regionGroups[0][0].Region.NumVertices() == 0 || before.Revision() != revision {
		t.Fatal("resolved definition changed with its source asset")
	}
	if registry, err := LoadAirportFiles(ProductionAirportCatalog(), selected, options); err == nil || registry != nil {
		t.Fatal("accepted corrupted selected asset")
	}
}
