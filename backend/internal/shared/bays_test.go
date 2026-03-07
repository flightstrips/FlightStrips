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

	bay := GetDepartureBay(strip, nil, 200)
	if bay != BAY_CLEARED {
		t.Fatalf("expected CLEARED below threshold, got %s", bay)
	}

	strip.Position.Altitude = AirportElevation + 600
	bay = GetDepartureBay(strip, nil, 500)
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

	bay := GetDepartureBayFromPosition(AirportLatitude, AirportLongitude, int64(AirportElevation+600), existing, 500)
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

	bay := GetDepartureBayFromPosition(AirportLatitude, AirportLongitude, int64(AirportElevation+600), existing, 500)
	if bay != BAY_DEPART {
		t.Fatalf("expected DEPART to remain unchanged, got %s", bay)
	}
}
