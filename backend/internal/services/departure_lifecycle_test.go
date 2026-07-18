package services

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/vatsim"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) current() time.Time      { return c.now }
func (c *fakeClock) set(value time.Time)     { c.now = value }
func (c *fakeClock) advance(d time.Duration) { c.now = c.now.Add(d) }

type wrongStandTestMessenger struct {
	available bool
	calls     int
	message   string
}

func (m *wrongStandTestMessenger) SendPrivateMessageFromDelivery(_ int32, _ string, message string) bool {
	m.calls++
	m.message = message
	return m.available
}

func TestDepartureLifecycle(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()

	t.Run("offline prefile allocates a 15-minute reservation", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		testdata.SeedTestStrip(t, queries, session, "SAS101")
		clock.set(time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC))

		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS101"), offlineFlight("SAS101", 1)))

		assignment, err := assignments.GetAssignment(ctx, session, "SAS101")
		require.NoError(t, err)
		assert.Equal(t, StageReserved, assignment.Stage)
		assert.Equal(t, "A1", assignment.Stand)
		require.NotNil(t, assignment.ExpiresAt)
		assert.Equal(t, clock.current().Add(15*time.Minute).UTC(), assignment.ExpiresAt.UTC())
		require.NotNil(t, assignment.VatsimRevision)
		assert.Equal(t, int64(1), *assignment.VatsimRevision)
	})

	t.Run("local EuroScope departure occupies its observed stand", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, _ := departureLifecycleFixture(t, pool, queries, "", "", nil)
		testdata.SeedTestStrip(t, queries, session, "SAS102")
		strip := loadStrip(t, strips, session, "SAS102")

		require.NoError(t, lifecycle.ObserveDeparturePosition(ctx, session, strip, 55.6285306, 12.642625))

		assignment, err := assignments.GetAssignment(ctx, session, "SAS102")
		require.NoError(t, err)
		assert.Equal(t, StageDepartureBlock, assignment.Stage)
		assert.Equal(t, "A1", assignment.Stand)
		assert.Nil(t, assignment.VatsimCID)
		assert.Nil(t, assignment.VatsimRevision)

		// A CDM update must not reintroduce an expiry while the aircraft occupies A1.
		tsat := "1030"
		_, err = strips.SetCdmData(ctx, session, "SAS102", &models.CdmData{Tsat: &tsat})
		require.NoError(t, err)
		require.NoError(t, lifecycle.ObserveDeparturePosition(ctx, session,
			loadStrip(t, strips, session, "SAS102"), 55.6285306, 12.642625))

		updated, err := assignments.GetAssignment(ctx, session, "SAS102")
		require.NoError(t, err)
		assert.Nil(t, updated.ExpiresAt)
		assert.Nil(t, updated.VatsimCID)
		assert.Nil(t, updated.VatsimRevision)
	})

	t.Run("repeated feed polls do not extend or reshuffle the reservation", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		testdata.SeedTestStrip(t, queries, session, "SAS201")
		clock.set(time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC))
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS201"), offlineFlight("SAS201", 1)))

		original, err := assignments.GetAssignment(ctx, session, "SAS201")
		require.NoError(t, err)

		clock.advance(5 * time.Minute)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS201"), offlineFlight("SAS201", 1)))

		repeated, err := assignments.GetAssignment(ctx, session, "SAS201")
		require.NoError(t, err)
		assert.Equal(t, original.ExpiresAt.UTC(), repeated.ExpiresAt.UTC(), "expiry must not move on a repeated poll")
		assert.Equal(t, original.Stand, repeated.Stand)
		assert.Equal(t, original.Version, repeated.Version)
	})

	t.Run("a later revision renews the reservation in place", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		testdata.SeedTestStrip(t, queries, session, "SAS301")
		clock.set(time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC))
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS301"), offlineFlight("SAS301", 1)))

		clock.advance(5 * time.Minute)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS301"), offlineFlight("SAS301", 2)))

		renewed, err := assignments.GetAssignment(ctx, session, "SAS301")
		require.NoError(t, err)
		assert.Equal(t, StageReserved, renewed.Stage)
		assert.Equal(t, "A1", renewed.Stand, "renewal keeps the same stand when it remains free")
		require.NotNil(t, renewed.ExpiresAt)
		assert.Equal(t, clock.current().Add(15*time.Minute).UTC(), renewed.ExpiresAt.UTC())
		require.NotNil(t, renewed.VatsimRevision)
		assert.Equal(t, int64(2), *renewed.VatsimRevision)
	})

	t.Run("renewal reallocates when the stand becomes unavailable", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		testdata.SeedTestStrip(t, queries, session, "SAS401")
		clock.set(time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC))
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS401"), offlineFlight("SAS401", 1)))

		require.NoError(t, assignments.CreateBlock(ctx, &models.StandBlock{
			SessionID: session, Stand: "A1", BlockType: "CLOSURE", Source: "CONTROLLER", Manual: true,
		}))

		clock.advance(5 * time.Minute)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS401"), offlineFlight("SAS401", 2)))

		reallocated, err := assignments.GetAssignment(ctx, session, "SAS401")
		require.NoError(t, err)
		assert.Equal(t, StageReserved, reallocated.Stage)
		assert.Equal(t, "A2", reallocated.Stand, "the unavailable stand is replaced by a fresh 15-minute hold on A2")
		require.NotNil(t, reallocated.ExpiresAt)
		assert.Equal(t, clock.current().Add(15*time.Minute).UTC(), reallocated.ExpiresAt.UTC())
	})

	t.Run("expired offline reservations are released idempotently", func(t *testing.T) {
		lifecycle, allocations, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		var published []StandAllocationResult
		allocations.SetPublisher(func(_ context.Context, result StandAllocationResult) error {
			published = append(published, result)
			return nil
		})
		testdata.SeedTestStrip(t, queries, session, "SAS501")
		clock.set(time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC))
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS501"), offlineFlight("SAS501", 1)))
		published = nil
		strip := loadStrip(t, strips, session, "SAS501")
		require.NotNil(t, strip.Stand)

		clock.advance(16 * time.Minute)
		require.NoError(t, lifecycle.ReleaseExpired(ctx))

		_, err := assignments.GetAssignment(ctx, session, "SAS501")
		require.Error(t, err, "the reservation is removed once it expires")
		updated := loadStrip(t, strips, session, "SAS501")
		require.Nil(t, updated.Stand, "the operational stand is cleared")
		var matching []StandAllocationResult
		for _, result := range published {
			if result.Assignment.SessionID == session && result.Assignment.Callsign == "SAS501" {
				matching = append(matching, result)
			}
		}
		require.Len(t, matching, 1)
		assert.True(t, matching[0].Removed, "the lifecycle publishes the removal after it commits")

		publishedBeforeSecondSweep := len(published)
		require.NoError(t, lifecycle.ReleaseExpired(ctx), "a second sweep is a no-op")
		assert.Len(t, published, publishedBeforeSecondSweep, "an idempotent second sweep does not publish another removal")
	})

	t.Run("coming online converts the reservation to a departure block", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		testdata.SeedTestStrip(t, queries, session, "SAS601")
		clock.set(time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC))
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS601"), offlineFlight("SAS601", 1)))

		tsat := "1030"
		_, err := strips.SetCdmData(ctx, session, "SAS601", &models.CdmData{Tsat: &tsat})
		require.NoError(t, err)

		clock.advance(2 * time.Minute)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS601"), onlineFlight("SAS601", 1)))

		block, err := assignments.GetAssignment(ctx, session, "SAS601")
		require.NoError(t, err)
		assert.Equal(t, StageDepartureBlock, block.Stage)
		assert.Equal(t, "A1", block.Stand)
		assert.Nil(t, block.ExpiresAt, "an online aircraft on its assigned stand has no block expiry")
	})

	t.Run("coming online at a different free stand blocks the observed stand", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		testdata.SeedTestStrip(t, queries, session, "SAS602")
		clock.set(time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC))
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS602"), offlineFlight("SAS602", 1)))

		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS602"), onlineFlightAtA2("SAS602", 1)))

		block, err := assignments.GetAssignment(ctx, session, "SAS602")
		require.NoError(t, err)
		assert.Equal(t, StageDepartureBlock, block.Stage)
		assert.Equal(t, "A2", block.Stand)
	})

	t.Run("occupied observed stand records a task 19 mismatch without blocking the reserved stand", func(t *testing.T) {
		lifecycle, allocations, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		var published []StandAllocationResult
		allocations.SetPublisher(func(_ context.Context, result StandAllocationResult) error {
			published = append(published, result)
			return nil
		})
		testdata.SeedTestStrip(t, queries, session, "SAS603")
		clock.set(time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC))
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS603"), offlineFlight("SAS603", 1)))
		require.NoError(t, assignments.CreateBlock(ctx, &models.StandBlock{SessionID: session, Stand: "A2", BlockType: "MANUAL", Source: "CONTROLLER", Manual: true}))

		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS603"), onlineFlightAtA2("SAS603", 1)))
		assert.Empty(t, allocations.failures.List(), "an occupied observed stand is expected wrong-stand recovery, not a failed controller allocation")

		assignment, err := assignments.GetAssignment(ctx, session, "SAS603")
		require.NoError(t, err)
		assert.Equal(t, StageReserved, assignment.Stage)
		assert.Equal(t, "A1", assignment.Stand)
		require.NotNil(t, assignment.ConflictReason)
		assert.Contains(t, *assignment.ConflictReason, wrongStandPendingPrefix)
		require.NotNil(t, assignment.ExpiresAt)
		assert.Equal(t, clock.current().Add(5*time.Minute).UTC(), assignment.ExpiresAt.UTC())
		require.Len(t, published, 3, "reservation, awaiting delivery, and active deadline are published")
		assert.Equal(t, assignment.Version, published[2].Assignment.Version)

		version := assignment.Version
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS603"), onlineFlightAtA2("SAS603", 1)))
		unchanged, err := assignments.GetAssignment(ctx, session, "SAS603")
		require.NoError(t, err)
		assert.Equal(t, version, unchanged.Version, "an unchanged mismatch must not churn the optimistic version")
		assert.Len(t, published, 3, "an unchanged mismatch must not be republished")
	})

	t.Run("EuroScope spawn on occupied stand sends relocation message", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		messenger := &wrongStandTestMessenger{available: true}
		lifecycle.SetWrongStandMessenger(messenger)
		testdata.SeedTestStrip(t, queries, session, "SAS607")
		clock.set(time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC))
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS607"), offlineFlight("SAS607", 1)))
		require.NoError(t, assignments.CreateBlock(ctx, &models.StandBlock{
			SessionID: session, Stand: "A2", BlockType: "MANUAL", Source: "CONTROLLER", Manual: true,
		}))

		require.NoError(t, lifecycle.ObserveDeparturePosition(
			ctx, session, loadStrip(t, strips, session, "SAS607"), 55.6285306, 12.6434583,
		))

		require.Equal(t, 1, messenger.calls)
		require.Equal(t, "STAND ASSIGNMENT: PLEASE RELOCATE TO YOUR ASSIGNED STAND A1", messenger.message)
		assignment, err := assignments.GetAssignment(ctx, session, "SAS607")
		require.NoError(t, err)
		assert.Equal(t, "A1", assignment.Stand, "the occupied observed stand must not replace the reserved stand")
		require.NotNil(t, assignment.ConflictReason)
		assert.Contains(t, *assignment.ConflictReason, wrongStandPendingPrefix)
	})

	t.Run("EuroScope-only spawn on occupied stand receives an alternative and relocation message", func(t *testing.T) {
		lifecycle, allocations, session, assignments, strips, _ := departureLifecycleFixture(t, pool, queries, "", "", nil)
		messenger := &wrongStandTestMessenger{available: true}
		lifecycle.SetWrongStandMessenger(messenger)
		testdata.SeedTestStrip(t, queries, session, "SAS608")
		require.NoError(t, assignments.CreateBlock(ctx, &models.StandBlock{
			SessionID: session, Stand: "A2", BlockType: "MANUAL", Source: "CONTROLLER", Manual: true,
		}))

		require.NoError(t, lifecycle.ObserveDeparturePosition(
			ctx, session, loadStrip(t, strips, session, "SAS608"), 55.6285306, 12.6434583,
		))

		require.Equal(t, 1, messenger.calls)
		require.Equal(t, "STAND ASSIGNMENT: PLEASE RELOCATE TO YOUR ASSIGNED STAND A1", messenger.message)
		assignment, err := assignments.GetAssignment(ctx, session, "SAS608")
		require.NoError(t, err)
		assert.Equal(t, "A1", assignment.Stand)
		assert.Equal(t, StageDepartureBlock, assignment.Stage)
		require.NotNil(t, assignment.ConflictReason)
		assert.Contains(t, *assignment.ConflictReason, wrongStandPendingPrefix)

		require.NoError(t, lifecycle.ObserveDeparturePosition(
			ctx, session, loadStrip(t, strips, session, "SAS608"), 55.6285306, 12.6434583,
		))
		require.Equal(t, 1, messenger.calls, "the same occupied-stand episode must not send another message")
		assert.Empty(t, allocations.failures.List(), "observed-stand probing must not create allocation failures")
	})

	t.Run("EuroScope-only spawn with no alternative receives one occupied message", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, _ := departureLifecycleFixture(t, pool, queries, "", "", nil)
		messenger := &wrongStandTestMessenger{available: true}
		lifecycle.SetWrongStandMessenger(messenger)
		testdata.SeedTestStrip(t, queries, session, "SAS609")
		for _, stand := range []string{"A1", "A2"} {
			require.NoError(t, assignments.CreateBlock(ctx, &models.StandBlock{
				SessionID: session, Stand: stand, BlockType: "MANUAL", Source: "CONTROLLER", Manual: true,
			}))
		}
		strip := loadStrip(t, strips, session, "SAS609")

		require.NoError(t, lifecycle.ObserveDeparturePosition(ctx, session, strip, 55.6285306, 12.6434583))
		require.NoError(t, lifecycle.ObserveDeparturePosition(ctx, session, strip, 55.6285306, 12.6434583))

		require.Equal(t, 1, messenger.calls)
		require.Equal(t, "STAND ASSIGNMENT: STAND A2 IS OCCUPIED. PLEASE RELOCATE", messenger.message)
		_, err := assignments.GetAssignment(ctx, session, "SAS609")
		require.Error(t, err)

		require.NoError(t, lifecycle.ObserveDeparturePosition(ctx, session, strip, 55.6285306, 12.6434583))
		require.Equal(t, 1, messenger.calls, "the same occupied-stand episode must remain one-shot")
	})

	t.Run("wrong stand timeout forces an explicit override", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		testdata.SeedTestStrip(t, queries, session, "SAS604")
		clock.set(time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC))
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS604"), offlineFlight("SAS604", 1)))
		require.NoError(t, assignments.CreateBlock(ctx, &models.StandBlock{SessionID: session, Stand: "A2", BlockType: "MANUAL", Source: "CONTROLLER", Manual: true}))
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS604"), onlineFlightAtA2("SAS604", 1)))
		clock.advance(5 * time.Minute)
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		forced, err := assignments.GetAssignment(ctx, session, "SAS604")
		require.NoError(t, err)
		assert.Equal(t, "A2", forced.Stand)
		assert.Equal(t, StageDepartureBlock, forced.Stage)
		require.NotNil(t, forced.ConflictReason)
		assert.Contains(t, *forced.ConflictReason, wrongStandForcedPrefix)
	})

	t.Run("warning failure does not start deadline and retries", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		messenger := &wrongStandTestMessenger{}
		lifecycle.SetWrongStandMessenger(messenger)
		testdata.SeedTestStrip(t, queries, session, "SAS606")
		clock.set(time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC))
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS606"), offlineFlight("SAS606", 1)))
		require.NoError(t, assignments.CreateBlock(ctx, &models.StandBlock{SessionID: session, Stand: "A2", BlockType: "MANUAL", Source: "CONTROLLER", Manual: true}))
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS606"), onlineFlightAtA2("SAS606", 1)))
		awaiting, err := assignments.GetAssignment(ctx, session, "SAS606")
		require.NoError(t, err)
		require.NotNil(t, awaiting.ConflictReason)
		assert.Contains(t, *awaiting.ConflictReason, wrongStandAwaitingPrefix)
		assert.Equal(t, clock.current().Add(15*time.Minute).UTC(), awaiting.ExpiresAt.UTC())

		messenger.available = true
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS606"), onlineFlightAtA2("SAS606", 1)))
		active, err := assignments.GetAssignment(ctx, session, "SAS606")
		require.NoError(t, err)
		assert.Contains(t, *active.ConflictReason, wrongStandPendingPrefix)
		assert.Equal(t, clock.current().Add(5*time.Minute).UTC(), active.ExpiresAt.UTC())
		assert.Equal(t, 2, messenger.calls)
		assert.Equal(t, "STAND ASSIGNMENT: PLEASE RELOCATE TO YOUR ASSIGNED STAND A1", messenger.message)
	})

	t.Run("moving away cancels the wrong stand deadline", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		testdata.SeedTestStrip(t, queries, session, "SAS605")
		clock.set(time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC))
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS605"), offlineFlight("SAS605", 1)))
		require.NoError(t, assignments.CreateBlock(ctx, &models.StandBlock{SessionID: session, Stand: "A2", BlockType: "MANUAL", Source: "CONTROLLER", Manual: true}))
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS605"), onlineFlightAtA2("SAS605", 1)))
		moved := onlineFlightAtA2("SAS605", 1)
		moved.Latitude = 55.62
		moved.Longitude = 12.65
		clock.advance(time.Minute)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS605"), moved))
		cancelled, err := assignments.GetAssignment(ctx, session, "SAS605")
		require.NoError(t, err)
		assert.Nil(t, cancelled.ConflictReason)
		require.NotNil(t, cancelled.ExpiresAt)
		assert.Equal(t, clock.current().Add(15*time.Minute).UTC(), cancelled.ExpiresAt.UTC())
	})

	t.Run("TSAT does not release an occupied departure block", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		testdata.SeedTestStrip(t, queries, session, "SAS701")
		clock.set(time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC))
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS701"), offlineFlight("SAS701", 1)))
		tsat := "1030"
		_, err := strips.SetCdmData(ctx, session, "SAS701", &models.CdmData{Tsat: &tsat})
		require.NoError(t, err)
		clock.advance(2 * time.Minute)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS701"), onlineFlight("SAS701", 1)))

		clock.set(time.Date(2026, 7, 12, 10, 39, 0, 0, time.UTC))
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		if assignment, err := assignments.GetAssignment(ctx, session, "SAS701"); assert.NoError(t, err) {
			assert.Nil(t, assignment.ExpiresAt, "the active block should not retain a TSAT expiry")
		}

		clock.set(time.Date(2026, 7, 12, 10, 41, 0, 0, time.UTC))
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		assignment, err := assignments.GetAssignment(ctx, session, "SAS701")
		require.NoError(t, err, "the occupied stand remains assigned after TSAT+10")
		assert.Nil(t, assignment.ExpiresAt)
	})

	t.Run("TOBT does not release an occupied departure block", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		testdata.SeedTestStrip(t, queries, session, "SAS801")
		clock.set(time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC))
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS801"), offlineFlight("SAS801", 1)))
		tobt := "1030"
		_, err := strips.SetCdmData(ctx, session, "SAS801", &models.CdmData{Tobt: &tobt})
		require.NoError(t, err)
		clock.advance(2 * time.Minute)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS801"), onlineFlight("SAS801", 1)))

		block, err := assignments.GetAssignment(ctx, session, "SAS801")
		require.NoError(t, err)
		assert.Nil(t, block.ExpiresAt, "an online aircraft on its assigned stand has no TOBT expiry")

		clock.set(time.Date(2026, 7, 12, 10, 41, 0, 0, time.UTC))
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		_, err = assignments.GetAssignment(ctx, session, "SAS801")
		require.NoError(t, err, "the occupied stand remains assigned after TOBT+10")
	})

	t.Run("stale TOBT does not recreate an already expired online block", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		testdata.SeedTestStrip(t, queries, session, "SAS802")
		clock.set(time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC))
		tobt := "0905"
		invalidReason := models.CdmInvalidReasonStaleTobt
		_, err := strips.SetCdmData(ctx, session, "SAS802", &models.CdmData{
			Tobt:        &tobt,
			Calculation: &models.CdmCalculation{InvalidReason: &invalidReason},
		})
		require.NoError(t, err)

		require.NoError(t, lifecycle.ObserveDeparturePosition(
			ctx, session, loadStrip(t, strips, session, "SAS802"), 55.6285306, 12.642625,
		))
		assignment, err := assignments.GetAssignment(ctx, session, "SAS802")
		require.NoError(t, err)
		assert.Nil(t, assignment.ExpiresAt, "stale TOBT must not create an already-expired deadline")

		clock.advance(30 * time.Minute)
		require.NoError(t, lifecycle.ReleaseExpired(ctx))
		_, err = assignments.GetAssignment(ctx, session, "SAS802")
		require.NoError(t, err, "an online assignment without a valid CDM deadline must remain assigned")
	})

	t.Run("revalidation reallocates when EuroScope facts invalidate the stand", func(t *testing.T) {
		aircraft := mustLoadAircraftRegistry(t, "A320", "B737")
		engines := mustLoadEngineRegistry(t, aircraft, []engineRecord{{ICAO: "A320", WTC: "M", Engine: "J"}, {ICAO: "B737", WTC: "M", Engine: "J"}})
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixtureWithEngines(t, pool, queries, "ATYP:A320", "ATYP:B737", aircraft, engines)
		testdata.SeedTestStrip(t, queries, session, "SAS901")
		clock.set(time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC))
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS901"), offlineFlight("SAS901", 1)))

		_, err := pool.Exec(ctx, "UPDATE strips SET engine_type = $3, aircraft_type = $4 WHERE session = $1 AND callsign = $2", session, "SAS901", "J", "B737")
		require.NoError(t, err)

		clock.advance(2 * time.Minute)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS901"), onlineFlight("SAS901", 1)))

		reallocated, err := assignments.GetAssignment(ctx, session, "SAS901")
		require.NoError(t, err)
		assert.Equal(t, "A2", reallocated.Stand, "an incompatible stand is replaced once better facts arrive")
		assert.Equal(t, StageDepartureBlock, reallocated.Stage)
	})

	t.Run("a temporary TSAT gap does not add a block expiry", func(t *testing.T) {
		lifecycle, _, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		testdata.SeedTestStrip(t, queries, session, "SAS920")
		clock.set(time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC))
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS920"), offlineFlight("SAS920", 1)))
		tsat := "1030"
		_, err := strips.SetCdmData(ctx, session, "SAS920", &models.CdmData{Tsat: &tsat})
		require.NoError(t, err)
		clock.advance(2 * time.Minute)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS920"), onlineFlight("SAS920", 1)))

		block, err := assignments.GetAssignment(ctx, session, "SAS920")
		require.NoError(t, err)
		assert.Nil(t, block.ExpiresAt)

		_, err = strips.SetCdmData(ctx, session, "SAS920", &models.CdmData{})
		require.NoError(t, err)
		clock.advance(2 * time.Minute)
		require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, "SAS920"), onlineFlight("SAS920", 1)))

		polled, err := assignments.GetAssignment(ctx, session, "SAS920")
		require.NoError(t, err)
		assert.Nil(t, polled.ExpiresAt, "a CDM gap must not add an expiry to an occupied departure block")
	})

	t.Run("restart reconstructs pending deadlines from persisted timestamps", func(t *testing.T) {
		_, allocations, session, assignments, strips, clock := departureLifecycleFixture(t, pool, queries, "", "", nil)
		testdata.SeedTestStrip(t, queries, session, "SAS110")
		clock.set(time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC))
		expiry := time.Date(2026, 7, 12, 10, 5, 0, 0, time.UTC)
		require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{
			SessionID: session, Callsign: "SAS110", Stand: "A1", Direction: "DEPARTURE",
			Stage: StageReserved, Source: "AUTOMATIC", AssignedAt: &expiry, ExpiresAt: &expiry,
		}))

		restarted, err := NewDepartureLifecycleService(
			allocations, assignments, strips, postgres.NewSessionRepository(pool),
			allocations.stands, nil, nil, nil,
			WithDepartureLifecycleClock(func() time.Time { return time.Date(2026, 7, 12, 10, 6, 0, 0, time.UTC) }),
		)
		require.NoError(t, err)

		require.NoError(t, restarted.ReleaseExpired(ctx))
		_, err = assignments.GetAssignment(ctx, session, "SAS110")
		require.Error(t, err, "the restarted sweep releases the assignment from its persisted expiry alone")
		updated := loadStrip(t, strips, session, "SAS110")
		require.Nil(t, updated.Stand, "the operational stand is cleared from persisted state")
	})
}

