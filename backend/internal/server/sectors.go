package server

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/vatsim"
	"cmp"
	"context"
	"log/slog"
	"slices"
)

func (s *Server) UpdateSectors(sessionId int32) ([]shared.SectorChange, error) {
	return s.UpdateSectorsContext(context.Background(), sessionId)
}

func (s *Server) UpdateSectorsContext(ctx context.Context, sessionId int32) ([]shared.SectorChange, error) {
	unlock := s.sessionLocks.lock(sessionId)
	defer unlock()

	return s.updateSectorsContextUnlocked(ctx, sessionId)
}

func (s *Server) updateSectorsContextUnlocked(ctx context.Context, sessionId int32) ([]shared.SectorChange, error) {
	sessionRepo := s.sessionRepo
	sectorRepo := s.sectorRepo
	controllerRepo := s.controllerRepo

	previousOwners, err := sectorRepo.ListBySession(ctx, sessionId)
	if err != nil {
		return nil, err
	}

	slog.Debug("Updating sectors", slog.Int("session", int(sessionId)), slog.Int("previousOwners", len(previousOwners)))

	session, err := getSessionForUpdate(ctx, sessionRepo, sessionId)
	if err != nil {
		return nil, err
	}

	// If the runways are not set, we cannot calculate the sector ownerships
	if len(session.ActiveRunways.ArrivalRunways) == 0 || len(session.ActiveRunways.DepartureRunways) == 0 {
		slog.Debug("No active runways found for sector update", slog.Int("session", int(sessionId)))
		return nil, nil
	}

	coverage, err := getCurrentControllerCoverage(ctx, controllerRepo, sessionId, s.frequencyProviders)
	if err != nil {
		return nil, err
	}

	active := session.ActiveRunways.GetAllActiveRunways()
	sectors := config.GetControllerSectorsWithCoverage(coverage, active)
	if len(sectors) == 0 {
		return nil, nil
	}

	currentOwners := make([]*models.SectorOwner, 0)
	for key, sectors := range sectors {
		names := make([]string, 0)
		sorted := slices.SortedFunc(slices.Values(sectors), sectorsCompare)
		identifier := ""
		priority := 0
		for _, sector := range sorted {
			names = append(names, sector.KeyOrName())
			if sector.NamePriority > priority {
				priority = sector.NamePriority
				identifier = sector.Name
			}
		}
		currentOwners = append(currentOwners, &models.SectorOwner{
			Session:    sessionId,
			Position:   key,
			Sector:     names,
			Identifier: identifier,
		})
	}

	slog.Debug("Found sectors for session", slog.Int("session", int(sessionId)), slog.Int("sectors", len(currentOwners)))

	changes := computeSectorChanges(previousOwners, currentOwners)

	if !slices.EqualFunc(currentOwners, previousOwners, sectorsEqual) {
		tx, err := s.GetDatabasePool().Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(ctx)

		txSectorRepo := sectorRepo.WithTx(tx)

		err = txSectorRepo.RemoveBySession(ctx, sessionId)
		if err != nil {
			return nil, err
		}

		err = txSectorRepo.CreateBulk(ctx, currentOwners)
		if err != nil {
			return nil, err
		}

		err = tx.Commit(ctx)
		if err != nil {
			return nil, err
		}

		if err := s.sendControllerUpdates(sessionId, currentOwners, controllerRepo); err != nil {
			return nil, err
		}
	}

	return changes, nil
}

func (s *Server) RefreshAllSectors(ctx context.Context) error {
	return refreshSessionSectors(ctx, s.sessionRepo, func(sessionID int32) error {
		if _, err := s.UpdateSectorsContext(ctx, sessionID); err != nil {
			return err
		}
		return s.UpdateRoutesForSession(sessionID, true)
	})
}

// computeSectorChanges computes which sectors changed owning position between two snapshots.
func computeSectorChanges(previous, current []*models.SectorOwner) []shared.SectorChange {
	prev := make(map[string]string)
	for _, o := range previous {
		for _, sector := range o.Sector {
			prev[sector] = o.Position
		}
	}
	curr := make(map[string]string)
	for _, o := range current {
		for _, sector := range o.Sector {
			curr[sector] = o.Position
		}
	}

	allSectors := make(map[string]struct{})
	for s := range prev {
		allSectors[s] = struct{}{}
	}
	for s := range curr {
		allSectors[s] = struct{}{}
	}

	var changes []shared.SectorChange
	for sector := range allSectors {
		fromFreq := prev[sector]
		toFreq := curr[sector]
		if fromFreq == toFreq {
			continue
		}
		changes = append(changes, shared.SectorChange{
			SectorName:   config.GetSectorDisplayName(sector),
			FromPosition: freqToPositionName(fromFreq),
			ToPosition:   freqToPositionName(toFreq),
		})
	}
	return changes
}

// freqToPositionName converts a position frequency string to its human-readable
// config Name. Falls back to the frequency string itself if not found in config.
func freqToPositionName(freq string) string {
	if freq == "" {
		return ""
	}
	if pos, err := config.GetPositionBasedOnFrequency(freq); err == nil {
		return pos.Name
	}
	return freq
}

