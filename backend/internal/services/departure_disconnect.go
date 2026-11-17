package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"FlightStrips/internal/sat"
)

const departureDisconnectGrace = 30 * time.Second

var errDepartureReconnected = errors.New("departure reconnected before stand release")

// expireDisconnectedDepartureBlock breaks the retention loop in which a stand
// assignment keeps an absent strip alive and its stale gate position keeps the
// assignment alive. Persist the first absence deadline so polls and restarts
// cannot extend it. Live observations clear it through the normal block path.
func (s *DepartureLifecycleService) expireDisconnectedDepartureBlock(ctx context.Context, session int32, callsign string) error {
	assignment, err := s.assignments.GetAssignment(ctx, session, callsign)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	if assignment == nil || assignment.Direction != string(sat.AssignmentDirectionDeparture) || assignment.Stage != StageDepartureBlock {
		return nil
	}
	strip, err := s.strips.GetByCallsign(ctx, session, callsign)
	if err != nil {
		if isNotFound(err) {
			return nil // The sweep releases assignments whose strip was removed.
		}
		return err
	}
	if strip == nil || strip.EuroscopeSeenAt != nil {
		return nil
	}
	deadline := s.now().Add(departureDisconnectGrace)
	if assignment.ExpiresAt != nil && !deadline.Before(*assignment.ExpiresAt) {
		return nil
	}
	updated := *assignment
	updated.ExpiresAt = &deadline
	affected, err := s.assignments.UpdateAssignment(ctx, &updated)
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("departure disconnect expiry version conflict for %s", callsign)
	}
	updated.Version++
	slog.InfoContext(ctx, "SAT departure disconnect grace started", slog.Int("session", int(session)), slog.String("callsign", callsign), slog.String("stand", assignment.Stand), slog.Time("expires_at", deadline))
	return s.allocations.PublishAssignment(ctx, updated)
}
