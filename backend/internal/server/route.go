package server

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/vatsim"
	"FlightStrips/pkg/helpers"
	"errors"
	"log/slog"
	"slices"
	"strings"
)

// This is dumb please optimize

type computedRouteState struct {
	NextOwners  []string
	NextDisplay *models.NextDisplay
}

type resolvedRouteStage struct {
	Identifier     string
	Owner          string
	Display        *models.NextDisplay
	LogicalCarried bool
}

type routeOwnership struct {
	sectorToOwner   map[string]string
	ownerIdentifier map[string]string
}

type routeRadioState struct {
	coverage      map[string]map[string]struct{}
	roleByPrimary map[string]string
}

// DepartureFrequency applies the existing EFB handover policy to current
// operational coverage, regardless of the persistence adapter supplying it.
func DepartureFrequency(priority []string, coverage []config.ControllerCoverage) *string {

	onlineFrequencies := make(map[string]struct{})
	for _, controller := range coverage {
		if frequency := vatsim.NormalizeFrequency(controller.Frequency); frequency != "" {
			onlineFrequencies[frequency] = struct{}{}
		}
		for _, frequency := range controller.CoveredFrequencies {
			if frequency = vatsim.NormalizeFrequency(frequency); frequency != "" {
				onlineFrequencies[frequency] = struct{}{}
			}
		}
	}

	for _, positionName := range priority {
		position, err := config.GetPositionByName(positionName)
		if err != nil {
			continue
		}
		frequency := strings.TrimSpace(position.Frequency)
		if !isEfbAutoHandoverFrequency(frequency) {
			continue
		}
		if _, online := onlineFrequencies[vatsim.NormalizeFrequency(frequency)]; online {
			return &frequency
		}
	}

	return nil
}

func isEfbAutoHandoverFrequency(frequency string) bool {
	switch vatsim.NormalizeFrequency(frequency) {
	case vatsim.NormalizeFrequency("124.980"), vatsim.NormalizeFrequency("120.255"):
		return true
	default:
		return false
	}
}

