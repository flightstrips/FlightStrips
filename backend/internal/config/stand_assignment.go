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
	standAssignmentConfigDir = GetConfigDir
)

// InitializeStandAssignment loads SAT-only configuration when explicitly
// enabled. A failed load leaves the rest of FlightStrips usable and records the
// actionable reason that prevents SAT from becoming ready.
func InitializeStandAssignment(enabled bool) StandAssignmentReadiness {
	if !enabled {
		standAssignmentReadiness = StandAssignmentReadiness{}
		aircraftReference = nil
		return standAssignmentReadiness
	}

	registry, err := sat.LoadAircraftReferenceFile(filepath.Join(standAssignmentConfigDir(), "ekch", "GRpluginAircraftInfo.txt"))
	if err != nil {
		standAssignmentReadiness = StandAssignmentReadiness{
			Enabled: true,
			Reason:  fmt.Sprintf("load aircraft reference data: %v", err),
		}
		aircraftReference = nil
		return standAssignmentReadiness
	}

	aircraftReference = registry
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
