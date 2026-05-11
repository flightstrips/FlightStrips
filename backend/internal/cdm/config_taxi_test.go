package cdm

import "testing"

func TestTaxiMinutesForPosition_MatchesConfiguredPolygon(t *testing.T) {
	cfg := NewDefaultAirportConfig("EKCH")
	cfg.TaxiZones = []CdmTaxiZone{
		{
			Airport: "EKCH",
			Runway:  "04L",
			Minutes: 14,
			Polygon: []CdmTaxiPoint{
				{Lat: 55.0, Lon: 12.0},
				{Lat: 55.0, Lon: 12.2},
				{Lat: 55.2, Lon: 12.2},
				{Lat: 55.2, Lon: 12.0},
			},
		},
	}

	got, ok := cfg.TaxiMinutesForPosition("04L", 55.1, 12.1)
	if !ok {
		t.Fatal("expected taxi-zone match")
	}
	if got != 14 {
		t.Fatalf("TaxiMinutesForPosition() = %d, want 14", got)
	}
}

func TestTaxiMinutesForPosition_ZeroCoordinatesDoNotMatch(t *testing.T) {
	cfg := NewDefaultAirportConfig("EKCH")
	cfg.TaxiZones = []CdmTaxiZone{
		{
			Airport: "EKCH",
			Runway:  "04L",
			Minutes: 14,
			Polygon: []CdmTaxiPoint{
				{Lat: 55.0, Lon: 12.0},
				{Lat: 55.0, Lon: 12.2},
				{Lat: 55.2, Lon: 12.2},
				{Lat: 55.2, Lon: 12.0},
			},
		},
	}

	if _, ok := cfg.TaxiMinutesForPosition("04L", 0, 0); ok {
		t.Fatal("expected zero coordinates to be treated as unset")
	}
}
