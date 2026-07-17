package vatsim

import (
	"FlightStrips/internal/models"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFiledArrivalETANearestUTCServiceDayAndMidnightRollover(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 10, 0, 0, time.UTC)
	eta, err := filedArrivalETA(now, "2350", "0040")
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 7, 12, 0, 30, 0, 0, time.UTC), eta)

	now = time.Date(2026, 7, 11, 23, 50, 0, 0, time.UTC)
	eta, err = filedArrivalETA(now, "0010", "0020")
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 7, 12, 0, 30, 0, 0, time.UTC), eta)
}

func TestCalculateArrivalETAUsesLiveOnlyForReliableAirborneMovement(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	flight := Flight{
		State:       FlightStateOnline,
		Latitude:    55,
		Longitude:   12,
		Altitude:    minimumLiveAltitude,
		Groundspeed: minimumLiveGroundspeed,
		FlightPlan:  FlightPlan{EOBT: "0800", EnrouteDuration: "0300"},
	}
	eta, ok := calculateArrivalETA(now, flight, AirportCoordinates{Latitude: 55, Longitude: 13})
	require.True(t, ok)
	assert.Equal(t, ETALive, eta.Source)
	assert.InDelta(t, 34.45, *eta.DistanceNM, 0.1)
	assert.Equal(t, int32(minimumLiveGroundspeed), *eta.Groundspeed)

	flight.Groundspeed = minimumLiveGroundspeed - 1
	eta, ok = calculateArrivalETA(now, flight, AirportCoordinates{Latitude: 55, Longitude: 13})
	require.True(t, ok)
	assert.Equal(t, ETAFiled, eta.Source)
	assert.Equal(t, time.Date(2026, 7, 12, 11, 0, 0, 0, time.UTC), eta.Time)

	flight.Groundspeed = minimumLiveGroundspeed
	flight.Altitude = minimumLiveAltitude - 1
	eta, ok = calculateArrivalETA(now, flight, AirportCoordinates{Latitude: 55, Longitude: 13})
	require.True(t, ok)
	assert.Equal(t, ETAFiled, eta.Source)
}

func TestGreatCircleDistanceNM(t *testing.T) {
	assert.InDelta(t, 60.04, greatCircleDistanceNM(0, 0, 0, 1), 0.1)
}

func TestAcceptedArrivalETAHoldsNormalFeedJitter(t *testing.T) {
	previousTime := time.Date(2026, 7, 12, 11, 0, 0, 0, time.UTC)
	previous := &models.ArrivalETA{Time: previousTime, Source: ETALive}
	candidate := models.ArrivalETA{Time: previousTime.Add(90 * time.Second), Source: ETALive}

	accepted, changed := acceptedArrivalETA(previous, candidate)
	assert.False(t, changed)
	assert.Equal(t, previousTime, accepted.Time)

	candidate.Time = previousTime.Add(3 * time.Minute)
	accepted, changed = acceptedArrivalETA(previous, candidate)
	assert.True(t, changed)
	assert.Equal(t, candidate.Time, accepted.Time)
}

func TestUpdateArrivalETARetainsLastEstimateWhenFeedCannotRecalculate(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	previous := &models.ArrivalETA{Time: now.Add(time.Hour), Source: ETAFiled, CalculatedAt: now}
	strip := &models.Strip{Callsign: "SAS101", Session: 7, ArrivalETA: previous}
	reconciler := &Reconciler{now: func() time.Time { return now }}

	changed, err := reconciler.updateArrivalETA(context.Background(), 7, strip, Flight{
		State:      FlightStatePrefile,
		FlightPlan: FlightPlan{Destination: "EKCH"},
	})
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Same(t, previous, strip.ArrivalETA)
}

func TestReconcileKeepsHiddenArrivalOutOfFinalAtETAminusFortyFiveMinutes(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	flight := Flight{
		CID: "1", Callsign: "SAS101", State: FlightStatePrefile, LastUpdated: now,
		FlightPlan: FlightPlan{Origin: "EGLL", Destination: "EKCH", EOBT: "0800", EnrouteDuration: "0245", Revision: 1},
	}
	cache := newReconciliationTestCache(now, flight)
	strips := &reconciliationTestStrips{bySession: map[int32][]*models.Strip{}}
	reconciler := newTestReconciler(cache, reconciliationTestSessions{items: []*models.Session{{ID: 7, Airport: "EKCH"}}}, strips, reconciliationTestAssignments{}, nil, time.Second, WithClock(func() time.Time { return now }))

	require.NoError(t, reconciler.Reconcile(context.Background()))
	require.Len(t, strips.created, 1)
	strip := strips.created[0]
	require.NotNil(t, strip.ArrivalETA)
	assert.Equal(t, ETAFiled, strip.ArrivalETA.Source)
	assert.Equal(t, now.Add(45*time.Minute), strip.ArrivalETA.Time)
	assert.Equal(t, hiddenArrivalBay, strip.Bay, "SAT must not move arrivals into FINAL")
	assert.Nil(t, strip.Stand, "ETA calculation must not allocate a stand")
}
