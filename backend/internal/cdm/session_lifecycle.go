package cdm

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
)

type sessionWork struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func (s *Service) workForSession(session int32) *sessionWork {
	if existing, ok := s.sessionWork.Load(session); ok {
		return existing.(*sessionWork)
	}
	ctx, cancel := context.WithCancel(context.Background())
	work := &sessionWork{ctx: ctx, cancel: cancel}
	actual, loaded := s.sessionWork.LoadOrStore(session, work)
	if loaded {
		cancel()
	}
	return actual.(*sessionWork)
}

func (s *Service) sessionContext(ctx context.Context, session int32) (context.Context, context.CancelFunc) {
	work := s.workForSession(session)
	derived, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(work.ctx, cancel)
	if work.ctx.Err() != nil {
		cancel()
	}
	return derived, func() { stop(); cancel() }
}

func (s *Service) isSessionRemoved(session int32) bool {
	work, ok := s.sessionWork.Load(session)
	return ok && work.(*sessionWork).ctx.Err() != nil
}

// RemoveSession stops outstanding work and rejects scheduling from stale snapshots.
// Session IDs are never reused, so the cancelled lifecycle marker is retained.
func (s *Service) RemoveSession(session int32) {
	s.workForSession(session).cancel()
	s.sessionUsesViff.Delete(session)
	prefix := strconv.Itoa(int(session)) + ":"
	s.debouncer.CancelPrefix(prefix)
	for _, cache := range []*sync.Map{&s.atotPushInFlight, &s.aobtPushInFlight} {
		cache.Range(func(key, _ any) bool {
			if strings.HasPrefix(key.(string), prefix) {
				cache.Delete(key)
			}
			return true
		})
	}
	if s.masterViffSync != nil {
		s.masterViffSync.clearViffPushSession(session)
	}
}

// Only missing targets and lifecycle cancellation are benign. Other database
// failures remain errors even if cleanup happened concurrently.
func (s *Service) sessionDisappeared(ctx context.Context, session int32, err error) bool {
	if errors.Is(err, context.Canceled) {
		return s.isSessionRemoved(session)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false
	}
	if s.isSessionRemoved(session) {
		return true
	}
	_, lookupErr := s.sessionRepo.GetByID(ctx, session)
	return errors.Is(lookupErr, pgx.ErrNoRows)
}
