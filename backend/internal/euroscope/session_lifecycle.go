package euroscope

import (
	"fmt"
	"strings"
)

// RemoveSession detaches operational memberships and their deferred work after
// expiry. Physical control transports survive, including zero-membership demand.
func (hub *Hub) RemoveSession(session int32) {
	for _, member := range hub.clientsSnapshot() {
		if member.session == session && member.membership != nil {
			hub.RemoveMembership(member, "session expired")
		}
	}
	hub.syncedSessions.Delete(session)
	hub.offlineMu.Lock()
	for key, work := range hub.offlineTimers {
		if work.session == session {
			work.cancel()
			delete(hub.offlineTimers, key)
		}
	}
	hub.offlineMu.Unlock()
	hub.aircraftDisconnectMu.Lock()
	for key, work := range hub.aircraftDisconnectTimers {
		if work.session == session {
			work.cancel()
			delete(hub.aircraftDisconnectTimers, key)
		}
	}
	prefix := fmt.Sprintf("%d:", session)
	for key := range hub.aircraftDisconnects {
		if strings.HasPrefix(key, prefix) {
			delete(hub.aircraftDisconnects, key)
		}
	}
	hub.aircraftDisconnectMu.Unlock()
	hub.sessionUpdateMu.Lock()
	if work := hub.sessionUpdateTimers[session]; work != nil {
		if work.timer != nil {
			work.timer.Stop()
		}
		delete(hub.sessionUpdateTimers, session)
	}
	hub.sessionUpdateMu.Unlock()
}
