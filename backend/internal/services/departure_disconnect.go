package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/sat"
)

const (
	departureDisconnectGrace = 30 * time.Second
	// defaultDepartureObservationTimeout bounds how long a departure block may
	// outlive the last live sighting of its aircraft. EuroScope positions arrive
	// every few seconds and the VATSIM feed every 15 seconds, so ten silent
	// minutes means neither source still sees the aircraft.
	defaultDepartureObservationTimeout = 10 * time.Minute
)

var (
	errDepartureReconnected   = errors.New("departure reconnected before stand release")
	errDepartureObservedAgain = errors.New("departure observed again before stand release")
)

// departureLastSighting returns the most recent live evidence that the
// aircraft exists. EuroScope refreshes euroscope_seen_at on every position
// report and sync; the VATSIM reconciler refreshes vatsim_seen_at from the
// pilot's last_updated on every feed poll. Neither timestamp advances once
// its source stops seeing the aircraft, even when no disconnect is ever
// reported (EuroScope crash, dropped disconnect timer, non-LIVE session).
// Without either timestamp, the block's own assignment time is the last
// observation the lifecycle recorded.
func departureLastSighting(strip *models.Strip, assignment *models.StandAssignment) *time.Time {
	var latest *time.Time
	for _, candidate := range []*time.Time{strip.EuroscopeSeenAt, strip.VatsimSeenAt} {
		if candidate != nil && (latest == nil || candidate.After(*latest)) {
			latest = candidate
		}
	}
	if latest == nil && assignment != nil {
		latest = assignment.AssignedAt
	}
	return latest
}

// departureBlockUnobserved reports whether a departure block's aircraft has
// not been seen by any live source within the observation timeout. Departure
// blocks deliberately carry no ExpiresAt (the allocator reads it as the end of
// occupancy), so this heartbeat is their only time-based release.
func (s *DepartureLifecycleService) departureBlockUnobserved(strip *models.Strip, assignment *models.StandAssignment, now time.Time) bool {
	if strip == nil || assignment == nil ||
		!strings.EqualFold(assignment.Direction, string(sat.AssignmentDirectionDeparture)) ||
		assignment.Stage != StageDepartureBlock {
		return false
	}
	lastSeen := departureLastSighting(strip, assignment)
	return lastSeen == nil || now.Sub(*lastSeen) > s.observationTimeout
}

// releaseUnobservedDepartureBlock frees a departure block whose aircraft no
// live source has seen within the observation timeout. The locked strip is
// rechecked so a sighting that lands after the sweep's read keeps the stand.
func (s *DepartureLifecycleService) releaseUnobservedDepartureBlock(ctx context.Context, assignment *models.StandAssignment, now time.Time) error {
	err := s.allocations.releaseAssignmentChecked(ctx, assignment, false, func(locked *models.Strip) error {
		if locked != nil && !s.departureBlockUnobserved(locked, assignment, now) {
			return errDepartureObservedAgain
		}
		return nil
	})
	if errors.Is(err, errDepartureObservedAgain) {
		return nil
	}
	recordSATExpiry(ctx, assignment, "unobserved", err)
	if err == nil {
		slog.InfoContext(ctx, "SAT departure block released after aircraft went unobserved",
			slog.Int("session", int(assignment.SessionID)),
			slog.String("callsign", assignment.Callsign),
			slog.String("stand", assignment.Stand),
			slog.Duration("timeout", s.observationTimeout))
	}
	return err
}

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
