package services

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/vatsim"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setDepartureSightings writes both heartbeat inputs: the EuroScope presence
// timestamp and the last online VATSIM feed sighting. A nil value clears it.
func setDepartureSightings(t *testing.T, pool *pgxpool.Pool, session int32, callsign string, euroscopeSeenAt, onlineSeenAt *time.Time) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `UPDATE strips SET euroscope_seen_at = $1 WHERE session = $2 AND callsign = $3`,
		euroscopeSeenAt, session, callsign)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `DELETE FROM departure_online_sightings USING strips
		WHERE departure_online_sightings.strip_id = strips.id AND strips.session = $1 AND strips.callsign = $2`, session, callsign)
	require.NoError(t, err)
	if onlineSeenAt != nil {
		_, err = pool.Exec(ctx, `INSERT INTO departure_online_sightings (strip_id, seen_at)
			SELECT id, $3 FROM strips WHERE session = $1 AND callsign = $2`, session, callsign, *onlineSeenAt)
		require.NoError(t, err)
	}
}

// seenByEuroScope and seenByFeed record the live sighting that production
// sources write while an aircraft stays connected, so tests that advance the
// clock with a live aircraft at the gate do not trip the observation timeout.
func seenByEuroScope(t *testing.T, pool *pgxpool.Pool, session int32, at time.Time, callsigns ...string) {
	t.Helper()
	for _, callsign := range callsigns {
		setDepartureSightings(t, pool, session, callsign, &at, nil)
	}
}

func seenByFeed(t *testing.T, pool *pgxpool.Pool, session int32, at time.Time, callsigns ...string) {
	t.Helper()
	for _, callsign := range callsigns {
		setDepartureSightings(t, pool, session, callsign, nil, &at)
	}
}

// onlineFlightSeenAt is an online feed record whose pilot was last updated at seenAt.
func onlineFlightSeenAt(callsign string, revision int64, seenAt time.Time) vatsim.DepartureFlightInfo {
	flight := onlineFlight(callsign, revision)
	flight.SeenAt = seenAt
	return flight
}

func TestDepartureBlockUnobservedRule(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	at := func(offset time.Duration) *time.Time { value := now.Add(offset); return &value }
	lifecycle := &DepartureLifecycleService{observationTimeout: defaultDepartureObservationTimeout}
	block := func(assignedAt *time.Time) *models.StandAssignment {
		return &models.StandAssignment{Direction: string(sat.AssignmentDirectionDeparture), Stage: StageDepartureBlock, AssignedAt: assignedAt}
	}
	graceBlock := func(expiresAt *time.Time) *models.StandAssignment {
		assignment := block(at(-time.Hour))
		assignment.ExpiresAt = expiresAt
		return assignment
	}

	for _, tc := range []struct {
		name       string
		strip      models.Strip
		onlineSeen *time.Time
		assignment *models.StandAssignment
		want       bool
	}{
		{"fresh EuroScope sighting", models.Strip{EuroscopeSeenAt: at(-time.Minute)}, nil, block(at(-time.Hour)), false},
		{"silent EuroScope after a crash", models.Strip{EuroscopeSeenAt: at(-11 * time.Minute)}, nil, block(at(-time.Hour)), true},
		{"fresh online feed sighting covers silent EuroScope", models.Strip{EuroscopeSeenAt: at(-time.Hour)}, at(-15 * time.Second), block(at(-time.Hour)), false},
		{"both sources silent", models.Strip{EuroscopeSeenAt: at(-20 * time.Minute)}, at(-12 * time.Minute), block(at(-time.Hour)), true},
		{"prefile refreshing vatsim_seen_at is not a sighting", models.Strip{EuroscopeSeenAt: at(-20 * time.Minute), VatsimSeenAt: at(-time.Second)}, nil, block(at(-time.Hour)), true},
		{"exactly at the timeout", models.Strip{EuroscopeSeenAt: at(-10 * time.Minute)}, nil, block(nil), false},
		{"no sightings falls back to the block time", models.Strip{}, nil, block(at(-5 * time.Minute)), false},
		{"no sightings and an old block", models.Strip{}, nil, block(at(-11 * time.Minute)), true},
		{"no evidence at all", models.Strip{}, nil, block(nil), true},
		{"running disconnect grace is left to the disconnect path", models.Strip{}, nil, graceBlock(at(20 * time.Second)), false},
		{"elapsed disconnect grace follows the heartbeat", models.Strip{}, nil, graceBlock(at(-time.Second)), true},
		{"reservation is not a block", models.Strip{}, nil, &models.StandAssignment{Direction: string(sat.AssignmentDirectionDeparture), Stage: StageReserved}, false},
		{"arrival is not a departure block", models.Strip{}, nil, &models.StandAssignment{Direction: string(sat.AssignmentDirectionArrival), Stage: StageDepartureBlock}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			strip := tc.strip
			assert.Equal(t, tc.want, lifecycle.departureBlockUnobserved(&strip, tc.onlineSeen, tc.assignment, now))
		})
	}
}

