package server

import (
	"FlightStrips/internal/config"
	"context"
	"fmt"
)

func (s *Server) UpdateLayouts(sessionId int32) error {
	sessionRepo := s.sessionRepo
	controllerRepo := s.controllerRepo
	
	session, err := sessionRepo.GetByID(context.Background(), sessionId)
	if err != nil {
		return err
	}

	// If the runways are not set, we cannot calculate the sector ownerships
	if len(session.ActiveRunways.ArrivalRunways) == 0 || len(session.ActiveRunways.DepartureRunways) == 0 {
		fmt.Println("No active runways found")
		return nil
	}

	positions, err := getCurrentPositions(controllerRepo, sessionId)
	if err != nil {
		return err
	}

	active := session.ActiveRunways.GetAllActiveRunways()

	layouts := config.GetLayouts(positions, active)
	if len(layouts) == 0 {
		return nil
	}

	result := make(map[string]string)

	for position, layout := range layouts {
		if layout == nil {
			continue
		}
		_, err = controllerRepo.SetLayout(context.Background(), sessionId, position, layout)
		if err != nil {
			return err
		}

		result[position] = *layout
	}

	s.frontendHub.SendLayoutUpdates(sessionId, result)

	return nil
}
