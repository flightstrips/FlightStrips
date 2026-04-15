package server

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	"cmp"
	"context"
	"log/slog"
	"slices"
)

func (s *Server) UpdateSectors(sessionId int32) ([]shared.SectorChange, error) {
	sessionRepo := s.sessionRepo
	sectorRepo := s.sectorRepo
	controllerRepo := s.controllerRepo

	previousOwners, err := sectorRepo.ListBySession(context.Background(), sessionId)
	if err != nil {
		return nil, err
	}

	slog.Debug("Updating sectors", slog.Int("session", int(sessionId)), slog.Int("previousOwners", len(previousOwners)))

	session, err := sessionRepo.GetByID(context.Background(), sessionId)
	if err != nil {
		return nil, err
	}

	// If the runways are not set, we cannot calculate the sector ownerships
	if len(session.ActiveRunways.ArrivalRunways) == 0 || len(session.ActiveRunways.DepartureRunways) == 0 {
		slog.Debug("No active runways found for sector update", slog.Int("session", int(sessionId)))
		return nil, nil
	}

	positions, err := getCurrentPositions(controllerRepo, sessionId)
	if err != nil {
		return nil, err
	}

	active := session.ActiveRunways.GetAllActiveRunways()
	sectors := config.GetControllerSectors(positions, active)
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
		tx, err := s.GetDatabasePool().Begin(context.Background())
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(context.Background())

		txSectorRepo := sectorRepo.WithTx(tx)

		err = txSectorRepo.RemoveBySession(context.Background(), sessionId)
		if err != nil {
			return nil, err
		}

		err = txSectorRepo.CreateBulk(context.Background(), currentOwners)
		if err != nil {
			return nil, err
		}

		err = tx.Commit(context.Background())
		if err != nil {
			return nil, err
		}

		if err := s.sendControllerUpdates(sessionId, currentOwners, controllerRepo); err != nil {
			return nil, err
		}
	}

	return changes, nil
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

func getCurrentPositions(controllerRepo repository.ControllerRepository, sessionId int32) ([]*config.Position, error) {
	controllers, err := controllerRepo.List(context.Background(), sessionId)
	if err != nil {
		return nil, err
	}

	positions := make([]*config.Position, 0)
	for _, controller := range controllers {
		if position, err := config.GetPositionBasedOnFrequency(controller.Position); err == nil {
			positions = append(positions, position)
		}
	}

	return positions, nil
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
		if sector, ok := ownerMap[controller.Position]; ok {
			identifier = sector.Identifier
			ownedSectors = slices.Clone(sector.Sector)
		}

		s.frontendHub.SendControllerOnline(sessionId, controller.Callsign, controller.Position, identifier, ownedSectors)
	}

	return nil
}
