package server

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/vatsim"
	"FlightStrips/pkg/helpers"
	"context"
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

func (s *Server) UpdateRouteForStrip(callsign string, sessionId int32, sendUpdate bool) error {
	return s.UpdateRouteForStripContext(context.Background(), callsign, sessionId, sendUpdate)
}

func (s *Server) UpdateRouteForStripContext(ctx context.Context, callsign string, sessionId int32, sendUpdate bool) error {
	stripRepo := s.stripRepo
	sessionRepo := s.sessionRepo

	slog.Debug("Route recalculation requested for strip",
		slog.Int("session", int(sessionId)),
		slog.String("callsign", callsign),
		slog.Bool("send_update", sendUpdate))

	strip, err := routeStripForCallsign(ctx, stripRepo, sessionId, callsign)
	if err != nil {
		return err
	}

	session, err := routeSessionByID(ctx, sessionRepo, sessionId)
	if err != nil {
		return err
	}

	owners, err := routeSectorOwners(ctx, s.sectorRepo, sessionId)
	if err != nil {
		return err
	}

	coverage, err := routeDisplayCoverage(ctx, s.controllerRepo, sessionId, s.transceiverLookup)
	if err != nil {
		return err
	}

	err = s.updateRouteForStripHelper(ctx, strip, session, owners, coverage, sendUpdate)
	if err == nil {
		slog.Debug("Route recalculation finished for strip",
			slog.Int("session", int(sessionId)),
			slog.String("callsign", callsign),
			slog.Bool("send_update", sendUpdate))
	}
	return err
}

func (s *Server) ComputeNextOwnersForStripContext(ctx context.Context, strip *models.Strip, sessionId int32) ([]string, bool, error) {
	session, err := routeSessionByID(ctx, s.sessionRepo, sessionId)
	if err != nil {
		return nil, false, err
	}

	owners, err := routeSectorOwners(ctx, s.sectorRepo, sessionId)
	if err != nil {
		return nil, false, err
	}

	result, shouldUpdate, err := computeRouteStateForStrip(strip, session, owners, nil)
	if err != nil {
		return nil, false, err
	}

	return result.NextOwners, shouldUpdate, nil
}

func (s *Server) ComputeNextDisplayForStripContext(ctx context.Context, strip *models.Strip, sessionId int32) (*models.NextDisplay, error) {
	session, err := routeSessionByID(ctx, s.sessionRepo, sessionId)
	if err != nil {
		return nil, err
	}

	owners, err := routeSectorOwners(ctx, s.sectorRepo, sessionId)
	if err != nil {
		return nil, err
	}

	coverage, err := routeDisplayCoverage(ctx, s.controllerRepo, sessionId, s.transceiverLookup)
	if err != nil {
		return nil, err
	}

	result, _, err := computeRouteStateForStrip(strip, session, owners, coverage)
	if err != nil {
		return nil, err
	}

	return cloneNextDisplay(result.NextDisplay), nil
}

func (s *Server) ComputeNextDisplaysForStripsContext(ctx context.Context, strips []*models.Strip, sessionId int32) error {
	if len(strips) == 0 {
		return nil
	}

	session, err := routeSessionByID(ctx, s.sessionRepo, sessionId)
	if err != nil {
		return err
	}

	owners, err := routeSectorOwners(ctx, s.sectorRepo, sessionId)
	if err != nil {
		return err
	}

	coverage, err := routeDisplayCoverage(ctx, s.controllerRepo, sessionId, s.transceiverLookup)
	if err != nil {
		return err
	}

	for _, strip := range strips {
		if strip == nil {
			continue
		}

		result, _, err := computeRouteStateForStrip(strip, session, owners, coverage)
		if err != nil {
			return err
		}

		strip.NextDisplay = cloneNextDisplay(result.NextDisplay)
	}

	return nil
}

// UpdateRoutesForSession recalculates routes for all strips in the session.
// sendUpdate controls whether frontend clients are notified of the updated route ownership.
func (s *Server) UpdateRoutesForSession(sessionId int32, sendUpdate bool) error {
	stripRepo := s.stripRepo
	sessionRepo := s.sessionRepo

	slog.Debug("Route recalculation requested for session",
		slog.Int("session", int(sessionId)),
		slog.Bool("send_update", sendUpdate))

	strips, err := stripRepo.List(context.Background(), sessionId)
	if err != nil {
		return err
	}

	session, err := sessionRepo.GetByID(context.Background(), sessionId)
	if err != nil {
		return err
	}

	owners, err := s.sectorRepo.ListBySession(context.Background(), sessionId)
	if err != nil {
		return err
	}

	coverage, err := routeDisplayCoverage(context.Background(), s.controllerRepo, sessionId, s.transceiverLookup)
	if err != nil {
		return err
	}

	slog.Debug("Route recalculation loaded strips for session",
		slog.Int("session", int(sessionId)),
		slog.Int("strip_count", len(strips)),
		slog.Bool("send_update", sendUpdate))

	for _, strip := range strips {
		err := s.updateRouteForStripHelper(context.Background(), strip, session, owners, coverage, sendUpdate)
		if err != nil {
			return err
		}
	}

	slog.Debug("Route recalculation completed for session",
		slog.Int("session", int(sessionId)),
		slog.Int("strip_count", len(strips)),
		slog.Bool("send_update", sendUpdate))

	return nil
}

