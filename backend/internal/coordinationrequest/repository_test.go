package coordinationrequest

import (
	"context"
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
