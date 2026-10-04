package aman

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDerivedGroundspeedUsesFreshMotionAndRejectsStaleOrStationarySamples(t *testing.T) {
	now := time.Now().UTC()
	later := now.Add(time.Second)
	previous := &FlightObservation{Surveillance: &SurveillanceFact{ObservedAt: &now}}
	current := &SurveillanceFact{LongitudeDegrees: 0.001, ObservedAt: &later}
	speed, ok := DerivedGroundspeed(previous, current)
	require.True(t, ok)
	require.InDelta(t, 216.145, speed, 0.01)
	current.LongitudeDegrees = 0
	_, ok = DerivedGroundspeed(previous, current)
	require.False(t, ok)
	current.LongitudeDegrees = 0.001
	later = now.Add(31 * time.Second)
	_, ok = DerivedGroundspeed(previous, current)
	require.False(t, ok)
	later = now
	_, ok = DerivedGroundspeed(previous, current)
	require.False(t, ok)
	later = now.Add(time.Second)
	current.LongitudeDegrees = 1
	_, ok = DerivedGroundspeed(previous, current)
	require.False(t, ok)
	_, ok = DerivedGroundspeed(nil, current)
	require.False(t, ok)
}
