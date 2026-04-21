package services

import (
	"FlightStrips/internal/metrics"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

var onStandBays = map[string]bool{
	shared.BAY_NOT_CLEARED: true,
	shared.BAY_CLEARED:     true,
	shared.BAY_STAND:       true,
}

var taxiingBays = map[string]bool{
	shared.BAY_PUSH:     true,
	shared.BAY_TAXI:     true,
	shared.BAY_TAXI_LWR: true,
	shared.BAY_TAXI_TWR: true,
}

type TrafficMetricsService struct {
	sessionRepo repository.SessionRepository
	stripRepo   repository.StripRepository
	interval    time.Duration
}

func NewTrafficMetricsService(sessionRepo repository.SessionRepository, stripRepo repository.StripRepository) *TrafficMetricsService {
	return &TrafficMetricsService{
		sessionRepo: sessionRepo,
		stripRepo:   stripRepo,
		interval:    30 * time.Second,
	}
}

func (s *TrafficMetricsService) Start(ctx context.Context) {
	s.collect(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.collect(ctx)
		}
	}
}

func (s *TrafficMetricsService) collect(ctx context.Context) {
	sessions, err := s.sessionRepo.List(ctx)
	if err != nil {
		slog.Error("traffic metrics: failed to list sessions", slog.Any("error", err))
		return
	}

	now := time.Now().UTC()

	for _, session := range sessions {
		strips, err := s.stripRepo.List(ctx, session.ID)
		if err != nil {
			slog.Error("traffic metrics: failed to list strips",
				slog.Int("session", int(session.ID)),
				slog.Any("error", err),
			)
			continue
		}

		var onStand, taxiing, arr15m, dep15m int64
		for _, strip := range strips {
			if onStandBays[strip.Bay] {
				onStand++
			}
			if taxiingBays[strip.Bay] {
				taxiing++
			}
			if strip.CdmData != nil {
				if strip.CdmData.Aldt != nil && withinLast15Min(*strip.CdmData.Aldt, now) {
					arr15m++
				}
				if strip.CdmData.Aobt != nil && withinLast15Min(*strip.CdmData.Aobt, now) {
					dep15m++
				}
			}
		}

		metrics.RecordTrafficSnapshot(ctx, session.Name, session.Airport, onStand, taxiing, arr15m, dep15m)
	}
}

// withinLast15Min reports whether the HHMM or HHMMSS clock string represents a
// time within the last 15 minutes relative to now (UTC), handling midnight wraps.
func withinLast15Min(hhmm string, now time.Time) bool {
	nowSec, ok := parseClockToSeconds(now.Format("150405"))
	if !ok {
		return false
	}
	eventSec, ok := parseClockToSeconds(hhmm)
	if !ok {
		return false
	}
	// minutes elapsed since the event; negative diff means event is ahead of now on the clock
	diff := float64(nowSec-eventSec) / 60.0
	// apply midnight wrap: if the gap exceeds ±720 min (12 h), shift by one day
	if diff <= -720 {
		diff += 1440
	} else if diff > 720 {
		diff -= 1440
	}
	return diff >= 0 && diff <= 15
}

func parseClockToSeconds(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if len(s) == 4 {
		s += "00"
	}
	if len(s) != 6 {
		return 0, false
	}
	hh, err := strconv.Atoi(s[0:2])
	if err != nil {
		return 0, false
	}
	mm, err := strconv.Atoi(s[2:4])
	if err != nil {
		return 0, false
	}
	ss, err := strconv.Atoi(s[4:6])
	if err != nil {
		return 0, false
	}
	return hh*3600 + mm*60 + ss, true
}
