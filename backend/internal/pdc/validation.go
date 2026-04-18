package pdc

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"fmt"
	"strings"
	"time"
)

type FlightPlanValidationFaultKind string

const (
	FlightPlanValidationFaultKindSID    FlightPlanValidationFaultKind = "sid_invalid"
	FlightPlanValidationFaultKindRunway FlightPlanValidationFaultKind = "runway_invalid"
	FlightPlanValidationFaultKindEOBT   FlightPlanValidationFaultKind = "eobt_invalid"
)

type FlightPlanValidationFault struct {
	Kind    FlightPlanValidationFaultKind
	Message string
}

// PDCStripValidationFaults returns only the SID/runway-related PDC faults that should surface
// as strip validations for clearance-delivery positions.
func PDCStripValidationFaults(strip *models.Strip, activeDepartureRunways []string) []FlightPlanValidationFault {
	faults := validatePDCFlightPlanFaults(strip, activeDepartureRunways, time.Now().UTC())
	filtered := make([]FlightPlanValidationFault, 0, len(faults))
	for _, fault := range faults {
		if fault.Kind == FlightPlanValidationFaultKindSID || fault.Kind == FlightPlanValidationFaultKindRunway {
			filtered = append(filtered, fault)
		}
	}
	return filtered
}

func validationFaultMessages(faults []FlightPlanValidationFault) []string {
	messages := make([]string, 0, len(faults))
	for _, fault := range faults {
		messages = append(messages, fault.Message)
	}
	return messages
}

func validatePDCFlightPlanFaults(strip *models.Strip, activeDepartureRunways []string, now time.Time) []FlightPlanValidationFault {
	if strip == nil {
		return nil
	}

	cfg := config.GetPDCValidationConfig()
	now = now.UTC()
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

	aircraftType := ""
	if strip.AircraftType != nil {
		aircraftType = strings.ToUpper(strings.SplitN(strings.TrimSpace(*strip.AircraftType), "/", 2)[0])
	}

	runway := ""
	if strip.Runway != nil {
		runway = strings.ToUpper(strings.TrimSpace(*strip.Runway))
	}

	hasSpecificRunwayRequirement := false
	if strip.AircraftType != nil && strip.Runway != nil {
		for _, heavyType := range cfg.HeavyRunwayRestriction.AircraftTypes {
			if strings.ToUpper(strings.TrimSpace(heavyType)) == aircraftType {
				hasSpecificRunwayRequirement = true
				break
			}
		}

		if hasSpecificRunwayRequirement {
			allowed := false
			for _, r := range cfg.HeavyRunwayRestriction.AllowedRunways {
				if strings.ToUpper(strings.TrimSpace(r)) == runway {
					allowed = true
					break
				}
			}
			if !allowed {
				faults = append(faults, FlightPlanValidationFault{
					Kind:    FlightPlanValidationFaultKindRunway,
					Message: fmt.Sprintf("Aircraft type %s is not allowed on runway %s", *strip.AircraftType, *strip.Runway),
				})
			}
		}
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

	if strip.EffectiveEobt() != nil && *strip.EffectiveEobt() != "" {
		eobtStr := *strip.EffectiveEobt()
		if len(eobtStr) >= 4 {
			hourStr := eobtStr[:2]
			minStr := eobtStr[2:4]
			hour := 0
			min := 0
			fmt.Sscanf(hourStr, "%d", &hour)
			fmt.Sscanf(minStr, "%d", &min)

			eobtTime := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, time.UTC)
			if eobtTime.Before(now.Add(-12 * time.Hour)) {
				eobtTime = eobtTime.Add(24 * time.Hour)
			}

			windowMin := cfg.EOBTWindowMin
			if windowMin <= 0 {
				windowMin = 10
			}
			windowMax := cfg.EOBTWindowMax
			if windowMax <= 0 {
				windowMax = 30
			}

			earliest := now.Add(time.Duration(windowMin) * time.Minute)
			latest := now.Add(time.Duration(windowMax) * time.Minute)

			switch {
			case eobtTime.Before(earliest):
				faults = append(faults, FlightPlanValidationFault{
					Kind:    FlightPlanValidationFaultKindEOBT,
					Message: fmt.Sprintf("EOBT %s is too early (minimum %d minutes from now)", eobtStr, windowMin),
				})
			case eobtTime.After(latest):
				faults = append(faults, FlightPlanValidationFault{
					Kind:    FlightPlanValidationFaultKindEOBT,
					Message: fmt.Sprintf("EOBT %s is too late (maximum %d minutes from now)", eobtStr, windowMax),
				})
			}
		}
	}

	return faults
}