func computeRouteStateForStrip(strip *models.Strip, session *models.Session, owners []*models.SectorOwner, radio routeRadioState) (computedRouteState, bool, error) {
	isArrival, isLocal := stripDirectionForSession(strip, session)
	currentOwner := helpers.ValueOrDefault(strip.Owner)
	currentStand := helpers.ValueOrDefault(strip.Stand)
	currentRunway := helpers.ValueOrDefault(strip.Runway)
	// A strip can retain an assigned departure runway after the session has
	// switched runway configuration. Route selection must follow the aircraft's
	// runway in that case; the session's actual runway state remains available
	// to the separate runway/PDC validations.
	activeRunways := routeActiveRunwaysForStrip(strip, session, isArrival)

	slog.Debug("Recalculating strip route",
		slog.Int("session", int(session.ID)),
		slog.String("callsign", strip.Callsign),
		slog.Bool("is_arrival", isArrival),
		slog.String("owner", currentOwner),
		slog.String("stand", currentStand),
		slog.String("runway", currentRunway),
		slog.Any("current_next_owners", strip.NextOwners))

	// A shared VATSIM feed can contain traffic that neither departs from nor
	// arrives at this session's airport. It has no local ground route.
	if !isLocal {
		slog.Debug("Clearing route for traffic unrelated to session airport",
			slog.Int("session", int(session.ID)),
			slog.String("callsign", strip.Callsign),
			slog.String("origin", strip.Origin),
			slog.String("destination", strip.Destination))
		return computedRouteState{}, true, nil
	}

	// A completed departure can remain in the shared EuroScope feed after it has
	// left the airport, and its stand field may then refer to the destination
	// airport. It no longer has a local ground route. Apply this in the central
	// route computer so initial strip syncs and every later recalculation path use
	// the same guard as the bay/stand lifecycle.
	if !isArrival && departureIsOutsideRouteArea(strip) {
		slog.Debug("Clearing route for departure outside session airport",
			slog.Int("session", int(session.ID)),
			slog.String("callsign", strip.Callsign),
			slog.Float64("latitude", helpers.ValueOrDefault(strip.PositionLatitude)),
			slog.Float64("longitude", helpers.ValueOrDefault(strip.PositionLongitude)))
		return computedRouteState{}, true, nil
	}

	// Departures require a runway to compute a route.
	if !isArrival && (strip.Runway == nil || *strip.Runway == "") {
		slog.Debug("Skipping route recalculation for departure without runway",
			slog.Int("session", int(session.ID)),
			slog.String("callsign", strip.Callsign))
		return computedRouteState{}, true, nil
	}
	if !isArrival && strings.TrimSpace(currentStand) == "" {
		slog.Debug("Skipping route recalculation for departure without stand",
			slog.Int("session", int(session.ID)),
			slog.String("callsign", strip.Callsign))
		return computedRouteState{}, true, nil
	}

	var route config.ResolvedRoute

	if !isArrival {
		var success bool
		route, success = config.ComputeDepartureRoute(
			activeRunways,
			currentStand,
			currentRunway,
		)
		if !success {
			_, _, routeDiagnostics := config.ComputeDepartureRouteWithDiagnostics(
				activeRunways,
				currentStand,
				currentRunway,
			)
			slog.Warn("Could not compute complete departure route for strip",
				slog.Int("session", int(session.ID)),
				slog.String("session_name", session.Name),
				slog.String("airport", session.Airport),
				slog.String("callsign", strip.Callsign),
				slog.Int("strip_id", int(strip.ID)),
				slog.Int("strip_version", int(strip.Version)),
				slog.String("origin", strip.Origin),
				slog.String("destination", strip.Destination),
				slog.String("sid", helpers.ValueOrDefault(strip.Sid)),
				slog.String("owner", currentOwner),
				slog.String("runway", currentRunway),
				slog.String("stand", currentStand),
				slog.Any("active_runways", activeRunways),
				slog.Any("active_departure_runways", session.ActiveRunways.DepartureRunways),
				slog.Any("active_arrival_runways", session.ActiveRunways.ArrivalRunways),
				slog.Any("runway_status", session.ActiveRunways.RunwayStatus),
				slog.String("route_failure_reason", routeDiagnostics.FailureReason),
				slog.String("normalized_runway", routeDiagnostics.NormalizedRunway),
				slog.Int("candidate_route_count", routeDiagnostics.CandidateCount),
				slog.String("stand_parse_error", routeDiagnostics.StandParseError),
				slog.Any("route_candidates", routeDiagnostics.Candidates),
				slog.Any("previous_owners", strip.PreviousOwners),
				slog.Any("next_owners", strip.NextOwners))
			return computedRouteState{}, false, nil
		}
	} else if strip.Stand == nil || *strip.Stand == "" {
		// No stand yet: use the receiving tower sector so the strip always has
		// at least the tower controller as its next owner.
		towerSector, ok := config.GetArrivalTowerSector(session.ActiveRunways.ArrivalRunways)
		if !ok {
			if len(session.ActiveRunways.ArrivalRunways) == 0 {
				slog.Debug("Skipping arrival route recalculation until arrival runways are configured",
					slog.Int("session", int(session.ID)),
					slog.String("callsign", strip.Callsign))
				return computedRouteState{}, true, nil
			}
			slog.Warn("Skipping arrival route recalculation because no arrival tower sector is configured",
				slog.Int("session", int(session.ID)),
				slog.String("callsign", strip.Callsign))
			return computedRouteState{}, false, nil
		}
		slog.Debug("Arrival route recalculation is using tower fallback because stand is empty",
			slog.Int("session", int(session.ID)),
			slog.String("callsign", strip.Callsign))
		route.Path = []string{towerSector}
	} else {
		region, err := config.GetRegionForPosition(helpers.ValueOrDefault(strip.PositionLatitude), helpers.ValueOrDefault(strip.PositionLongitude))
		if errors.Is(err, config.ErrUnsupportedRegion) {
			// Arrival is still airborne (outside known ground regions) but already has
			// a stand assigned. Use the receiving tower sector as the start of the
			// stand route so the route can still continue onward to apron/ground.
			towerSector, ok := config.GetArrivalTowerSector(session.ActiveRunways.ArrivalRunways)
			if !ok {
				slog.Warn("Skipping arrival route recalculation because no arrival tower sector is configured for airborne fallback",
					slog.Int("session", int(session.ID)),
					slog.String("callsign", strip.Callsign),
					slog.String("stand", currentStand))
				return computedRouteState{}, false, nil
			}

			slog.Debug("Arrival route recalculation is using tower fallback as route start because aircraft position is outside supported regions",
				slog.Int("session", int(session.ID)),
				slog.String("callsign", strip.Callsign),
				slog.String("stand", currentStand),
				slog.String("tower_sector", towerSector))

			var success bool
			route, success = config.ComputeToStand(session.ActiveRunways.ArrivalRunways, towerSector, currentStand)
			if !success {
				slog.Warn("Arrival route recalculation could not build full route from tower fallback start; using tower-only fallback",
					slog.Int("session", int(session.ID)),
					slog.String("callsign", strip.Callsign),
					slog.String("stand", currentStand),
					slog.String("tower_sector", towerSector))
				route.Path = []string{towerSector}
			}
		} else if err != nil {
			return computedRouteState{}, false, err
		} else {
			sector, err := config.GetSectorFromRegion(region, isArrival)
			if err != nil {
				slog.Warn("Sector not found based on region", slog.String("callsign", strip.Callsign), slog.String("region", region.Name))
				return computedRouteState{}, false, nil
			}

			// Use only arrival runways to select the correct arrival route.
			// Mixing in departure runways can cause the wrong cargo route to match.
			var success bool
			route, success = config.ComputeToStand(session.ActiveRunways.ArrivalRunways, sector, helpers.ValueOrDefault(strip.Stand))

			if !success {
				runway := helpers.ValueOrDefault(strip.Runway)
				stand := helpers.ValueOrDefault(strip.Stand)
				slog.Warn("Could not compute route for strip",
					slog.String("callsign", strip.Callsign),
					slog.String("sector", sector),
					slog.Bool("is_arrival", isArrival),
					slog.String("runway", runway),
					slog.String("stand", stand))

				// Fall back to the tower sector so arrivals always have at least
				// the receiving tower controller, even when the full route fails.
				if towerSector, ok := config.GetArrivalTowerSector(session.ActiveRunways.ArrivalRunways); ok {
					route = config.ResolvedRoute{Path: []string{towerSector}}
				} else {
					return computedRouteState{}, false, nil
				}
			}
		}
	}

	ownership := buildRouteOwnership(owners)

	stages := make([]resolvedRouteStage, 0, len(route.Path)+1)
	for _, sector := range route.Path {
		stage, ok := resolveRouteStage(strip, session, sector, ownership, route.OwnerOverrides, radio, isArrival)
		if !ok {
			continue
		}
		if len(stages) > 0 && vatsim.NormalizeFrequency(stages[len(stages)-1].Owner) == vatsim.NormalizeFrequency(stage.Owner) {
			previous := &stages[len(stages)-1]
			if strings.EqualFold(previous.Identifier, "SQ") &&
				strings.EqualFold(stage.Identifier, "AD") &&
				previous.LogicalCarried &&
				stage.LogicalCarried &&
				previous.Display != nil {
				previous.Identifier = "AD"
				previous.Display.Label = config.GetSectorDisplayName("AD")
			}
			continue
		}
		stages = append(stages, stage)
	}

	if !isArrival && strip.Sid != nil && *strip.Sid != "" {
		as, err := config.GetAirborneSector(*strip.Sid)
		if err != nil {
			slog.Debug("Error getting airborne frequency", slog.String("sid", *strip.Sid), slog.Any("error", err))
		} else if owner, ok := ownership.sectorToOwner[normalizeRouteSectorRef(as)]; ok {
			if !slices.ContainsFunc(stages, func(stage resolvedRouteStage) bool {
				return vatsim.NormalizeFrequency(stage.Owner) == vatsim.NormalizeFrequency(owner)
			}) {
				stages = append(stages, resolvedRouteStage{
					Owner:   owner,
					Display: buildRouteNextDisplay(strip, session, as, owner, radio.coverage[vatsim.NormalizeFrequency(owner)], isArrival),
				})
			}
		}
	}

	if currentOwner != "" {
		index := resolveCurrentRouteStageIndex(stages, currentOwner, strip.PreviousOwners)
		if index != -1 {
			// Trim everything up to and including the current owner.
			// The owner already holds the strip, so neither the owner nor any earlier
			// position in the route should appear in next_owners.
			stages = stages[index+1:]
		}
	}

	actualRoute := make([]string, 0, len(stages))
	for _, stage := range stages {
		actualRoute = append(actualRoute, stage.Owner)
	}
	var nextDisplay *models.NextDisplay
	if len(stages) > 0 {
		nextDisplay = stages[0].Display
	}

	return computedRouteState{
		NextOwners:  actualRoute,
		NextDisplay: cloneNextDisplay(nextDisplay),
	}, true, nil
}

