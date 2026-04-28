package shared

import (
	internalModels "FlightStrips/internal/models"
	"context"
)

type websocketMessageStateKey struct{}

// WebsocketMessageState carries reusable per-message data for non-sync websocket
// handlers so follow-up validation and routing work can avoid reloading the same
// session-scoped entities repeatedly.
type WebsocketMessageState struct {
	MessageType         string
	Session             *internalModels.Session
	ExistingControllers map[string]*internalModels.Controller
	ControllerList      []*internalModels.Controller
	ExistingStrips      map[string]*internalModels.Strip
	StripList           []*internalModels.Strip
	SectorOwners        map[string]*internalModels.SectorOwner
	DBOperations        int
}

func WithWebsocketMessageState(ctx context.Context, state *WebsocketMessageState) context.Context {
	return context.WithValue(ctx, websocketMessageStateKey{}, state)
}

func GetWebsocketMessageState(ctx context.Context) *WebsocketMessageState {
	state, _ := ctx.Value(websocketMessageStateKey{}).(*WebsocketMessageState)
	return state
}

func (s *WebsocketMessageState) AddDBOperations(count int) {
	if s == nil {
		return
	}
	s.DBOperations += count
}

func AddDBOperations(ctx context.Context, count int) {
	if count <= 0 {
		return
	}
	if syncState := GetSyncState(ctx); syncState != nil {
		syncState.AddDBOperations(count)
		return
	}
	if messageState := GetWebsocketMessageState(ctx); messageState != nil {
		messageState.AddDBOperations(count)
	}
}
