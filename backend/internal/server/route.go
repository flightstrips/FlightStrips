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

	"github.com/jackc/pgx/v5"
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
	if s.coordRepo != nil {
		_, err := s.coordRepo.GetByStripID(ctx, sessionId, strip.ID)
		switch {
		case err == nil:
			return nil
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
	}

	session, err := routeSessionByID(ctx, sessionRepo, sessionId)
	if err != nil {
		return err
	}

	owners, err := routeSectorOwners(ctx, s.sectorRepo, sessionId)
	if err != nil {
		return err
	}

	radio, err := routeRadioStateForSession(ctx, s.controllerRepo, sessionId, s.frequencyProviders)
	if err != nil {
		return err
	}

	err = s.updateRouteForStripHelper(ctx, strip, session, owners, radio, sendUpdate)
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

	radio, err := routeRadioStateForSession(ctx, s.controllerRepo, sessionId, s.frequencyProviders)
	if err != nil {
		return nil, false, err
	}

	result, shouldUpdate, err := computeRouteStateForStrip(strip, session, owners, radio)
	if err != nil {
		return nil, false, err
	}

	return result.NextOwners, shouldUpdate, nil
}

func (s *Server) ComputeNextDisplayForStripContext(ctx context.Context, strip *models.Strip, sessionId int32) (*models.NextDisplay, error) {
	if strip == nil {
		return nil, nil
	}
	if s.coordRepo != nil {
		_, err := s.coordRepo.GetByStripID(ctx, sessionId, strip.ID)
		switch {
		case err == nil:
			return cloneNextDisplay(strip.NextDisplay), nil
		case !errors.Is(err, pgx.ErrNoRows):
			return nil, err
		}
	}

	session, err := routeSessionByID(ctx, s.sessionRepo, sessionId)
	if err != nil {
		return nil, err
	}

	owners, err := routeSectorOwners(ctx, s.sectorRepo, sessionId)
	if err != nil {
		return nil, err
	}

	radio, err := routeRadioStateForSession(ctx, s.controllerRepo, sessionId, s.frequencyProviders)
	if err != nil {
		return nil, err
	}

	result, _, err := computeRouteStateForStrip(strip, session, owners, radio)
	if err != nil {
		return nil, err
	}

	return cloneNextDisplay(result.NextDisplay), nil
}

