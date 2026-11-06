package frontend

// RemoveSession evicts presentation partitions only. It cannot clear station
// weather observations, source generations or immutable airport definitions.
func (hub *Hub) RemoveSession(session int32) {
	hub.removedSessions.Store(session, struct{}{})
	hub.msgMu.Lock()
	delete(hub.messages, session)
	hub.msgMu.Unlock()
	hub.metarMu.Lock()
	delete(hub.metarCache, session)
	delete(hub.arrAtisCodeCache, session)
	delete(hub.depAtisCodeCache, session)
	hub.metarMu.Unlock()
	hub.clxMu.Lock()
	delete(hub.clxOverrides, session)
	hub.clxMu.Unlock()
}

func (hub *Hub) sessionRemoved(session int32) bool {
	_, removed := hub.removedSessions.Load(session)
	return removed
}
