package server

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"context"
	"errors"
	"fmt"
	"slices"
)

// This is dumb please optimize

func (s *Server) UpdateRouteForStrip(callsign string, sessionId int32, sendUpdate bool) error {
	db := database.New(s.GetDatabasePool())

	strip, err := db.GetStrip(context.Background(), database.GetStripParams{Callsign: callsign, Session: sessionId})
	if err != nil {
		return err
	}

	session, err := db.GetSessionById(context.Background(), sessionId)
	if err != nil {
		return err
	}

	return s.updateRouteForStripHelper(db, strip, session, sendUpdate)
}

func (s *Server) UpdateRoutesForSession(sessionId int32, sendUpdate bool) error {
	db := database.New(s.GetDatabasePool())

	strips, err := db.ListStrips(context.Background(), sessionId)
	if err != nil {
		return err
	}

	session, err := db.GetSessionById(context.Background(), sessionId)
	if err != nil {
		return err
	}

	for _, strip := range strips {
		err := s.updateRouteForStripHelper(db, strip, session, sendUpdate)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) updateRouteForStripHelper(db *database.Queries, strip database.Strip, session database.Session, sendUpdate bool) error {
	isArrival := strip.Destination == session.Airport

	region, err := config.GetRegionForPosition(strip.PositionLatitude.Float64, strip.PositionLongitude.Float64)
	if err != nil {
		return err
	}

	sector, err := config.GetSectorFromRegion(region, isArrival)
	if err != nil {
		return err
	}

	allRunways := session.ActiveRunways.GetAllActiveRunways()

	var path []string
	var success bool
	if isArrival {
		path, success = config.ComputeToStand(allRunways, sector, strip.Stand.String)
	} else {
		path, success = config.ComputeToRunway(allRunways, sector, strip.Runway.String)
	}

	if !success {
		return errors.New("unable to compute route")
	}

	owners, err := db.GetSectorOwners(context.Background(), session.ID)
	if err != nil {
		return err
	}

	sectorToOnwer := make(map[string]string)
	for _, owner := range owners {
		for _, s := range owner.Sector {
			sectorToOnwer[s] = owner.Position
		}
	}

	actualRoute := make([]string, 0)
	for _, s := range path {
		if owner, ok := sectorToOnwer[s]; ok && !slices.Contains(actualRoute, owner) {
			actualRoute = append(actualRoute, owner)
		}
	}

	if !isArrival && strip.Sid.Valid && strip.Sid.String != "" {
		as, err := config.GetAirborneSector(strip.Sid.String)
		if err != nil {
			fmt.Printf("Error getting airborne frequency: %v\n", err)
		} else if owner, ok := sectorToOnwer[as]; ok && !slices.Contains(actualRoute, owner) {
			path = append(path, as)
			actualRoute = append(actualRoute, owner)
		}
	}

	if strip.Owner.Valid && strip.Owner.String != "" {
		index := slices.Index(actualRoute, strip.Owner.String)
		if index != -1 {
			actualRoute = append(actualRoute[:index], actualRoute[index+1:]...)
		}
	}

	if slices.Equal(strip.NextOwners, actualRoute) {
		// No need to update
		return nil
	}

	err = db.SetNextOwners(context.Background(), database.SetNextOwnersParams{NextOwners: actualRoute, Session: session.ID, Callsign: strip.Callsign})

	if sendUpdate {
		s.frontendHub.SendOwnersUpdate(session.ID, strip.Callsign, actualRoute, strip.PreviousOwners)
	}

	return err
}
