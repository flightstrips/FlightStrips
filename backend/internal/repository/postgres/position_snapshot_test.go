package postgres

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/shared"
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPositionSnapshotAndPresence(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	session := testdata.SeedTestSessionNamedWithSectors(t, q, "POSITION", nil)
	testdata.SeedTestStrip(t, q, session, "SAS123")
	r := NewStripRepository(pool)
	ctx := shared.WithWebsocketMessageState(context.Background(), &shared.WebsocketMessageState{})
	snapshot, err := r.GetPositionSnapshot(ctx, session, "SAS123")
	require.NoError(t, err)
	require.Nil(t, snapshot.Assignment)
	assignments := NewStandAssignmentRepository(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	observed := "A1"
	a := &models.StandAssignment{SessionID: session, Callsign: "SAS123", Stand: "A1", Direction: "departure", Stage: "departure_block", Source: "automatic", Version: 1, AssignedAt: &now, ObservedStand: &observed}
	require.NoError(t, assignments.CreateAssignment(ctx, a))
	snapshot, err = r.GetPositionSnapshot(ctx, session, "SAS123")
	require.NoError(t, err)
	stored, err := assignments.GetAssignment(ctx, session, "SAS123")
	require.NoError(t, err)
	require.True(t, stored.AssignedAt.Equal(*snapshot.Assignment.AssignedAt))
	require.True(t, stored.CreatedAt.Equal(snapshot.Assignment.CreatedAt))
	require.True(t, stored.UpdatedAt.Equal(snapshot.Assignment.UpdatedAt))
	stored.AssignedAt = snapshot.Assignment.AssignedAt
	stored.CreatedAt = snapshot.Assignment.CreatedAt
	stored.UpdatedAt = snapshot.Assignment.UpdatedAt
	require.Equal(t, stored, snapshot.Assignment)
	shared.CachePositionAssignment(ctx, session, "SAS123", nil)
	_, err = assignments.GetAssignment(ctx, session, "SAS123")
	require.ErrorIs(t, err, pgx.ErrNoRows)
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	_, err = assignments.WithTx(tx).GetAssignment(ctx, session, "SAS123")
	require.NoError(t, err)
	require.NoError(t, tx.Rollback(ctx))
	lat, lon, alt := 55.6, 12.6, int32(1000)
	n, err := r.UpdateAircraftPositionAndBay(ctx, session, "SAS123", &lat, &lon, &alt, snapshot.Strip.Bay, 0, snapshot.Strip.Version)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
	snapshot, err = r.GetPositionSnapshot(ctx, session, "SAS123")
	require.NoError(t, err)
	require.NotNil(t, snapshot.Strip.EuroscopeSeenAt)
	require.Equal(t, lat, *snapshot.Strip.PositionLatitude)
}

func TestConcurrentPositionAndFrontendBayAppendsReserveUniqueSequences(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	session := testdata.SeedTestSessionNamedWithSectors(t, q, "POSITION_APPEND", nil)
	r := NewStripRepository(pool)
	const count = 16
	versions := make([]int32, count)
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("APP%03d", i)
		testdata.SeedTestStrip(t, q, session, name)
		s, err := r.GetByCallsign(context.Background(), session, name)
		require.NoError(t, err)
		versions[i] = s.Version
	}
	var wg sync.WaitGroup
	errs := make(chan error, count)
	sequences := make(chan int32, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("APP%03d", i)
			var seq int32
			var err error
			if i%4 == 0 {
				var n int64
				n, seq, err = r.UpdateAircraftPositionAndAppendBay(context.Background(), session, name, nil, nil, nil, "DEPART", versions[i], 1000)
				if err == nil && n != 1 {
					err = fmt.Errorf("updated %d rows", n)
				}
			} else if i%4 == 1 {
				seq, err = r.AppendToBay(context.Background(), session, name, "DEPART", 1000)
			} else if i%4 == 2 {
				var strip *models.Strip
				strip, err = r.GetByCallsign(context.Background(), session, name)
				if err == nil {
					strip.Bay = "DEPART"
					err = r.PersistAtEndOfBay(context.Background(), strip, false, 1000)
					seq = *strip.Sequence
				}
			} else {
				var strip *models.TacticalStrip
				strip, err = NewTacticalStripRepository(pool).CreateAtEndOfBay(context.Background(), session, "START", "DEPART", "", nil, "121.630", 1000)
				if err == nil {
					seq = strip.Sequence
				}
			}
			errs <- err
			sequences <- seq
		}(i)
	}
	wg.Wait()
	close(errs)
	close(sequences)
	for err := range errs {
		require.NoError(t, err)
	}
	seen := map[int32]bool{}
	for seq := range sequences {
		require.False(t, seen[seq], "duplicate bay sequence %d", seq)
		seen[seq] = true
	}
}

type positionQueryCounter struct{ n atomic.Int32 }

func (c *positionQueryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.n.Add(1)
	return ctx
}
func (*positionQueryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func TestPositionRepositoryQueryBudgets(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	session := testdata.SeedTestSessionNamedWithSectors(t, q, "QUERY_BUDGET", nil)
	testdata.SeedTestStrip(t, q, session, "SAS123")
	counter := &positionQueryCounter{}
	cfg := pool.Config()
	cfg.ConnConfig.Tracer = counter
	traced, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	defer traced.Close()
	repo := NewStripRepository(traced)
	snapshot, err := repo.GetPositionSnapshot(context.Background(), session, "SAS123")
	require.NoError(t, err)
	n, err := repo.UpdateAircraftPositionAndBay(context.Background(), session, "SAS123", nil, nil, nil, snapshot.Strip.Bay, 0, snapshot.Strip.Version)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
	require.Equal(t, int32(2), counter.n.Load())
	identities := NewAMANRepository(traced)
	identity := aman.VATSIMFlightIdentity{VATSIMCID: "123456", CurrentCallsign: "SAS123"}
	id, err := identities.BindVATSIMFlight(context.Background(), identity)
	require.NoError(t, err)
	counter.n.Store(0)
	same, err := identities.BindVATSIMFlight(context.Background(), identity)
	require.NoError(t, err)
	require.Equal(t, id, same)
	require.Equal(t, int32(1), counter.n.Load(), "established AMAN identity adds exactly one read")
}
