package config

import "testing"

func TestGetArrivalTowerSector_ReturnsFirstSectorOfMatchingRoute(t *testing.T) {
	original := standRoutes
	t.Cleanup(func() { standRoutes = original })

	standRoutes = []Route{
		{
			ForStandRanges: []StandRange{{Prefix: "A", From: 1, To: 99}},
			Active:         []string{"22L"},
			Path:           []string{"TE", "GW", "AA"},
		},
	}

	sector, ok := GetArrivalTowerSector([]string{"22L"})
	if !ok {
		t.Fatal("expected a tower sector to be found")
	}
	if sector != "TE" {
		t.Fatalf("expected TE, got %q", sector)
	}
}

func TestGetArrivalTowerSector_RespectsActiveRunways(t *testing.T) {
	original := standRoutes
	t.Cleanup(func() { standRoutes = original })

	standRoutes = []Route{
		{
			ForStandRanges: []StandRange{{Prefix: "A", From: 1, To: 99}},
			Active:         []string{"22L"},
			Path:           []string{"TE", "GW", "AA"},
		},
		{
			ForStandRanges: []StandRange{{Prefix: "A", From: 1, To: 99}},
			Active:         []string{"04L"},
			Path:           []string{"TW", "GW", "AA"},
		},
	}

	sector, ok := GetArrivalTowerSector([]string{"04L"})
	if !ok {
		t.Fatal("expected a tower sector to be found")
	}
	if sector != "TW" {
		t.Fatalf("expected TW for active runway 04L, got %q", sector)
	}
}

func TestGetArrivalTowerSector_ReturnsFalseWhenNoMatchingRoute(t *testing.T) {
	original := standRoutes
	t.Cleanup(func() { standRoutes = original })

	standRoutes = []Route{
		{
			ForStandRanges: []StandRange{{Prefix: "A", From: 1, To: 99}},
			Active:         []string{"22L"},
			Path:           []string{"TE", "GW", "AA"},
		},
	}

	_, ok := GetArrivalTowerSector([]string{"99X"})
	if ok {
		t.Fatal("expected no match for unknown runway")
	}
}

func TestGetArrivalTowerSector_ReturnsFalseWhenNoStandRoutes(t *testing.T) {
	original := standRoutes
	t.Cleanup(func() { standRoutes = original })

	standRoutes = nil

	_, ok := GetArrivalTowerSector([]string{"22L"})
	if ok {
		t.Fatal("expected false with no stand routes configured")
	}
}

func TestGetAirborneControllerPriority(t *testing.T) {
	originalRoutes := airborneRoutes
	originalSectors := sectors
	t.Cleanup(func() {
		airborneRoutes = originalRoutes
		sectors = originalSectors
	})

	airborneRoutes = []AirborneRoutes{
		{Name: "K_DEP", Sids: []string{"BETUD2A"}},
	}
	sectors = []Sector{
		{Name: "K_DEP", Owner: []string{"EKCH_K_DEP", "EKCH_W_APP"}},
	}

	owners, err := GetAirborneControllerPriority("BETUD2A")
	if err != nil {
		t.Fatalf("expected owners, got error: %v", err)
	}

	if len(owners) != 2 || owners[0] != "EKCH_K_DEP" || owners[1] != "EKCH_W_APP" {
		t.Fatalf("unexpected airborne owner priority: %#v", owners)
	}
}

func TestGetAirborneControllerPriorityReturnsErrorForUnknownSid(t *testing.T) {
	originalRoutes := airborneRoutes
	t.Cleanup(func() {
		airborneRoutes = originalRoutes
	})

	airborneRoutes = nil

	if _, err := GetAirborneControllerPriority("UNKNOWN1A"); err == nil {
		t.Fatal("expected error for unknown SID")
	}
}
