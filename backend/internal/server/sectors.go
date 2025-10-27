package server

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"cmp"
	"context"
	"fmt"
	"slices"
)

func (s *Server) UpdateSectors(sessionId int32) error {
	db := database.New(s.GetDatabasePool())

	previousOwners, err := db.GetSectorOwners(context.Background(), sessionId)
	if err != nil {
		return err
	}

	fmt.Printf("Updating sectors for session %d\n", sessionId)
	fmt.Printf("Found %d previous owners\n", len(previousOwners))

	session, err := db.GetSessionById(context.Background(), sessionId)
	if err != nil {
		return err
	}

	// If the runways are not set, we cannot calculate the sector ownerships
	if len(session.ActiveRunways.ArrivalRunways) == 0 || len(session.ActiveRunways.DepartureRunways) == 0 {
		fmt.Println("No active runways found")
		return nil
	}

	positions, err := getCurrentPositions(db, sessionId)
	if err != nil {
		return err
	}

	active := session.ActiveRunways.GetAllActiveRunways()

	sectors := config.GetControllerSectors(positions, active)
	if len(sectors) == 0 {
		return nil
	}

	currentOwners := make([]database.SectorOwner, 0)
	for key, sectors := range sectors {
		names := make([]string, 0)
		sorted := slices.SortedFunc(slices.Values(sectors), sectorsCompare)
		identifier := ""
		priority := 0

		for _, sector := range sorted {
			names = append(names, sector.Name)
			if sector.NamePriority > priority {
				priority = sector.NamePriority
				identifier = sector.Name
			}
		}

		currentOwners = append(currentOwners, database.SectorOwner{
			Session:    sessionId,
			Position:   key,
			Sector:     names,
			Identifier: identifier,
		})
	}

	fmt.Printf("Found %d sectors\n", len(currentOwners))

	if slices.EqualFunc(currentOwners, previousOwners, sectorsEqual) {
		// No changes
		return nil
	}

	tx, err := s.GetDatabasePool().Begin(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	db = db.WithTx(tx)

	err = db.RemoveSectorOwners(context.Background(), sessionId)
	if err != nil {
		return err
	}

	dbParams := make([]database.InsertSectorOwnersParams, 0)
	for _, owner := range currentOwners {
		dbParams = append(dbParams, database.InsertSectorOwnersParams{
			Session:    owner.Session,
			Position:   owner.Position,
			Sector:     owner.Sector,
			Identifier: owner.Identifier,
		})
	}

	_, err = db.InsertSectorOwners(context.Background(), dbParams)
	if err != nil {
		return err
	}
	err = tx.Commit(context.Background())
	if err != nil {
		return err
	}

	return s.sendControllerUpdates(sessionId, currentOwners, db)
}

func sectorsEqual(a, b database.SectorOwner) bool {
	return a.Session == b.Session && a.Position == b.Position && slices.Equal(a.Sector, b.Sector)
}

func sectorsCompare(e, e2 config.Sector) int {
	return cmp.Compare(e.Name, e2.Name)
}

func getCurrentPositions(db *database.Queries, sessionId int32) ([]*config.Position, error) {

	controllers, err := db.GetControllers(context.Background(), sessionId)
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

func (s *Server) sendControllerUpdates(sessionId int32, owners []database.SectorOwner, db *database.Queries) error {
	controllers, err := db.GetControllers(context.Background(), sessionId)
	if err != nil {
		return err
	}

	ownerMap := make(map[string]database.SectorOwner)
	for _, owner := range owners {
		ownerMap[owner.Position] = owner
	}

	for _, controller := range controllers {
		identifier := ""
		if sector, ok := ownerMap[controller.Position]; ok {
			identifier = sector.Identifier
		}

		s.frontendHub.SendControllerOnline(sessionId, controller.Callsign, controller.Position, identifier)
	}

	return nil
}
