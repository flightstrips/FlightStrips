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

	bay := GetDepartureBay(strip, nil, 200, "EKCH", true)
	if bay != BAY_CLEARED {
		t.Fatalf("expected CLEARED below threshold, got %s", bay)
	}

	strip.Position.Altitude = AirportElevation + 600
	bay = GetDepartureBay(strip, nil, 500, "EKCH", true)
	if bay != BAY_AIRBORNE {
		t.Fatalf("expected AIRBORNE above configured threshold, got %s", bay)
	}
}

func TestGetDepartureBayWithoutPosition_UnclearedDepartureReturnsNotCleared(t *testing.T) {
	strip := euroscope.Strip{
		Origin:  "EKCH",
		Cleared: false,
	}

	bay := GetDepartureBay(strip, nil, 500, "EKCH", true)
	if bay != BAY_NOT_CLEARED {
		t.Fatalf("expected NOT_CLEARED without position, got %s", bay)
	}
}

func TestGetDepartureBayWithoutPosition_ClearedDepartureReturnsCleared(t *testing.T) {
	strip := euroscope.Strip{
		Origin:  "EKCH",
		Cleared: true,
	}

	bay := GetDepartureBay(strip, nil, 500, "EKCH", true)
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

	bay := GetDepartureBay(strip, existing, 500, "EKCH", true)
	if bay != BAY_NOT_CLEARED {
		t.Fatalf("expected hidden departure to be reclassified as NOT_CLEARED, got %s", bay)
	}
}