func TestStaleOnlineFeedSighting(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	lifecycle := &DepartureLifecycleService{observationTimeout: defaultDepartureObservationTimeout, now: func() time.Time { return now }}
	assert.False(t, lifecycle.staleOnlineFeedSighting(onlineFlightSeenAt("SAS1", 1, now.Add(-time.Minute))))
	assert.True(t, lifecycle.staleOnlineFeedSighting(onlineFlightSeenAt("SAS1", 1, now.Add(-11*time.Minute))))
	assert.False(t, lifecycle.staleOnlineFeedSighting(onlineFlight("SAS1", 1)), "an unknown sighting time is not proof of staleness")
	assert.False(t, lifecycle.staleOnlineFeedSighting(offlineFlight("SAS1", 1)), "prefiles are not online sightings")
}

func TestDepartureBlockObservationTimeout(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()

	t.Run("silent EuroScope presence no longer holds the stand", func(t *testing.T) {
		// A crashed EuroScope never reports the disconnect, so the presence flag
		// stays set while its timestamp stops advancing.
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		const callsign = "SASOBS1"
		testdata.SeedTestStrip(t, queries, session, callsign)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlight(callsign, 1)))
		lastSeen := clock.current()
		setDepartureSightings(t, pool, session, callsign, &lastSeen, nil)

		clock.advance(9 * time.Minute)
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		block, err := assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err, "a recently seen aircraft keeps its stand")
		assert.Nil(t, block.ExpiresAt, "the heartbeat must not publish an occupancy end")

		clock.advance(2 * time.Minute)
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		_, err = assignments.GetAssignment(ctx, session, callsign)
		require.Error(t, err, "an aircraft unseen past the timeout releases its stand")
		strip := loadStrip(t, strips, session, callsign)
		require.NotNil(t, strip.Stand)
		assert.Equal(t, "A1", *strip.Stand, "keep the origin stand for downstream routing")
		assert.NotNil(t, strip.EuroscopeSeenAt, "the release does not rewrite EuroScope presence")
	})

	t.Run("online feed sightings keep the stand while EuroScope is silent", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		const callsign = "SASOBS2"
		testdata.SeedTestStrip(t, queries, session, callsign)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlight(callsign, 1)))
		euroscopeSeen := clock.current()
		setDepartureSightings(t, pool, session, callsign, &euroscopeSeen, nil)
		for i := 0; i < 6; i++ {
			clock.advance(5 * time.Minute)
			// Each feed poll records the pilot's last_updated through the lifecycle.
			flight := onlineFlightSeenAt(callsign, 1, clock.current().Add(-15*time.Second))
			require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), flight))
			require.NoError(t, lifecycle.ReleaseExpired(ctx))
		}
		_, err := assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err, "the feed still sees the pilot online at the gate")
	})

	t.Run("prefile refreshes of vatsim_seen_at do not hold the stand", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		const callsign = "SASOBS6"
		testdata.SeedTestStrip(t, queries, session, callsign)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlight(callsign, 1)))
		euroscopeSeen := clock.current()
		setDepartureSightings(t, pool, session, callsign, &euroscopeSeen, nil)
		clock.advance(11 * time.Minute)
		// A prefile without last_updated advances vatsim_seen_at to each snapshot time.
		_, err := pool.Exec(ctx, `UPDATE strips SET vatsim_seen_at = $1 WHERE session = $2 AND callsign = $3`, clock.current(), session, callsign)
		require.NoError(t, err)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), offlineFlight(callsign, 1)))
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		_, err = assignments.GetAssignment(ctx, session, callsign)
		require.Error(t, err, "only online feed sightings count as live evidence")
	})

	t.Run("disconnect cleared presence in a session without the VATSIM feed", func(t *testing.T) {
		// The retainer keeps the strip because its block has no expiry, and no
		// reconciler ever starts the disconnect grace outside LIVE sessions.
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		const callsign = "SASOBS3"
		testdata.SeedTestStrip(t, queries, session, callsign)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlight(callsign, 1)))
		setDepartureSightings(t, pool, session, callsign, nil, nil)

		clock.advance(5 * time.Minute)
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		_, err := assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err, "the block's own assignment time counts as the last sighting")

		clock.advance(6 * time.Minute)
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		_, err = assignments.GetAssignment(ctx, session, callsign)
		require.Error(t, err)
	})

	t.Run("running disconnect grace still lets a EuroScope reconnect keep the stand", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		const callsign = "SASOBS7"
		testdata.SeedTestStrip(t, queries, session, callsign)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlight(callsign, 1)))
		// The aircraft sat at the gate long past the timeout, then the disconnect
		// worker cleared its EuroScope presence and the grace began.
		clock.advance(20 * time.Minute)
		setDepartureSightings(t, pool, session, callsign, nil, nil)
		require.NoError(t, lifecycle.CancelDeparture(ctx, session, callsign))
		pending, err := assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err)
		require.NotNil(t, pending.ExpiresAt)

		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		_, err = assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err, "the heartbeat must not cut a running disconnect grace short")

		clock.advance(10 * time.Second)
		require.NoError(t, strips.MarkEuroscopeSeen(ctx, session, callsign))
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		reconnected, err := assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err, "a reconnect within the grace keeps the stand")
		assert.Nil(t, reconnected.ExpiresAt)
	})

	t.Run("a stale cached snapshot does not recreate a released block", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		const callsign = "SASOBS8"
		testdata.SeedTestStrip(t, queries, session, callsign)
		cached := onlineFlightSeenAt(callsign, 1, clock.current())
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), cached))
		euroscopeSeen := clock.current()
		setDepartureSightings(t, pool, session, callsign, &euroscopeSeen, &euroscopeSeen)

		clock.advance(11 * time.Minute)
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		_, err := assignments.GetAssignment(ctx, session, callsign)
		require.Error(t, err, "neither source has seen the aircraft for the timeout")

		// The feed stopped refreshing, so the reconciler replays the same snapshot.
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), cached))
		_, err = assignments.GetAssignment(ctx, session, callsign)
		require.Error(t, err, "the sighting that expired the block must not recreate it")

		fresh := onlineFlightSeenAt(callsign, 1, clock.current())
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), fresh))
		_, err = assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err, "a fresh online sighting at the gate recreates the block")
	})

	t.Run("every newer online sighting postpones the release", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		const callsign = "SASOBS9"
		testdata.SeedTestStrip(t, queries, session, callsign)
		first := clock.current()
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlightSeenAt(callsign, 1, first)))
		clock.advance(20 * time.Second)
		latest := clock.current()
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlightSeenAt(callsign, 1, latest)))
		// A replayed older record must not move the sighting backwards.
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlightSeenAt(callsign, 1, first)))

		strip := loadStrip(t, strips, session, callsign)
		stored, err := lifecycle.departureOnlineSighting(ctx, strip.ID)
		require.NoError(t, err)
		require.NotNil(t, stored)
		assert.True(t, stored.Equal(latest), "stored %s, want %s", stored, latest)

		clock.set(first.Add(10*time.Minute + time.Second))
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		_, err = assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err, "the aircraft was seen 9m41s ago")

		clock.set(latest.Add(10*time.Minute + time.Second))
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		_, err = assignments.GetAssignment(ctx, session, callsign)
		require.Error(t, err)
	})

	t.Run("a sighting that overtakes the sweep read keeps the stand", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		const callsign = "SASOBS4"
		testdata.SeedTestStrip(t, queries, session, callsign)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlight(callsign, 1)))
		lastSeen := clock.current()
		setDepartureSightings(t, pool, session, callsign, &lastSeen, nil)
		clock.advance(11 * time.Minute)
		observed := false
		lifecycle.strips = &departureReconnectStripRepository{
			StripRepository: strips, callsign: callsign,
			afterRead: func(ctx context.Context, session int32, callsign string) error {
				observed = true
				seen := clock.current()
				setDepartureSightings(t, pool, session, callsign, &seen, nil)
				return nil
			},
		}
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		require.True(t, observed)
		_, err := assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err, "the locked strip must protect a sighting that lands after the first read")
	})

	t.Run("reservations are not departure blocks", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		const callsign = "SASOBS5"
		testdata.SeedTestStrip(t, queries, session, callsign)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), offlineFlight(callsign, 1)))
		reservation, err := assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err)
		require.Equal(t, StageReserved, reservation.Stage)
		assert.False(t, lifecycle.departureBlockUnobserved(loadStrip(t, strips, session, callsign), nil, reservation, clock.current().Add(time.Hour)))
	})
}
