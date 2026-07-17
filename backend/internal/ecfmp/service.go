package ecfmp

import (
	"FlightStrips/internal/dependencies"
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"context"
	"fmt"
	"log/slog"
	"time"
)

const refreshInterval = time.Minute

type Service struct {
	client       *Client
	stripRepo    StripStore
	sessionRepo  repository.SessionRepository
	publisher    shared.CdmEventPublisher
	euroscopeHub shared.EuroscopeHub
}

type ServiceDependencies struct {
	Client    *Client
	Strips    StripStore
	Sessions  repository.SessionRepository
	Frontend  shared.CdmEventPublisher
	Euroscope shared.EuroscopeHub
}

func NewService(deps ServiceDependencies) (*Service, error) {
	required := []struct {
		name  string
		value any
	}{
		{"client", deps.Client},
		{"strip store", deps.Strips},
		{"session repository", deps.Sessions},
		{"frontend publisher", deps.Frontend},
		{"EuroScope publisher", deps.Euroscope},
	}
	for _, dependency := range required {
		if dependencies.IsNil(dependency.value) {
			return nil, fmt.Errorf("ecfmp service requires %s", dependency.name)
		}
	}

	return &Service{
		client:       deps.Client,
		stripRepo:    deps.Strips,
		sessionRepo:  deps.Sessions,
		publisher:    deps.Frontend,
		euroscopeHub: deps.Euroscope,
	}, nil
}

func (s *Service) FlowMeasures(ctx context.Context) ([]FlowMeasure, error) {
	return s.client.FlowMeasures(ctx)
}

func (s *Service) Start(ctx context.Context) {
	if err := s.refreshMeasures(ctx); err != nil {
		slog.WarnContext(ctx, "Failed to fetch initial ECFMP measures", slog.Any("error", err))
	}

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.refreshMeasures(ctx); err != nil {
				slog.WarnContext(ctx, "Failed to refresh ECFMP measures", slog.Any("error", err))
			}
		}
	}
}

func (s *Service) InjectTestMeasures(ctx context.Context, measures []FlowMeasure) error {
	measures = normalizeTestMeasures(measures, time.Now())
	s.client.SetTestMeasures(measures)
	return s.refreshMeasures(ctx)
}

func (s *Service) ClearTestMeasures(ctx context.Context) error {
	return s.InjectTestMeasures(ctx, nil)
}

func (s *Service) refreshMeasures(ctx context.Context) error {
	measures, err := s.FlowMeasures(ctx)
	if err != nil {
		return err
	}

	return s.applyMeasures(ctx, measures)
}

func (s *Service) applyMeasures(ctx context.Context, measures []FlowMeasure) error {
	sessions, err := s.sessionRepo.List(ctx)
	if err != nil {
		return err
	}

	for _, session := range sessions {
		if session == nil {
			continue
		}

		strips, err := s.stripRepo.List(ctx, session.ID)
		if err != nil {
			slog.WarnContext(ctx, "Failed to list strips for ECFMP application",
				slog.Int("session", int(session.ID)), slog.Any("error", err))
			continue
		}

		cdmDataRows, err := s.stripRepo.GetCdmData(ctx, session.ID)
		if err != nil {
			slog.WarnContext(ctx, "Failed to get CDM data for ECFMP application",
				slog.Int("session", int(session.ID)), slog.Any("error", err))
			continue
		}

		cdmDataMap := make(map[string]*models.CdmData, len(cdmDataRows))
		for _, row := range cdmDataRows {
			cdmDataMap[row.Callsign] = row.Data
		}

		for _, strip := range strips {
			if strip == nil {
				continue
			}

			newRestrictions := MatchingRestrictions(strip, measures, time.Now())
			converted := convertStripRestrictions(newRestrictions)

			cdmData, ok := cdmDataMap[strip.Callsign]
			if !ok || cdmData == nil {
				cdmData = &models.CdmData{}
			}

			if ecfmpRestrictionsEqual(cdmData.EcfmpRestrictions, converted) {
				continue
			}

			updated := cdmData.Clone()
			updated.EcfmpRestrictions = converted
			if len(converted) == 0 {
				updated.EcfmpRestrictions = nil
			}

			rows, err := s.stripRepo.SetCdmData(ctx, session.ID, strip.Callsign, updated)
			if err != nil {
				slog.WarnContext(ctx, "Failed to persist ECFMP restrictions",
					slog.String("callsign", strip.Callsign), slog.Any("error", err))
				continue
			}
			if rows != 1 {
				slog.WarnContext(ctx, "Unexpected row count while persisting ECFMP restrictions",
					slog.String("callsign", strip.Callsign), slog.Int64("rows", rows))
				continue
			}

			s.broadcastEcfmpChanges(session.ID, strip.Callsign, updated)
		}
	}

	return nil
}

func (s *Service) broadcastEcfmpChanges(session int32, callsign string, cdmData *models.CdmData) {
	s.publisher.SendCdmUpdates(session, []frontendEvents.CdmDataEvent{shared.BuildFrontendCdmDataEvent(callsign, cdmData)})
	s.euroscopeHub.BroadcastCdmUpdates(session, []euroscopeEvents.CdmUpdateEvent{shared.BuildEuroscopeCdmUpdateEvent(callsign, cdmData)})
}

func convertStripRestrictions(restrictions []StripRestriction) []models.EcfmpRestriction {
	if len(restrictions) == 0 {
		return nil
	}

	result := make([]models.EcfmpRestriction, len(restrictions))
	for i, r := range restrictions {
		result[i] = models.EcfmpRestriction{
			MeasureID:   r.MeasureID,
			Ident:       r.Ident,
			Type:        r.Type,
			Reason:      r.Reason,
			Routes:      r.Routes,
			Destination: r.Destination,
			MaxLevel:    r.MaxLevel,
			MinLevel:    r.MinLevel,
			ExactLevels: r.ExactLevels,
			HasCtot:     r.HasCtot,
		}
	}
	return result
}

func ecfmpRestrictionsEqual(a, b []models.EcfmpRestriction) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].MeasureID != b[i].MeasureID ||
			a[i].Ident != b[i].Ident ||
			a[i].Type != b[i].Type ||
			a[i].Reason != b[i].Reason ||
			a[i].Destination != b[i].Destination ||
			a[i].HasCtot != b[i].HasCtot {
			return false
		}
		if !sliceEqual(a[i].Routes, b[i].Routes) {
			return false
		}
		if !intSliceEqual(a[i].ExactLevels, b[i].ExactLevels) {
			return false
		}
		if !intPtrEqual(a[i].MaxLevel, b[i].MaxLevel) {
			return false
		}
		if !intPtrEqual(a[i].MinLevel, b[i].MinLevel) {
			return false
		}
	}
	return true
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func intSliceEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func intPtrEqual(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func normalizeTestMeasures(measures []FlowMeasure, now time.Time) []FlowMeasure {
	if len(measures) == 0 {
		return nil
	}

	normalized := make([]FlowMeasure, len(measures))
	copy(normalized, measures)
	for i := range normalized {
		if normalized[i].StartTime.IsZero() {
			normalized[i].StartTime = now.Add(-1 * time.Hour)
		}
		if normalized[i].EndTime.IsZero() {
			normalized[i].EndTime = now.Add(24 * time.Hour)
		}
	}

	return normalized
}
