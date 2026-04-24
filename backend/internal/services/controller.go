package services

import (
	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
)

func (cs *ControllerService) isOperationalController(controller *internalModels.Controller) bool {
	return controller != nil && !controller.Observer
}

// ControllerService owns all controller online/offline business logic.
type ControllerService struct {
	controllerRepo repository.ControllerRepository
	server         shared.Server
	stripService   shared.StripService
}

func NewControllerService(controllerRepo repository.ControllerRepository) *ControllerService {
	return &ControllerService{
		controllerRepo: controllerRepo,
	}
}

func (cs *ControllerService) SetServer(server shared.Server) {
	cs.server = server
}

func (cs *ControllerService) SetStripService(stripService shared.StripService) {
	cs.stripService = stripService
}

// ControllerOnline handles all database mutations and orchestration for a
// controller coming online. positionName is pre-resolved from config by the caller.
func (cs *ControllerService) ControllerOnline(ctx context.Context, session int32, callsign, position, positionName string) (shared.ControllerOnlineResult, error) {
	s := cs.server
	controller, err := cs.controllerRepo.GetByCallsign(ctx, session, callsign)

	// Case A: new controller (not in database).
	if errors.Is(err, pgx.ErrNoRows) {
		newController := &internalModels.Controller{
			Callsign: callsign,
			Position: position,
			Session:  session,
		}
		if err = cs.controllerRepo.Create(ctx, newController); err != nil {
			return shared.ControllerOnlineResult{}, err
		}
		result, err := cs.performOnlineOrchestration(ctx, session, position, positionName)
		if err != nil {
			return shared.ControllerOnlineResult{}, err
		}
		result.NotifyOnline = true
		return result, nil
	}

	if err != nil {
		return shared.ControllerOnlineResult{}, err
	}

	// Case B: same position — EuroScope heartbeat, no meaningful change.
	if controller.Position == position {
		return shared.ControllerOnlineResult{NotifyOnline: false}, nil
	}

	// Case C: position changed.
	if _, err = cs.controllerRepo.SetPosition(ctx, session, callsign, position); err != nil {
		return shared.ControllerOnlineResult{}, err
	}

	shouldUpdate := false
	if _, configErr := config.GetPositionBasedOnFrequency(position); configErr == nil {
		shouldUpdate = true
	}
	if _, configErr := config.GetPositionBasedOnFrequency(controller.Position); configErr == nil {
		shouldUpdate = true
	}

	slog.DebugContext(ctx, "Controller online with updated position",
		slog.String("callsign", callsign),
		slog.String("position", position),
		slog.Bool("shouldUpdate", shouldUpdate))

	if cs.stripService != nil {
		if err := cs.stripService.AutoAssumeForControllerOnline(ctx, session, position); err != nil {
			slog.ErrorContext(ctx, "Failed to auto-assume strips on controller online",
				slog.String("position", position), slog.Any("error", err))
		}
	}

	if !shouldUpdate {
		return shared.ControllerOnlineResult{NotifyOnline: true}, nil
	}

	changes, err := s.UpdateSectors(session)
	if err != nil {
		return shared.ControllerOnlineResult{}, err
	}

	var singleOnPosition bool
	if positionName != "" {
		controllers, err := cs.controllerRepo.GetByPosition(ctx, session, position)
		if err == nil {
			operationalControllers := 0
			for _, controller := range controllers {
				if cs.isOperationalController(controller) {
					operationalControllers++
				}
			}
			singleOnPosition = operationalControllers == 1
			slog.DebugContext(ctx, "Controller online (position change): single-on-position check",
				slog.String("callsign", callsign),
				slog.String("position", positionName),
				slog.Int("controllersOnPosition", operationalControllers),
				slog.Bool("singleOnPosition", singleOnPosition))
		}
	}

	if err := s.UpdateLayouts(session); err != nil {
		return shared.ControllerOnlineResult{}, err
	}
	slog.DebugContext(ctx, "Controller online: recalculating routes for session",
		slog.Int("session", int(session)),
		slog.String("callsign", callsign),
		slog.String("position", position),
		slog.String("trigger", "position_changed"))
	if err := s.UpdateRoutesForSession(session, true); err != nil {
		return shared.ControllerOnlineResult{}, err
	}
	slog.DebugContext(ctx, "Controller online: route recalculation completed",
		slog.Int("session", int(session)),
		slog.String("callsign", callsign),
		slog.String("position", position),
		slog.String("trigger", "position_changed"))

	return shared.ControllerOnlineResult{
		SectorChanges:    changes,
		SingleOnPosition: singleOnPosition,
		NotifyOnline:     true,
	}, nil
}