func (s *Server) updateRouteForStripHelper(ctx context.Context, strip *models.Strip, session *models.Session, owners []*models.SectorOwner, coverage map[string]map[string]struct{}, sendUpdate bool) error {
	result, shouldUpdate, err := computeRouteStateForStrip(strip, session, owners, coverage)
	if err != nil {
		return err
	}
	if !shouldUpdate {
		return nil
	}

	nextOwnersChanged := !slices.Equal(strip.NextOwners, result.NextOwners)
	nextDisplayChanged := !nextDisplaysEqual(strip.NextDisplay, result.NextDisplay)
	if !nextOwnersChanged && !nextDisplayChanged {
		slog.Debug("Route recalculation produced no route-display change",
			slog.Int("session", int(session.ID)),
			slog.String("callsign", strip.Callsign),
			slog.Any("next_owners", result.NextOwners),
			slog.Any("next_display", result.NextDisplay))
		return nil
	}

	slog.Debug("Route recalculation updated route display",
		slog.Int("session", int(session.ID)),
		slog.String("callsign", strip.Callsign),
		slog.Any("previous_next_owners", strip.NextOwners),
		slog.Any("next_owners", result.NextOwners),
		slog.Any("previous_next_display", strip.NextDisplay),
		slog.Any("next_display", result.NextDisplay))

	if nextOwnersChanged {
		err = s.stripRepo.SetNextOwners(ctx, session.ID, strip.Callsign, result.NextOwners)
	} else {
		err = nil
	}
	if err == nil {
		strip.NextOwners = slices.Clone(result.NextOwners)
		strip.NextDisplay = cloneNextDisplay(result.NextDisplay)

		if nextOwnersChanged {
			shared.AddDBOperations(ctx, 1)
		}
		if syncState := shared.GetSyncState(ctx); syncState != nil && syncState.ExistingStrips != nil {
			if existing := syncState.ExistingStrips[strip.Callsign]; existing != nil {
				existing.NextOwners = slices.Clone(result.NextOwners)
				existing.NextDisplay = cloneNextDisplay(result.NextDisplay)
			}
		}
		if messageState := shared.GetWebsocketMessageState(ctx); messageState != nil && messageState.ExistingStrips != nil {
			if existing := messageState.ExistingStrips[strings.ToUpper(strings.TrimSpace(strip.Callsign))]; existing != nil {
				existing.NextOwners = slices.Clone(result.NextOwners)
				existing.NextDisplay = cloneNextDisplay(result.NextDisplay)
			}
		}
	}

	if sendUpdate {
		owner := ""
		if strip.Owner != nil {
			owner = *strip.Owner
		}
		s.frontendHub.SendOwnersUpdate(session.ID, strip.Callsign, owner, result.NextOwners, strip.PreviousOwners, cloneNextDisplay(result.NextDisplay))
	}

	return err
}

func computeNextOwnersForStrip(strip *models.Strip, session *models.Session, owners []*models.SectorOwner) ([]string, bool, error) {
	result, shouldUpdate, err := computeRouteStateForStrip(strip, session, owners, nil)
	if err != nil {
		return nil, false, err
	}

	return result.NextOwners, shouldUpdate, nil
}

