package coordinationrequest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/pdc/testdata"
	"github.com/stretchr/testify/require"
)

func TestRepositoryRestartReplayIsDeterministicAndAdditive(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	repository := NewRepository(pool)

	later := routeRequest(t, "command-b", testTime.Add(time.Minute))
	earlier := routeRequest(t, "command-a", testTime)
	require.NoError(t, repository.Save(ctx, later))
	require.NoError(t, repository.Save(ctx, earlier))

	_, err := pool.Exec(ctx, `UPDATE aman_coordination_requests
        SET payload = payload || '{"future_field":{"enabled":true}}'::jsonb WHERE request_id = $1`, earlier.ID)
	require.NoError(t, err)

	replayed, err := NewRepository(pool).ReplayAirport(ctx, "EKCH")
	require.NoError(t, err)
	require.Equal(t, []RequestID{earlier.ID, later.ID}, []RequestID{replayed[0].ID, replayed[1].ID})
	require.Equal(t, earlier, replayed[0])

	accepted, err := earlier.Transition(StateAccepted, testTime.Add(2*time.Minute))
	require.NoError(t, err)
	require.NoError(t, NewRepository(pool).Save(ctx, accepted))
	replayed, err = NewRepository(pool).ReplayAirport(ctx, "EKCH")
	require.NoError(t, err)
	require.Equal(t, StateAccepted, replayed[0].State)

	duplicate := routeRequest(t, "command-a", testTime.Add(3*time.Minute))
	require.Equal(t, earlier.ID, duplicate.ID)
	require.NoError(t, NewRepository(pool).Save(ctx, accepted), "exact retry remains idempotent after restart")
}

func TestSubmitSupersedesSameKindButKeepsKindsIndependent(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	repository := NewRepository(pool)

	first := routeRequest(t, "route-1", testTime)
	created, err := repository.Submit(ctx, first, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(1), created.Revision)

	speed, err := New("speed-1", "EKCH", "flight-1", "EKCH_APP", "1234567", "EKCH_FMH", KindSpeed,
		Payload{Speed: &SpeedPayload{Requested: "220 KT"}}, testTime.Add(time.Minute))
	require.NoError(t, err)
	independent, err := repository.Submit(ctx, speed, 1)
	require.NoError(t, err)
	require.Nil(t, independent.SupersededRequest)

	replacement := routeRequest(t, "route-2", testTime.Add(2*time.Minute))
	replaced, err := repository.Submit(ctx, replacement, 2)
	require.NoError(t, err)
	require.Equal(t, first.ID, *replaced.Request.Supersedes)
	require.Equal(t, replacement.ID, *replaced.SupersededRequest.SupersededBy)

	replayed, err := NewRepository(pool).ReplayAirport(ctx, "EKCH")
	require.NoError(t, err)
	require.Len(t, replayed, 3)
	require.Equal(t, StateSuperseded, replayed[0].State)
	require.Equal(t, StatePending, replayed[1].State, "speed remains independent")
	require.Equal(t, StatePending, replayed[2].State)
}

func TestSubmitRejectsStaleRevisionWithoutPartialSupersede(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	repository := NewRepository(pool)
	first := routeRequest(t, "route-1", testTime)
	_, err := repository.Submit(ctx, first, 0)
	require.NoError(t, err)

	_, err = repository.Submit(ctx, routeRequest(t, "route-stale", testTime.Add(time.Minute)), 0)
	require.ErrorIs(t, err, ErrRevisionConflict)
	replayed, err := repository.ReplayAirport(ctx, "EKCH")
	require.NoError(t, err)
	require.Equal(t, []Request{first}, replayed)
}

func TestSubmitRetryIsIdempotentAcrossRepositoryRestart(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	request := routeRequest(t, "route-retry", testTime)
	first, err := NewRepository(pool).Submit(ctx, request, 0)
	require.NoError(t, err)

	retry := request
	retry.CreatedAt, retry.UpdatedAt = testTime.Add(time.Minute), testTime.Add(time.Minute)
	duplicate, err := NewRepository(pool).Submit(ctx, retry, 0)
	require.NoError(t, err)
	require.True(t, duplicate.Duplicate)
	require.Equal(t, first.Request, duplicate.Request)
	require.Equal(t, uint64(1), duplicate.Revision)

	replayed, err := NewRepository(pool).ReplayAirport(ctx, "EKCH")
	require.NoError(t, err)
	require.Equal(t, []Request{request}, replayed)
}

func TestConcurrentSubmissionsCommitAtomically(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	repository := NewRepository(pool)
	requests := []Request{
		routeRequest(t, "concurrent-a", testTime),
		routeRequest(t, "concurrent-b", testTime.Add(time.Second)),
	}
	start := make(chan struct{})
	errs := make([]error, len(requests))
	var wait sync.WaitGroup
	for index := range requests {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, errs[index] = repository.Submit(ctx, requests[index], 0)
		}(index)
	}
	close(start)
	wait.Wait()
	require.Equal(t, 1, boolCount(errs[0] == nil, errs[1] == nil))
	require.Equal(t, 1, boolCount(errors.Is(errs[0], ErrRevisionConflict), errors.Is(errs[1], ErrRevisionConflict)))
	replayed, err := repository.ReplayAirport(ctx, "EKCH")
	require.NoError(t, err)
	require.Len(t, replayed, 1)
	require.Equal(t, StatePending, replayed[0].State)
}

func boolCount(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
}
