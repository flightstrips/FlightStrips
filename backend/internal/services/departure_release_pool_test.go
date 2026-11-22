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

	// Two connections: the release transaction and one more. Before the fix,
	// a sighting writer queued on the strip lock took that second connection,
	// and the locked heartbeat recheck then waited forever for a third.
	config := shared.Config()
	config.MaxConns, config.MinConns = 2, 0
	small, err := pgxpool.NewWithConfig(ctx, config)
	require.NoError(t, err)
	defer small.Close()

	lifecycle, allocations, session, assignments, strips, clock := departureLifecycleFixture(t, small, queries, "", "", nil)
	const callsign = "SASOBSPOOL"
	testdata.SeedTestStrip(t, queries, session, callsign)
	require.NoError(t, lifecycle.ProcessDeparture(ctx, session, loadStrip(t, strips, session, callsign), onlineFlightSeenAt(callsign, 1, clock.current())))
	clock.advance(11 * time.Minute)

	writerDone := make(chan error, 1)
	allocations.strips = &lockHookStripRepository{
		StripRepository: allocations.strips,
		once:            &sync.Once{},
		afterLock: func(context.Context) {
			go func() {
				writerDone <- lifecycle.recordOnlineSighting(ctx, session, callsign, onlineFlightSeenAt(callsign, 1, clock.current()))
			}()
			// Do not run the heartbeat recheck until both connections are in use.
			// With the old order this needs the writer to hold the second one
			// while it queues on the strip lock; with the new order the reader
			// and the release transaction already hold both. The hook runs on
			// the release goroutine, so report with Errorf rather than require.
			deadline := time.Now().Add(5 * time.Second)
			for small.Stat().AcquiredConns() < 2 {
				if time.Now().After(deadline) {
					t.Errorf("the pool never reached two acquired connections (have %d)", small.Stat().AcquiredConns())
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

	select {
	case err := <-writerDone:
		require.NoError(t, err, "the queued sighting is written once the release commits")
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
