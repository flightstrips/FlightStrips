package config

import (
	"FlightStrips/internal/sat"
	"fmt"
	"path/filepath"
)

// StandAssignmentReadiness describes whether SAT was requested and whether its
// currently available configuration has loaded successfully.
type StandAssignmentReadiness struct {
	Enabled bool
	Ready   bool
	Reason  string
}

var (
	standAssignmentReadiness StandAssignmentReadiness
	aircraftReference        *sat.AircraftRegistry
	standCapabilities        *sat.StandCapabilityRegistry
	airlineAssignment        *sat.AirlineAssignmentConfig
	standAssignmentConfigDir = GetConfigDir
)

// InitializeStandAssignment loads SAT-only configuration when explicitly
// enabled. A failed load leaves the rest of FlightStrips usable and records the
// actionable reason that prevents SAT from becoming ready.
func InitializeStandAssignment(enabled bool) StandAssignmentReadiness {
	if !enabled {
		standAssignmentReadiness = StandAssignmentReadiness{}
		aircraftReference = nil
		standCapabilities = nil
		airlineAssignment = nil
		return standAssignmentReadiness
	}

	configDir := filepath.Join(standAssignmentConfigDir(), "ekch")
	registry, err := sat.LoadAircraftReferenceFile(filepath.Join(configDir, "GRpluginAircraftInfo.txt"))
	if err != nil {
		standAssignmentReadiness = StandAssignmentReadiness{
			Enabled: true,
			Reason:  fmt.Sprintf("load aircraft reference data: %v", err),
		}
		aircraftReference = nil
		standCapabilities = nil
		airlineAssignment = nil
		return standAssignmentReadiness
	}

	capabilities, err := sat.LoadStandCapabilityFile(filepath.Join(configDir, "GRpluginStands.txt"))
	if err != nil {
		standAssignmentReadiness = StandAssignmentReadiness{
			Enabled: true,
			Reason:  fmt.Sprintf("load stand capabilities: %v", err),
		}
		aircraftReference = nil
		standCapabilities = nil
		airlineAssignment = nil
		return standAssignmentReadiness
	}

	assignments, err := sat.LoadAirlineAssignmentFile(filepath.Join(configDir, "airline_assignment.json"), capabilities)
	if err != nil {
		standAssignmentReadiness = StandAssignmentReadiness{
			Enabled: true,
			Reason:  fmt.Sprintf("load airline assignment: %v", err),
		}
		aircraftReference = nil
		standCapabilities = nil
		airlineAssignment = nil
		return standAssignmentReadiness
	}

	aircraftReference = registry
	standCapabilities = capabilities
	airlineAssignment = assignments
	standAssignmentReadiness = StandAssignmentReadiness{Enabled: true, Ready: true}
	return standAssignmentReadiness
}

// GetStandAssignmentReadiness returns the latest SAT startup state.
func GetStandAssignmentReadiness() StandAssignmentReadiness {
	return standAssignmentReadiness
}

// GetAircraftReference returns the read-only SAT aircraft registry, or nil
// while SAT is disabled or unavailable.
func GetAircraftReference() *sat.AircraftRegistry {
	return aircraftReference
}

// GetStandCapabilities returns the read-only SAT stand capability registry, or
// nil while SAT is disabled or unavailable.
func GetStandCapabilities() *sat.StandCapabilityRegistry {
	return standCapabilities
}

// GetAirlineAssignment returns the validated SAT airline preference model, or
// nil while SAT is disabled or unavailable.
func GetAirlineAssignment() *sat.AirlineAssignmentConfig {
	return airlineAssignment
}
