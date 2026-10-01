package server

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/vatsim"
	"strings"
)

type resolvedHandover struct {
	Identifier     string
	Owner          string
	Display        *models.NextDisplay
	LogicalCarried bool
}

func resolveHandoverTargetForOwner(identifier string, owner string, strip *models.Strip, session *models.Session, ownership routeOwnership, radio routeRadioState) *resolvedHandover {
	frequency, ok := resolveLogicalSectorFrequency(identifier, strip, session)
	if ok && ownerCarriesFrequency(owner, frequency, radio.coverage) {
		return &resolvedHandover{
			Identifier:     strings.ToUpper(strings.TrimSpace(identifier)),
			Owner:          owner,
			LogicalCarried: true,
			Display: &models.NextDisplay{
				Label:     config.GetSectorDisplayName(identifier),
				Frequency: frequency,
			},
		}
	}

	return &resolvedHandover{
		Identifier: ownership.ownerIdentifier[vatsim.NormalizeFrequency(owner)],
		Owner:      owner,
		Display:    buildConfiguredOwnerDisplay(strip, session, owner, ownership, radio),
	}
}

func resolveLogicalSectorFrequency(identifier string, strip *models.Strip, session *models.Session) (string, bool) {
	isArrival, isLocal := stripDirectionForSession(strip, session)
	if !isLocal {
		return "", false
	}
	active := routeActiveRunwaysForStrip(strip, session, isArrival)
	return config.GetSectorDisplayFrequency(active, identifier, isArrival)
}

func resolveConfiguredRouteSector(identifier string, strip *models.Strip, session *models.Session) string {
	isArrival, isLocal := stripDirectionForSession(strip, session)
	if !isLocal {
		return identifier
	}
	active := routeActiveRunwaysForStrip(strip, session, isArrival)
	if resolved, ok := config.GetSectorIdentifier(active, identifier, isArrival); ok {
		return resolved
	}
	return identifier
}

// routeActiveRunwaysForStrip returns the runway configuration that should be
// used while resolving a strip's route. Departures are pathing-specific: when
// a strip's assigned runway differs from the session configuration, its own
// runway is the effective active runway for route selection and sector lookup.
// Arrival routing continues to use the session's active arrival runways.
func routeActiveRunwaysForStrip(strip *models.Strip, session *models.Session, isArrival bool) []string {
	if session == nil {
		return nil
	}
	if isArrival {
		return session.ActiveRunways.ArrivalRunways
	}
	if strip == nil || strip.Runway == nil || strings.TrimSpace(*strip.Runway) == "" {
		return session.ActiveRunways.DepartureRunways
	}
	return []string{strings.TrimSpace(*strip.Runway)}
}

func ownerCarriesFrequency(owner string, frequency string, coverage map[string]map[string]struct{}) bool {
	normalizedOwner := vatsim.NormalizeFrequency(owner)
	normalizedFrequency := vatsim.NormalizeFrequency(frequency)
	if normalizedOwner == "" || normalizedFrequency == "" {
		return false
	}
	if normalizedOwner == normalizedFrequency {
		return true
	}
	_, ok := coverage[normalizedOwner][normalizedFrequency]
	return ok
}

func buildRouteOwnership(owners []*models.SectorOwner) routeOwnership {
	ownership := routeOwnership{
		sectorToOwner:   make(map[string]string),
		ownerIdentifier: make(map[string]string),
	}
	for _, owner := range owners {
		if owner == nil {
			continue
		}
		normalizedOwner := vatsim.NormalizeFrequency(owner.Position)
		identifier := strings.TrimSpace(owner.Identifier)
		if identifier == "" && len(owner.Sector) > 0 {
			identifier = config.GetSectorDisplayName(owner.Sector[0])
		}
		if _, exists := ownership.ownerIdentifier[normalizedOwner]; !exists || strings.TrimSpace(ownership.ownerIdentifier[normalizedOwner]) == "" {
			ownership.ownerIdentifier[normalizedOwner] = identifier
		}
		for _, sector := range owner.Sector {
			ownership.sectorToOwner[normalizeRouteSectorRef(sector)] = owner.Position
		}
	}
	return ownership
}
