package postgres

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/arrivalinfo"
	"FlightStrips/internal/pdc/testdata"

	"github.com/stretchr/testify/require"
)

func TestArrivalTelexStoreReservesArrivalInfoOncePerEpisode(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	var session int32
	_, err := pool.Exec(ctx, `INSERT INTO airports (name) VALUES ('EKCH') ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO sessions (name, airport) VALUES ('LIVE', 'EKCH') RETURNING id`).Scan(&session))
	store := NewArrivalTelexStore(pool)
	sentAt := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

	info := arrivalinfo.SentMessage{Session: session, Callsign: "SAS123", Episode: 42, Kind: arrivalinfo.KindArrivalInfo,
		Text: "EKCH ARR INFO: DELAY 5-10MIN.", SentAt: sentAt}
	id, inserted, err := store.Reserve(ctx, info)
	require.NoError(t, err)
	require.True(t, inserted)

	_, inserted, err = store.Reserve(ctx, info)
	require.NoError(t, err)
	require.False(t, inserted, "a second arrival info for the same episode must not be recorded")

	update := arrivalinfo.SentMessage{Session: session, Callsign: "SAS123", Episode: 42, Kind: arrivalinfo.KindStandUpdate,
		Text: "EKCH ARR INFO: NEW STAND D2.", Stand: "D2", SentAt: sentAt.Add(time.Minute)}
	for range 2 {
		_, inserted, err = store.Reserve(ctx, update)
		require.NoError(t, err)
		require.True(t, inserted)
	}

	other := info
	other.Episode = 43
	_, inserted, err = store.Reserve(ctx, other)
	require.NoError(t, err)
	require.True(t, inserted, "a new arrival episode gets its own arrival info")

	messages, err := store.ListEpisode(ctx, session, 42)
	require.NoError(t, err)
	require.Len(t, messages, 3)
	require.Equal(t, arrivalinfo.KindArrivalInfo, messages[0].Kind)
	require.Equal(t, "", messages[0].Stand)
	require.Equal(t, "D2", messages[2].Stand)
	require.Equal(t, sentAt, messages[0].SentAt)

	require.NoError(t, store.Release(ctx, id))
	_, inserted, err = store.Reserve(ctx, info)
	require.NoError(t, err)
	require.True(t, inserted, "a released reservation can be retried")
}
