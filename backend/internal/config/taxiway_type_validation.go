package config

import "strings"

// TaxiwayTypeValidationConfig holds incompatible release-point rules by controller scope.
type TaxiwayTypeValidationConfig struct {
	Apron TaxiwayTypeValidationScopeConfig `yaml:"apron"`
	Tower TaxiwayTypeValidationScopeConfig `yaml:"tower"`
}

// TaxiwayTypeValidationScopeConfig groups category-wide and aircraft-specific restrictions.
type TaxiwayTypeValidationScopeConfig struct {
	Categories    map[string][]string `yaml:"categories"`
	AircraftTypes map[string][]string `yaml:"aircraft_types"`
}

var taxiwayTypeValidationConfig TaxiwayTypeValidationConfig

const (
	taxiwayTypeValidationScopeApron = "apron"
	taxiwayTypeValidationScopeTower = "tower"
)

// GetTaxiwayTypeValidationConfig returns a copy of the taxiway-type validation configuration.
func GetTaxiwayTypeValidationConfig() TaxiwayTypeValidationConfig {
	return TaxiwayTypeValidationConfig{
		Apron: cloneTaxiwayTypeValidationScopeConfig(taxiwayTypeValidationConfig.Apron),
		Tower: cloneTaxiwayTypeValidationScopeConfig(taxiwayTypeValidationConfig.Tower),
	}
}

// GetTaxiwayTypeValidationScopeForPosition resolves the configured taxiway validation
// scope for a position based on the layouts that position can operate.
func GetTaxiwayTypeValidationScopeForPosition(positionName string) (TaxiwayTypeValidationScopeConfig, bool) {
	scopeName, ok := getTaxiwayTypeValidationScopeName(positionName)
	if !ok {
		return TaxiwayTypeValidationScopeConfig{}, false
	}

	cfg := GetTaxiwayTypeValidationConfig()
	switch scopeName {
	case taxiwayTypeValidationScopeApron:
		return cfg.Apron, true
	case taxiwayTypeValidationScopeTower:
		return cfg.Tower, true
	default:
		return TaxiwayTypeValidationScopeConfig{}, false
	}
}

func cloneTaxiwayTypeValidationScopeConfig(scope TaxiwayTypeValidationScopeConfig) TaxiwayTypeValidationScopeConfig {
	return TaxiwayTypeValidationScopeConfig{
		Categories:    cloneTaxiwayTypeValidationMap(scope.Categories),
		AircraftTypes: cloneTaxiwayTypeValidationMap(scope.AircraftTypes),
	}
}

func cloneTaxiwayTypeValidationMap(source map[string][]string) map[string][]string {
	if len(source) == 0 {
		return map[string][]string{}
	}

	cloned := make(map[string][]string, len(source))
	for key, values := range source {
		copied := make([]string, len(values))
		copy(copied, values)
		cloned[key] = copied
	}

	return cloned
}

func normalizeTaxiwayTypeValidationConfig(cfg TaxiwayTypeValidationConfig) TaxiwayTypeValidationConfig {
	return TaxiwayTypeValidationConfig{
		Apron: TaxiwayTypeValidationScopeConfig{
			Categories:    normalizeTaxiwayTypeValidationMap(cfg.Apron.Categories),
			AircraftTypes: normalizeTaxiwayTypeValidationMap(cfg.Apron.AircraftTypes),
		},
		Tower: TaxiwayTypeValidationScopeConfig{
			Categories:    normalizeTaxiwayTypeValidationMap(cfg.Tower.Categories),
			AircraftTypes: normalizeTaxiwayTypeValidationMap(cfg.Tower.AircraftTypes),
		},
	}
}

func normalizeTaxiwayTypeValidationMap(source map[string][]string) map[string][]string {
	if len(source) == 0 {
		return map[string][]string{}
	}

	normalized := make(map[string][]string, len(source))
	for key, values := range source {
		normalizedKey := normalizeTaxiwayTypeValidationToken(key)
		if normalizedKey == "" {
			continue
		}

		seen := make(map[string]struct{}, len(values))
		normalizedValues := make([]string, 0, len(values))
		for _, value := range values {
			normalizedValue := normalizeTaxiwayTypeValidationToken(value)
			if normalizedValue == "" {
				continue
			}
			if _, exists := seen[normalizedValue]; exists {
				continue
			}
			seen[normalizedValue] = struct{}{}
			normalizedValues = append(normalizedValues, normalizedValue)
		}

		normalized[normalizedKey] = normalizedValues
	}

	return normalized
}

func normalizeTaxiwayTypeValidationToken(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func getTaxiwayTypeValidationScopeName(positionName string) (string, bool) {
	variants, ok := layouts[strings.TrimSpace(positionName)]
	if !ok {
		return "", false
	}

	hasApronLayout := false
	hasTowerLayout := false
	for _, variant := range variants {
		switch strings.ToUpper(strings.TrimSpace(variant.Layout)) {
		case "AA", "AD", "AAAD", "SEQPLN":
			hasApronLayout = true
		case "GEGW", "TWTE":
			hasTowerLayout = true
		}
	}

	switch {
	case hasApronLayout && !hasTowerLayout:
		return taxiwayTypeValidationScopeApron, true
	case hasTowerLayout && !hasApronLayout:
		return taxiwayTypeValidationScopeTower, true
	default:
		return "", false
	}
}
