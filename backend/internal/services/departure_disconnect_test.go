package services

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/sat"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type departureReconnectStripRepository struct {
	repository.StripRepository
	callsign  string
	afterRead func(context.Context, int32, string) error
}

func (r *departureReconnectStripRepository) GetByCallsign(ctx context.Context, session int32, callsign string) (*models.Strip, error) {
	strip, err := r.StripRepository.GetByCallsign(ctx, session, callsign)
	if err == nil && callsign == r.callsign && r.afterRead != nil {
		hook := r.afterRead
		r.afterRead = nil
		if err := hook(ctx, session, callsign); err != nil {
			return nil, err
		}
	}
	return strip, err
}

func TestDepartureDisconnectLifecycle(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()

	for _, withPosition := range []bool{false, true} {
		name := "stand text only"
		if withPosition {
			name = "stale gate position"
		}
		t.Run("both sources absent with "+name, func(t *testing.T) {
			lifecycle, allocations, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
			const callsign = "SASDISC1"
			testdata.SeedTestStrip(t, queries, session, callsign)
			require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlight(callsign, 1)))
			if withPosition {
				_, err := pool.Exec(ctx, `UPDATE strips SET position_latitude = $1, position_longitude = $2 WHERE session = $3 AND callsign = $4`, 55.6285306, 12.642625, session, callsign)
				require.NoError(t, err)
			}

			require.NoError(t, lifecycle.CancelDeparture(ctx, session, callsign))
			pending, err := assignments.GetAssignment(ctx, session, callsign)
			require.NoError(t, err)
			require.NotNil(t, pending.ExpiresAt)
			deadline := *pending.ExpiresAt
			assert.Equal(t, clock.current().Add(30*time.Second).UTC(), deadline.UTC())

			// Neither repeated feed polls, stale stand evidence, nor a backend
			// restart may keep extending or erase the disconnect deadline.
			restarted, err := NewDepartureLifecycleService(allocations, assignments, strips, lifecycle.sessions, lifecycle.stands, nil, nil, nil, WithDepartureLifecycleClock(clock.current))
			require.NoError(t, err)
			for i := 0; i < 9; i++ {
				clock.advance(3 * time.Second)
				require.NoError(t, restarted.CancelDeparture(ctx, session, callsign))
				require.NoError(t, restarted.ReleaseExpired(ctx))
				current, err := assignments.GetAssignment(ctx, session, callsign)
				require.NoError(t, err)
				require.NotNil(t, current.ExpiresAt)
				assert.Equal(t, deadline, *current.ExpiresAt)
				assert.Equal(t, pending.Version, current.Version)
			}
			clock.advance(3 * time.Second)
			require.NoError(t, restarted.ReleaseExpired(ctx))
			_, err = assignments.GetAssignment(ctx, session, callsign)
			require.Error(t, err)
			assert.Equal(t, "A1", *loadStrip(t, strips, session, callsign).Stand, "keep the origin stand for downstream routing")
			require.NoError(t, restarted.CancelDeparture(ctx, session, callsign))
		})
	}

	t.Run("returning feed at gate cancels disconnect expiry without EuroScope", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		const callsign = "SASDISC2"
		testdata.SeedTestStrip(t, queries, session, callsign)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlight(callsign, 1)))
		require.NoError(t, lifecycle.CancelDeparture(ctx, session, callsign))
		clock.advance(10 * time.Second)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlight(callsign, 1)))
		clock.advance(time.Hour)
		seenByFeed(t, pool, session, clock.current(), callsign)
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		assignment, err := assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err)
		assert.Nil(t, assignment.ExpiresAt)
	})

	t.Run("remaining prefile does not retain a disconnected departure block", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		lifecycle.allowPrefiles = false
		const callsign = "SASDISC6"
		testdata.SeedTestStrip(t, queries, session, callsign)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlight(callsign, 1)))
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), offlineFlight(callsign, 1)))
		pending, err := assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err)
		require.NotNil(t, pending.ExpiresAt)
		clock.advance(30 * time.Second)
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		_, err = assignments.GetAssignment(ctx, session, callsign)
		require.Error(t, err)
	})

	for _, tc := range []struct {
		name, conflictPrefix string
		warningAvailable     bool
	}{
		{name: "no wrong-stand conflict"},
		{name: "warning awaiting delivery", conflictPrefix: wrongStandAwaitingPrefix},
		{name: "warning already delivered", conflictPrefix: wrongStandPendingPrefix, warningAvailable: true},
	} {
		t.Run("enabled prefiles preserve disconnect grace with "+tc.name, func(t *testing.T) {
			lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
			lifecycle.allowPrefiles = true
			const callsign = "SASDISC10"
			testdata.SeedTestStrip(t, queries, session, callsign)
			if tc.conflictPrefix != "" {
				lifecycle.SetWrongStandMessenger(&wrongStandTestMessenger{available: tc.warningAvailable})
				createProtectedStandOccupant(t, assignments, session, "SASDISC10O", "A2")
				require.NoError(t, lifecycle.ObserveDeparturePosition(ctx, session, loadStrip(t, strips, session, callsign), 55.6285306, 12.6434583))
			} else {
				require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlight(callsign, 1)))
			}
			original, err := assignments.GetAssignment(ctx, session, callsign)
			require.NoError(t, err)
			require.Equal(t, StageDepartureBlock, original.Stage)
			if tc.conflictPrefix != "" {
				require.NotNil(t, original.ConflictReason)
				require.Equal(t, tc.conflictPrefix+"A2", *original.ConflictReason)
			}
			deadline := clock.current().Add(30 * time.Second)
			require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), offlineFlight(callsign, 1)))
			pending, err := assignments.GetAssignment(ctx, session, callsign)
			require.NoError(t, err)
			require.NotNil(t, pending.ExpiresAt)
			assert.Equal(t, deadline.UTC(), pending.ExpiresAt.UTC(), "wrong-stand cleanup must not replace the disconnect deadline with a reservation hold")
			assert.Nil(t, pending.ConflictReason)
			assert.Equal(t, original.ID, pending.ID)
			clock.advance(15 * time.Second)
			require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), offlineFlight(callsign, 1)))
			require.NoError(t, lifecycle.ReleaseExpired(ctx))
			unchanged, err := assignments.GetAssignment(ctx, session, callsign)
			require.NoError(t, err)
			assert.Equal(t, pending.Version, unchanged.Version)
			require.NotNil(t, unchanged.ExpiresAt)
			assert.Equal(t, deadline.UTC(), unchanged.ExpiresAt.UTC())
			clock.advance(15 * time.Second)
			require.NoError(t, lifecycle.ReleaseExpired(ctx))
			_, err = assignments.GetAssignment(ctx, session, callsign)
			require.Error(t, err)
		})
	}

	t.Run("returning feed without coordinates cancels disconnect expiry", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		const callsign = "SASDISC8"
		testdata.SeedTestStrip(t, queries, session, callsign)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlight(callsign, 1)))
		require.NoError(t, lifecycle.CancelDeparture(ctx, session, callsign))
		returning := onlineFlight(callsign, 1)
		returning.Latitude, returning.Longitude = 0, 0
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), returning))
		clock.advance(time.Hour)
		seenByFeed(t, pool, session, clock.current(), callsign)
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		assignment, err := assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err)
		assert.Nil(t, assignment.ExpiresAt)
	})

	for _, warning := range []struct {
		name, initialPrefix                   string
		initiallyAvailable, availableOnReturn bool
	}{
		{name: "awaiting warning", initialPrefix: wrongStandAwaitingPrefix},
		{name: "warning becomes available", initialPrefix: wrongStandAwaitingPrefix, availableOnReturn: true},
		{name: "warning delivered", initialPrefix: wrongStandPendingPrefix, initiallyAvailable: true, availableOnReturn: true},
	} {
		for _, source := range []string{"VATSIM", "EuroScope"} {
			t.Run("wrong-stand reconnect via "+source+" with "+warning.name, func(t *testing.T) {
				lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
				lifecycle.allowPrefiles = false
				messenger := &wrongStandTestMessenger{available: warning.initiallyAvailable}
				lifecycle.SetWrongStandMessenger(messenger)
				const callsign = "SASDISC9"
				testdata.SeedTestStrip(t, queries, session, callsign)
				createProtectedStandOccupant(t, assignments, session, "SASDISC9O", "A2")
				require.NoError(t, lifecycle.ObserveDeparturePosition(ctx, session, loadStrip(t, strips, session, callsign), 55.6285306, 12.6434583))
				original, err := assignments.GetAssignment(ctx, session, callsign)
				require.NoError(t, err)
				require.Equal(t, StageDepartureBlock, original.Stage)
				require.Equal(t, "A1", original.Stand)
				require.NotNil(t, original.ConflictReason)
				require.Equal(t, warning.initialPrefix+"A2", *original.ConflictReason)

				require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), offlineFlight(callsign, 1)))
				pending, err := assignments.GetAssignment(ctx, session, callsign)
				require.NoError(t, err)
				require.NotNil(t, pending.ExpiresAt)
				clock.advance(10 * time.Second)
				messenger.available = warning.availableOnReturn
				warningAttempts := messenger.calls
				observe := func() error {
					strip := loadStrip(t, strips, session, callsign)
					if source == "EuroScope" {
						return lifecycle.ObserveDeparturePosition(ctx, session, strip, 55.6285306, 12.6434583)
					}
					return lifecycle.ProcessDeparture(ctx, session, strip, onlineFlightAtA2(callsign, 1))
				}
				require.NoError(t, observe())
				resumed, err := assignments.GetAssignment(ctx, session, callsign)
				require.NoError(t, err)
				require.Nil(t, resumed.ExpiresAt, "a live mismatch must cancel disconnect expiry before the early return")
				assert.Equal(t, original.ID, resumed.ID)
				assert.Equal(t, original.Stand, resumed.Stand)
				require.NotNil(t, resumed.ConflictReason)
				prefix := warning.initialPrefix
				if warning.availableOnReturn {
					prefix = wrongStandPendingPrefix
				}
				assert.Equal(t, prefix+"A2", *resumed.ConflictReason)
				if warning.initiallyAvailable {
					assert.Equal(t, warningAttempts, messenger.calls, "do not resend a delivered warning")
				} else {
					assert.Equal(t, warningAttempts+1, messenger.calls, "the pending warning must use the updated assignment version")
				}
				require.NoError(t, observe())
				unchanged, err := assignments.GetAssignment(ctx, session, callsign)
				require.NoError(t, err)
				assert.Equal(t, resumed.Version, unchanged.Version)
				clock.advance(time.Minute)
				require.NoError(t, lifecycle.ReleaseExpired(ctx))
				_, err = assignments.GetAssignment(ctx, session, callsign)
				require.NoError(t, err, "the returned aircraft retains its assignment past the old disconnect deadline")
			})
		}
	}

	t.Run("returning feed movement releases stand immediately", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, _ := departureLifecycleFixture(t, pool, queries, "", "", nil)
		const callsign = "SASDISC7"
		testdata.SeedTestStrip(t, queries, session, callsign)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlight(callsign, 1)))
		require.NoError(t, lifecycle.CancelDeparture(ctx, session, callsign))
		moving := onlineFlight(callsign, 1)
		moving.Latitude, moving.Longitude = 56, 13
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), moving))
		_, err := assignments.GetAssignment(ctx, session, callsign)
		require.Error(t, err)
		assert.Equal(t, "A1", *loadStrip(t, strips, session, callsign).Stand)
	})

	t.Run("fresh assigned-stand observation overrides stale wrong-stand position", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		lifecycle.allowPrefiles = false
		const callsign = "SASDISC11"
		testdata.SeedTestStrip(t, queries, session, callsign)
		createProtectedStandOccupant(t, assignments, session, "SASDISC11O", "A2")
		require.NoError(t, lifecycle.ObserveDeparturePosition(ctx, session, loadStrip(t, strips, session, callsign), 55.6285306, 12.6434583))
		_, err := pool.Exec(ctx, `UPDATE strips SET position_latitude = $1, position_longitude = $2 WHERE session = $3 AND callsign = $4`, 55.6285306, 12.6434583, session, callsign)
		require.NoError(t, err)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), offlineFlight(callsign, 1)))
		pending, err := assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err)
		require.NotNil(t, pending.ExpiresAt)
		require.NotNil(t, pending.ConflictReason)
		stale := loadStrip(t, strips, session, callsign)
		require.False(t, lifecycle.stripIsAtAssignedStand(stale, pending.Stand))
		clock.advance(10 * time.Second)
		require.NoError(t, lifecycle.ObserveDeparturePosition(ctx, session, stale, 55.6285306, 12.642625))
		resumed, err := assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err)
		require.Nil(t, resumed.ExpiresAt, "use the fresh observation rather than the pre-update strip coordinates")
		assert.Nil(t, resumed.ConflictReason)
		require.NotNil(t, resumed.ObservedStand)
		assert.Equal(t, "A1", *resumed.ObservedStand)
		assert.Equal(t, pending.ID, resumed.ID)
		clock.advance(time.Minute)
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		_, err = assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err)
	})

	t.Run("reconnect between sweep read and release retains the stand", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		const callsign = "SASDISC12"
		testdata.SeedTestStrip(t, queries, session, callsign)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlight(callsign, 1)))
		require.NoError(t, lifecycle.CancelDeparture(ctx, session, callsign))
		clock.advance(30 * time.Second)
		reconnected := false
		lifecycle.strips = &departureReconnectStripRepository{
			StripRepository: strips, callsign: callsign,
			afterRead: func(ctx context.Context, session int32, callsign string) error {
				reconnected = true
				return strips.MarkEuroscopeSeen(ctx, session, callsign)
			},
		}
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		require.True(t, reconnected)
		assignment, err := assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err, "the locked strip must protect a reconnect that overtakes the sweep's first read")
		assert.Nil(t, assignment.ExpiresAt, "the sweep should retry and cancel expiry after seeing the reconnect")
	})

	t.Run("connected aircraft movement still releases stand immediately", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, _ := departureLifecycleFixture(t, pool, queries, "", "", nil)
		const callsign = "SASDISC13"
		testdata.SeedTestStrip(t, queries, session, callsign)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlight(callsign, 1)))
		require.NoError(t, strips.MarkEuroscopeSeen(ctx, session, callsign))
		moving := onlineFlight(callsign, 1)
		moving.Latitude, moving.Longitude = 56, 13
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), moving))
		_, err := assignments.GetAssignment(ctx, session, callsign)
		require.Error(t, err)
	})

	t.Run("EuroScope reconnect cancels disconnect expiry", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		const callsign = "SASDISC3"
		testdata.SeedTestStrip(t, queries, session, callsign)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlight(callsign, 1)))
		require.NoError(t, lifecycle.CancelDeparture(ctx, session, callsign))
		require.NoError(t, strips.MarkEuroscopeSeen(ctx, session, callsign))
		clock.advance(6 * time.Minute)
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		assignment, err := assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err)
		assert.Nil(t, assignment.ExpiresAt)
	})

	t.Run("EuroScope presence protects departure during feed loss", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		const callsign = "SASDISC4"
		testdata.SeedTestStrip(t, queries, session, callsign)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlight(callsign, 1)))
		seenByEuroScope(t, pool, session, clock.current(), callsign)
		require.NoError(t, lifecycle.CancelDeparture(ctx, session, callsign))
		clock.advance(time.Hour)
		seenByEuroScope(t, pool, session, clock.current(), callsign)
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		assignment, err := assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err)
		assert.Nil(t, assignment.ExpiresAt)
	})

	t.Run("arrival assignments retain their own disconnect policy", func(t *testing.T) {
		lifecycle, _, session, assignments, _, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		const callsign = "SASDISC5"
		testdata.SeedTestStrip(t, queries, session, callsign)
		require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{SessionID: session, Callsign: callsign, Stand: "A1", Direction: string(sat.AssignmentDirectionArrival), Stage: StageEstimated, Source: "AUTOMATIC"}))
		require.NoError(t, lifecycle.CancelDeparture(ctx, session, callsign))
		clock.advance(time.Hour)
		assignment, err := assignments.GetAssignment(ctx, session, callsign)
		require.NoError(t, err)
		assert.Nil(t, assignment.ExpiresAt)
	})
}