func (s *Server) ComputeNextDisplaysForStripsContext(ctx context.Context, strips []*models.Strip, sessionId int32) error {
	if len(strips) == 0 {
		return nil
	}

	pendingStripIDs := make(map[int32]struct{})
	if s.coordRepo != nil {
		coordinations, err := s.coordRepo.ListBySession(ctx, sessionId)
		if err != nil {
			return err
		}
		for _, coordination := range coordinations {
			if coordination != nil {
				pendingStripIDs[coordination.StripID] = struct{}{}
			}
		}
	}

	hasUncoordinatedStrip := false
	for _, strip := range strips {
		if strip == nil {
			continue
		}
		if _, pending := pendingStripIDs[strip.ID]; !pending {
			hasUncoordinatedStrip = true
			break
		}
	}
	if !hasUncoordinatedStrip {
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

	radio, err := routeRadioStateForSession(ctx, s.controllerRepo, sessionId, s.frequencyProviders)
	if err != nil {
		return err
	}

	for _, strip := range strips {
		if strip == nil {
			continue
		}
		if _, pending := pendingStripIDs[strip.ID]; pending {
			continue
		}

		result, _, err := computeRouteStateForStrip(strip, session, owners, radio)
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
	unlock := s.sessionLocks.lock(sessionId)
	defer unlock()

	return s.updateRoutesForSessionContextUnlocked(context.Background(), sessionId, sendUpdate)
}

func (s *Server) updateRoutesForSessionContextUnlocked(ctx context.Context, sessionId int32, sendUpdate bool) error {
	stripRepo := s.stripRepo
	sessionRepo := s.sessionRepo

	slog.Debug("Route recalculation requested for session",
		slog.Int("session", int(sessionId)),
		slog.Bool("send_update", sendUpdate))

	strips, err := stripRepo.List(ctx, sessionId)
	if err != nil {
		return err
	}

	session, err := sessionRepo.GetByID(ctx, sessionId)
	if err != nil {
		return err
	}

	owners, err := s.sectorRepo.ListBySession(ctx, sessionId)
	if err != nil {
		return err
	}

	radio, err := routeRadioStateForSession(ctx, s.controllerRepo, sessionId, s.frequencyProviders)
	if err != nil {
		return err
	}

	pendingStripIDs := make(map[int32]struct{})
	if s.coordRepo != nil {
		coordinations, err := s.coordRepo.ListBySession(ctx, sessionId)
		if err != nil {
			return err
		}
		for _, coordination := range coordinations {
			if coordination != nil {
				pendingStripIDs[coordination.StripID] = struct{}{}
			}
		}
	}

	slog.Debug("Route recalculation loaded strips for session",
		slog.Int("session", int(sessionId)),
		slog.Int("strip_count", len(strips)),
		slog.Bool("send_update", sendUpdate))

	for _, strip := range strips {
		if _, pending := pendingStripIDs[strip.ID]; pending {
			continue
		}
		err := s.updateRouteForStripHelper(ctx, strip, session, owners, radio, sendUpdate)
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

func (s *Server) updateRouteForStripHelper(ctx context.Context, strip *models.Strip, session *models.Session, owners []*models.SectorOwner, radio routeRadioState, sendUpdate bool) error {
	result, shouldUpdate, err := computeRouteStateForStrip(strip, session, owners, radio)
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
	result, shouldUpdate, err := computeRouteStateForStrip(strip, session, owners, routeRadioState{})
	if err != nil {
		return nil, false, err
	}

	return result.NextOwners, shouldUpdate, nil
}

func computeRouteStateForStrip(strip *models.Strip, session *models.Session, owners []*models.SectorOwner, radio routeRadioState) (computedRouteState, bool, error) {
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

	if !isArrival {
		var success bool
		route, success = config.ComputeDepartureRoute(
			session.ActiveRunways.GetAllActiveRunways(),
			currentStand,
			currentRunway,
		)
		if !success {
			slog.Warn("Could not compute complete departure route for strip",
				slog.String("callsign", strip.Callsign),
				slog.String("runway", currentRunway),
				slog.String("stand", currentStand))
			return computedRouteState{}, false, nil
		}
	} else if strip.Stand == nil || *strip.Stand == "" {
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
					Display: buildRouteNextDisplay(session, as, owner, radio.coverage[vatsim.NormalizeFrequency(owner)], isArrival),
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
			Display:    buildRouteNextDisplay(session, sector, owner, radio.coverage[vatsim.NormalizeFrequency(owner)], isArrival),
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
		active := session.ActiveRunways.DepartureRunways
		isArrival := strip.Destination == session.Airport
		if isArrival {
			active = session.ActiveRunways.ArrivalRunways
		}
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

func routeRadioStateForSession(ctx context.Context, controllerRepo repository.ControllerRepository, sessionId int32, frequencyProviders []TransceiverLookup) (routeRadioState, error) {
	if controllerRepo == nil {
		return routeRadioState{
			coverage:      map[string]map[string]struct{}{},
			roleByPrimary: map[string]string{},
		}, nil
	}

	controllerCoverage, err := getCurrentControllerCoverage(ctx, controllerRepo, sessionId, frequencyProviders)
	if err != nil {
		return routeRadioState{}, err
	}

	result := routeRadioState{
		coverage:      make(map[string]map[string]struct{}, len(controllerCoverage)),
		roleByPrimary: make(map[string]string, len(controllerCoverage)),
	}
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
		result.coverage[primaryFrequency] = covered
		result.roleByPrimary[primaryFrequency] = controller.Name
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

type routeStripReader interface {
	GetByCallsign(ctx context.Context, session int32, callsign string) (*models.Strip, error)
}

func routeStripForCallsign(ctx context.Context, stripRepo routeStripReader, sessionId int32, callsign string) (*models.Strip, error) {
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