// performOnlineOrchestration is the common path for a newly-created controller.
// It calls UpdateSectors, AutoAssume, UpdateLayouts, UpdateRoutesForSession.
func (cs *ControllerService) performOnlineOrchestration(ctx context.Context, session int32, position, positionName string) (shared.ControllerOnlineResult, error) {
	s := cs.server

	changes, err := s.UpdateSectors(session)
	if err != nil {
		return shared.ControllerOnlineResult{}, err
	}

	if cs.stripService != nil {
		if err := cs.stripService.AutoAssumeForControllerOnline(ctx, session, position); err != nil {
			slog.ErrorContext(ctx, "Failed to auto-assume strips on controller online",
				slog.String("position", position), slog.Any("error", err))
		}
	}

	var singleOnPosition bool
	if positionName != "" {
		controllers, err := cs.controllerRepo.GetByPosition(ctx, session, position)
		if err == nil {
			operationalControllers := 0
			for _, controller := range controllers {
				if cs.isOperationalController(controller) {
					operationalControllers++
				}
			}
			singleOnPosition = operationalControllers == 1
			slog.DebugContext(ctx, "Controller online (new): single-on-position check",
				slog.String("position", positionName),
				slog.Int("controllersOnPosition", operationalControllers),
				slog.Bool("singleOnPosition", singleOnPosition))
		}
	}

	if err := s.UpdateLayouts(session); err != nil {
		return shared.ControllerOnlineResult{}, err
	}
	slog.DebugContext(ctx, "Controller online: recalculating routes for session",
		slog.Int("session", int(session)),
		slog.String("position", position),
		slog.String("trigger", "new_controller"))
	if err := s.UpdateRoutesForSession(session, true); err != nil {
		return shared.ControllerOnlineResult{}, err
	}
	slog.DebugContext(ctx, "Controller online: route recalculation completed",
		slog.Int("session", int(session)),
		slog.String("position", position),
		slog.String("trigger", "new_controller"))

	return shared.ControllerOnlineResult{
		SectorChanges:    changes,
		SingleOnPosition: singleOnPosition,
		NotifyOnline:     true,
	}, nil
}

// ControllerOffline checks whether the caller should schedule a delayed offline timer.
// If not (position already covered or unknown), performs immediate cleanup and returns
// ShouldScheduleTimer=false.
func (cs *ControllerService) ControllerOffline(ctx context.Context, session int32, callsign string) (shared.ControllerOfflineResult, error) {
	controller, err := cs.controllerRepo.GetByCallsign(ctx, session, callsign)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return shared.ControllerOfflineResult{}, err
	}

	// If the controller is not in the database, send the notification immediately.
	if errors.Is(err, pgx.ErrNoRows) {
		slog.DebugContext(ctx, "Controller going offline does not exist in database", slog.String("callsign", callsign))
		cs.server.GetFrontendHub().SendControllerOffline(session, callsign, "", "")
		return shared.ControllerOfflineResult{ShouldScheduleTimer: false}, nil
	}

	// Resolve the position name from the frequency.
	posConfig, configErr := config.GetPositionBasedOnFrequency(controller.Position)
	if configErr != nil {
		// Unknown position — immediate offline handling (no timer).
		_ = cs.controllerRepo.Delete(ctx, session, callsign)
		cs.server.GetFrontendHub().SendControllerOffline(session, callsign, controller.Position, "")
		return shared.ControllerOfflineResult{ShouldScheduleTimer: false}, nil
	}
	positionName := posConfig.Name

	// Check whether any OTHER controller is still on this position.
	others, err := cs.controllerRepo.GetByPosition(ctx, session, controller.Position)
	if err != nil {
		return shared.ControllerOfflineResult{}, err
	}
	for _, other := range others {
		if other.Callsign != callsign && cs.isOperationalController(other) {
			slog.DebugContext(ctx, "Controller offline but position still covered by another controller — deleting stale row without offline notification",
				slog.String("callsign", callsign),
				slog.String("position", positionName),
				slog.String("other", other.Callsign))
			_ = cs.controllerRepo.Delete(ctx, session, callsign)
			return shared.ControllerOfflineResult{ShouldScheduleTimer: false}, nil
		}
	}

	slog.DebugContext(ctx, "Controller offline: ready for grace period timer",
		slog.String("callsign", callsign),
		slog.String("position", positionName))

	return shared.ControllerOfflineResult{
		ShouldScheduleTimer: true,
		PositionFrequency:   controller.Position,
		PositionName:        positionName,
	}, nil
}

// UpsertController creates or updates a controller's position (used by sync).
func (cs *ControllerService) UpsertController(ctx context.Context, session int32, callsign, position string) error {
	_, err := cs.controllerRepo.GetByCallsign(ctx, session, callsign)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	if errors.Is(err, pgx.ErrNoRows) {
		newController := &internalModels.Controller{
			Callsign: callsign,
			Session:  session,
			Position: position,
			Cid:      nil,
		}
		if err = cs.controllerRepo.Create(ctx, newController); err != nil {
			return err
		}
		slog.DebugContext(ctx, "Inserted controller", slog.String("callsign", callsign))
	} else {
		if _, err = cs.controllerRepo.SetPosition(ctx, session, callsign, position); err != nil {
			return err
		}
		slog.DebugContext(ctx, "Updated controller", slog.String("callsign", callsign))
	}
	return nil
}
