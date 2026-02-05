package server

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/pkg/helpers"
	"context"
	"errors"
	"log/slog"
	"slices"
)

// This is dumb please optimize

func (s *Server) UpdateRouteForStrip(callsign string, sessionId int32, sendUpdate bool) error {
	stripRepo := s.stripRepo
	sessionRepo := s.sessionRepo

	strip, err := stripRepo.GetByCallsign(context.Background(), sessionId, callsign)
	if err != nil {
		return err
	}

	session, err := sessionRepo.GetByID(context.Background(), sessionId)
	if err != nil {
		return err
	}

	return s.updateRouteForStripHelper(strip, session, sendUpdate)
}

func (s *Server) UpdateRoutesForSession(sessionId int32, sendUpdate bool) error {
	stripRepo := s.stripRepo
	sessionRepo := s.sessionRepo

	strips, err := stripRepo.List(context.Background(), sessionId)
	if err != nil {
		return err
	}

	session, err := sessionRepo.GetByID(context.Background(), sessionId)
	if err != nil {
		return err
	}

	for _, strip := range strips {
		err := s.updateRouteForStripHelper(strip, session, sendUpdate)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) updateRouteForStripHelper(strip *models.Strip, session *models.Session, sendUpdate bool) error {
	isArrival := strip.Destination == session.Airport

	region, err := config.GetRegionForPosition(helpers.ValueOrDefault(strip.PositionLatitude), helpers.ValueOrDefault(strip.PositionLongitude))
	if errors.Is(err, config.UnsupportedRegion) {
		return nil
	}
	if err != nil {
		return err
	}

	sector, err := config.GetSectorFromRegion(region, isArrival)
	if err != nil {
		slog.Warn("Sector not found based on region", slog.String("callsign", strip.Callsign), slog.String("region", region.Name))
		return nil
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
		stand := helpers.ValueOrDefault(strip.Stand)
		slog.Warn("Could not compute route for strip", slog.String("callsign", strip.Callsign), slog.String("sector", sector), slog.Bool("is_arrival", isArrival), slog.Any("stand", stand))
		return nil
	}

	owners, err := s.sectorRepo.ListBySession(context.Background(), session.ID)
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
			slog.Info("Error getting airborne frequency", slog.String("sid", *strip.Sid), slog.Any("error", err))
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

	err = s.stripRepo.SetNextOwners(context.Background(), session.ID, strip.Callsign, actualRoute)

	if sendUpdate {
		s.frontendHub.SendOwnersUpdate(session.ID, strip.Callsign, actualRoute, strip.PreviousOwners)
	}

	return err
}
