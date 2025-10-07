package main

import (
	"FlightStrips/config"
	"FlightStrips/database"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// This is dumb please optimize

func (s *Server) UpdateRouteForStrip(callsign string, sessionId int32) error {
	db := database.New(s.DBPool)

	strip, err := db.GetStrip(context.Background(), database.GetStripParams{Callsign: callsign, Session: sessionId})
	if err != nil {
		return err
	}

	session, err := db.GetSessionById(context.Background(), sessionId)
	if err != nil {
		return err
	}

	isArrival := strip.Destination == session.Airport

	region, err := config.GetRegionForPosition(strip.PositionLatitude.Float64, strip.PositionLongitude.Float64)
	if err != nil {
		return err
	}

	sector, err := config.GetSectorFromRegion(region, isArrival)
	if err != nil {
		return err
	}

	var runways ActiveRunways
	err = json.Unmarshal([]byte(session.ActiveRunways.String), &runways)
	if err != nil {
		return err
	}

	allRunways := runways.GetAllActiveRunways()

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

	// this is even more stupid
	owners, err := db.GetSectorOwners(context.Background(), sessionId)
	if err != nil {
		return err
	}

	sectorToOnwer := make(map[string]string)
	for _, owner := range owners {
		var ownerSectors []string
		err := json.Unmarshal([]byte(owner.Sector), &ownerSectors)
		if err != nil {
			return err
		}
		for _, s := range ownerSectors {
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

	fmt.Printf("Found route: %v (%v) for strip: %v\n", actualRoute, path, callsign)

	return nil
}