func departureLifecycleFixture(t *testing.T, pool *pgxpool.Pool, queries *database.Queries, a1Directive, a2Directive string, aircraft *sat.AircraftRegistry) (*DepartureLifecycleService, *StandAllocationService, int32, repository.StandAssignmentRepository, repository.StripRepository, *fakeClock) {
	t.Helper()
	return departureLifecycleFixtureWithEngines(t, pool, queries, a1Directive, a2Directive, aircraft, nil)
}

func departureLifecycleFixtureWithEngines(t *testing.T, pool *pgxpool.Pool, queries *database.Queries, a1Directive, a2Directive string, aircraft *sat.AircraftRegistry, engines *sat.AircraftEngineRegistry) (*DepartureLifecycleService, *StandAllocationService, int32, repository.StandAssignmentRepository, repository.StripRepository, *fakeClock) {
	t.Helper()
	registry, err := sat.LoadStandCapabilities(strings.NewReader(`
STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30
` + a1Directive + `
STAND:EKCH:A2:N055.37.42.710:E012.38.36.450:30
` + a2Directive + `
`))
	require.NoError(t, err)
	policy, err := sat.LoadAirlineAssignment(strings.NewReader(`{
  "rules": [{"id":"sas","callsigns":["SAS"],"stands":{"tier1":{"A1":100,"A2":100}}}],
  "stand_groups": {}, "fallback_rules": {`+testFallbackJSON("A1")+`}
}`), registry)
	require.NoError(t, err)
	assignments := postgres.NewStandAssignmentRepository(pool)
	strips := postgres.NewStripRepository(pool)
	sessions := postgres.NewSessionRepository(pool)
	clock := &fakeClock{now: time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)}
	allocations, err := NewStandAllocationService(pool, strips, assignments, registry, policy,
		WithStandAllocationRandom(func() float64 { return 0 }),
		WithStandAllocationClock(clock.current),
	)
	require.NoError(t, err)
	lifecycle, err := NewDepartureLifecycleService(allocations, assignments, strips, sessions, registry, aircraft, engines, sat.NewAirportCountryRegistry(),
		WithDepartureLifecycleClock(clock.current),
	)
	require.NoError(t, err)
	lifecycle.SetWrongStandMessenger(&wrongStandTestMessenger{available: true})
	name := fmt.Sprintf("%s-%d", t.Name(), standAllocationSessionSequence.Add(1))
	session := testdata.SeedTestSessionNamedWithSectors(t, queries, name, nil)
	return lifecycle, allocations, session, assignments, strips, clock
}

