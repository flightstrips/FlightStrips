package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// lockHookStripRepository runs afterLock once, right after a transaction
// locks a strip row, so a test can act while the release holds that lock.
type lockHookStripRepository struct {
	repository.StripRepository
	once      *sync.Once
	afterLock func(context.Context)
}

func (r *lockHookStripRepository) WithTx(tx pgx.Tx) repository.StripRepository {
	return &lockHookStripRepository{StripRepository: r.StripRepository.WithTx(tx), once: r.once, afterLock: r.afterLock}
}

func (r *lockHookStripRepository) LockByCallsign(ctx context.Context, session int32, callsign string) (*models.Strip, error) {
	strip, err := r.StripRepository.LockByCallsign(ctx, session, callsign)
	if err == nil {
		r.once.Do(func() { r.afterLock(ctx) })
	}
	return strip, err
}

func TestDepartureReleaseRecheckDoesNotStarveThePool(t *testing.T) {
	shared, queries := testdata.SetupTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Three connections and two sighting writers. With the old order the
	// release transaction and both writers, queued on the strip lock, hold all
	// three, and the locked heartbeat recheck waits forever for a fourth. With
	// the new order the reader and the transaction hold two, and one writer
	// takes the third and really queues on the strip lock during the recheck.
	const maxConns, writers = 3, 2
	config := shared.Config()
	config.MaxConns, config.MinConns = maxConns, 0
	small, err := pgxpool.NewWithConfig(ctx, config)
	require.NoError(t, err)
	defer small.Close()
	// Close waits for every acquired connection. Cancel first, so a stalled
	// release unwinds immediately when the test fails.
	defer cancel()

	lifecycle, allocations, session, assignments, strips, clock := departureLifecycleFixture(t, small, queries, "", "", nil)
	const callsign = "SASOBSPOOL"
	testdata.SeedTestStrip(t, queries, session, callsign)
	require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlightSeenAt(callsign, 1, clock.current())))
	clock.advance(11 * time.Minute)

	lockWaiters := func() int {
		var waiting int
		if err := shared.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'
			AND query LIKE 'INSERT INTO departure_online_sightings%'`).Scan(&waiting); err != nil {
			return -1
		}
		return waiting
	}
	writerDone := make(chan error, writers)
	allocations.strips = &lockHookStripRepository{
		StripRepository: allocations.strips,
		once:            &sync.Once{},
		afterLock: func(context.Context) {
			for range writers {
				go func() {
					writerDone <- lifecycle.recordOnlineSighting(ctx, session, callsign, onlineFlightSeenAt(callsign, 1, clock.current()))
				}()
			}
			// Do not run the heartbeat recheck until every connection is in use
			// and a writer is queued on the strip lock this release holds. The
			// hook runs on the release goroutine, so report with Errorf.
			deadline := time.Now().Add(5 * time.Second)
			for small.Stat().AcquiredConns() < maxConns || lockWaiters() < 1 {
				if time.Now().After(deadline) {
					t.Errorf("want %d acquired connections and a writer queued on the strip lock, have %d and %d",
						maxConns, small.Stat().AcquiredConns(), lockWaiters())
					return
				}
				time.Sleep(5 * time.Millisecond)
			}
		},
	}

	releaseDone := make(chan error, 1)
	go func() { releaseDone <- lifecycle.ReleaseExpired(ctx) }()
	select {
	case err := <-releaseDone:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("the release stalled waiting for a pool connection while holding the strip lock")
	}
	_, err = assignments.GetAssignment(ctx, session, callsign)
	require.Error(t, err, "the recheck read the committed sighting from 11 minutes ago and released")

	for range writers {
		select {
		case err := <-writerDone:
			require.NoError(t, err, "queued sightings are written once the release commits")
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}