func departureIsOutsideRouteArea(strip *models.Strip) bool {
	if strip == nil || strip.PositionLatitude == nil || strip.PositionLongitude == nil ||
		(*strip.PositionLatitude == 0 && *strip.PositionLongitude == 0) {
		return false
	}

	airportLatitude, airportLongitude := config.GetAirportCoordinates()
	return shared.GetDistance(
		*strip.PositionLatitude,
		*strip.PositionLongitude,
		airportLatitude,
		airportLongitude,
	) > shared.RelevantDistance
}

func resolveRouteStage(
	strip *models.Strip,
	session *models.Session,
	sector string,
	ownership routeOwnership,
	ownerOverrides map[string]string,
	radio routeRadioState,
	isArrival bool,
) (resolvedRouteStage, bool) {
	normalizedSector := normalizeRouteSectorRef(sector)
	if normalizedSector == "" {
		return resolvedRouteStage{}, false
	}

	if overrideTarget, ok := ownerOverrides[normalizedSector]; ok {
		owner, ok := ownership.sectorToOwner[normalizeRouteSectorRef(overrideTarget)]
		if !ok {
			return resolvedRouteStage{}, false
		}
		return resolvedRouteStage{
			Identifier: sector,
			Owner:      owner,
			Display:    buildRouteNextDisplay(strip, session, sector, owner, radio.coverage[vatsim.NormalizeFrequency(owner)], isArrival),
		}, true
	}

	ownerSector := resolveConfiguredRouteSector(sector, strip, session)
	owner, ok := resolveRouteSectorOwner(ownerSector, ownership.sectorToOwner, nil)
	if !ok {
		return resolvedRouteStage{}, false
	}
	resolution := resolveHandoverTargetForOwner(sector, owner, strip, session, ownership, radio)
	return resolvedRouteStage{
		Identifier:     resolution.Identifier,
		Owner:          resolution.Owner,
		Display:        cloneNextDisplay(resolution.Display),
		LogicalCarried: resolution.LogicalCarried,
	}, true
}