func loadStrip(t *testing.T, strips repository.StripRepository, session int32, callsign string) *models.Strip {
	t.Helper()
	strip, err := strips.GetByCallsign(context.Background(), session, callsign)
	require.NoError(t, err)
	return strip
}

func offlineFlight(callsign string, revision int64) vatsim.DepartureFlightInfo {
	return vatsim.DepartureFlightInfo{
		Callsign: callsign, CID: "1001", Online: false, Revision: revision,
		Origin: "EKCH", Destination: "ESSA", AircraftType: "A320",
	}
}

func onlineFlight(callsign string, revision int64) vatsim.DepartureFlightInfo {
	info := offlineFlight(callsign, revision)
	info.Online = true
	info.Latitude = 55.6285306
	info.Longitude = 12.642625
	return info
}

func onlineFlightAtA2(callsign string, revision int64) vatsim.DepartureFlightInfo {
	info := offlineFlight(callsign, revision)
	info.Online = true
	info.Latitude = 55.6285306
	info.Longitude = 12.6434583
	return info
}

type engineRecord struct {
	ICAO   string
	WTC    string
	Engine string
}

func mustLoadAircraftRegistry(t *testing.T, types ...string) *sat.AircraftRegistry {
	t.Helper()
	var rows []string
	for _, aircraftType := range types {
		rows = append(rows, aircraftType+"\t35.8\t37.6\t11.8\t78000\tA")
	}
	registry, err := sat.LoadAircraftReference(strings.NewReader(strings.Join(rows, "\n")))
	require.NoError(t, err)
	return registry
}

func mustLoadEngineRegistry(t *testing.T, aircraft *sat.AircraftRegistry, records []engineRecord) *sat.AircraftEngineRegistry {
	t.Helper()
	var parts []string
	for _, record := range records {
		parts = append(parts, fmt.Sprintf(`{"ICAO":%q,"Description":"%s %s","WTC":%q}`, record.ICAO, record.ICAO, record.Engine, record.WTC))
	}
	engines, err := sat.LoadAircraftEngineReference(strings.NewReader("["+strings.Join(parts, ",")+"]"), aircraft)
	require.NoError(t, err)
	return engines
}
