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

func TestGetDepartureBayFromPositionTransitionsToAirborne(t *testing.T) {
	state := euroscope.GroundStateDepart
	existing := database.Strip{
		Origin: "EKCH",
		Bay:    BAY_DEPART,
		State:  &state,
	}

	bay := GetDepartureBayFromPosition(AirportLatitude, AirportLongitude, int64(AirportElevation+600), existing, 500, "EKCH")
	if bay != BAY_AIRBORNE {
		t.Fatalf("expected AIRBORNE, got %s", bay)
	}
}

func TestGetDepartureBayFromPositionStaysInDepartForNonDepartureState(t *testing.T) {
	state := euroscope.GroundStateTaxi
	existing := database.Strip{
		Origin: "EKCH",
		Bay:    BAY_DEPART,
		State:  &state,
	}

	bay := GetDepartureBayFromPosition(AirportLatitude, AirportLongitude, int64(AirportElevation+600), existing, 500, "EKCH")
	if bay != BAY_DEPART {
		t.Fatalf("expected DEPART to remain unchanged, got %s", bay)
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
		{BAY_DEPART, euroscope.GroundStateDepart},
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

func TestGetDepartureBayFromGroundState_TaxiReturnsTaxi(t *testing.T) {
	existing := database.Strip{Bay: BAY_TAXI_LWR}
	bay := GetDepartureBayFromGroundState(euroscope.GroundStateTaxi, existing)
	if bay != BAY_TAXI {
		t.Fatalf("expected BAY_TAXI from GroundStateTaxi, got %s", bay)
	}
}

func TestGetDepartureBayFromGroundState_UnknownFallsBackToExisting(t *testing.T) {
	existing := database.Strip{Bay: BAY_TAXI_LWR}
	bay := GetDepartureBayFromGroundState("UNKNOWN_STATE", existing)
	if bay != BAY_TAXI_LWR {
		t.Fatalf("expected existing bay BAY_TAXI_LWR, got %s", bay)
	}
}