func buildConfiguredOwnerDisplay(strip *models.Strip, session *models.Session, owner string, ownership routeOwnership, radio routeRadioState) *models.NextDisplay {
	normalizedOwner := vatsim.NormalizeFrequency(owner)
	identifier := ""
	if role := strings.TrimSpace(radio.roleByPrimary[normalizedOwner]); role != "" {
		isArrival, isLocal := stripDirectionForSession(strip, session)
		if !isLocal {
			return nil
		}
		active := routeActiveRunwaysForStrip(strip, session, isArrival)
		if resolved, ok := config.GetPositionLogicalIdentifier(active, role, isArrival); ok {
			identifier = resolved
		}
	}
	if identifier == "" {
		identifier = strings.TrimSpace(ownership.ownerIdentifier[normalizedOwner])
	}
	if identifier == "" {
		return nil
	}

	frequency, ok := resolveLogicalSectorFrequency(identifier, strip, session)
	if !ok {
		return nil
	}
	if !ownerCarriesFrequency(owner, frequency, radio.coverage) {
		return nil
	}
	return &models.NextDisplay{
		Label:     config.GetSectorDisplayName(identifier),
		Frequency: frequency,
	}
}

func stripDirectionForSession(strip *models.Strip, session *models.Session) (isArrival bool, isLocal bool) {
	if strip == nil || session == nil {
		return false, false
	}
	airport := strings.TrimSpace(session.Airport)
	if airport == "" {
		return false, false
	}
	if strings.EqualFold(strings.TrimSpace(strip.Destination), airport) {
		return true, true
	}
	if strings.EqualFold(strings.TrimSpace(strip.Origin), airport) {
		return false, true
	}
	return false, false
}

