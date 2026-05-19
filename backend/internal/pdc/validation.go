package pdc

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"fmt"
	"strings"
)

type FlightPlanValidationFaultKind string

const (
	FlightPlanValidationFaultKindSID    FlightPlanValidationFaultKind = "sid_invalid"
	FlightPlanValidationFaultKindRunway FlightPlanValidationFaultKind = "runway_invalid"
)

type FlightPlanValidationFault struct {
	Kind    FlightPlanValidationFaultKind
	Message string
}

// PDCStripValidationFaults returns the PDC request faults that should surface as strip
// validations. These should align with REQUESTED_WITH_FAULTS so controllers get the
// shared validation flow instead of separate strip-local highlighting.
func PDCStripValidationFaults(strip *models.Strip, activeDepartureRunways []string) []FlightPlanValidationFault {
	return validatePDCFlightPlanFaults(strip, activeDepartureRunways)
}

func validationFaultMessages(faults []FlightPlanValidationFault) []string {
	messages := make([]string, 0, len(faults))
	for _, fault := range faults {
		messages = append(messages, fault.Message)
	}
	return messages
}

func normalizedValidationAircraftType(aircraftType *string) string {
	if aircraftType == nil {
		return ""
	}

	return strings.ToUpper(strings.SplitN(strings.TrimSpace(*aircraftType), "/", 2)[0])
}

func normalizedValidationRunway(runway *string) string {
	if runway == nil {
		return ""
	}

	return strings.ToUpper(strings.TrimSpace(*runway))
}

// RunwayTypeValidationFault returns the configured aircraft/runway incompatibility fault, if any.
func RunwayTypeValidationFault(strip *models.Strip) *FlightPlanValidationFault {
	if strip == nil || strip.AircraftType == nil || strip.Runway == nil {
		return nil
	}

	cfg := config.GetPDCValidationConfig()
	aircraftType := normalizedValidationAircraftType(strip.AircraftType)
	runway := normalizedValidationRunway(strip.Runway)
	if aircraftType == "" || runway == "" {
		return nil
	}

	restriction := cfg.HeavyRunwayRestriction
	restrictedType := false
	for _, heavyType := range restriction.AircraftTypes {
		if strings.ToUpper(strings.TrimSpace(heavyType)) == aircraftType {
			restrictedType = true
			break
		}
	}
	if !restrictedType {
		return nil
	}

	for _, allowedRunway := range restriction.AllowedRunways {
		if strings.ToUpper(strings.TrimSpace(allowedRunway)) == runway {
			return nil
		}
	}

	return &FlightPlanValidationFault{
		Kind:    FlightPlanValidationFaultKindRunway,
		Message: fmt.Sprintf("Aircraft type %s is not allowed on runway %s", *strip.AircraftType, *strip.Runway),
	}
}

func validatePDCFlightPlanFaults(strip *models.Strip, activeDepartureRunways []string) []FlightPlanValidationFault {
	if strip == nil {
		return nil
	}

	cfg := config.GetPDCValidationConfig()
	var faults []FlightPlanValidationFault

	if strip.Sid != nil {
		sidUpper := strings.ToUpper(*strip.Sid)
		for _, restriction := range cfg.SIDRestrictions {
			if strings.ToUpper(restriction.SID) != sidUpper {
				continue
			}
			if len(restriction.EngineTypes) == 0 {
				faults = append(faults, FlightPlanValidationFault{
					Kind:    FlightPlanValidationFaultKindSID,
					Message: fmt.Sprintf("SID %s is not available via PDC", restriction.SID),
				})
			} else {
				engineType := strings.ToUpper(strip.EngineType)
				allowed := false
				for _, et := range restriction.EngineTypes {
					if strings.ToUpper(et) == engineType {
						allowed = true
						break
					}
				}
				if !allowed {
					faults = append(faults, FlightPlanValidationFault{
						Kind:    FlightPlanValidationFaultKindSID,
						Message: fmt.Sprintf("SID %s is not available for engine type %s", restriction.SID, strip.EngineType),
					})
				}
			}
			break
		}
	}

	aircraftType := normalizedValidationAircraftType(strip.AircraftType)
	runway := normalizedValidationRunway(strip.Runway)

	hasSpecificRunwayRequirement := false
	if aircraftType != "" {
		for _, heavyType := range cfg.HeavyRunwayRestriction.AircraftTypes {
			if strings.ToUpper(strings.TrimSpace(heavyType)) == aircraftType {
				hasSpecificRunwayRequirement = true
				break
			}
		}
	}

	if fault := RunwayTypeValidationFault(strip); fault != nil {
		faults = append(faults, *fault)
	}

	if runway != "" && len(activeDepartureRunways) > 0 && !hasSpecificRunwayRequirement {
		isActiveDepartureRunway := false
		for _, activeRunway := range activeDepartureRunways {
			if strings.EqualFold(strings.TrimSpace(activeRunway), runway) {
				isActiveDepartureRunway = true
				break
			}
		}
		if !isActiveDepartureRunway {
			faults = append(faults, FlightPlanValidationFault{
				Kind:    FlightPlanValidationFaultKindRunway,
				Message: fmt.Sprintf("Runway %s is not an active departure runway", *strip.Runway),
			})
		}
	}

	return faults
}
