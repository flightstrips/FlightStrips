package pdc

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	pkgModels "FlightStrips/pkg/models"
	"fmt"
	"sort"
	"strings"
)

type FlightPlanValidationFaultKind string

const (
	FlightPlanValidationFaultKindSID            FlightPlanValidationFaultKind = "sid_invalid"
	FlightPlanValidationFaultKindRunway         FlightPlanValidationFaultKind = "runway_invalid"
	FlightPlanValidationFaultKindRouting        FlightPlanValidationFaultKind = "routing_missing"
	FlightPlanValidationFaultKindMandatoryRoute FlightPlanValidationFaultKind = "mandatory_route_review"
)

type FlightPlanValidationFault struct {
	Kind    FlightPlanValidationFaultKind
	Message string
}

// PDCStripValidationFaults returns the PDC request faults that should surface as strip
// validations. These should align with REQUESTED_WITH_FAULTS so controllers get the
// shared validation flow instead of separate strip-local highlighting.
func PDCStripValidationFaults(strip *models.Strip, activeDepartureRunways []string, availableSids pkgModels.AvailableSids) []FlightPlanValidationFault {
	return validatePDCFlightPlanFaults(strip, activeDepartureRunways, availableSids)
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

func validatePDCFlightPlanFaults(strip *models.Strip, activeDepartureRunways []string, availableSids pkgModels.AvailableSids) []FlightPlanValidationFault {
	if strip == nil {
		return nil
	}

	cfg := config.GetPDCValidationConfig()
	var faults []FlightPlanValidationFault

	var review *mandatoryRouteReview
	review = resolveMandatoryRouteReview(strip, availableSids)

	if fault := mandatoryRouteValidationFaultForReview(strip, review); fault != nil {
		faults = append(faults, *fault)
	}

	hasUsableSID := strip.Sid != nil && strings.TrimSpace(*strip.Sid) != ""
	if !hasUsableSID && review != nil && strings.TrimSpace(review.SID) != "" {
		hasUsableSID = true
	}

	hasUsableVectors := strip.Heading != nil && *strip.Heading != 0 &&
		strip.ClearedAltitude != nil && *strip.ClearedAltitude > 0
	if !hasUsableSID && !hasUsableVectors {
		faults = append(faults, FlightPlanValidationFault{
			Kind:    FlightPlanValidationFaultKindRouting,
			Message: "No SID or vectored departure assigned",
		})
	}

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

type mandatoryRouteReview struct {
	Route string
	SID   string
}

func mandatoryRouteValidationFaultForReview(strip *models.Strip, review *mandatoryRouteReview) *FlightPlanValidationFault {
	if review == nil {
		return nil
	}

	message := fmt.Sprintf("Mandatory route %s requires controller review before PDC can be issued.", review.Route)
	runway := normalizedValidationRunway(strip.Runway)
	switch {
	case review.SID != "":
		message = fmt.Sprintf("%s Use SID %s.", message, review.SID)
	case runway != "":
		message = fmt.Sprintf("%s No runway-matching SID was found for runway %s.", message, runway)
	default:
		message = fmt.Sprintf("%s No matching SID could be resolved from the live runway setup.", message)
	}

	return &FlightPlanValidationFault{
		Kind:    FlightPlanValidationFaultKindMandatoryRoute,
		Message: message,
	}
}

func resolveMandatoryRouteReview(strip *models.Strip, availableSids pkgModels.AvailableSids) *mandatoryRouteReview {
	if strip == nil || strip.CdmData == nil {
		return nil
	}

	routes := mandatoryRouteOptions(strip.CdmData.EcfmpRestrictions)
	if len(routes) == 0 {
		return nil
	}

	selectedRoute := selectMandatoryRoute(strip, routes)
	if selectedRoute == "" {
		selectedRoute = routes[0]
	}
	if selectedRoute == "" {
		return nil
	}

	waypoint := firstMandatoryRouteToken(selectedRoute)
	return &mandatoryRouteReview{
		Route: selectedRoute,
		SID:   resolveMandatoryRouteSID(strip, availableSids, waypoint),
	}
}

func mandatoryRouteOptions(restrictions []models.EcfmpRestriction) []string {
	for _, restriction := range restrictions {
		if restriction.Type != "mandatory_route" {
			continue
		}
		routes := make([]string, 0, len(restriction.Routes))
		for _, route := range restriction.Routes {
			trimmed := strings.ToUpper(strings.TrimSpace(route))
			if trimmed != "" {
				routes = append(routes, trimmed)
			}
		}
		return routes
	}
	return nil
}

func selectMandatoryRoute(strip *models.Strip, routes []string) string {
	if len(routes) == 0 {
		return ""
	}

	currentRoute := strings.ToUpper(strings.TrimSpace(stringValue(strip.Route)))
	if currentRoute != "" {
		for _, route := range routes {
			if strings.EqualFold(route, currentRoute) {
				return route
			}
		}
	}

	return routes[0]
}

func firstMandatoryRouteToken(route string) string {
	for _, token := range strings.FieldsFunc(strings.ToUpper(route), func(r rune) bool {
		return (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '/'
	}) {
		trimmed := strings.TrimSpace(token)
		if trimmed == "" || trimmed == "DCT" {
			continue
		}
		return trimmed
	}
	return ""
}

func resolveMandatoryRouteSID(strip *models.Strip, availableSids pkgModels.AvailableSids, firstWaypoint string) string {
	family := sidFamily(firstWaypoint)
	if family == "" {
		return ""
	}

	runway := normalizedValidationRunway(strip.Runway)
	currentSID := strings.ToUpper(strings.TrimSpace(stringValue(strip.Sid)))

	candidates := make([]string, 0, len(availableSids))
	for _, sid := range availableSids {
		if family != sidFamily(sid.Name) {
			continue
		}
		if runway != "" && !strings.EqualFold(strings.TrimSpace(sid.Runway), runway) {
			continue
		}
		name := strings.ToUpper(strings.TrimSpace(sid.Name))
		if name != "" {
			candidates = append(candidates, name)
		}
	}

	if len(candidates) == 0 {
		if family == sidFamily(currentSID) {
			return currentSID
		}
		return ""
	}

	variant := sidVariant(currentSID)
	if variant != "" {
		for _, candidate := range candidates {
			if sidVariant(candidate) == variant {
				return candidate
			}
		}
	}

	if currentSID != "" {
		for _, candidate := range candidates {
			if candidate == currentSID {
				return candidate
			}
		}
	}

	sort.Strings(candidates)
	return candidates[0]
}

func sidFamily(sid string) string {
	sid = strings.ToUpper(strings.TrimSpace(sid))
	if sid == "" {
		return ""
	}
	for i, r := range sid {
		if r >= '0' && r <= '9' {
			return sid[:i]
		}
	}
	return sid
}

func sidVariant(sid string) string {
	sid = strings.ToUpper(strings.TrimSpace(sid))
	if sid == "" {
		return ""
	}
	family := sidFamily(sid)
	if len(family) >= len(sid) {
		return ""
	}
	return sid[len(family):]
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
