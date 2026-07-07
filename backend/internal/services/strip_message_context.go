package services

import (
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
)

func messageStateStripsLoaded(state *shared.WebsocketMessageState) bool {
	return state != nil && state.StripList != nil
}

func messageStateControllersLoaded(state *shared.WebsocketMessageState) bool {
	return state != nil && state.ControllerList != nil
}

func ensureMessageStateStripIndex(state *shared.WebsocketMessageState) {
	if state == nil || state.ExistingStrips != nil {
		return
	}
	state.ExistingStrips = make(map[string]*internalModels.Strip, len(state.StripList))
	for _, strip := range state.StripList {
		if strip == nil {
			continue
		}
		state.ExistingStrips[strings.ToUpper(strings.TrimSpace(strip.Callsign))] = strip
	}
}

func ensureMessageStateControllerIndex(state *shared.WebsocketMessageState) {
	if state == nil || state.ExistingControllers != nil {
		return
	}
	state.ExistingControllers = make(map[string]*internalModels.Controller, len(state.ControllerList))
	for _, controller := range state.ControllerList {
		if controller == nil {
			continue
		}
		state.ExistingControllers[strings.ToUpper(strings.TrimSpace(controller.Callsign))] = controller
	}
}

func normalizedCallsignKey(callsign string) string {
	return strings.ToUpper(strings.TrimSpace(callsign))
}

func (s *StripService) cacheSession(ctx context.Context, sessionData *internalModels.Session) {
	if sessionData == nil {
		return
	}
	if state := shared.GetWebsocketMessageState(ctx); state != nil {
		state.Session = sessionData
	}
}

func (s *StripService) cacheStrip(ctx context.Context, strip *internalModels.Strip) {
	if strip == nil {
		return
	}
	state := shared.GetWebsocketMessageState(ctx)
	if state == nil {
		return
	}
	ensureMessageStateStripIndex(state)
	if state.ExistingStrips == nil {
		state.ExistingStrips = make(map[string]*internalModels.Strip)
	}
	key := normalizedCallsignKey(strip.Callsign)
	state.ExistingStrips[key] = strip

	if !messageStateStripsLoaded(state) {
		return
	}
	for i, existing := range state.StripList {
		if existing != nil && normalizedCallsignKey(existing.Callsign) == key {
			state.StripList[i] = strip
			return
		}
	}
	state.StripList = append(state.StripList, strip)
}

func (s *StripService) cacheController(ctx context.Context, controller *internalModels.Controller) {
	if controller == nil {
		return
	}
	state := shared.GetWebsocketMessageState(ctx)
	if state == nil {
		return
	}
	ensureMessageStateControllerIndex(state)
	if state.ExistingControllers == nil {
		state.ExistingControllers = make(map[string]*internalModels.Controller)
	}
	key := normalizedCallsignKey(controller.Callsign)
	state.ExistingControllers[key] = controller

	if !messageStateControllersLoaded(state) {
		return
	}
	for i, existing := range state.ControllerList {
		if existing != nil && normalizedCallsignKey(existing.Callsign) == key {
			state.ControllerList[i] = controller
			return
		}
	}
	state.ControllerList = append(state.ControllerList, controller)
}

func (s *StripService) getCachedSession(ctx context.Context, session int32) (*internalModels.Session, error) {
	if syncState := shared.GetSyncState(ctx); syncState != nil && syncState.Session != nil && syncState.Session.ID == session {
		return syncState.Session, nil
	}
	if state := shared.GetWebsocketMessageState(ctx); state != nil && state.Session != nil && state.Session.ID == session {
		return state.Session, nil
	}

	sessionRepo := s.getSessionRepository()
	if sessionRepo == nil {
		return nil, nil
	}

	sessionData, err := sessionRepo.GetByID(ctx, session)
	if err != nil {
		return nil, err
	}
	shared.AddDBOperations(ctx, 1)
	s.cacheSession(ctx, sessionData)
	return sessionData, nil
}

func stripSliceFromMap(values map[string]*internalModels.Strip) []*internalModels.Strip {
	strips := make([]*internalModels.Strip, 0, len(values))
	for _, strip := range values {
		strips = append(strips, strip)
	}
	slices.SortFunc(strips, func(a, b *internalModels.Strip) int {
		switch {
		case a == nil && b == nil:
			return 0
		case a == nil:
			return -1
		case b == nil:
			return 1
		default:
			return strings.Compare(a.Callsign, b.Callsign)
		}
	})
	return strips
}

