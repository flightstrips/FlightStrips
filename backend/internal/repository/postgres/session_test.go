package postgres

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetExpiredSessionsIgnoresObserverHeartbeats(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()
	repository := NewSessionRepository(pool)

	require.NoError(t, queries.InsertAirport(ctx, "EKCH"))
	observerSession, err := repository.Create(ctx, &models.Session{Name: "OBSERVER", Airport: "EKCH"})
	require.NoError(t, err)
	operationalSession, err := repository.Create(ctx, &models.Session{Name: "OPERATIONAL", Airport: "EKCH"})
	require.NoError(t, err)

	now := time.Now().UTC()
	_, err = pool.Exec(ctx, "UPDATE sessions SET created_at = $1", now.Add(-10*time.Minute))
	require.NoError(t, err)
	require.NoError(t, queries.InsertController(ctx, database.InsertControllerParams{
		Callsign:          "EKCH_OBS",
		Session:           observerSession,
		Position:          "199.998",
		Observer:          true,
		LastSeenEuroscope: TimeToPgTimestamp(&now),
	}))
	require.NoError(t, queries.InsertController(ctx, database.InsertControllerParams{
		Callsign:          "EKCH_TWR",
		Session:           operationalSession,
		Position:          "118.105",
		Observer:          false,
		LastSeenEuroscope: TimeToPgTimestamp(&now),
	}))

	expiredBefore := now.Add(-5 * time.Minute)
	expired, err := repository.GetExpiredSessions(ctx, &expiredBefore)
	require.NoError(t, err)

	expiredIDs := make([]int32, 0, len(expired))
	for _, session := range expired {
		expiredIDs = append(expiredIDs, session.ID)
	}
	assert.Contains(t, expiredIDs, observerSession)
	assert.NotContains(t, expiredIDs, operationalSession)
}

func TestGetExpiredSessionsAllowsInitialControllerConnection(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()
	repository := NewSessionRepository(pool)
	require.NoError(t, queries.InsertAirport(ctx, "EKCH"))
	session, err := repository.Create(ctx, &models.Session{Name: "LIVE", Airport: "EKCH"})
	require.NoError(t, err)
	var createdAt time.Time
	require.NoError(t, pool.QueryRow(ctx, "SELECT created_at FROM sessions WHERE id = $1", session).Scan(&createdAt))
	cutoff := createdAt.Add(-5 * time.Minute)

	// Login creates the session before its first controller is inserted. A
	// concurrent monitor sweep must leave that new, temporarily empty session.
	expired, err := repository.GetExpiredSessions(ctx, &cutoff)
	require.NoError(t, err)
	require.Empty(t, expired)

	// An abandoned login still expires after the same five-minute grace period.
	_, err = pool.Exec(ctx, "UPDATE sessions SET created_at = $1 WHERE id = $2", cutoff.Add(-time.Second), session)
	require.NoError(t, err)
	expired, err = repository.GetExpiredSessions(ctx, &cutoff)
	require.NoError(t, err)
	require.Len(t, expired, 1)
	require.Equal(t, session, expired[0].ID)

	// Once connected, the controller's heartbeat determines session liveness.
	require.NoError(t, queries.InsertController(ctx, database.InsertControllerParams{
		Callsign: "EKCH_TWR", Session: session, Position: "118.105",
		LastSeenEuroscope: TimeToPgTimestamp(&createdAt),
	}))
	expired, err = repository.GetExpiredSessions(ctx, &cutoff)
	require.NoError(t, err)
	require.Empty(t, expired)
}
