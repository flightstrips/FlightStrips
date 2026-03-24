package shared

import (
	"FlightStrips/internal/database"
	"FlightStrips/pkg/events/euroscope"
	"testing"
)

func TestGetDepartureBayUsesConfiguredAirborneThreshold(t *testing.T) {
	strip := euroscope.Strip{
		Origin:  "EKCH",
		Cleared: true,
	}
	strip.Position.Lat = AirportLatitude
	strip.Position.Lon = AirportLongitude
	strip.Position.Altitude = AirportElevation + 150

	bay := GetDepartureBay(strip, nil, 200, "EKCH")
	if bay != BAY_CLEARED {
		t.Fatalf("expected CLEARED below threshold, got %s", bay)
	}

	strip.Position.Altitude = AirportElevation + 600
	bay = GetDepartureBay(strip, nil, 500, "EKCH")
	if bay != BAY_AIRBORNE {
		t.Fatalf("expected AIRBORNE above configured threshold, got %s", bay)
	}
}

func TestGetDepartureBayWithoutPosition_UnclearedDepartureReturnsNotCleared(t *testing.T) {
	strip := euroscope.Strip{
		Origin:  "EKCH",
		Cleared: false,
	}

	bay := GetDepartureBay(strip, nil, 500, "EKCH")
	if bay != BAY_NOT_CLEARED {
		t.Fatalf("expected NOT_CLEARED without position, got %s", bay)
	}
}

func TestGetDepartureBayWithoutPosition_ClearedDepartureReturnsCleared(t *testing.T) {
	strip := euroscope.Strip{
		Origin:  "EKCH",
		Cleared: true,
	}

	bay := GetDepartureBay(strip, nil, 500, "EKCH")
	if bay != BAY_CLEARED {
		t.Fatalf("expected CLEARED without position, got %s", bay)
	}
}

func TestGetDepartureBayReevaluatesHiddenDepartureWithoutPosition(t *testing.T) {
	groundState := euroscope.GroundStateUnknown
	existing := &database.Strip{
		Origin:  "EKCH",
		Bay:     BAY_HIDDEN,
		Cleared: false,
		State:   &groundState,
	}
	strip := euroscope.Strip{
		Origin:      "EKCH",
		Cleared:     false,
		GroundState: euroscope.GroundStateUnknown,
	}

	bay := GetDepartureBay(strip, existing, 500, "EKCH")
	if bay != BAY_NOT_CLEARED {
		t.Fatalf("expected hidden departure to be reclassified as NOT_CLEARED, got %s", bay)
	}
}

func TestGetDepartureBayFromPositionTransitionsToAirborne(t *testing.T) {
	existing := database.Strip{
		Origin: "EKCH",
		Bay:    BAY_DEPART,
	}

	bay := GetDepartureBayFromPosition(AirportLatitude, AirportLongitude, int64(AirportElevation+600), existing, 500, "EKCH")
	if bay != BAY_AIRBORNE {
		t.Fatalf("expected AIRBORNE, got %s", bay)
	}
}

// TestGetDepartureBayFromPositionTransitionsToAirborne_NilState verifies that a strip
// in BAY_DEPART transitions to AIRBORNE above the altitude threshold even when the
// ground state is nil — the frontend does not always set the state when moving to rwy-dep.
func TestGetDepartureBayFromPositionTransitionsToAirborne_NilState(t *testing.T) {
	existing := database.Strip{
		Origin: "EKCH",
		Bay:    BAY_DEPART,
		State:  nil,
	}

	bay := GetDepartureBayFromPosition(AirportLatitude, AirportLongitude, int64(AirportElevation+600), existing, 500, "EKCH")
	if bay != BAY_AIRBORNE {
		t.Fatalf("expected AIRBORNE with nil state, got %s", bay)
	}
}

// TestGetDepartureBayFromPositionStaysInDepartBelowThreshold verifies that a strip in
// BAY_DEPART below the altitude threshold does not prematurely transition to AIRBORNE.
func TestGetDepartureBayFromPositionStaysInDepartBelowThreshold(t *testing.T) {
	existing := database.Strip{
		Origin: "EKCH",
		Bay:    BAY_DEPART,
	}

	bay := GetDepartureBayFromPosition(AirportLatitude, AirportLongitude, int64(AirportElevation+100), existing, 500, "EKCH")
	if bay != BAY_DEPART {
		t.Fatalf("expected DEPART below threshold, got %s", bay)
	}
}

