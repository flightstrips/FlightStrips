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
	"FlightStrips/internal/vatsim"

	"github.com/jackc/pgx/v5"
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
// report and sync. onlineSeen is the last time the VATSIM feed showed the
// pilot online; strips.vatsim_seen_at is not used because offline prefiles
// refresh it too. Neither timestamp advances once its source stops seeing the
// aircraft, even when no disconnect is ever reported (EuroScope crash,
// dropped disconnect timer, non-LIVE session). Without either timestamp, the
// block's own assignment time is the last observation the lifecycle recorded.
func departureLastSighting(strip *models.Strip, onlineSeen *time.Time, assignment *models.StandAssignment) *time.Time {
	var latest *time.Time
	for _, candidate := range []*time.Time{strip.EuroscopeSeenAt, onlineSeen} {
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
// occupancy), so this heartbeat is their only time-based release. A running
// disconnect grace deadline is left to the disconnect path: it ends at that
// deadline and lets a EuroScope reconnect keep the stand.
func (s *DepartureLifecycleService) departureBlockUnobserved(strip *models.Strip, onlineSeen *time.Time, assignment *models.StandAssignment, now time.Time) bool {
	if strip == nil || assignment == nil ||
		!strings.EqualFold(assignment.Direction, string(sat.AssignmentDirectionDeparture)) ||
		assignment.Stage != StageDepartureBlock ||
		(assignment.ExpiresAt != nil && assignment.ExpiresAt.After(now)) {
		return false
	}
	lastSeen := departureLastSighting(strip, onlineSeen, assignment)
	return lastSeen == nil || now.Sub(*lastSeen) > s.observationTimeout
}

// staleOnlineFeedSighting reports whether the feed claims an online pilot but
// its sighting is older than the observation timeout. A cached snapshot that
// stopped refreshing keeps replaying the last gate position; it must neither
// recreate a block the heartbeat released nor refresh the heartbeat.
func (s *DepartureLifecycleService) staleOnlineFeedSighting(flight vatsim.DepartureFlightInfo) bool {
	return flight.Online && !flight.SeenAt.IsZero() && s.now().Sub(flight.SeenAt) > s.observationTimeout
}

// recordOnlineSighting persists when the feed last showed this departure's
// pilot online. Every newer sighting is stored, so the heartbeat never sees a
// sighting older than the feed reported; replaying the same or an older one
// writes nothing. The row is removed with its strip.
// Lock the strip before writing so a sighting cannot commit between the
// release transaction's locked heartbeat check and its assignment deletion.
func (s *DepartureLifecycleService) recordOnlineSighting(ctx context.Context, session int32, callsign string, flight vatsim.DepartureFlightInfo) error {
	if !flight.Online || flight.SeenAt.IsZero() {
		return nil
	}
	_, err := s.allocations.pool.Exec(ctx, `INSERT INTO departure_online_sightings (strip_id, seen_at)
		SELECT id, $3 FROM strips WHERE session = $1 AND callsign = $2 FOR UPDATE
		ON CONFLICT (strip_id) DO UPDATE SET seen_at = EXCLUDED.seen_at
		WHERE departure_online_sightings.seen_at < EXCLUDED.seen_at`,
		session, callsign, flight.SeenAt.UTC())
	return err
}

// sightingReader is a pool or a dedicated pool connection; both read the
// latest committed sighting outside any release transaction.
type sightingReader interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// departureOnlineSighting returns the last online feed sighting for a strip.
// The final release check uses this current committed read while holding the
// strip lock. Reading through the serializable release transaction could miss
// a sighting committed after that transaction's snapshot was established.
func (s *DepartureLifecycleService) departureOnlineSighting(ctx context.Context, reader sightingReader, stripID int32) (*time.Time, error) {
	var seenAt time.Time
	err := reader.QueryRow(ctx, "SELECT seen_at FROM departure_online_sightings WHERE strip_id = $1", stripID).Scan(&seenAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &seenAt, nil
}

// departureBlockUnobservedNow loads the strip's online feed sighting and
// applies the heartbeat rule. The sighting is only read once EuroScope alone
// no longer keeps the block alive.
func (s *DepartureLifecycleService) departureBlockUnobservedNow(ctx context.Context, reader sightingReader, strip *models.Strip, assignment *models.StandAssignment, now time.Time) (bool, error) {
	if !s.departureBlockUnobserved(strip, nil, assignment, now) {
		return false, nil
	}
	onlineSeen, err := s.departureOnlineSighting(ctx, reader, strip.ID)
	if err != nil {
		return false, err
	}
	return s.departureBlockUnobserved(strip, onlineSeen, assignment, now), nil
}

// releaseUnobservedDepartureBlock frees a departure block whose aircraft no
// live source has seen within the observation timeout. The locked strip is
// rechecked so a sighting that lands after the sweep's read keeps the stand.
func (s *DepartureLifecycleService) releaseUnobservedDepartureBlock(ctx context.Context, assignment *models.StandAssignment, now time.Time) error {
	// The locked recheck needs a committed read outside the release
	// transaction. Take that connection before the transaction locks the
	// strip: sighting writers queue on the strip lock while holding their own
	// connections, so asking the pool for one under the lock could wait for a
	// connection that is never returned.
	reader, err := s.allocations.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer reader.Release()
	err = s.allocations.releaseAssignmentChecked(ctx, assignment, false, func(locked *models.Strip) error {
		if locked == nil {
			return nil
		}
		unobserved, err := s.departureBlockUnobservedNow(ctx, reader, locked, assignment, now)
		if err != nil {
			return err
		}
		if !unobserved {
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
