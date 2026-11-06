package pdc

// RemoveSession cancels only the session's deferred transactions. The shared
// provider client, deployment logon and polling worker remain application-owned.
func (s *Service) RemoveSession(session int32) {
	s.removedSessions.Store(session, struct{}{})
	s.timeoutsMutex.Lock()
	defer s.timeoutsMutex.Unlock()
	for key, work := range s.timeouts {
		if work.sessionID == session {
			work.cancel()
			delete(s.timeouts, key)
		}
	}
}

func (s *Service) sessionRemoved(session int32) bool {
	_, removed := s.removedSessions.Load(session)
	return removed
}
