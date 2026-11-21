package services

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/sat"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setDepartureSightings(t *testing.T, pool *pgxpool.Pool, session int32, callsign string, euroscopeSeenAt, vatsimSeenAt *time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`UPDATE strips SET euroscope_seen_at = $1, vatsim_seen_at = $2 WHERE session = $3 AND callsign = $4`,
		euroscopeSeenAt, vatsimSeenAt, session, callsign)
	require.NoError(t, err)
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

func TestDepartureBlockUnobservedRule(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	at := func(offset time.Duration) *time.Time { value := now.Add(offset); return &value }
	lifecycle := &DepartureLifecycleService{observationTimeout: defaultDepartureObservationTimeout}
	block := func(assignedAt *time.Time) *models.StandAssignment {
		return &models.StandAssignment{Direction: string(sat.AssignmentDirectionDeparture), Stage: StageDepartureBlock, AssignedAt: assignedAt}
	}

	for _, tc := range []struct {
		name       string
		strip      models.Strip
		assignment *models.StandAssignment
		want       bool
	}{
		{"fresh EuroScope sighting", models.Strip{EuroscopeSeenAt: at(-time.Minute)}, block(at(-time.Hour)), false},
		{"silent EuroScope after a crash", models.Strip{EuroscopeSeenAt: at(-11 * time.Minute)}, block(at(-time.Hour)), true},
		{"fresh VATSIM sighting covers silent EuroScope", models.Strip{EuroscopeSeenAt: at(-time.Hour), VatsimSeenAt: at(-15 * time.Second)}, block(at(-time.Hour)), false},
		{"both sources silent", models.Strip{EuroscopeSeenAt: at(-20 * time.Minute), VatsimSeenAt: at(-12 * time.Minute)}, block(at(-time.Hour)), true},
		{"exactly at the timeout", models.Strip{EuroscopeSeenAt: at(-10 * time.Minute)}, block(nil), false},
		{"no sightings falls back to the block time", models.Strip{}, block(at(-5 * time.Minute)), false},
		{"no sightings and an old block", models.Strip{}, block(at(-11 * time.Minute)), true},
		{"no evidence at all", models.Strip{}, block(nil), true},
		{"reservation is not a block", models.Strip{}, &models.StandAssignment{Direction: string(sat.AssignmentDirectionDeparture), Stage: StageReserved}, false},
		{"arrival is not a departure block", models.Strip{}, &models.StandAssignment{Direction: string(sat.AssignmentDirectionArrival), Stage: StageDepartureBlock}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			strip := tc.strip
			assert.Equal(t, tc.want, lifecycle.departureBlockUnobserved(&strip, tc.assignment, now))
		})
	}
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

	t.Run("live VATSIM sightings keep the stand while EuroScope is silent", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		const callsign = "SASOBS2"
		testdata.SeedTestStrip(t, queries, session, callsign)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlight(callsign, 1)))
		euroscopeSeen := clock.current()
		for i := 0; i < 6; i++ {
			clock.advance(5 * time.Minute)
			vatsimSeen := clock.current().Add(-15 * time.Second)
			setDepartureSightings(t, pool, session, callsign, &euroscopeSeen, &vatsimSeen)
			require.NoError(t, lifecycle.ReleaseExpired(ctx))
		}
		_, err := assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err, "the feed still sees the pilot at the gate")
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
		assert.False(t, lifecycle.departureBlockUnobserved(loadStrip(t, strips, session, callsign), reservation, clock.current().Add(time.Hour)))
	})
}
