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
		{FlightID: "flight-1", Callsign: "SAS100", Holding: "SOK", SourceStatus: aman.DataFresh, ObservedAt: now},
		{FlightID: "flight-2", Callsign: " sas200 ", Holding: "OLPIB", EAT: readModelTimePointer(now.Add(15 * time.Minute)), ClearedAltitude: &altitude, SourceStatus: aman.DataFresh, ObservedAt: now},
	}, model.Entries)
}

func readModelTimePointer(value time.Time) *time.Time { return &value }

func TestBuildReadModelOrderingDoesNotDependOnAggregateOrder(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	state := aman.AirportState{Airport: "EKCH", Flights: []aman.AMANFlight{
		readModelFlight("b", "same", "EKCH", aman.HoldingClearanceEnroute, "SOK", "", nil, now),
		readModelFlight("a", "SAME", "EKCH", aman.HoldingClearanceEnroute, "OLPIB", "", nil, now),
	}}
	want := BuildReadModel(state)
	slices.Reverse(state.Flights)

	require.Equal(t, want, BuildReadModel(state))
	require.Equal(t, []aman.FlightID{"a", "b"}, []aman.FlightID{want.Entries[0].FlightID, want.Entries[1].FlightID})
}

func TestBuildReadModelRetainsInvalidLegacyEATAsMissing(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	flight := readModelFlight("flight-1", "SAS100", "EKCH", aman.HoldingClearanceEnroute, "SOK", "25:00", nil, now)

	model := BuildReadModel(aman.AirportState{Airport: "EKCH", Flights: []aman.AMANFlight{flight}})
	require.Len(t, model.Entries, 1)
	require.Nil(t, model.Entries[0].EAT)
}

func readModelFlight(id aman.FlightID, callsign, destination string, holdType aman.HoldingClearanceType, hold, eat string, altitude *int32, observedAt time.Time) aman.AMANFlight {
	return aman.AMANFlight{
		ID: id, CurrentCallsign: callsign, DataStatus: aman.DataFresh,
		LatestObservation: &aman.FlightObservation{Destination: destination},
		HoldingClearance:  &aman.HoldingClearance{Hold: hold, HoldType: holdType, HoldEAT: eat, ClearedAltitude: altitude, ObservedAt: observedAt},
	}
}