func resolveCurrentRouteStageIndex(stages []resolvedRouteStage, currentOwner string, previousOwners []string) int {
	normalizedCurrent := vatsim.NormalizeFrequency(currentOwner)
	if normalizedCurrent == "" {
		return -1
	}

	history := make([]string, 0, len(previousOwners)+1)
	for _, owner := range previousOwners {
		normalized := vatsim.NormalizeFrequency(owner)
		if normalized != "" {
			history = append(history, normalized)
		}
	}
	history = append(history, normalizedCurrent)

	bestIndex := -1
	bestMatched := -1
	for candidate, stage := range stages {
		if vatsim.NormalizeFrequency(stage.Owner) != normalizedCurrent {
			continue
		}

		matched := 1
		stageIndex := candidate - 1
		for historyIndex := len(history) - 2; historyIndex >= 0 && stageIndex >= 0; historyIndex-- {
			for stageIndex >= 0 && vatsim.NormalizeFrequency(stages[stageIndex].Owner) != history[historyIndex] {
				stageIndex--
			}
			if stageIndex >= 0 {
				matched++
				stageIndex--
			}
		}
		if matched > bestMatched {
			bestMatched = matched
			bestIndex = candidate
		}
	}
	return bestIndex
}

func resolveRouteSectorOwner(sector string, sectorToOwner map[string]string, ownerOverrides map[string]string) (string, bool) {
	normalizedSector := normalizeRouteSectorRef(sector)
	if normalizedSector == "" {
		return "", false
	}

	if overrideTarget, ok := ownerOverrides[normalizedSector]; ok {
		if owner, ok := sectorToOwner[normalizeRouteSectorRef(overrideTarget)]; ok {
			return owner, true
		}
	}

	owner, ok := sectorToOwner[normalizedSector]
	return owner, ok
}

func normalizeRouteSectorRef(sector string) string {
	return strings.ToUpper(strings.TrimSpace(sector))
}

func buildRouteNextDisplay(strip *models.Strip, session *models.Session, sectorRef string, owner string, coveredFrequencies map[string]struct{}, isArrival bool) *models.NextDisplay {
	active := routeActiveRunwaysForStrip(strip, session, isArrival)

	if frequency, ok := config.GetSectorDisplayFrequency(active, sectorRef, isArrival); ok {
		normalizedFrequency := vatsim.NormalizeFrequency(frequency)
		normalizedOwner := vatsim.NormalizeFrequency(owner)
		if normalizedFrequency != normalizedOwner {
			if coveredFrequencies == nil {
				return nil
			}
			if _, ok := coveredFrequencies[normalizedFrequency]; !ok {
				return nil
			}
		}

		return &models.NextDisplay{
			Label:     config.GetSectorDisplayName(sectorRef),
			Frequency: frequency,
		}
	}

	return nil
}

func cloneNextDisplay(nextDisplay *models.NextDisplay) *models.NextDisplay {
	if nextDisplay == nil {
		return nil
	}

	clone := *nextDisplay
	return &clone
}
