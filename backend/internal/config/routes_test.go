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

func TestGetArrivalTowerSector_TieBreaksByFirstInFile(t *testing.T) {
	original := standRoutes
	t.Cleanup(func() { standRoutes = original })

	// Both routes score 1 for active runway "22L" — first must win.
	standRoutes = []Route{
		{
			ForStandRanges: []StandRange{{Prefix: "A", From: 1, To: 99}},
			Active:         []string{"22L"},
			Path:           []string{"TE", "GW", "AA"},
		},
		{
			ForStandRanges: []StandRange{{Prefix: "A", From: 1, To: 99}},
			Active:         []string{"22L"},
			Path:           []string{"TW", "GW", "AA"},
		},
	}

	sector, ok := GetArrivalTowerSector([]string{"22L"})
	if !ok {
		t.Fatal("expected a tower sector to be found")
	}
	if sector != "TE" {
		t.Fatalf("expected first route (TE) to win tie, got %q", sector)
	}
}

func TestComputeToRunway_RequireAll_MatchesWhenAllPresent(t *testing.T) {
	original := runwayRoutes
	t.Cleanup(func() { runwayRoutes = original })

	runwayRoutes = map[string][]Route{
		"22R": {
			{Active: []string{}, Path: []string{"AD", "GW", "TW"}, ForRunway: "22R"},
			{Active: []string{"22R", "04L"}, RequireAll: true, Path: []string{"AD", "TW"}, ForRunway: "22R"},
		},
	}

	path, ok := ComputeToRunway([]string{"22R", "04L"}, "AD", "22R")
	if !ok {
		t.Fatal("expected a route to be found")
	}
	if len(path) != 2 || path[1] != "TW" {
		t.Fatalf("expected require_all route [AD TW], got %v", path)
	}
}

func TestComputeToRunway_RequireAll_FallsBackWhenPartialMatch(t *testing.T) {
	original := runwayRoutes
	t.Cleanup(func() { runwayRoutes = original })

	runwayRoutes = map[string][]Route{
		"22R": {
			{Active: []string{}, Path: []string{"AD", "GW", "TW"}, ForRunway: "22R"},
			{Active: []string{"22R", "04L"}, RequireAll: true, Path: []string{"AD", "TW"}, ForRunway: "22R"},
		},
	}

	path, ok := ComputeToRunway([]string{"22R"}, "AD", "22R")
	if !ok {
		t.Fatal("expected fallback route to be found")
	}
	if len(path) != 3 || path[1] != "GW" {
		t.Fatalf("expected fallback route [AD GW TW], got %v", path)
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

func TestGetDefaultAirborneControllerPriority(t *testing.T) {
	originalRoutes := airborneRoutes
	originalSectors := sectors
	t.Cleanup(func() {
		airborneRoutes = originalRoutes
		sectors = originalSectors
	})

	airborneRoutes = []AirborneRoutes{
		{Name: "K_DEP", UseAsDefault: true},
		{Name: "R_DEP"},
	}
	sectors = []Sector{
		{Name: "K_DEP", Owner: []string{"EKCH_K_DEP", "EKCH_R_DEP"}},
	}

	owners, err := GetDefaultAirborneControllerPriority()
	if err != nil {
		t.Fatalf("expected default owners, got error: %v", err)
	}

	if len(owners) != 2 || owners[0] != "EKCH_K_DEP" || owners[1] != "EKCH_R_DEP" {
		t.Fatalf("unexpected default airborne owner priority: %#v", owners)
	}
}

func TestGetDefaultAirborneControllerPriorityReturnsErrorWithoutDefaultRoute(t *testing.T) {
	originalRoutes := airborneRoutes
	t.Cleanup(func() {
		airborneRoutes = originalRoutes
	})

	airborneRoutes = []AirborneRoutes{
		{Name: "K_DEP"},
	}

	if _, err := GetDefaultAirborneControllerPriority(); err == nil {
		t.Fatal("expected error without default airborne route")
	}
}
