package services

import (
	"fmt"
	"strings"
)

func (s *StandAllocationService) RemoveSession(session int32) {
	s.removedSessions.Store(session, struct{}{})
	prefix := fmt.Sprintf("%d:", session)
	s.automaticFailureMu.Lock()
	defer s.automaticFailureMu.Unlock()
	for key := range s.automaticTerminalFailures {
		if strings.HasPrefix(key, prefix) {
			delete(s.automaticTerminalFailures, key)
		}
	}
}

func (s *StandAllocationService) sessionRemoved(session int32) bool {
	if s == nil {
		return false
	}
	_, removed := s.removedSessions.Load(session)
	return removed
}

func (s *DepartureLifecycleService) RemoveSession(session int32) {
	prefix := fmt.Sprintf("%d:", session)
	s.warningMu.Lock()
	defer s.warningMu.Unlock()
	for key := range s.warnings {
		if strings.HasPrefix(key, prefix) {
			delete(s.warnings, key)
		}
	}
}
