package postgres

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestMembershipHeartbeatAndDetachDoNotRefreshOtherInstanceRows(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := t.Context()
	sessions := NewSessionRepository(pool)
	repo := NewControllerRepository(pool)
	require.NoError(t, queries.InsertAirport(ctx, "EKCH"))
	session, err := sessions.Create(ctx, &models.Session{Name: "LIVE", Airport: "EKCH"})
	require.NoError(t, err)
	cid := "123"
	old := time.Now().UTC().Add(-time.Hour)
	now := time.Now().UTC()
	for _, callsign := range []string{"FIRST", "SECOND"} {
		require.NoError(t, queries.InsertController(ctx, database.InsertControllerParams{Callsign: callsign, Session: session, Position: "118.000", Cid: &cid, LastSeenEuroscope: TimeToPgTimestamp(&old)}))
	}
	count, err := repo.SetEuroscopeMembershipSeen(ctx, cid, session, "SECOND", &now)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	first, err := repo.Get(ctx, "FIRST", session)
	require.NoError(t, err)
	require.WithinDuration(t, old, *first.LastSeenEuroscope, time.Millisecond)
	count, err = repo.SetFrontendMembershipSeen(ctx, cid, session, "SECOND", &now)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	first, err = repo.Get(ctx, "FIRST", session)
	require.NoError(t, err)
	require.Nil(t, first.LastSeenFrontend)
	count, err = repo.DetachMembership(ctx, "wrong-cid", session, "FIRST")
	require.NoError(t, err)
	require.Zero(t, count)
	count, err = repo.DetachMembership(ctx, cid, session, "FIRST")
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	first, err = repo.Get(ctx, "FIRST", session)
	require.NoError(t, err)
	require.Nil(t, first.Cid)
	require.Nil(t, first.LastSeenEuroscope)
	count, err = repo.SetEuroscopeMembershipSeen(ctx, cid, session, "FIRST", &now)
	require.NoError(t, err)
	require.Zero(t, count)
	second, err := repo.Get(ctx, "SECOND", session)
	require.NoError(t, err)
	require.NotNil(t, second.Cid)
	require.WithinDuration(t, now, *second.LastSeenEuroscope, time.Millisecond)
	_, err = pool.Exec(ctx, "UPDATE sessions SET created_at=$1 WHERE id=$2", old, session)
	require.NoError(t, err)
	_, err = repo.DetachMembership(ctx, cid, session, "SECOND")
	require.NoError(t, err)
	threshold := now.Add(-5 * time.Minute)
	expired, err := sessions.GetExpiredSessions(ctx, &threshold)
	require.NoError(t, err)
	ids := []int32{}
	for _, value := range expired {
		ids = append(ids, value.ID)
	}
	require.Contains(t, ids, session, "a retained zero-member transport must not keep session cleanup alive")
}
