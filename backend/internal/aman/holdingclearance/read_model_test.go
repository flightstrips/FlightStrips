package holdingclearance

import (
	"slices"
	"testing"
	"time"

	"FlightStrips/internal/aman"

	"github.com/stretchr/testify/require"
)

func TestBuildReadModelSelectsOnlyAuthoritativeEnrouteArrivals(t *testing.T) {
	now := time.Date(2026, 9, 11, 23, 50, 0, 0, time.UTC)
	altitude := int32(12000)
	eligible := readModelFlight("flight-2", " sas200 ", " ekch ", aman.HoldingClearanceEnroute, "OLPIB", "0005", &altitude, now)
	eligible.Prediction = &aman.Prediction{Publishable: true, HoldingPlan: &aman.HoldingPlan{ApproachReleaseTime: now.Add(15 * time.Minute)}}
	missing := readModelFlight("flight-1", "SAS100", "EKCH", aman.HoldingClearanceEnroute, "SOK", "", nil, now)
	state := aman.AirportState{Airport: "ekch", Flights: []aman.AMANFlight{
		eligible,
		readModelFlight("tsa", "TSA1", "EKCH", aman.HoldingClearanceTSA, "AREA", "0010", &altitude, now),
		readModelFlight("departure", "DEP1", "ESSA", aman.HoldingClearanceEnroute, "SOK", "0010", &altitude, now),
		readModelFlight("cancelled", "CAN1", "EKCH", "", "", "", nil, now),
		missing,
	}}

	model := BuildReadModel(state)
	require.Equal(t, []Entry{
		{Callsign: "SAS100", Holding: "SOK", SourceStatus: aman.DataFresh, ObservedAt: now},
		{Callsign: " sas200 ", Holding: "OLPIB", EAT: readModelTimePointer(now.Add(15 * time.Minute)), ClearedAltitude: &altitude, SourceStatus: aman.DataFresh, ObservedAt: now},
	}, model.Entries)
}

func readModelTimePointer(value time.Time) *time.Time { return &value }

func TestBuildReadModelOmitsRemovedAndLandedAircraft(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	for _, terminalState := range []aman.FlightState{aman.StateRemoved, aman.StateLanded} {
		flight := readModelFlight("", "SAS123", "EKCH", aman.HoldingClearanceEnroute, "OLPIB", "1210", nil, now)
		flight.State, flight.DataStatus = terminalState, aman.DataDisconnected
		flight.Prediction = &aman.Prediction{Publishable: true, HoldingPlan: &aman.HoldingPlan{ApproachReleaseTime: now.Add(10 * time.Minute)}}
		require.Empty(t, BuildReadModel(aman.AirportState{Airport: "EKCH", Flights: []aman.AMANFlight{flight}}).Entries)
	}
}

func TestBuildReadModelOrderingDoesNotDependOnAggregateOrder(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	state := aman.AirportState{Airport: "EKCH", Flights: []aman.AMANFlight{
		readModelFlight("b", "same", "EKCH", aman.HoldingClearanceEnroute, "SOK", "", nil, now),
		readModelFlight("a", "SAME", "EKCH", aman.HoldingClearanceEnroute, "OLPIB", "", nil, now),
	}}
	want := BuildReadModel(state)
	slices.Reverse(state.Flights)

	require.Equal(t, want, BuildReadModel(state))
	require.Equal(t, []aman.Callsign{"SAME", "same"}, []aman.Callsign{want.Entries[0].Callsign, want.Entries[1].Callsign})
}

func TestBuildReadModelRetainsInvalidLegacyEATAsMissing(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	flight := readModelFlight("flight-1", "SAS100", "EKCH", aman.HoldingClearanceEnroute, "SOK", "25:00", nil, now)

	model := BuildReadModel(aman.AirportState{Airport: "EKCH", Flights: []aman.AMANFlight{flight}})
	require.Len(t, model.Entries, 1)
	require.Nil(t, model.Entries[0].EAT)
}

func TestBuildReadModelUsesCalculatedEATWithoutEuroScopeAssignment(t *testing.T) {
	now := time.Date(2026, 10, 4, 23, 50, 0, 0, time.UTC)
	release := now.Add(20 * time.Minute)
	for _, assigned := range []string{"", "2355", "25:00"} {
		t.Run("assignment="+assigned, func(t *testing.T) {
			flight := readModelFlight("", "SAS123", "EKCH", aman.HoldingClearanceEnroute, "OLPIB", assigned, nil, now)
			flight.Prediction = &aman.Prediction{Publishable: true, HoldingPlan: &aman.HoldingPlan{ApproachReleaseTime: release}}
			model := BuildReadModel(aman.AirportState{Airport: "EKCH", Flights: []aman.AMANFlight{flight}})
			require.Equal(t, &release, model.Entries[0].EAT)
			require.Equal(t, assigned, flight.HoldingClearance.HoldEAT, "display must not change the assigned clearance")
		})
	}
}