func computeRouteStateForStrip(strip *models.Strip, session *models.Session, owners []*models.SectorOwner, coverage map[string]map[string]struct{}) (computedRouteState, bool, error) {
	isArrival := strip.Destination == session.Airport
	currentOwner := helpers.ValueOrDefault(strip.Owner)
	currentStand := helpers.ValueOrDefault(strip.Stand)
	currentRunway := helpers.ValueOrDefault(strip.Runway)

	slog.Debug("Recalculating strip route",
		slog.Int("session", int(session.ID)),
		slog.String("callsign", strip.Callsign),
		slog.Bool("is_arrival", isArrival),
		slog.String("owner", currentOwner),
		slog.String("stand", currentStand),
		slog.String("runway", currentRunway),
		slog.Any("current_next_owners", strip.NextOwners))

	// Departures require a runway to compute a route.
	if !isArrival && (strip.Runway == nil || *strip.Runway == "") {
		slog.Debug("Skipping route recalculation for departure without runway",
			slog.Int("session", int(session.ID)),
			slog.String("callsign", strip.Callsign))
		return computedRouteState{}, false, nil
	}

	var route config.ResolvedRoute

	if isArrival && (strip.Stand == nil || *strip.Stand == "") {
		// No stand yet: use the receiving tower sector so the strip always has
		// at least the tower controller as its next owner.
		towerSector, ok := config.GetArrivalTowerSector(session.ActiveRunways.ArrivalRunways)
		if !ok {
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
			if !isArrival {
				slog.Debug("Skipping departure route recalculation because aircraft position is outside supported regions",
					slog.Int("session", int(session.ID)),
					slog.String("callsign", strip.Callsign))
				return computedRouteState{}, false, nil
			}
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

			allRunways := session.ActiveRunways.GetAllActiveRunways()

			var success bool
			if isArrival {
				// Use only arrival runways to select the correct arrival route.
				// Mixing in departure runways can cause the wrong cargo route to match.
				route, success = config.ComputeToStand(session.ActiveRunways.ArrivalRunways, sector, helpers.ValueOrDefault(strip.Stand))
			} else {
				route, success = config.ComputeToRunway(allRunways, sector, helpers.ValueOrDefault(strip.Runway))
			}

			if !success {
				runway := helpers.ValueOrDefault(strip.Runway)
				stand := helpers.ValueOrDefault(strip.Stand)
				slog.Warn("Could not compute route for strip",
					slog.String("callsign", strip.Callsign),
					slog.String("sector", sector),
					slog.Bool("is_arrival", isArrival),
					slog.String("runway", runway),
					slog.String("stand", stand))

				if isArrival {
					// Fall back to the tower sector so arrivals always have at least
					// the receiving tower controller, even when the full route fails.
					if towerSector, ok := config.GetArrivalTowerSector(session.ActiveRunways.ArrivalRunways); ok {
						route = config.ResolvedRoute{Path: []string{towerSector}}
					} else {
						return computedRouteState{}, false, nil
					}
				} else {
					return computedRouteState{}, false, nil
				}
			}
		}
	}

	sectorToOnwer := make(map[string]string)
	for _, owner := range owners {
		for _, s := range owner.Sector {
			sectorToOnwer[normalizeRouteSectorRef(s)] = owner.Position
		}
	}

	actualRoute := make([]string, 0)
	var nextDisplay *models.NextDisplay
	nextDisplayResolved := false
	for _, s := range route.Path {
		owner, ok := resolveRouteSectorOwner(s, sectorToOnwer, route.OwnerOverrides)
		if !ok {
			continue
		}
		if !nextDisplayResolved && (currentOwner == "" || owner != currentOwner) {
			nextDisplay = buildRouteNextDisplay(session, s, owner, coverage[vatsim.NormalizeFrequency(owner)], isArrival)
			nextDisplayResolved = true
		}
		if len(actualRoute) == 0 || actualRoute[len(actualRoute)-1] != owner {
			actualRoute = append(actualRoute, owner)
		}
	}

	if !isArrival && strip.Sid != nil && *strip.Sid != "" {
		as, err := config.GetAirborneSector(*strip.Sid)
		if err != nil {
			slog.Debug("Error getting airborne frequency", slog.String("sid", *strip.Sid), slog.Any("error", err))
		} else if owner, ok := sectorToOnwer[normalizeRouteSectorRef(as)]; ok {
			if !nextDisplayResolved && (currentOwner == "" || owner != currentOwner) {
				nextDisplay = buildRouteNextDisplay(session, as, owner, coverage[vatsim.NormalizeFrequency(owner)], isArrival)
				nextDisplayResolved = true
			}
			if !slices.Contains(actualRoute, owner) {
				actualRoute = append(actualRoute, owner)
			}
		}
	}

	if strip.Owner != nil && *strip.Owner != "" {
		index := slices.Index(actualRoute, *strip.Owner)
		if index != -1 {
			// Trim everything up to and including the current owner.
			// The owner already holds the strip, so neither the owner nor any earlier
			// position in the route should appear in next_owners.
			actualRoute = actualRoute[index+1:]
		}
	}

	return computedRouteState{
		NextOwners:  actualRoute,
		NextDisplay: cloneNextDisplay(nextDisplay),
	}, true, nil
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

func buildRouteNextDisplay(session *models.Session, sectorRef string, owner string, coveredFrequencies map[string]struct{}, isArrival bool) *models.NextDisplay {
	active := session.ActiveRunways.GetAllActiveRunways()
	if isArrival {
		active = session.ActiveRunways.ArrivalRunways
	}

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

func routeDisplayCoverage(ctx context.Context, controllerRepo repository.ControllerRepository, sessionId int32, transceiverLookup TransceiverLookup) (map[string]map[string]struct{}, error) {
	if controllerRepo == nil {
		return map[string]map[string]struct{}{}, nil
	}

	controllerCoverage, err := getCurrentControllerCoverage(ctx, controllerRepo, sessionId, transceiverLookup)
	if err != nil {
		return nil, err
	}

	result := make(map[string]map[string]struct{}, len(controllerCoverage))
	for _, controller := range controllerCoverage {
		primaryFrequency := vatsim.NormalizeFrequency(controller.Frequency)
		if primaryFrequency == "" {
			continue
		}

		covered := make(map[string]struct{}, len(controller.CoveredFrequencies))
		for _, coveredFrequency := range controller.CoveredFrequencies {
			normalizedCoveredFrequency := vatsim.NormalizeFrequency(coveredFrequency)
			if normalizedCoveredFrequency == "" {
				continue
			}
			covered[normalizedCoveredFrequency] = struct{}{}
		}
		result[primaryFrequency] = covered
	}

	return result, nil
}

func cloneNextDisplay(nextDisplay *models.NextDisplay) *models.NextDisplay {
	if nextDisplay == nil {
		return nil
	}

	clone := *nextDisplay
	return &clone
}

func nextDisplaysEqual(left, right *models.NextDisplay) bool {
	if left == nil || right == nil {
		return left == right
	}

	return left.Label == right.Label && left.Frequency == right.Frequency
}

func routeStripForCallsign(ctx context.Context, stripRepo repository.StripRepository, sessionId int32, callsign string) (*models.Strip, error) {
	if syncState := shared.GetSyncState(ctx); syncState != nil && syncState.ExistingStrips != nil {
		if strip := syncState.ExistingStrips[callsign]; strip != nil {
			return strip, nil
		}
	}
	if messageState := shared.GetWebsocketMessageState(ctx); messageState != nil && messageState.ExistingStrips != nil {
		if strip := messageState.ExistingStrips[strings.ToUpper(strings.TrimSpace(callsign))]; strip != nil {
			return strip, nil
		}
	}
	strip, err := stripRepo.GetByCallsign(ctx, sessionId, callsign)
	if err == nil {
		shared.AddDBOperations(ctx, 1)
		if messageState := shared.GetWebsocketMessageState(ctx); messageState != nil {
			if messageState.ExistingStrips == nil {
				messageState.ExistingStrips = make(map[string]*models.Strip)
			}
			messageState.ExistingStrips[strings.ToUpper(strings.TrimSpace(callsign))] = strip
		}
	}
	return strip, err
}

func routeSessionByID(ctx context.Context, sessionRepo repository.SessionRepository, sessionId int32) (*models.Session, error) {
	if syncState := shared.GetSyncState(ctx); syncState != nil && syncState.Session != nil && syncState.Session.ID == sessionId {
		return syncState.Session, nil
	}
	if messageState := shared.GetWebsocketMessageState(ctx); messageState != nil && messageState.Session != nil && messageState.Session.ID == sessionId {
		return messageState.Session, nil
	}
	session, err := sessionRepo.GetByID(ctx, sessionId)
	if err == nil {
		shared.AddDBOperations(ctx, 1)
		if messageState := shared.GetWebsocketMessageState(ctx); messageState != nil {
			messageState.Session = session
		}
	}
	return session, err
}

func routeSectorOwners(ctx context.Context, sectorRepo repository.SectorOwnerRepository, sessionId int32) ([]*models.SectorOwner, error) {
	if syncState := shared.GetSyncState(ctx); syncState != nil && syncState.SectorOwners != nil {
		owners := make([]*models.SectorOwner, 0, len(syncState.SectorOwners))
		for _, owner := range syncState.SectorOwners {
			owners = append(owners, owner)
		}
		return owners, nil
	}
	if messageState := shared.GetWebsocketMessageState(ctx); messageState != nil && messageState.SectorOwners != nil {
		owners := make([]*models.SectorOwner, 0, len(messageState.SectorOwners))
		for _, owner := range messageState.SectorOwners {
			owners = append(owners, owner)
		}
		return owners, nil
	}

	owners, err := sectorRepo.ListBySession(ctx, sessionId)
	if err != nil {
		return nil, err
	}
	shared.AddDBOperations(ctx, 1)
	if syncState := shared.GetSyncState(ctx); syncState != nil {
		syncState.SectorOwners = make(map[string]*models.SectorOwner, len(owners))
		for _, owner := range owners {
			syncState.SectorOwners[owner.Position] = owner
		}
	}
	if messageState := shared.GetWebsocketMessageState(ctx); messageState != nil {
		messageState.SectorOwners = make(map[string]*models.SectorOwner, len(owners))
		for _, owner := range owners {
			messageState.SectorOwners[owner.Position] = owner
		}
	}
	return owners, nil
}
