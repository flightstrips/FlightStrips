package config

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"go.yaml.in/yaml/v4"
)

type Config struct {
	Routes         []Route                    `yaml:"routes"`
	AirborneRoutes []AirborneRoutes           `yaml:"airborne_routes"`
	Positions      []Position                 `yaml:"positions"`
	Sectors        []Sector                   `yaml:"sectors"`
	AirborneOwners []string                   `yaml:"airborne_owners"`
	Layouts        map[string][]LayoutVariant `yaml:"layouts"`
}

// TestModeConfig holds test/replay mode configuration
type TestModeConfig struct {
	Enabled       bool
	RecordMode    bool
	RecordingPath string
}

var testMode TestModeConfig

var sectors []Sector
var regions []Region
var positions []Position
var airborneRoutes []AirborneRoutes
var layouts map[string][]LayoutVariant

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
	airborneRoutes = cfg.AirborneRoutes
	layouts = cfg.Layouts

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
	// Initialize test mode configuration
	initTestMode()

	err := loadConfigurationFiles("ekch")
	if err != nil {
		panic(err)
	}
}

// initTestMode initializes test/replay mode configuration from environment variables
func initTestMode() {
	testModeEnv := os.Getenv("TEST_MODE")
	recordModeEnv := os.Getenv("RECORD_MODE")
	environment := os.Getenv("ENV")

	testMode.Enabled = testModeEnv == "true"
	testMode.RecordMode = recordModeEnv == "true"
	testMode.RecordingPath = os.Getenv("RECORDING_PATH")

	if testMode.RecordingPath == "" {
		testMode.RecordingPath = "recordings"
	}

	// Security safeguard: panic if test mode is enabled in production
	if testMode.Enabled && environment == "production" {
		panic("TEST_MODE cannot be enabled in production environment")
	}

	// Log warning if test mode is active
	if testMode.Enabled {
		slog.Warn("TEST_MODE is enabled - authentication will be bypassed")
	}

	if testMode.RecordMode {
		slog.Info("RECORD_MODE is enabled - WebSocket events will be recorded", slog.String("path", testMode.RecordingPath))
	}
}

// IsTestMode returns true if test/replay mode is enabled
func IsTestMode() bool {
	return testMode.Enabled
}

// IsRecordMode returns true if recording mode is enabled
func IsRecordMode() bool {
	return testMode.RecordMode
}

// GetRecordingPath returns the path where recordings are stored
func GetRecordingPath() string {
	return testMode.RecordingPath
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
