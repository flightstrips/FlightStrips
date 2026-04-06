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