func TestBuildReadModelDoesNotSubstituteAssignedEATForMissingPrediction(t *testing.T) {
	now := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	for _, prediction := range []*aman.Prediction{
		nil,
		{Publishable: true},
		{Publishable: false, HoldingPlan: &aman.HoldingPlan{ApproachReleaseTime: now.Add(time.Minute)}},
		{Publishable: true, HoldingPlan: &aman.HoldingPlan{}},
	} {
		flight := readModelFlight("", "SAS123", "EKCH", aman.HoldingClearanceEnroute, "OLPIB", "0810", nil, now)
		flight.Prediction = prediction
		model := BuildReadModel(aman.AirportState{Airport: "EKCH", Flights: []aman.AMANFlight{flight}})
		require.Nil(t, model.Entries[0].EAT)
	}
}

func readModelFlight(_ aman.Callsign, callsign, destination string, holdType aman.HoldingClearanceType, hold, eat string, altitude *int32, observedAt time.Time) aman.AMANFlight {
	return aman.AMANFlight{
		Callsign: callsign, DataStatus: aman.DataFresh,
		LatestObservation: &aman.FlightObservation{Destination: destination},
		HoldingClearance:  &aman.HoldingClearance{Hold: hold, HoldType: holdType, HoldEAT: eat, ClearedAltitude: altitude, ObservedAt: observedAt},
	}
}

func TestBuildReadModelShowsRetainedEATDuringSourceOutage(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	release := now.Add(20 * time.Minute)
	for _, status := range []aman.DataStatus{aman.DataStale, aman.DataDisconnected} {
		t.Run(string(status), func(t *testing.T) {
			flight := readModelFlight("", "SAS123", "EKCH", aman.HoldingClearanceEnroute, "OLPIB", "", nil, now)
			flight.DataStatus = status
			flight.Prediction = &aman.Prediction{Publishable: true, GeneratedAt: now.Add(-15 * time.Minute), HoldingPlan: &aman.HoldingPlan{ApproachReleaseTime: release}}
			model := BuildReadModel(aman.AirportState{Airport: "EKCH", GeneratedAt: now, Flights: []aman.AMANFlight{flight}})
			require.Len(t, model.Entries, 1)
			require.Equal(t, &release, model.Entries[0].EAT)
			require.Equal(t, status, model.Entries[0].SourceStatus)
		})
	}
}

func TestMissingHoldingReleaseExplainsBlockerAndResolvesWithAcceptedPlan(t *testing.T) {
	now := time.Date(2026, 10, 4, 19, 0, 0, 0, time.UTC)
	flight := readModelFlight("", "LATER", "EKCH", aman.HoldingClearanceEnroute, "LUGAS", "", nil, now)
	blocker := aman.Callsign("EARLIER")
	entry := now.Add(time.Minute)
	flight.Slot = &aman.Slot{Time: now.Add(30 * time.Minute)}
	flight.Prediction = &aman.Prediction{Publishable: true, RawTETA: now.Add(20 * time.Minute), HoldingFixETA: &entry, HoldingPlanBlockedBy: &blocker}
	state := aman.AirportState{Airport: "EKCH", Flights: []aman.AMANFlight{flight}}
	model := BuildReadModel(state)
	require.Nil(t, model.Entries[0].EAT)
	require.Len(t, model.Warnings, 1)
	require.Equal(t, "holding_release_order_conflict", model.Warnings[0].Code)
	require.Equal(t, flight.Callsign, *model.Warnings[0].Callsign)
	require.Equal(t, blocker, *model.Warnings[0].RelatedCallsign)
	require.Contains(t, model.Warnings[0].Message, "safe release behind EARLIER")
	require.Contains(t, model.Warnings[0].Message, "assigned runway slot is retained")
	state.Flights[0].Prediction.HoldingPlanBlockedBy = nil
	state.Flights[0].Prediction.HoldingPlan = &aman.HoldingPlan{ApproachReleaseTime: now.Add(10 * time.Minute)}
	model = BuildReadModel(state)
	require.Empty(t, model.Warnings)
	require.Equal(t, now.Add(10*time.Minute), *model.Entries[0].EAT)
}

func TestMissingHoldingReleaseNamesMissingCalculationInput(t *testing.T) {
	now := time.Date(2026, 10, 4, 19, 0, 0, 0, time.UTC)
	flight := readModelFlight("", "SAS123", "EKCH", aman.HoldingClearanceEnroute, "OLPIB", "", nil, now)
	flight.Slot = &aman.Slot{Time: now.Add(30 * time.Minute)}
	flight.Prediction = &aman.Prediction{Publishable: true, RawTETA: now.Add(20 * time.Minute)}
	model := BuildReadModel(aman.AirportState{Airport: "EKCH", Flights: []aman.AMANFlight{flight}})
	require.Contains(t, model.Warnings[0].Message, "no time at the cleared holding fix")
	entry := now
	flight.Prediction.HoldingFixETA = &entry
	flight.Prediction.RawTETA = now.Add(35 * time.Minute)
	model = BuildReadModel(aman.AirportState{Airport: "EKCH", Flights: []aman.AMANFlight{flight}})
	require.Contains(t, model.Warnings[0].Message, "slot 19:30 UTC")
	require.Contains(t, model.Warnings[0].Message, "free-flight arrival 19:35 UTC")
	require.Nil(t, model.Entries[0].EAT, "a diagnostic must not invent a release for an infeasible slot")
}
