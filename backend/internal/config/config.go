package config

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"go.yaml.in/yaml/v4"
)

type CdmDeicePlatformConfig struct {
	Name string `yaml:"name"`
	Time int    `yaml:"time"`
}

type CdmDeiceConfig struct {
	Light    int                      `yaml:"light"`
	Medium   int                      `yaml:"medium"`
	Heavy    int                      `yaml:"heavy"`
	Super    int                      `yaml:"super"`
	Platform []CdmDeicePlatformConfig `yaml:"platform"`
}

type CdmConfig struct {
	Rate           int            `yaml:"rate"`
	RateLvo        int            `yaml:"rateLvo"`
	RateUri        string         `yaml:"rateUri"`
	SidIntervalUri string         `yaml:"sidIntervalUri"`
	TaxizonesUri   string         `yaml:"taxizonesUri"`
	Deice          CdmDeiceConfig `yaml:"deice"`
}

type Config struct {
	Latitude            float64                    `yaml:"latitude"`
	Longitude           float64                    `yaml:"longitude"`
	Routes              []Route                    `yaml:"routes"`
	AirborneRoutes      []AirborneRoutes           `yaml:"airborne_routes"`
	Positions           []Position                 `yaml:"positions"`
	Sectors             []Sector                   `yaml:"sectors"`
	AirborneOwners      []string                   `yaml:"airborne_owners"`
	AirborneAltitudeAGL int64                      `yaml:"airborne_altitude_agl"`
	Layouts             map[string][]LayoutVariant `yaml:"layouts"`
	Runways             []string                   `yaml:"runways"`
	MessageAreas        map[string][]string        `yaml:"message_areas"`
	PDCValidation          PDCValidationConfig        `yaml:"pdc_validation"`
	MissedApproachHandover map[string]string          `yaml:"missed_approach_handover"`
	TransitionAltitude     int                        `yaml:"transition_altitude"`
	RunwayInitialCFL       map[string]int             `yaml:"runway_initial_cfl"`
	Cdm                    CdmConfig                  `yaml:"cdm"`
}

// TestModeConfig holds test/replay mode configuration
type TestModeConfig struct {
	Enabled       bool
	RecordMode    bool
	RecordingPath string
}

var testMode TestModeConfig

var cdmConfig CdmConfig

var sectors []Sector
var regions []Region
var positions []Position
var airborneOwners []string
var airborneRoutes []AirborneRoutes
var airborneAltitudeAGL int64
var layouts map[string][]LayoutVariant
var runways []string
var messageAreas map[string][]string
var airportLatitude float64
var airportLongitude float64

// runwayRoutes maps a runway to all available routes for that runway.
var runwayRoutes = map[string][]Route{}

// standRoutes lists all stand-based routes (ranges are matched at runtime).
var standRoutes []Route

// missedApproachHandover maps a landing runway to the approach controller position that should
// receive a missed approach handover for that runway.
var missedApproachHandover map[string]string
var transitionAltitude int
var runwayInitialCFL map[string]int

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
	if cfg.AirborneAltitudeAGL <= 0 {
		return fmt.Errorf("airborne_altitude_agl must be greater than 0")
	}

	positions = cfg.Positions
	sectors = cfg.Sectors
	airborneOwners = cfg.AirborneOwners
	airborneRoutes = cfg.AirborneRoutes
	airborneAltitudeAGL = cfg.AirborneAltitudeAGL
	layouts = cfg.Layouts
	runways = cfg.Runways
	airportLatitude = cfg.Latitude
	airportLongitude = cfg.Longitude
	messageAreas = cfg.MessageAreas
	if messageAreas == nil {
		messageAreas = make(map[string][]string)
	}
	pdcValidationConfig = cfg.PDCValidation
	missedApproachHandover = cfg.MissedApproachHandover
	if missedApproachHandover == nil {
		missedApproachHandover = make(map[string]string)
	}
	transitionAltitude = cfg.TransitionAltitude
	runwayInitialCFL = cfg.RunwayInitialCFL
	if runwayInitialCFL == nil {
		runwayInitialCFL = make(map[string]int)
	}
	cdmConfig = cfg.Cdm

	return nil
}

// GetMissedApproachHandoverPosition returns the approach controller position that should receive
// a missed approach handover for the given landing runway. Returns ("", false) if not configured.
func GetMissedApproachHandoverPosition(runway string) (string, bool) {
	pos, ok := missedApproachHandover[runway]
	return pos, ok && pos != ""
}

// GetAirportCoordinates returns the latitude and longitude of the configured airport.
func GetAirportCoordinates() (float64, float64) {
	return airportLatitude, airportLongitude
}

// GetMessageAreas returns the area→position mapping for the configured airport.
func GetMessageAreas() map[string][]string {
	return messageAreas
}

// GetRunways returns the list of valid runway identifiers for the configured airport.
func GetRunways() []string {
	return runways
}

func GetAirborneAltitudeAGL() int64 {
	return airborneAltitudeAGL
}

// GetTransitionAltitude returns the transition altitude (in feet) for the configured airport.
func GetTransitionAltitude() int {
	return transitionAltitude
}

// GetCdmConfig returns the CDM configuration parsed from the airport YAML.
func GetCdmConfig() CdmConfig {
	return cdmConfig
}

// GetConfigDir returns the base directory used for resolving relative CDM URIs.
func GetConfigDir() string {
	return "config"
}

// GetInitialCFLForRunway returns the initial cleared altitude (in feet) to auto-assign
// to departures entering the NOT_CLEARED bay for the given runway. Returns (0, false) if
// not configured for that runway.
func GetInitialCFLForRunway(runway string) (int, bool) {
	cfl, ok := runwayInitialCFL[runway]
	return cfl, ok
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
