package services

import (
	"context"
	"strings"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
)

type departureStandReleaser interface {
	ReleaseDepartureStand(ctx context.Context, session int32, callsign string) error
}

// ReleaseDepartureStand frees SAT capacity once PUSH is operationally active.
// The strip keeps its origin stand because downstream routing still needs it.
func (s *DepartureLifecycleService) ReleaseDepartureStand(ctx context.Context, session int32, callsign string) error {
	s.clearUnassignedStandWarning(session, callsign)
	assignment, err := s.assignments.GetAssignment(ctx, session, callsign)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	return s.allocations.ReleaseAssignmentRetainingStand(ctx, assignment)
}

func (s *StripService) releaseDepartureStandOnPush(ctx context.Context, session int32, strip *models.Strip, groundState, airport string) error {
	if strip == nil || !strings.EqualFold(strings.TrimSpace(groundState), shared.BAY_PUSH) ||
		!strings.EqualFold(strings.TrimSpace(strip.Origin), strings.TrimSpace(airport)) {
		return nil
	}
	releaser, ok := s.departureObserver.(departureStandReleaser)
	if !ok {
		return nil
	}
	return releaser.ReleaseDepartureStand(ctx, session, strip.Callsign)
}
