package aman

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFreshArrivalSurveillance(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	require.False(t, HasFreshArrivalSurveillance(nil, now))
	for _, tc := range []struct {
		name   string
		change func(*FlightObservation)
		want   bool
	}{
		{"fresh", func(o *FlightObservation) {}, true},
		{"two_minute_boundary", func(o *FlightObservation) { *o.Surveillance.ObservedAt = now.Add(-ArrivalSurveillanceFreshness) }, true},
		{"expired", func(o *FlightObservation) {
			*o.Surveillance.ObservedAt = now.Add(-ArrivalSurveillanceFreshness - time.Nanosecond)
		}, false},
		{"future", func(o *FlightObservation) { *o.Surveillance.ObservedAt = now.Add(time.Second) }, false},
		{"missing", func(o *FlightObservation) { o.Missing = true }, false},
		{"stale", func(o *FlightObservation) { o.SourceStatus = DataStale }, false},
		{"disconnected", func(o *FlightObservation) { o.SourceStatus = DataDisconnected }, false},
		{"no_surveillance", func(o *FlightObservation) { o.Surveillance = nil }, false},
		{"no_time", func(o *FlightObservation) { o.Surveillance.ObservedAt = nil }, false},
		{"no_speed", func(o *FlightObservation) { o.Surveillance.GroundspeedKnots = nil }, false},
		{"no_altitude", func(o *FlightObservation) { o.Surveillance.AltitudeFeet = nil }, false},
		{"grounded", func(o *FlightObservation) { *o.Surveillance.AltitudeFeet, *o.Surveillance.GroundspeedKnots = 500, 40 }, false},
		{"slow_airborne", func(o *FlightObservation) { *o.Surveillance.AltitudeFeet, *o.Surveillance.GroundspeedKnots = 1500, 40 }, true},
		{"zero_speed", func(o *FlightObservation) { *o.Surveillance.GroundspeedKnots = 0 }, false},
		{"nan_speed", func(o *FlightObservation) { *o.Surveillance.GroundspeedKnots = math.NaN() }, false},
		{"infinite_speed", func(o *FlightObservation) { *o.Surveillance.GroundspeedKnots = math.Inf(1) }, false},
		{"negative_altitude", func(o *FlightObservation) { *o.Surveillance.AltitudeFeet = -1 }, false},
		{"invalid_latitude", func(o *FlightObservation) { o.Surveillance.LatitudeDegrees = 91 }, false},
		{"nan_longitude", func(o *FlightObservation) { o.Surveillance.LongitudeDegrees = math.NaN() }, false},
		{"invalid_longitude", func(o *FlightObservation) { o.Surveillance.LongitudeDegrees = math.Inf(-1) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observed, altitude, speed := now, 10000, 200.0
			o := FlightObservation{SourceStatus: DataFresh, Surveillance: &SurveillanceFact{ObservedAt: &observed, AltitudeFeet: &altitude, GroundspeedKnots: &speed, LatitudeDegrees: 56, LongitudeDegrees: 12}}
			tc.change(&o)
			require.Equal(t, tc.want, HasFreshArrivalSurveillance(&o, now))
		})
	}
}
