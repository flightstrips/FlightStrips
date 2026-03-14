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

// UpdateRoutesForSession recalculates routes for all strips in the session.
// sendUpdate controls whether frontend clients are notified of the updated route ownership.
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

	// Departures require a runway to compute a route.
	if !isArrival && (strip.Runway == nil || *strip.Runway == "") {
		return nil
	}

	var path []string

	if isArrival && (strip.Stand == nil || *strip.Stand == "") {
		// No stand yet: use the receiving tower sector so the strip always has
		// at least the tower controller as its next owner.
		towerSector, ok := config.GetArrivalTowerSector(session.ActiveRunways.ArrivalRunways)
		if !ok {
			return nil
		}
		path = []string{towerSector}
	} else {
		region, err := config.GetRegionForPosition(helpers.ValueOrDefault(strip.PositionLatitude), helpers.ValueOrDefault(strip.PositionLongitude))
		if errors.Is(err, config.ErrUnsupportedRegion) {
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

		var success bool
		if isArrival {
			// Use only arrival runways to select the correct arrival route.
			// Mixing in departure runways can cause the wrong cargo route to match.
			path, success = config.ComputeToStand(session.ActiveRunways.ArrivalRunways, sector, helpers.ValueOrDefault(strip.Stand))
		} else {
			path, success = config.ComputeToRunway(allRunways, sector, helpers.ValueOrDefault(strip.Runway))
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
					path = []string{towerSector}
				} else {
					return nil
				}
			} else {
				return nil
			}
		}
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
			actualRoute = append(actualRoute, owner)
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

	if slices.Equal(strip.NextOwners, actualRoute) {
		// No need to update
		return nil
	}

	err = s.stripRepo.SetNextOwners(context.Background(), session.ID, strip.Callsign, actualRoute)

	if sendUpdate {
		owner := ""
		if strip.Owner != nil {
			owner = *strip.Owner
		}
		s.frontendHub.SendOwnersUpdate(session.ID, strip.Callsign, owner, actualRoute, strip.PreviousOwners)
	}

	return err
}
