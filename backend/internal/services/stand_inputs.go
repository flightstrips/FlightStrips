package services

import (
	"FlightStrips/internal/sat"
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func WithStandAirportInputs(inputs *sat.AirportInputRegistry) StandAllocationOption {
	return func(s *StandAllocationService) { s.airportInputs = inputs }
}

func (s *StandAllocationService) policyForAirport(airport string) *sat.AirlineAssignmentConfig {
	if s != nil && s.airportInputs != nil {
		input, _ := s.airportInputs.Lookup(airport)
		return input.Policy
	}
	return s.policy
}

func (s *StandAllocationService) referencesForAirport(airport string, aircraft *sat.AircraftRegistry, engines *sat.AircraftEngineRegistry, borders *sat.AirportCountryRegistry) (*sat.AircraftRegistry, *sat.AircraftEngineRegistry, *sat.AirportCountryRegistry) {
	if s != nil && s.airportInputs != nil {
		input, _ := s.airportInputs.Lookup(airport)
		return input.Aircraft, input.Engines, input.Borders
	}
	return aircraft, engines, borders
}

func (s *StandAllocationService) validateAirport(ctx context.Context, session int32, airport string) error {
	if s.sessionRemoved(session) {
		return fmt.Errorf("stand session has expired")
	}
	if s.airportInputs == nil {
		return nil
	}
	if _, ok := s.airportInputs.Lookup(airport); !ok {
		return fmt.Errorf("stand assignment unavailable for airport %s", airport)
	}
	var actual string
	if err := s.pool.QueryRow(ctx, "SELECT airport FROM sessions WHERE id = $1", session).Scan(&actual); err != nil {
		return err
	}
	if !strings.EqualFold(actual, airport) {
		return fmt.Errorf("stand assignment airport does not match session")
	}
	return nil
}

func (s *StandAllocationService) validateLockedAirport(ctx context.Context, tx pgx.Tx, session int32, airport string) error {
	if s.sessionRemoved(session) {
		return fmt.Errorf("stand session has expired")
	}
	if s.airportInputs == nil {
		return nil
	}
	var actual string
	if err := tx.QueryRow(ctx, "SELECT airport FROM sessions WHERE id = $1", session).Scan(&actual); err != nil {
		return err
	}
	if !strings.EqualFold(actual, airport) {
		return fmt.Errorf("stand assignment airport does not match session")
	}
	return nil
}
