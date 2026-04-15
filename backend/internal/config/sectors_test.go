package config

import (
	"slices"
	"testing"
)

func TestMatchScore_EmptyActiveAlwaysMatches(t *testing.T) {
	s := Sector{Name: "DEL", Active: []string{}}
	if matchScore(s, []string{"22L", "22R"}) != 0 {
		t.Fatal("empty active list should return score 0")
	}
}

func TestMatchScore_NoOverlapReturnsMinusOne(t *testing.T) {
	s := Sector{Name: "TE", Active: []string{"22L", "22R"}}
	if matchScore(s, []string{"04L", "04R"}) != -1 {
		t.Fatal("no overlap should return -1")
	}
}

func TestMatchScore_PartialOverlap(t *testing.T) {
	s := Sector{Name: "TE", Active: []string{"22L", "22R"}}
	if matchScore(s, []string{"22R", "30"}) != 1 {
		t.Fatal("one matching runway should return score 1")
	}
}

func TestMatchScore_FullOverlap(t *testing.T) {
	s := Sector{Name: "TE", Active: []string{"22L", "22R"}}
	if matchScore(s, []string{"22L", "22R"}) != 2 {
		t.Fatal("two matching runways should return score 2")
	}
}

func TestGetControllerSectors_MostSpecificWins(t *testing.T) {
	original := sectors
	t.Cleanup(func() { sectors = original })

	// Two configs for the same sector name; [30, 22R] should win when
	// active runways are {30, 22R} because it has more matches.
	sectors = []Sector{
		{Name: "TE", Active: []string{"22L", "22R"}, Owner: []string{"EKCH_A_TWR"}, Region: []string{"TOWER_EAST"}},
		{Name: "TE", Active: []string{"30", "22R"}, Owner: []string{"EKCH_A_TWR"}, Region: []string{"TOWER_EAST_ALT"}},
	}

	controllers := []*Position{{Name: "EKCH_A_TWR", Frequency: "118.100"}}
	result := GetControllerSectors(controllers, []string{"30", "22R"})

	sectorList := result["118.100"]
	if len(sectorList) != 1 {
		t.Fatalf("expected 1 sector, got %d", len(sectorList))
	}
	if !slices.Contains(sectorList[0].Region, "TOWER_EAST_ALT") {
		t.Fatalf("expected TOWER_EAST_ALT (the [30,22R] config), got region %v", sectorList[0].Region)
	}
}

func TestGetControllerSectors_AlwaysOnSectorIncluded(t *testing.T) {
	original := sectors
	t.Cleanup(func() { sectors = original })

	sectors = []Sector{
		{Name: "DEL", Active: []string{}, Owner: []string{"EKCH_DEL"}, Region: []string{"DELIVERY"}},
	}

	controllers := []*Position{{Name: "EKCH_DEL", Frequency: "121.900"}}
	result := GetControllerSectors(controllers, []string{"22L"})

	if len(result["121.900"]) != 1 {
		t.Fatal("always-on sector should be included regardless of active runways")
	}
}

func TestGetControllerSectors_NoMatchExcludesSector(t *testing.T) {
	original := sectors
	t.Cleanup(func() { sectors = original })

	sectors = []Sector{
		{Name: "TE", Active: []string{"22L", "22R"}, Owner: []string{"EKCH_A_TWR"}},
	}

	controllers := []*Position{{Name: "EKCH_A_TWR", Frequency: "118.100"}}
	result := GetControllerSectors(controllers, []string{"04L", "04R"})

	if len(result["118.100"]) != 0 {
		t.Fatal("sector should not be included when no runways match")
	}
}

func TestGetControllerSectors_TieBreaksByFirstConfig(t *testing.T) {
	original := sectors
	t.Cleanup(func() { sectors = original })

	// [22L, 22R] and [30, 22L] both score 1 when active is {22L}.
	// The first config in the list must win.
	sectors = []Sector{
		{Name: "TE", Active: []string{"22L", "22R"}, Owner: []string{"EKCH_A_TWR"}, Region: []string{"EAST"}},
		{Name: "TE", Active: []string{"30", "22L"}, Owner: []string{"EKCH_A_TWR"}, Region: []string{"MIXED"}},
	}

	controllers := []*Position{{Name: "EKCH_A_TWR", Frequency: "118.100"}}
	result := GetControllerSectors(controllers, []string{"22L"})

	sectorList := result["118.100"]
	if len(sectorList) != 1 {
		t.Fatalf("expected 1 sector, got %d", len(sectorList))
	}
	if !slices.Contains(sectorList[0].Region, "EAST") {
		t.Fatalf("first config should win on tie, got region %v", sectorList[0].Region)
	}
}