func TestGetDepartureBayFromPositionWithoutPositionKeepsExistingBay(t *testing.T) {
	existing := database.Strip{
		Origin: "EKCH",
		Bay:    BAY_NOT_CLEARED,
	}

	bay := GetDepartureBayFromPosition(0, 0, int64(AirportElevation), existing, 500, "EKCH")
	if bay != BAY_NOT_CLEARED {
		t.Fatalf("expected existing bay without position, got %s", bay)
	}
}

func TestGetGroundState_TaxiLwrMapsTaxi(t *testing.T) {
	state := GetGroundState(BAY_TAXI_LWR)
	if state != euroscope.GroundStateTaxi {
		t.Fatalf("expected GroundStateTaxi for BAY_TAXI_LWR, got %s", state)
	}
}

func TestGetGroundState_TaxiMapsTaxi(t *testing.T) {
	state := GetGroundState(BAY_TAXI)
	if state != euroscope.GroundStateTaxi {
		t.Fatalf("expected GroundStateTaxi for BAY_TAXI, got %s", state)
	}
}

func TestGetGroundState_TaxiTwrMapsTaxi(t *testing.T) {
	state := GetGroundState(BAY_TAXI_TWR)
	if state != euroscope.GroundStateTaxi {
		t.Fatalf("expected GroundStateTaxi for BAY_TAXI_TWR, got %s", state)
	}
}

func TestGetGroundState_NonTaxiBaysDoNotMapTaxi(t *testing.T) {
	cases := []struct {
		bay      string
		expected string
	}{
		{BAY_PUSH, euroscope.GroundStatePush},
		{BAY_DEPART, euroscope.GroundStateLineup},
		{BAY_CLEARED, euroscope.GroundStateUnknown},
		{BAY_HIDDEN, euroscope.GroundStateUnknown},
	}
	for _, tc := range cases {
		got := GetGroundState(tc.bay)
		if got != tc.expected {
			t.Errorf("GetGroundState(%q) = %q, want %q", tc.bay, got, tc.expected)
		}
	}
}

func TestGetGroundState_DepartMapsLineup(t *testing.T) {
	state := GetGroundState(BAY_DEPART)
	if state != euroscope.GroundStateLineup {
		t.Fatalf("expected GroundStateLineup for BAY_DEPART (drag-and-drop), got %s", state)
	}
}

func TestGetDepartureBayFromGroundState_TaxiReturnsTaxi(t *testing.T) {
	existing := database.Strip{Bay: BAY_TAXI_LWR}
	bay := GetDepartureBayFromGroundState(euroscope.GroundStateTaxi, existing)
	if bay != BAY_TAXI {
		t.Fatalf("expected BAY_TAXI from GroundStateTaxi, got %s", bay)
	}
}

func TestGetDepartureBayFromGroundState_LineupReturnsDepart(t *testing.T) {
	existing := database.Strip{Bay: BAY_TAXI_LWR}
	bay := GetDepartureBayFromGroundState(euroscope.GroundStateLineup, existing)
	if bay != BAY_DEPART {
		t.Fatalf("expected BAY_DEPART from GroundStateLineup, got %s", bay)
	}
}

func TestGetDepartureBayFromGroundState_DepartReturnsDepart(t *testing.T) {
	existing := database.Strip{Bay: BAY_TAXI_LWR}
	bay := GetDepartureBayFromGroundState(euroscope.GroundStateDepart, existing)
	if bay != BAY_DEPART {
		t.Fatalf("expected BAY_DEPART from GroundStateDepart, got %s", bay)
	}
}

func TestGetDepartureBayFromGroundState_UnknownFallsBackToExisting(t *testing.T) {
	existing := database.Strip{Bay: BAY_TAXI_LWR}
	bay := GetDepartureBayFromGroundState("UNKNOWN_STATE", existing)
	if bay != BAY_TAXI_LWR {
		t.Fatalf("expected existing bay BAY_TAXI_LWR, got %s", bay)
	}
}
