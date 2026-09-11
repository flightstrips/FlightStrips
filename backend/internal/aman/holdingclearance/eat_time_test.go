package holdingclearance

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResolveEATUTCRejectsInvalidHHMM(t *testing.T) {
	reference := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	tests := []string{
		"",
		"000",
		"00000",
		"12:30",
		" 1230",
		"1230 ",
		"1a30",
		"2400",
		"2360",
		"9999",
	}

	for _, hhmm := range tests {
		t.Run(hhmm, func(t *testing.T) {
			resolved, err := ResolveEATUTC(hhmm, reference)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidEATHHMM)
			require.True(t, resolved.IsZero())
		})
	}
}

func TestResolveEATUTCSelectsNearestOccurrence(t *testing.T) {
	utcPlusTwo := time.FixedZone("UTC+2", 2*60*60)
	tests := []struct {
		name      string
		hhmm      string
		reference time.Time
		want      time.Time
	}{
		{
			name: "same instant",
			hhmm: "1200", reference: time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC),
			want: time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC),
		},
		{
			name: "exact tie chooses previous occurrence",
			hhmm: "0000", reference: time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC),
			want: time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "positive boundary tie still chooses previous occurrence",
			hhmm: "1200", reference: time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC),
			want: time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC),
		},
		{
			name: "reference precision moves past boundary to next occurrence",
			hhmm: "0000", reference: time.Date(2026, 9, 11, 12, 0, 0, 1, time.UTC),
			want: time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "previous-day overdue",
			hhmm: "2355", reference: time.Date(2026, 9, 12, 0, 10, 0, 0, time.UTC),
			want: time.Date(2026, 9, 11, 23, 55, 0, 0, time.UTC),
		},
		{
			name: "next-day future",
			hhmm: "0005", reference: time.Date(2026, 9, 11, 23, 50, 0, 0, time.UTC),
			want: time.Date(2026, 9, 12, 0, 5, 0, 0, time.UTC),
		},
		{
			name: "reference date is normalized to UTC",
			hhmm: "0015", reference: time.Date(2026, 9, 12, 1, 0, 0, 0, utcPlusTwo),
			want: time.Date(2026, 9, 12, 0, 15, 0, 0, time.UTC),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveEATUTC(test.hhmm, test.reference)
			require.NoError(t, err)
			require.Equal(t, test.want, got)
			require.Same(t, time.UTC, got.Location())
			require.LessOrEqual(t, durationMagnitude(got.Sub(test.reference.UTC())), 12*time.Hour)
		})
	}
}
