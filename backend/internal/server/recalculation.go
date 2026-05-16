package server

import (
	"FlightStrips/internal/shared"
	"context"
	"sync"
)

type sessionRecalcLockManager struct {
	mu    sync.Mutex
	locks map[int32]*sessionRecalcLock
}

type sessionRecalcLock struct {
	mu   sync.Mutex
	refs int
}

func (m *sessionRecalcLockManager) lock(sessionId int32) func() {
	m.mu.Lock()
	if m.locks == nil {
		m.locks = make(map[int32]*sessionRecalcLock)
	}

	lock := m.locks[sessionId]
	if lock == nil {
		lock = &sessionRecalcLock{}
		m.locks[sessionId] = lock
	}
	lock.refs++
	m.mu.Unlock()

	lock.mu.Lock()

	return func() {
		lock.mu.Unlock()

		m.mu.Lock()
		defer m.mu.Unlock()

		lock.refs--
		if lock.refs == 0 {
			delete(m.locks, sessionId)
		}
	}
}

func (s *Server) RecalculateSession(sessionId int32, sendUpdate bool) ([]shared.SectorChange, error) {
	return s.RecalculateSessionContext(context.Background(), sessionId, sendUpdate)
}

func (s *Server) RecalculateSessionContext(ctx context.Context, sessionId int32, sendUpdate bool) ([]shared.SectorChange, error) {
	unlock := s.sessionLocks.lock(sessionId)
	defer unlock()

	changes, err := s.updateSectorsContextUnlocked(ctx, sessionId)
	if err != nil {
		return nil, err
	}

	if err := s.updateLayoutsContextUnlocked(ctx, sessionId); err != nil {
		return nil, err
	}

	if err := s.updateRoutesForSessionContextUnlocked(ctx, sessionId, sendUpdate); err != nil {
		return nil, err
	}

	return changes, nil
}
