package server

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/vatsim"
	"context"
	"strings"
)

type resolvedHandover struct {
	Identifier     string
	Owner          string
	Display        *models.NextDisplay
	LogicalCarried bool
}

func (s *Server) ResolveClearedStripOwnerContext(ctx context.Context, strip *models.Strip, sessionID int32) (string, bool, error) {
	if strip == nil {
		return "", false, nil
	}

	session, err := routeSessionByID(ctx, s.sessionRepo, sessionID)
	if err != nil {
		return "", false, err
	}
	owners, err := routeSectorOwners(ctx, s.sectorRepo, sessionID)
	if err != nil {
		return "", false, err
	}
	radio, err := routeRadioStateForSession(ctx, s.controllerRepo, sessionID, s.frequencyProviders)
	if err != nil {
		return "", false, err
	}

	route, ok := config.ComputeDepartureRoute(
		session.ActiveRunways.GetAllActiveRunways(),
		sharedValue(strip.Stand),
		sharedValue(strip.Runway),
	)
	if !ok {
		return "", false, nil
	}
	resolution := resolveClearedRouteTarget(route.Path, strip, session, buildRouteOwnership(owners), radio)
	if resolution == nil {
		return "", false, nil
	}
	return resolution.Owner, true, nil
}

func resolveClearedRouteTarget(path []string, strip *models.Strip, session *models.Session, ownership routeOwnership, radio routeRadioState) *resolvedHandover {
	if len(path) >= 2 && strings.EqualFold(path[0], "SQ") && strings.EqualFold(path[1], "AD") {
		sequence := resolveOwnedHandoverTarget(path[0], strip, session, ownership, radio)
		apronDeparture := resolveOwnedHandoverTarget(path[1], strip, session, ownership, radio)
		if sequence != nil && apronDeparture != nil &&
			sequence.LogicalCarried && apronDeparture.LogicalCarried &&
			vatsim.NormalizeFrequency(sequence.Owner) == vatsim.NormalizeFrequency(apronDeparture.Owner) {
			sequence.Identifier = "AD"
			sequence.Display.Label = config.GetSectorDisplayName("AD")
			return sequence
		}
	}

	for _, identifier := range path {
		if resolution := resolveOwnedHandoverTarget(identifier, strip, session, ownership, radio); resolution != nil {
			return resolution
		}
	}
	return nil
}

func resolveOwnedHandoverTarget(identifier string, strip *models.Strip, session *models.Session, ownership routeOwnership, radio routeRadioState) *resolvedHandover {
	ownerSector := resolveConfiguredRouteSector(identifier, strip, session)
	owner, ok := resolveRouteSectorOwner(ownerSector, ownership.sectorToOwner, nil)
	if !ok {
		return nil
	}
	return resolveHandoverTargetForOwner(identifier, owner, strip, session, ownership, radio)
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
	isArrival := strip.Destination == session.Airport
	active := session.ActiveRunways.DepartureRunways
	if isArrival {
		active = session.ActiveRunways.ArrivalRunways
	}
	return config.GetSectorDisplayFrequency(active, identifier, isArrival)
}

func resolveConfiguredRouteSector(identifier string, strip *models.Strip, session *models.Session) string {
	isArrival := strip.Destination == session.Airport
	active := session.ActiveRunways.DepartureRunways
	if isArrival {
		active = session.ActiveRunways.ArrivalRunways
	}
	if resolved, ok := config.GetSectorIdentifier(active, identifier, isArrival); ok {
		return resolved
	}
	return identifier
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

func sharedValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