func TestMatchScore_RequireAll_AllPresent(t *testing.T) {
	s := Sector{Name: "TE", Active: []string{"12", "30"}, RequireAll: true}
	if matchScore(s, []string{"12", "30"}) != 2 {
		t.Fatal("require_all with all runways present should return full score")
	}
}

func TestMatchScore_RequireAll_PartialReturnsMinusOne(t *testing.T) {
	s := Sector{Name: "TE", Active: []string{"12", "30"}, RequireAll: true}
	if matchScore(s, []string{"12"}) != -1 {
		t.Fatal("require_all with only partial match should return -1")
	}
}

func TestMatchScore_RequireAll_NoMatchReturnsMinusOne(t *testing.T) {
	s := Sector{Name: "TE", Active: []string{"12", "30"}, RequireAll: true}
	if matchScore(s, []string{"22L", "22R"}) != -1 {
		t.Fatal("require_all with no match should return -1")
	}
}

func TestGetControllerSectors_RequireAllExcludedOnPartialMatch(t *testing.T) {
	original := sectors
	t.Cleanup(func() { sectors = original })

	sectors = []Sector{
		{Name: "TE", Active: []string{"12", "30"}, RequireAll: true, Owner: []string{"EKCH_A_TWR"}, Region: []string{"TOWER_12_EAST"}},
	}

	controllers := []*Position{{Name: "EKCH_A_TWR", Frequency: "118.100"}}
	result := GetControllerSectors(controllers, []string{"12"})

	if len(result["118.100"]) != 0 {
		t.Fatal("require_all sector should not match when only one of its runways is active")
	}
}

func TestGetControllerSectors_AlwaysOnLosesToSpecificMatch(t *testing.T) {
	original := sectors
	t.Cleanup(func() { sectors = original })

	// If both an always-on and a specific config exist for the same name,
	// the specific one wins when it matches.
	sectors = []Sector{
		{Name: "GW", Active: []string{}, Owner: []string{"EKCH_GND"}, Region: []string{"GROUND_FALLBACK"}},
		{Name: "GW", Active: []string{"22L"}, Owner: []string{"EKCH_GND"}, Region: []string{"GROUND_WEST"}},
	}

	controllers := []*Position{{Name: "EKCH_GND", Frequency: "121.600"}}
	result := GetControllerSectors(controllers, []string{"22L"})

	sectorList := result["121.600"]
	if len(sectorList) != 1 {
		t.Fatalf("expected 1 sector, got %d", len(sectorList))
	}
	if !slices.Contains(sectorList[0].Region, "GROUND_WEST") {
		t.Fatalf("specific match should win over always-on, got region %v", sectorList[0].Region)
	}
}

func TestGetSectorFromRegion_UsesArrivalDepartureSpecificSectors(t *testing.T) {
	original := sectors
	t.Cleanup(func() { sectors = original })

	sectors = []Sector{
		{
			Name:   "GW",
			Key:    "GWA",
			Region: []string{"GROUND_WEST"},
			Constraints: &constraints{
				Departure: false,
				Arrival:   true,
			},
		},
		{
			Name:   "GW",
			Key:    "GWD",
			Region: []string{"GROUND_WEST"},
			Constraints: &constraints{
				Departure: true,
				Arrival:   false,
			},
		},
	}

	region := &Region{Name: "GROUND_WEST"}

	arrivalSector, err := GetSectorFromRegion(region, true)
	if err != nil {
		t.Fatalf("expected arrival sector, got error: %v", err)
	}
	if arrivalSector != "GWA" {
		t.Fatalf("expected GWA for arrivals, got %q", arrivalSector)
	}

	departureSector, err := GetSectorFromRegion(region, false)
	if err != nil {
		t.Fatalf("expected departure sector, got error: %v", err)
	}
	if departureSector != "GWD" {
		t.Fatalf("expected GWD for departures, got %q", departureSector)
	}
}

