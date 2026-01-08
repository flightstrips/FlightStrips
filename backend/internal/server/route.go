package server

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/pkg/helpers"
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

	region, err := config.GetRegionForPosition(helpers.ValueOrDefault(strip.PositionLatitude), helpers.ValueOrDefault(strip.PositionLongitude))
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
		path, success = config.ComputeToStand(allRunways, sector, helpers.ValueOrDefault(strip.Stand))
	} else {
		path, success = config.ComputeToRunway(allRunways, sector, helpers.ValueOrDefault(strip.Runway))
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

	if !isArrival && strip.Sid != nil && *strip.Sid != "" {
		as, err := config.GetAirborneSector(*strip.Sid)
		if err != nil {
			fmt.Printf("Error getting airborne frequency: %v\n", err)
		} else if owner, ok := sectorToOnwer[as]; ok && !slices.Contains(actualRoute, owner) {
			path = append(path, as)
			actualRoute = append(actualRoute, owner)
		}
	}

	if strip.Owner != nil && *strip.Owner != "" {
		index := slices.Index(actualRoute, *strip.Owner)
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
