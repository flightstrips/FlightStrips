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
	MessageType           string
	Session               *internalModels.Session
	ExistingControllers   map[string]*internalModels.Controller
	ControllerList        []*internalModels.Controller
	ExistingStrips        map[string]*internalModels.Strip
	StripList             []*internalModels.Strip
	SectorOwners          map[string]*internalModels.SectorOwner
	DBOperations          int
	DBRetries             map[string]int
	AutoCountDBOperations bool
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
		if messageState.AutoCountDBOperations {
			return
		}
		messageState.AddDBOperations(count)
	}
}

// TraceDBOperation records a database operation observed by the pgx query
// tracer. Automatic counting is enabled only for handlers whose full query
// budget is measured at this boundary.
func TraceDBOperation(ctx context.Context) {
	if messageState := GetWebsocketMessageState(ctx); messageState != nil && messageState.AutoCountDBOperations {
		messageState.AddDBOperations(1)
	}
}

func AddDBRetry(ctx context.Context, class string) {
	messageState := GetWebsocketMessageState(ctx)
	if messageState == nil || class == "" {
		return
	}
	if messageState.DBRetries == nil {
		messageState.DBRetries = make(map[string]int)
	}
	messageState.DBRetries[class]++
}
