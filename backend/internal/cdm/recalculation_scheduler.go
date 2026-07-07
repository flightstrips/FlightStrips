package cdm

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
)

type RecalculationScheduler struct {
	service *Service
}

func (c *RecalculationScheduler) TriggerRecalculate(ctx context.Context, session int32, airport string) {
	s := c.service
	if s.sequenceService == nil || airport == "" {
		return
	}
	if !s.canRunLocalRecalculation(session) {
		return
	}
	normalizedAirport := strings.ToUpper(strings.TrimSpace(airport))
	recalcCtx := detachedContext(ctx)
	s.debouncer.Schedule(recalcDebounceKey(session, normalizedAirport), func() {
		if err := s.sequenceService.RecalculateAirport(recalcCtx, session, airport); err != nil {
			slog.ErrorContext(recalcCtx, "CDM recalculation failed", slog.Int("session", int(session)), slog.String("airport", airport), slog.Any("error", err))
		}
	})
}

func (c *RecalculationScheduler) TriggerRecalculateForAirport(ctx context.Context, airport string) error {
	s := c.service
	if s.sequenceService == nil || strings.TrimSpace(airport) == "" {
		return nil
	}

	sessions, err := s.sessionRepo.List(ctx)
	if err != nil {
		return err
	}

	for _, session := range sessions {
		if session == nil || !strings.EqualFold(session.Airport, airport) {
			continue
		}
		s.TriggerRecalculate(ctx, session.ID, session.Airport)
	}

	return nil
}

func (c *RecalculationScheduler) schedulePeriodicRecalculate(ctx context.Context) error {
	s := c.service
	if s.sequenceService == nil {
		return nil
	}

	sessions, err := s.sessionRepo.List(ctx)
	if err != nil {
		return err
	}

	for _, session := range sessions {
		if session == nil || strings.TrimSpace(session.Airport) == "" {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		s.TriggerRecalculate(ctx, session.ID, session.Airport)
	}

	return nil
}

func (c *RecalculationScheduler) canRunLocalRecalculation(session int32) bool {
	s := c.service
	return s.isMasterSession(session)
}

func recalcDebounceKey(session int32, airport string) string {
	return strconv.FormatInt(int64(session), 10) + ":" + airport
}