func TestGetDepartureBayArrivalTransitionFromHiddenNonArrivalReturnsArrivalHidden(t *testing.T) {
	existing := &database.Strip{
		Origin: "ESSA",
		Bay:    BAY_HIDDEN,
	}
	strip := euroscope.Strip{
		Destination: "EKCH",
	}

	bay := GetDepartureBay(strip, existing, 500, "EKCH", true)
	if bay != BAY_ARR_HIDDEN {
		t.Fatalf("expected reclassified arrival to start in ARR_HIDDEN, got %s", bay)
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

func TestGetDepartureBayFromPositionArrivalHiddenPreserved(t *testing.T) {
	existing := database.Strip{
		Destination: "EKCH",
		Bay:         BAY_HIDDEN,
	}

	bay := GetDepartureBayFromPosition(AirportLatitude, AirportLongitude, int64(AirportElevation), existing, 500, "EKCH")
	if bay != BAY_HIDDEN {
		t.Fatalf("expected auto-hidden arrival to remain HIDDEN, got %s", bay)
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

func TestGetGroundState_StandMapsParked(t *testing.T) {
	state := GetGroundState(BAY_STAND)
	if state != euroscope.GroundStateParked {
		t.Fatalf("expected GroundStateParked for BAY_STAND, got %s", state)
	}
}

func TestGetDepartureBayFromGroundState_TaxiReturnsTaxi(t *testing.T) {
	// When the existing bay is a plain TAXI/PUSH state, TAXI ground state should assign BAY_TAXI.
	existing := database.Strip{Origin: "EKCH", Bay: BAY_PUSH}
	bay := GetDepartureBayFromGroundState(euroscope.GroundStateTaxi, existing, "EKCH", true)
	if bay != BAY_TAXI {
		t.Fatalf("expected BAY_TAXI from GroundStateTaxi with PUSH existing bay, got %s", bay)
	}
}

func TestGetDepartureBayFromGroundState_TaxiLwrPreserved(t *testing.T) {
	// Task 078: when the strip is already in TAXI_LWR, a TAXI ground state must not move it backward.
	existing := database.Strip{Origin: "EKCH", Bay: BAY_TAXI_LWR}
	bay := GetDepartureBayFromGroundState(euroscope.GroundStateTaxi, existing, "EKCH", true)
	if bay != BAY_TAXI_LWR {
		t.Fatalf("expected TAXI_LWR to be preserved on TAXI ground state, got %s", bay)
	}
}

func TestGetDepartureBayFromGroundState_TaxiTwrPreserved(t *testing.T) {
	// Task 078: when the strip is already in TAXI_TWR, a TAXI ground state must not move it backward.
	existing := database.Strip{Origin: "EKCH", Bay: BAY_TAXI_TWR}
	bay := GetDepartureBayFromGroundState(euroscope.GroundStateTaxi, existing, "EKCH", true)
	if bay != BAY_TAXI_TWR {
		t.Fatalf("expected TAXI_TWR to be preserved on TAXI ground state, got %s", bay)
	}
}

func TestGetDepartureBayFromGroundState_LineupReturnsDepart(t *testing.T) {
	existing := database.Strip{Origin: "EKCH", Bay: BAY_TAXI_LWR}
	bay := GetDepartureBayFromGroundState(euroscope.GroundStateLineup, existing, "EKCH", true)
	if bay != BAY_DEPART {
		t.Fatalf("expected BAY_DEPART from GroundStateLineup, got %s", bay)
	}
}

func TestGetDepartureBayFromGroundState_DepartReturnsDepart(t *testing.T) {
	existing := database.Strip{Origin: "EKCH", Bay: BAY_TAXI_LWR}
	bay := GetDepartureBayFromGroundState(euroscope.GroundStateDepart, existing, "EKCH", true)
	if bay != BAY_DEPART {
		t.Fatalf("expected BAY_DEPART from GroundStateDepart, got %s", bay)
	}
}

func TestGetDepartureBayFromGroundState_UnknownFallsBackToExisting(t *testing.T) {
	existing := database.Strip{Origin: "EKCH", Bay: BAY_TAXI_LWR}
	bay := GetDepartureBayFromGroundState("UNKNOWN_STATE", existing, "EKCH", true)
	if bay != BAY_TAXI_LWR {
		t.Fatalf("expected existing bay BAY_TAXI_LWR, got %s", bay)
	}
}

func TestGetDepartureBayFromGroundState_ArrivalPreservesExistingBay(t *testing.T) {
	existing := database.Strip{
		Destination: "EKCH",
		Bay:         BAY_FINAL,
	}

	bay := GetDepartureBayFromGroundState(euroscope.GroundStateDepart, existing, "EKCH", true)
	if bay != BAY_FINAL {
		t.Fatalf("expected arrival to keep BAY_FINAL, got %s", bay)
	}
}

func TestGetDepartureBayFromGroundState_ArrivalHiddenPreserved(t *testing.T) {
	existing := database.Strip{
		Destination: "EKCH",
		Bay:         BAY_HIDDEN,
	}

	bay := GetDepartureBayFromGroundState(euroscope.GroundStateDepart, existing, "EKCH", true)
	if bay != BAY_HIDDEN {
		t.Fatalf("expected auto-hidden arrival to remain HIDDEN, got %s", bay)
	}
}

func TestPromoteTaxiBayForSoloTwr_TaxiNoGnd_ReturnsTaxiLwr(t *testing.T) {
	if got := PromoteTaxiBayForSoloTwr(BAY_TAXI, false); got != BAY_TAXI_LWR {
		t.Fatalf("expected BAY_TAXI_LWR, got %s", got)
	}
}

func TestPromoteTaxiBayForSoloTwr_TaxiGndOnline_ReturnsTaxi(t *testing.T) {
	if got := PromoteTaxiBayForSoloTwr(BAY_TAXI, true); got != BAY_TAXI {
		t.Fatalf("expected BAY_TAXI unchanged, got %s", got)
	}
}

func TestPromoteTaxiBayForSoloTwr_NonTaxiBay_Unchanged(t *testing.T) {
	for _, bay := range []string{BAY_PUSH, BAY_TAXI_LWR, BAY_DEPART, BAY_CLEARED} {
		if got := PromoteTaxiBayForSoloTwr(bay, false); got != bay {
			t.Fatalf("expected %s unchanged, got %s", bay, got)
		}
	}
}
