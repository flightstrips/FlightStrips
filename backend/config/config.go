package config

import (
	"fmt"
	"io"
	"os"
	"strings"

	"go.yaml.in/yaml/v4"
)

type Config struct {
	Routes    []Route    `yaml:"routes"`
	Positions []Position `yaml:"positions"`
	Sectors   []Sector   `yaml:"sectors"`
}

var sectors []Sector
var regions []Region
var positions []Position

// runwayRoutes maps a runway to all available routes for that runway.
var runwayRoutes = map[string][]Route{}

// standRoutes lists all stand-based routes (ranges are matched at runtime).
var standRoutes []Route

func loadAirportConfig(r io.Reader) error {
	var cfg Config
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return fmt.Errorf("decode routes YAML: %w", err)
	}

	if err := loadRoutes(cfg); err != nil {
		return err
	}

	positions = cfg.Positions
	sectors = cfg.Sectors

	return nil
}

func loadRoutes(cfg Config) error {
	newRunway := make(map[string][]Route)
	var newStands []Route

	for i, rt := range cfg.Routes {
		runway := strings.TrimSpace(rt.ForRunway)
		hasRunway := runway != ""
		hasStands := len(rt.ForStandRanges) > 0

		if hasRunway == hasStands {
			return fmt.Errorf("route %d (%q): must specify exactly one of forRunway or forStandRanges", i, rt.Name)
		}
		if len(rt.Path) == 0 {
			return fmt.Errorf("route %d (%q): path must not be empty", i, rt.Name)
		}

		// Normalize required actives and path elements for consistency.
		for j := range rt.Active {
			rt.Active[j] = strings.TrimSpace(rt.Active[j])
		}
		for j := range rt.Path {
			rt.Path[j] = strings.TrimSpace(rt.Path[j])
		}

		if hasRunway {
			key := normalizeRunway(runway)
			rt.ForRunway = key
			newRunway[key] = append(newRunway[key], rt)
		} else {
			// normalize prefixes in stand ranges
			for j := range rt.ForStandRanges {
				rt.ForStandRanges[j].Prefix = strings.ToUpper(strings.TrimSpace(rt.ForStandRanges[j].Prefix))
			}
			newStands = append(newStands, rt)
		}
	}

	runwayRoutes = newRunway
	standRoutes = newStands
	return nil
}

func InitConfig() {
	err := loadConfigurationFiles("ekch")
	if err != nil {
		panic(err)
	}
}

func loadConfigurationFiles(airport string) error {
	// Load YAML config file
	if err := loadConfigFile(fmt.Sprintf("config/%s.yaml", airport), loadAirportConfig); err != nil {
		return err
	}

	// Load JSON regions file
	if err := loadConfigFile(fmt.Sprintf("config/%s_regions.json", airport), loadRegions); err != nil {
		return err
	}

	return nil
}

func loadConfigFile(path string, loader func(io.Reader) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil {
			panic(err)
		}
	}()

	return loader(f)
}
