package euroscope

import (
	"context"
	"time"
)

type deferredSource struct {
	client *Client
	epoch  uint64
}

func (hub *Hub) captureDeferredSource(session int32) deferredSource {
	return deferredSource{client: hub.getMasterClient(session), epoch: hub.election(session).epoch.Load()}
}
func (hub *Hub) sourceStillCurrent(session int32, source deferredSource) bool {
	if source.epoch != hub.election(session).epoch.Load() || source.client != hub.getMasterClient(session) {
		return false
	}
	return source.client == nil || (!source.client.isClosed() && (source.client.membership == nil || source.client.IsAuthenticated()))
}

// Timer cancellation must not wait behind an election that is itself waiting
// for the sync canceling this timer. TryRLock lets cancellation join the worker.
func (hub *Hub) enterDeferredSource(ctx context.Context, session int32, source deferredSource) (context.Context, func(), bool) {
	election := hub.election(session)
	for !election.gate.TryRLock() {
		select {
		case <-ctx.Done():
			return ctx, func() {}, false
		case <-time.After(time.Millisecond):
		}
	}
	if ctx.Err() != nil || !hub.sourceStillCurrent(session, source) {
		election.gate.RUnlock()
		return ctx, func() {}, false
	}
	return context.WithValue(ctx, sessionApplicationKey{}, election), election.gate.RUnlock, true
}

// Called under the session election gate. Old absences cannot keep a new strip
// hidden or be relabeled as observations by the replacement master.
func (hub *Hub) invalidateSessionAbsence(session int32) {
	hub.aircraftDisconnectMu.Lock()
	for key, entry := range hub.aircraftDisconnectTimers {
		if entry.session == session {
			entry.cancel()
			delete(hub.aircraftDisconnectTimers, key)
		}
	}
	hub.aircraftDisconnectMu.Unlock()
	hub.offlineMu.Lock()
	for key, entry := range hub.offlineTimers {
		if entry.session == session {
			entry.cancel()
			delete(hub.offlineTimers, key)
		}
	}
	hub.offlineMu.Unlock()
}
