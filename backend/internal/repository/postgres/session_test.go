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
