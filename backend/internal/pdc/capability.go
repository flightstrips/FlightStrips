package pdc

import (
	"FlightStrips/internal/config"
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrCapabilityDisabled = errors.New("PDC is disabled for this airport")

func (s *Service) airportEnabled(airport string) bool {
	if s.airports == nil {
		return true
	} // legacy isolated domain fixtures
	definition, ok := s.airports.Lookup(strings.ToUpper(strings.TrimSpace(airport)))
	return ok && definition.ServiceEnabled("pdc")
}

func (s *Service) requireCapability(ctx context.Context, session int32) error {
	if s.sessionRemoved(session) {
		return fmt.Errorf("PDC session has expired")
	}
	if s.airports == nil {
		return nil
	}
	_, err := s.getSessionInfo(ctx, session)
	return err
}

func (s *Service) requireSessionInfo(ctx context.Context, claimed sessionInformation, external bool) error {
	if s.sessionRemoved(claimed.id) {
		return fmt.Errorf("PDC session has expired")
	}
	if s.airports == nil {
		return nil
	}
	actual, err := s.getSessionInfo(ctx, claimed.id)
	if err != nil {
		return err
	}
	if claimed.airport != actual.airport || claimed.name != actual.name || claimed.callsign != actual.callsign {
		return fmt.Errorf("PDC session scope does not match persisted session")
	}
	if external && actual.name != "LIVE" {
		return fmt.Errorf("external PDC requires a LIVE session")
	}
	return nil
}

func (s *Service) validationForAirport(airport string) config.PDCValidationConfig {
	if s.airports != nil {
		if definition, ok := s.airports.Lookup(strings.ToUpper(strings.TrimSpace(airport))); ok {
			return definition.DomainConfig().PDCValidation
		}
		return config.PDCValidationConfig{}
	}
	return config.GetPDCValidationConfig()
}