func TestGetControllerSectors_KeepsDistinctKeysWithSamePublicName(t *testing.T) {
	original := sectors
	t.Cleanup(func() { sectors = original })

	sectors = []Sector{
		{Name: "GW", Key: "GWA", Active: []string{"22L"}, Owner: []string{"EKCH_A_TWR"}},
		{Name: "GW", Key: "GWD", Active: []string{"22L"}, Owner: []string{"EKCH_D_TWR"}},
	}

	controllers := []*Position{
		{Name: "EKCH_A_TWR", Frequency: "118.100"},
		{Name: "EKCH_D_TWR", Frequency: "119.300"},
	}
	result := GetControllerSectors(controllers, []string{"22L"})

	if len(result["118.100"]) != 1 {
		t.Fatalf("expected A_TWR to own 1 GW variant, got %d", len(result["118.100"]))
	}
	if len(result["119.300"]) != 1 {
		t.Fatalf("expected D_TWR to own 1 GW variant, got %d", len(result["119.300"]))
	}
	if result["118.100"][0].Name != "GW" || result["118.100"][0].KeyOrName() != "GWA" {
		t.Fatalf("expected A_TWR to get GWA/GW, got %#v", result["118.100"][0])
	}
	if result["119.300"][0].Name != "GW" || result["119.300"][0].KeyOrName() != "GWD" {
		t.Fatalf("expected D_TWR to get GWD/GW, got %#v", result["119.300"][0])
	}
}

func TestGetControllerSectors_FallsBackToAirborneOwnerPriority(t *testing.T) {
	originalSectors := sectors
	originalAirborneOwners := airborneOwners
	t.Cleanup(func() {
		sectors = originalSectors
		airborneOwners = originalAirborneOwners
	})

	sectors = []Sector{
		{Name: "DEL", Owner: []string{"EKCH_DEL"}},
	}
	airborneOwners = []string{"EKCH_W_APP", "EKCH_O_APP"}

	controllers := []*Position{
		{Name: "EKCH_W_APP", Frequency: "119.805"},
		{Name: "EKCH_O_APP", Frequency: "118.455"},
	}

	result := GetControllerSectors(controllers, []string{"22L"})

	if len(result["119.805"]) != 1 {
		t.Fatalf("expected highest-priority airborne owner to inherit sector, got %d sectors", len(result["119.805"]))
	}
	if len(result["118.455"]) != 0 {
		t.Fatalf("expected lower-priority airborne owner to inherit no sectors, got %d", len(result["118.455"]))
	}
	if result["119.805"][0].KeyOrName() != "DEL" {
		t.Fatalf("expected DEL to be inherited by highest-priority airborne owner, got %q", result["119.805"][0].KeyOrName())
	}
}

func TestGetControllerSectors_PrefersConfiguredOwnerOverAirborneFallback(t *testing.T) {
	originalSectors := sectors
	originalAirborneOwners := airborneOwners
	t.Cleanup(func() {
		sectors = originalSectors
		airborneOwners = originalAirborneOwners
	})

	sectors = []Sector{
		{Name: "DEL", Owner: []string{"EKCH_DEL"}},
	}
	airborneOwners = []string{"EKCH_W_APP"}

	controllers := []*Position{
		{Name: "EKCH_DEL", Frequency: "119.905"},
		{Name: "EKCH_W_APP", Frequency: "119.805"},
	}

	result := GetControllerSectors(controllers, []string{"22L"})

	if len(result["119.905"]) != 1 {
		t.Fatalf("expected configured delivery owner to keep sector, got %d sectors", len(result["119.905"]))
	}
	if len(result["119.805"]) != 0 {
		t.Fatalf("expected airborne fallback to stay idle when configured owner is online, got %d sectors", len(result["119.805"]))
	}
}

func TestKeyOrName_ReturnsKeyWhenPresent(t *testing.T) {
	sector := Sector{Name: "GW", Key: "GWA"}

	if sector.KeyOrName() != "GWA" {
		t.Fatalf("expected key to be preferred, got %q", sector.KeyOrName())
	}
}

func TestKeyOrName_FallsBackToNameWhenKeyMissing(t *testing.T) {
	sector := Sector{Name: "GW"}

	if sector.KeyOrName() != "GW" {
		t.Fatalf("expected name fallback, got %q", sector.KeyOrName())
	}
}

func TestGetSectorDisplayName_MapsInternalKeyToPublicName(t *testing.T) {
	original := sectors
	t.Cleanup(func() { sectors = original })

	sectors = []Sector{
		{Name: "GW", Key: "GWA"},
		{Name: "GW", Key: "GWD"},
	}

	if display := GetSectorDisplayName("GWA"); display != "GW" {
		t.Fatalf("expected GWA to display as GW, got %q", display)
	}
	if display := GetSectorDisplayName("GWD"); display != "GW" {
		t.Fatalf("expected GWD to display as GW, got %q", display)
	}
	if display := GetSectorDisplayName("GW"); display != "GW" {
		t.Fatalf("expected GW to remain GW, got %q", display)
	}
}

func TestGetSectorDisplayName_UnknownSectorPassesThrough(t *testing.T) {
	if display := GetSectorDisplayName("UNKNOWN"); display != "UNKNOWN" {
		t.Fatalf("expected unknown sector to pass through, got %q", display)
	}
}
