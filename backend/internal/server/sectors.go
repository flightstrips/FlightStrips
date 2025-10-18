package server

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/internal/shared"
	"cmp"
	"context"
	"encoding/json"
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
	if !session.ActiveRunways.Valid || session.ActiveRunways.String == "" {
		fmt.Println("No active runways found")
		return nil
	}

	positions, err := getCurrentPositions(db, sessionId)
	if err != nil {
		return err
	}

	var runways shared.ActiveRunways
	err = json.Unmarshal([]byte(session.ActiveRunways.String), &runways)
	if err != nil {
		return err
	}

	active := runways.GetAllActiveRunways()

	sectors := config.GetControllerSectors(positions, active)
	if len(sectors) == 0 {
		return nil
	}

	currentOwners := make([]database.SectorOwner, 0)
	for key, sectors := range sectors {
		names := make([]string, 0)
		sorted := slices.SortedFunc(slices.Values(sectors), sectorsCompare)
		for _, sector := range sorted {
			names = append(names, sector.Name)
		}

		jsonSectors, err := json.Marshal(names)
		if err != nil {
			continue
		}

		currentOwners = append(currentOwners, database.SectorOwner{
			Session:  sessionId,
			Position: key,
			Sector:   string(jsonSectors),
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
			Session:  owner.Session,
			Position: owner.Position,
			Sector:   owner.Sector,
		})
	}

	_, err = db.InsertSectorOwners(context.Background(), dbParams)
	if err != nil {
		return err
	}
	err = tx.Commit(context.Background())
	return err
}

func sectorsEqual(a, b database.SectorOwner) bool {
	return a.Session == b.Session && a.Position == b.Position && a.Sector == b.Sector
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
