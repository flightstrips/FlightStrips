package app

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/repository"
	"context"
	"fmt"
	"strings"
)

func airportCapabilityEnabled(cfg Config, service, airport string) bool {
	airport = strings.ToUpper(strings.TrimSpace(airport))
	if cfg.Airports != nil {
		definition, ok := cfg.Airports.Lookup(airport)
		return ok && definition.ServiceEnabled(service)
	}
	if cfg.EKCH != nil {
		return cfg.EKCH.Airport() == airport && cfg.EKCH.ServiceEnabled(service)
	}
	return true // explicit legacy assembly fixtures
}

func enforceAirportCapabilities(cfg Config) (Config, error) {
	if cfg.Airports == nil {
		return cfg, nil
	}
	cfg.EnablePDC = anyAirportService(cfg.Airports, "pdc")
	cfg.EnableCDMConfigStore = anyAirportService(cfg.Airports, "cdm")
	cfg.EnableStandAssignment = anyAirportService(cfg.Airports, "stand_assignment")
	cfg.EnableECFMP = cfg.EnableECFMP && anyAirportService(cfg.Airports, "ecfmp")
	cfg.EnableECFMPAPI = cfg.EnableECFMPAPI && anyAirportService(cfg.Airports, "ecfmp")
	if !anyAirportService(cfg.Airports, "aman") {
		cfg.AMAN.Mode = aman.ModeDisabled
		cfg.AMAN.EnabledAirports = nil
	}
	for _, airport := range cfg.AMAN.EnabledAirports {
		if !airportCapabilityEnabled(cfg, "aman", airport) {
			return cfg, fmt.Errorf("AMAN airport %s is unselected or disabled", airport)
		}
	}
	return cfg, nil
}

func amanAirportScope(cfg Config, sessions repository.SessionRepository) func(context.Context, string) error {
	return func(ctx context.Context, airport string) error {
		if !airportCapabilityEnabled(cfg, "aman", airport) {
			return &aman.DomainError{Class: aman.ErrorReadOnly, Message: "AMAN is disabled for this airport"}
		}
		enabled := false
		for _, selected := range cfg.AMAN.EnabledAirports {
			if strings.EqualFold(selected, airport) {
				enabled = true
				break
			}
		}
		if !enabled {
			return &aman.DomainError{Class: aman.ErrorReadOnly, Message: "AMAN airport is outside the enabled runtime"}
		}
		if session := aman.SessionID(ctx); session > 0 {
			actual, err := sessions.GetByID(ctx, session)
			if err != nil {
				return err
			}
			if actual == nil || !strings.EqualFold(actual.Airport, airport) {
				return &aman.DomainError{Class: aman.ErrorReadOnly, Message: "AMAN airport does not match session"}
			}
		}
		return nil
	}
}
