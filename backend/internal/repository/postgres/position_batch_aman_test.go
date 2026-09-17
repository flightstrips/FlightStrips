package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/database"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/shared"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestPositionBatchAMANHundredIdentitiesOneSlotOneQuery(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	seed := NewAMANRepository(pool)
	expected := make(map[string]aman.FlightID)
	for i := 0; i < 100; i++ {
		name := fmt.Sprintf("BATCH%d", i)
		id, err := seed.BindVATSIMFlight(ctx, aman.VATSIMFlightIdentity{VATSIMCID: "123", CurrentCallsign: name})
		require.NoError(t, err)
		expected[name] = id
	}
	counter := &positionQueryCounter{}
	cfg := pool.Config()
	cfg.ConnConfig.Tracer = counter
	traced, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer traced.Close()
	repo := NewAMANRepository(traced)
	d := shared.NewBatchPositionDispatcher(1, 256, make(chan struct{}, 1), func(size int) shared.PositionBatchScope {
		b := NewPositionBatch(size).(*positionBatch)
		b.maxWait = time.Second // deterministic under the race detector
		return b
	})
	results := make(chan error, 100)
	require.NoError(t, d.RunBarrier(ctx, func() {
		for name, want := range expected {
			require.NoError(t, d.Submit(ctx, name, func(ctx context.Context) {
				id, err := repo.BindVATSIMFlight(ctx, aman.VATSIMFlightIdentity{VATSIMCID: " 123 ", CurrentCallsign: " " + name + " "})
				if err == nil && id != want {
					err = fmt.Errorf("%s: wrong flight ID", name)
				}
				results <- err
			}))
		}
	}))
	deadline, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	require.NoError(t, d.Close(deadline))
	for i := 0; i < 100; i++ {
		require.NoError(t, <-results)
	}
	require.Equal(t, int32(1), counter.n.Load(), "100 established identities must share one read")
}

func TestPositionBatchAMANChangesRetirementAndInvalidMember(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	repo := NewAMANRepository(pool)
	ctx := context.Background()
	old := make(map[string]aman.FlightID)
	for _, name := range []string{"STABLE", "CHANGED", "RETIRED"} {
		id, err := repo.BindVATSIMFlight(ctx, aman.VATSIMFlightIdentity{VATSIMCID: "123", CurrentCallsign: name})
		require.NoError(t, err)
		old[name] = id
	}
	require.NoError(t, repo.RetireVATSIMFlight(ctx, old["RETIRED"]))
	inputs := []aman.VATSIMFlightIdentity{
		{VATSIMCID: "123", CurrentCallsign: " stable "},
		{VATSIMCID: "456", CurrentCallsign: "CHANGED"},
		{VATSIMCID: "123", CurrentCallsign: "RETIRED"},
		{VATSIMCID: "123", CurrentCallsign: "NEWCALLSIGN"},
		{VATSIMCID: "123", CurrentCallsign: ""},
	}
	scope := NewPositionBatch(len(inputs)).(*positionBatch)
	scope.maxWait = time.Second
	type result struct {
		index int
		id    aman.FlightID
		err   error
	}
	results := make(chan result, len(inputs))
	for i, input := range inputs {
		go func() {
			defer scope.Done(i)
			id, err := repo.BindVATSIMFlight(scope.Context(ctx, i), input)
			results <- result{i, id, err}
		}()
	}
	ids := make([]aman.FlightID, len(inputs))
	for range inputs {
		select {
		case r := <-results:
			ids[r.index] = r.id
			if r.index == 4 {
				require.Error(t, r.err)
			} else {
				require.NoError(t, r.err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("identity batch stalled behind invalid member")
		}
	}
	require.Equal(t, old["STABLE"], ids[0])
	require.Equal(t, old["CHANGED"], ids[1])
	require.NotEqual(t, old["RETIRED"], ids[2])
	require.NotEqual(t, old["STABLE"], ids[3], "same CID must not merge callsigns")
	var cid string
	require.NoError(t, pool.QueryRow(ctx, "SELECT vatsim_cid FROM aman_vatsim_observation_identities WHERE flight_id = $1", string(ids[1])).Scan(&cid))
	require.Equal(t, "456", cid)
	// A new report must not reuse a prior group's now-retired identity.
	require.NoError(t, repo.RetireVATSIMFlight(ctx, ids[0]))
	nextScope := NewPositionBatch(1)
	next, err := repo.BindVATSIMFlight(nextScope.Context(ctx, 0), inputs[0])
	nextScope.Done(0)
	require.NoError(t, err)
	require.NotEqual(t, ids[0], next)
}

func TestPositionBatchAMANConcurrentCreationKeepsOneActiveIdentity(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	repo := NewAMANRepository(pool)
	scope := NewPositionBatch(2)
	type result struct {
		id  aman.FlightID
		err error
	}
	results := make(chan result, 2)
	for i, cid := range []string{"123", "456"} {
		go func() {
			defer scope.Done(i)
			id, err := repo.BindVATSIMFlight(scope.Context(ctx, i), aman.VATSIMFlightIdentity{VATSIMCID: cid, CurrentCallsign: "SAME"})
			results <- result{id, err}
		}()
	}
	first, second := <-results, <-results
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.Equal(t, first.id, second.id)
	var active int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM aman_vatsim_observation_identities WHERE current_callsign = 'SAME' AND retired_at IS NULL").Scan(&active))
	require.Equal(t, 1, active)
}

func TestPositionBatchAMANCancelledMemberDoesNotFailHealthyRead(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	repo := NewAMANRepository(pool)
	ctx := context.Background()
	want, err := repo.BindVATSIMFlight(ctx, aman.VATSIMFlightIdentity{VATSIMCID: "123", CurrentCallsign: "HEALTHY"})
	require.NoError(t, err)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	calls := []*batchCall{
		{ctx: cancelled, input: positionIdentityRead{repo, "CANCELLED"}, result: make(chan batchResult, 1)},
		{ctx: ctx, input: positionIdentityRead{repo, "HEALTHY"}, result: make(chan batchResult, 1)},
	}
	batchPositionIdentities(calls)
	require.ErrorIs(t, (<-calls[0].result).err, context.Canceled)
	healthy := <-calls[1].result
	require.NoError(t, healthy.err)
	require.Equal(t, string(want), healthy.value.(database.AmanVatsimObservationIdentity).FlightID)
}