func controllerSliceFromMap(values map[string]*internalModels.Controller) []*internalModels.Controller {
	controllers := make([]*internalModels.Controller, 0, len(values))
	for _, controller := range values {
		controllers = append(controllers, controller)
	}
	slices.SortFunc(controllers, func(a, b *internalModels.Controller) int {
		switch {
		case a == nil && b == nil:
			return 0
		case a == nil:
			return -1
		case b == nil:
			return 1
		default:
			return strings.Compare(a.Callsign, b.Callsign)
		}
	})
	return controllers
}

func (s *StripService) listCachedStrips(ctx context.Context, session int32) ([]*internalModels.Strip, bool, error) {
	if syncState := shared.GetSyncState(ctx); syncState != nil && syncState.ExistingStrips != nil {
		return stripSliceFromMap(syncState.ExistingStrips), true, nil
	}
	if state := shared.GetWebsocketMessageState(ctx); state != nil {
		if messageStateStripsLoaded(state) {
			return state.StripList, true, nil
		}
	}

	strips, err := s.stripReader.List(ctx, session)
	if err != nil {
		return nil, false, err
	}
	shared.AddDBOperations(ctx, 1)

	if state := shared.GetWebsocketMessageState(ctx); state != nil {
		state.StripList = strips
		state.ExistingStrips = make(map[string]*internalModels.Strip, len(strips))
		for _, strip := range strips {
			if strip == nil {
				continue
			}
			state.ExistingStrips[normalizedCallsignKey(strip.Callsign)] = strip
		}
	}

	return strips, true, nil
}

func (s *StripService) getCachedStrip(ctx context.Context, session int32, callsign string) (*internalModels.Strip, bool, error) {
	key := normalizedCallsignKey(callsign)
	if syncState := shared.GetSyncState(ctx); syncState != nil && syncState.ExistingStrips != nil {
		strip := syncState.ExistingStrips[callsign]
		if strip == nil {
			strip = syncState.ExistingStrips[key]
		}
		if strip == nil {
			return nil, false, nil
		}
		return strip, true, nil
	}
	if state := shared.GetWebsocketMessageState(ctx); state != nil {
		ensureMessageStateStripIndex(state)
		if state.ExistingStrips != nil {
			if strip := state.ExistingStrips[key]; strip != nil {
				return strip, true, nil
			}
			if messageStateStripsLoaded(state) {
				return nil, false, nil
			}
		}
	}

	strip, err := s.stripReader.GetByCallsign(ctx, session, callsign)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	shared.AddDBOperations(ctx, 1)
	s.cacheStrip(ctx, strip)
	return strip, true, nil
}

func (s *StripService) listCachedControllers(ctx context.Context, session int32) ([]*internalModels.Controller, error) {
	if syncState := shared.GetSyncState(ctx); syncState != nil && syncState.ExistingControllers != nil {
		return controllerSliceFromMap(syncState.ExistingControllers), nil
	}
	if state := shared.GetWebsocketMessageState(ctx); state != nil {
		if messageStateControllersLoaded(state) {
			return state.ControllerList, nil
		}
	}

	if s.controllerRepo == nil {
		return nil, nil
	}

	controllers, err := s.controllerRepo.ListBySession(ctx, session)
	if err != nil {
		return nil, err
	}
	shared.AddDBOperations(ctx, 1)

	if state := shared.GetWebsocketMessageState(ctx); state != nil {
		state.ControllerList = controllers
		state.ExistingControllers = make(map[string]*internalModels.Controller, len(controllers))
		for _, controller := range controllers {
			if controller == nil {
				continue
			}
			state.ExistingControllers[normalizedCallsignKey(controller.Callsign)] = controller
		}
	}

	return controllers, nil
}

func (s *StripService) getCachedControllerByCallsign(ctx context.Context, session int32, callsign string) (*internalModels.Controller, bool, error) {
	key := normalizedCallsignKey(callsign)
	if syncState := shared.GetSyncState(ctx); syncState != nil && syncState.ExistingControllers != nil {
		controller := syncState.ExistingControllers[callsign]
		if controller == nil {
			controller = syncState.ExistingControllers[key]
		}
		if controller == nil {
			return nil, false, nil
		}
		return controller, true, nil
	}
	if state := shared.GetWebsocketMessageState(ctx); state != nil {
		ensureMessageStateControllerIndex(state)
		if state.ExistingControllers != nil {
			if controller := state.ExistingControllers[key]; controller != nil {
				return controller, true, nil
			}
			if messageStateControllersLoaded(state) {
				return nil, false, nil
			}
		}
	}

	if s.controllerRepo == nil {
		return nil, false, nil
	}

	controller, err := s.controllerRepo.GetByCallsign(ctx, session, callsign)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	shared.AddDBOperations(ctx, 1)
	s.cacheController(ctx, controller)
	return controller, true, nil
}
