package vatsim

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman/compatibility"
	"FlightStrips/internal/models"
	"github.com/stretchr/testify/require"
)

func TestArrivalETARevisionGuardUsesSingleReadAndChecksExpiry(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	expiry := now.Add(time.Minute)
	strips := map[string]*models.Strip{
		"SAS1": {Callsign: "SAS1", ArrivalETA: &models.ArrivalETA{
			Time: now.Add(time.Hour), Source: compatibility.ArrivalETASource,
			CalculatedAt: expiry.Add(-compatibility.FreshFor),
		}},
		"SAS2": {Callsign: "SAS2"},
	}
	ctx := withArrivalETASnapshot(context.Background(), 7, strips, []Flight{{Callsign: "SAS1"}, {Callsign: "SAS2"}}, 42)
	reads := 0
	lock := func(context.Context, int32) (int64, error) { reads++; return 42, nil }
	require.NoError(t, CheckArrivalETASnapshot(ctx, 7, lock, func() time.Time { return now }))
	require.Equal(t, 1, reads)
	require.ErrorIs(t, CheckArrivalETASnapshot(ctx, 7, lock, func() time.Time { return expiry.Add(time.Nanosecond) }), ErrArrivalETAChanged)
	require.ErrorIs(t, CheckArrivalETASnapshot(ctx, 7, func(context.Context, int32) (int64, error) { return 43, nil }, func() time.Time { return now }), ErrArrivalETAChanged)
}
