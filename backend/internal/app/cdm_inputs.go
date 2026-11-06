package app

import (
	"FlightStrips/internal/cdm"
	appconfig "FlightStrips/internal/config"
	"path/filepath"
	"strings"
)

func configureCDMInputs(cfg Config) *cdm.CdmConfigStore {
	var inputs []cdm.AirportConfigInput
	appendInput := func(airport, root string, data appconfig.CdmConfig) {
		resolve := func(uri string) string {
			if uri == "" || strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
				return uri
			}
			return filepath.Join(root, uri)
		}
		deice := cdm.CdmDeiceConfig{Light: data.Deice.Light, Medium: data.Deice.Medium, Heavy: data.Deice.Heavy, Super: data.Deice.Super}
		for _, platform := range data.Deice.Platform {
			deice.Platform = append(deice.Platform, cdm.CdmDeicePlatformConfig{Name: platform.Name, Time: platform.Time})
		}
		inputs = append(inputs, cdm.AirportConfigInput{
			Config:  &cdm.CdmAirportConfig{Airport: airport, DefaultRate: data.Rate, DefaultRateLvo: data.RateLvo, DefaultTaxiMinutes: data.DefaultTaxiTime, DeiceConfig: deice},
			RateURI: resolve(data.RateUri), SIDIntervalURI: resolve(data.SidIntervalUri), TaxiZoneURI: resolve(data.TaxizonesUri),
		})
	}
	if cfg.Airports != nil {
		for _, airport := range cfg.Airports.Airports() {
			definition, _ := cfg.Airports.Lookup(airport)
			if !definition.ServiceEnabled("cdm") {
				continue
			}
			root, _ := definition.Resource("services.cdm.config_root")
			appendInput(airport, root, definition.DomainConfig().Cdm)
		}
	} else if cfg.EKCH != nil {
		if cfg.EKCH.ServiceEnabled("cdm") {
			root, _ := cfg.EKCH.Resource("services.cdm.config_root")
			appendInput(cfg.EKCH.Airport(), root, cfg.EKCH.DomainConfig().Cdm)
		}
	} else {
		// Existing isolated assembly fixtures explicitly use the legacy EKCH input.
		appendInput("EKCH", cfg.CDMConfigDir, appconfig.GetCdmConfig())
	}
	return cdm.NewAirportConfigStore(inputs, cfg.CDMConfigRefreshInterval, nil)
}