func sectorsEqual(a, b *models.SectorOwner) bool {
	return a.Session == b.Session && a.Position == b.Position && slices.Equal(a.Sector, b.Sector)
}

func sectorsCompare(e, e2 config.Sector) int {
	if c := cmp.Compare(e.Name, e2.Name); c != 0 {
		return c
	}
	return cmp.Compare(e.KeyOrName(), e2.KeyOrName())
}

func getSessionForUpdate(ctx context.Context, sessionRepo repository.SessionRepository, sessionId int32) (*models.Session, error) {
	if syncState := shared.GetSyncState(ctx); syncState != nil && syncState.Session != nil && syncState.Session.ID == sessionId {
		return syncState.Session, nil
	}
	return sessionRepo.GetByID(ctx, sessionId)
}

func getControllersForUpdate(ctx context.Context, controllerRepo repository.ControllerRepository, sessionId int32) ([]*models.Controller, error) {
	if syncState := shared.GetSyncState(ctx); syncState != nil && syncState.ExistingControllers != nil {
		controllers := make([]*models.Controller, 0, len(syncState.ExistingControllers))
		for _, controller := range syncState.ExistingControllers {
			controllers = append(controllers, controller)
		}
		return controllers, nil
	}
	return controllerRepo.List(ctx, sessionId)
}

func getCurrentControllerCoverage(ctx context.Context, controllerRepo repository.ControllerRepository, sessionId int32, frequencyProviders []TransceiverLookup) ([]config.ControllerCoverage, error) {
	controllers, err := getControllersForUpdate(ctx, controllerRepo, sessionId)
	if err != nil {
		return nil, err
	}

	coverage := make([]config.ControllerCoverage, 0)
	for _, controller := range controllers {
		position, ok := resolveOperationalPosition(controller)
		if !ok || !shared.IsOperationalControllerForPosition(controller, position) {
			continue
		}

		controllerCoverage := config.ControllerCoverage{
			Name:      position.Name,
			Frequency: controllerPrimaryFrequency(controller, position),
		}
		for _, provider := range frequencyProviders {
			controllerCoverage.CoveredFrequencies = append(controllerCoverage.CoveredFrequencies, provider.GetFrequencies(controller.Callsign)...)
		}
		coverage = append(coverage, controllerCoverage)
	}

	return coverage, nil
}

func getCurrentPositions(ctx context.Context, controllerRepo repository.ControllerRepository, sessionId int32) ([]*config.Position, error) {
	controllers, err := getControllersForUpdate(ctx, controllerRepo, sessionId)
	if err != nil {
		return nil, err
	}

	positions := make([]*config.Position, 0)
	for _, controller := range controllers {
		position, ok := resolveOperationalPosition(controller)
		if !ok || !shared.IsOperationalControllerForPosition(controller, position) {
			continue
		}
		actualPosition := *position
		actualPosition.Frequency = controllerPrimaryFrequency(controller, position)
		positions = append(positions, &actualPosition)
	}

	return positions, nil
}

func controllerPrimaryFrequency(controller *models.Controller, role *config.Position) string {
	if controller != nil {
		if frequency := vatsim.NormalizeFrequency(controller.Position); frequency != "" {
			return frequency
		}
	}
	if role == nil {
		return ""
	}
	return vatsim.NormalizeFrequency(role.Frequency)
}

func refreshSessionSectors(ctx context.Context, sessionRepo repository.SessionRepository, update func(sessionID int32) error) error {
	sessions, err := sessionRepo.List(ctx)
	if err != nil {
		return err
	}

	var firstErr error
	for _, session := range sessions {
		if err := update(session.ID); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			slog.Error("Failed to refresh sectors for session", slog.Int("session", int(session.ID)), slog.Any("error", err))
		}
	}

	return firstErr
}

func (s *Server) sendControllerUpdates(sessionId int32, owners []*models.SectorOwner, controllerRepo repository.ControllerRepository) error {
	controllers, err := controllerRepo.List(context.Background(), sessionId)
	if err != nil {
		return err
	}

	ownerMap := make(map[string]*models.SectorOwner)
	for _, owner := range owners {
		ownerMap[owner.Position] = owner
	}

	for _, controller := range controllers {
		identifier := ""
		ownedSectors := []string{}
		position, ok := resolveOperationalPosition(controller)
		if ok && shared.IsOperationalControllerForPosition(controller, position) {
			if sector, ok := ownerMap[controllerPrimaryFrequency(controller, position)]; ok {
				identifier = sector.Identifier
				ownedSectors = slices.Clone(sector.Sector)
			}
		}

		s.frontendHub.SendControllerUpdate(sessionId, controller.Callsign, controller.Position, identifier, ownedSectors)
	}

	return nil
}

func resolveOperationalPosition(controller *models.Controller) (*config.Position, bool) {
	if controller == nil {
		return nil, false
	}

	if position, err := config.GetPositionByName(controller.Callsign); err == nil {
		return position, true
	}

	if position, err := config.GetPositionBasedOnFrequency(controller.Position); err == nil {
		return position, true
	}

	return nil, false
}
