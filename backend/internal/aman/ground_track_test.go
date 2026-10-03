package aman

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDerivedGroundTrackUsesSuccessivePositions(t *testing.T) {
	now := time.Now()
	previous := &FlightObservation{Surveillance: &SurveillanceFact{ObservedAt: &now}}
	later := now.Add(time.Second)
	for _, tt := range []struct {
		name           string
		lat, lon, want float64
	}{
		{"north", 1, 0, 0}, {"east", 0, 1, 90}, {"south", -1, 0, 180}, {"west", 0, -1, 270},
	} {
		t.Run(tt.name, func(t *testing.T) {
			track, ok := DerivedGroundTrack(previous, &SurveillanceFact{LatitudeDegrees: tt.lat, LongitudeDegrees: tt.lon, ObservedAt: &later})
			require.True(t, ok)
			require.InDelta(t, tt.want, track, 0.0001)
		})
	}
	_, ok := DerivedGroundTrack(nil, &SurveillanceFact{ObservedAt: &later})
	require.False(t, ok, "first sample has no track")
	_, ok = DerivedGroundTrack(previous, &SurveillanceFact{ObservedAt: &later})
	require.False(t, ok, "stationary sample has no track")
	_, ok = DerivedGroundTrack(previous, &SurveillanceFact{LongitudeDegrees: 1, ObservedAt: &now})
	require.False(t, ok, "duplicate or out of order timestamp has no track")
}
