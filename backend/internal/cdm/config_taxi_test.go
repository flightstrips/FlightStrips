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

func TestDeiceTaxiMinutesForPosition_UsesSelectedPlatform(t *testing.T) {
	cfg := NewDefaultAirportConfig("EKCH")
	cfg.TaxiZones = []CdmTaxiZone{
		{
			Airport:           "EKCH",
			Runway:            "22R",
			Minutes:           11,
			RemoteTaxiMinutes: []int{2, 3, 5},
			Polygon: []CdmTaxiPoint{
				{Lat: 55.0, Lon: 12.0},
				{Lat: 55.0, Lon: 12.2},
				{Lat: 55.2, Lon: 12.2},
				{Lat: 55.2, Lon: 12.0},
			},
		},
	}

	got, ok := cfg.DeiceTaxiMinutesForPosition("22R", "B", 55.1, 12.1)
	if !ok {
		t.Fatal("expected de-icing taxi-zone match")
	}
	if got != 3 {
		t.Fatalf("DeiceTaxiMinutesForPosition() = %d, want 3", got)
	}
}

func TestDeiceTaxiMinutesForRunway_RejectsUnknownPlatform(t *testing.T) {
	cfg := NewDefaultAirportConfig("EKCH")
	cfg.TaxiZones = []CdmTaxiZone{{
		Runway:            "22R",
		RemoteTaxiMinutes: []int{2, 3, 5},
	}}

	if _, ok := cfg.DeiceTaxiMinutesForRunway("22R", "X"); ok {
		t.Fatal("expected unknown platform not to match")
	}
}
