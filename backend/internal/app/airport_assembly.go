package app

import (
	"FlightStrips/internal/aman"
	appconfig "FlightStrips/internal/config"
	"FlightStrips/internal/gsx"
	"fmt"
)

func configureAirports(cfg Config) (Config, error) {
	if len(cfg.AirportFiles) == 0 {
		return cfg, nil
	} // Isolated application fixtures supply their own dependencies.
	registry, err := appconfig.LoadAirportFiles(appconfig.ProductionAirportCatalog(), cfg.AirportFiles, appconfig.AirportLoadOptions{
		RepositoryRoot:     cfg.AirportRepositoryRoot,
		DeploymentServices: map[string]bool{"cdm": cfg.EnableCDMConfigStore, "stand_assignment": cfg.EnableStandAssignment, "text_pilot": true, "aman": cfg.AMAN.Mode != aman.ModeDisabled, "pdc": cfg.EnablePDC, "ecfmp": cfg.EnableECFMP || cfg.EnableECFMPAPI},
		NavigationEnabled:  cfg.Navigation.Enabled() || cfg.EnableEFB, GSXEnabled: cfg.EnableGSXStandFeed,
	})
	if err != nil {
		return cfg, err
	}
	cfg.Airports = registry
	definition, ok := registry.Lookup("EKCH")
	if !ok {
		return cfg, fmt.Errorf("current production execution requires an explicitly selected EKCH definition; other handlers are delivered in MA-02")
	}
	cfg.EKCH = definition
	cfg.EnableCDMConfigStore = definition.ServiceEnabled("cdm")
	cfg.EnablePDC = definition.ServiceEnabled("pdc")
	cfg.EnableStandAssignment = definition.ServiceEnabled("stand_assignment")
	cfg.EnableECFMP = definition.ServiceEnabled("ecfmp") && cfg.EnableECFMP
	cfg.EnableECFMPAPI = definition.ServiceEnabled("ecfmp") && cfg.EnableECFMPAPI
	if !definition.ServiceEnabled("aman") {
		cfg.AMAN.Mode = aman.ModeDisabled
		cfg.AMAN.EnabledAirports = nil
	}
	for _, airport := range cfg.AMAN.EnabledAirports {
		selected, ok := registry.Lookup(airport)
		if !ok || !selected.ServiceEnabled("aman") {
			return cfg, fmt.Errorf("AMAN airport %s is unselected or disabled", airport)
		}
	}
	if path, ok := definition.Resource("services.cdm.config_root"); ok {
		cfg.CDMConfigDir = path
	}
	if path, ok := definition.Resource("services.stand_assignment.aircraft_icao"); ok {
		cfg.StandAssignmentAircraftJSON = path
	}
	if path, ok := definition.Resource("services.aman.terminal"); ok && cfg.Navigation.Enabled() {
		cfg.Navigation.TerminalGeometryPath = path
	}
	if err := appconfig.InitializeEKCHCompatibility(definition); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (a *App) Airports() *appconfig.AirportRegistry { return a.airports }

func configureStandAssignmentDefinition(cfg Config) appconfig.StandAssignmentReadiness {
	if cfg.EKCH == nil {
		return configureStandAssignment(cfg.EnableStandAssignment, cfg.StandAssignmentAircraftJSON)
	}
	resource := func(name string) string {
		path, _ := cfg.EKCH.Resource("services.stand_assignment." + name)
		return path
	}
	return appconfig.InitializeStandAssignmentFiles(cfg.EnableStandAssignment, appconfig.StandAssignmentFiles{
		AircraftReference: resource("aircraft_reference"), AircraftICAO: resource("aircraft_icao"), Stands: resource("stands"), AirlineAssignment: resource("airline_assignment"),
	})
}
func loadGSXSceneriesDefinition(cfg Config) (gsx.Sceneries, error) {
	if cfg.EKCH == nil {
		return loadGSXSceneries(cfg.EnableGSXStandFeed)
	}
	if !cfg.EnableGSXStandFeed {
		return nil, nil
	}
	path, ok := cfg.EKCH.Resource("services.stand_assignment.scenery")
	if !ok {
		return nil, fmt.Errorf("GSX scenery resource is unresolved")
	}
	scenery, err := gsx.LoadSceneryConfig(path)
	if err != nil {
		return nil, err
	}
	return gsx.Sceneries{cfg.EKCH.Airport(): scenery}, nil
}
