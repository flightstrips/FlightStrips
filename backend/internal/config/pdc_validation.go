package config

// PDCValidationConfig holds validation rules for PDC requests.
type PDCValidationConfig struct {
	HeavyRunwayRestriction HeavyRunwayRestrictionConfig `yaml:"heavy_runway_restriction"`
	SIDRestrictions        []SIDRestrictionConfig       `yaml:"sid_restrictions"`
	EOBTWindowMin          int                          `yaml:"eobt_window_min"`
	EOBTWindowMax          int                          `yaml:"eobt_window_max"`
}

// HeavyRunwayRestrictionConfig defines aircraft types that require specific runways.
type HeavyRunwayRestrictionConfig struct {
	AircraftTypes  []string `yaml:"aircraft_types"`
	AllowedRunways []string `yaml:"allowed_runways"`
}

// SIDRestrictionConfig defines which engine types may use a SID via PDC.
type SIDRestrictionConfig struct {
	SID         string   `yaml:"sid"`
	EngineTypes []string `yaml:"engine_types"`
}

var pdcValidationConfig PDCValidationConfig

// GetPDCValidationConfig returns the PDC validation configuration.
func GetPDCValidationConfig() PDCValidationConfig {
	return pdcValidationConfig
}
