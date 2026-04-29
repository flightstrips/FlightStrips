package server

import (
	"FlightStrips/internal/config"
	"context"
	"fmt"
	"log/slog"
)

func (s *Server) UpdateLayouts(sessionId int32) error {
	return s.UpdateLayoutsContext(context.Background(), sessionId)
}

func (s *Server) UpdateLayoutsContext(ctx context.Context, sessionId int32) error {
	slog.Debug("Updating layouts", slog.Int("session", int(sessionId)))
	sessionRepo := s.sessionRepo
	controllerRepo := s.controllerRepo

	session, err := getSessionForUpdate(ctx, sessionRepo, sessionId)
	if err != nil {
		return fmt.Errorf("error getting session: %w", err)
	}

	// If the runways are not set, we cannot calculate the sector ownerships
	if len(session.ActiveRunways.ArrivalRunways) == 0 || len(session.ActiveRunways.DepartureRunways) == 0 {
		slog.Debug("No active runways found", slog.Int("session", int(sessionId)))
		return nil
	}

	positions, err := getCurrentPositions(ctx, controllerRepo, sessionId)
	if err != nil {
		return fmt.Errorf("error getting positions: %w", err)
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
		_, err = controllerRepo.SetLayout(ctx, sessionId, position, layout)
		if err != nil {
			return err
		}

		result[position] = *layout
	}

	s.frontendHub.SendLayoutUpdates(sessionId, result)

	return nil
}
