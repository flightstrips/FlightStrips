package config

import "testing"

func TestIsArrivalTowerOwner_MatchesApplicableTwTeOwners(t *testing.T) {
	t.Cleanup(SetSectorsForTest([]Sector{
		{Name: "TE", Active: []string{"22L", "22R"}, Owner: []string{"EKCH_A_TWR", "EKCH_D_TWR"}},
		{Name: "TW", Active: []string{"22L", "22R"}, Owner: []string{"EKCH_D_TWR", "EKCH_A_TWR"}},
		{Name: "TE", Active: []string{"04L", "04R"}, Owner: []string{"EKCH_D_TWR", "EKCH_A_TWR"}},
		{Name: "TW", Active: []string{"04L", "04R"}, Owner: []string{"EKCH_A_TWR", "EKCH_D_TWR"}},
		{Name: "GW", Active: []string{"22L", "22R"}, Owner: []string{"EKCH_GW_TWR"}},
	}))

	if !IsArrivalTowerOwner("EKCH_A_TWR", []string{"22L", "22R"}) {
		t.Fatal("expected EKCH_A_TWR to match applicable TE/TW owners")
	}
	if !IsArrivalTowerOwner("EKCH_D_TWR", []string{"22L", "22R"}) {
		t.Fatal("expected EKCH_D_TWR to match applicable TE/TW owners")
	}
	if IsArrivalTowerOwner("EKCH_GW_TWR", []string{"22L", "22R"}) {
		t.Fatal("did not expect EKCH_GW_TWR to match TE/TW owners")
	}
}

func TestGetPositionLogicalIdentifier_UsesPrimaryConfiguredRole(t *testing.T) {
	t.Cleanup(SetSectorsForTest([]Sector{
		{Name: "SQ", NamePriority: 0, Owner: []string{"EKCH_B_GND", "EKCH_C_GND"}},
		{
			Name:         "AD",
			NamePriority: 20,
			Constraints:  &constraints{Departure: true},
			Owner:        []string{"EKCH_C_GND", "EKCH_B_GND"},
		},
		{Name: "GE", NamePriority: 40, Owner: []string{"EKCH_GE_TWR", "EKCH_GW_TWR"}},
		{Name: "GW", Key: "GWD", NamePriority: 40, Owner: []string{"EKCH_GW_TWR", "EKCH_GE_TWR"}},
		{Name: "TE", NamePriority: 50, Active: []string{"22R"}, Owner: []string{"EKCH_A_TWR", "EKCH_D_TWR"}},
		{Name: "TW", NamePriority: 50, Active: []string{"22R"}, Owner: []string{"EKCH_D_TWR", "EKCH_A_TWR"}},
	}))

	tests := []struct {
		name       string
		active     []string
		position   string
		isArrival  bool
		identifier string
	}{
		{name: "sequence", position: "EKCH_B_GND", identifier: "SQ"},
		{name: "apron departure", position: "EKCH_C_GND", identifier: "AD"},
		{name: "ground east", position: "EKCH_GE_TWR", identifier: "GE"},
		{name: "ground west", position: "EKCH_GW_TWR", identifier: "GWD"},
		{name: "tower east", active: []string{"22R"}, position: "EKCH_A_TWR", identifier: "TE"},
		{name: "tower west", active: []string{"22R"}, position: "EKCH_D_TWR", identifier: "TW"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identifier, ok := GetPositionLogicalIdentifier(test.active, test.position, test.isArrival)

			if !ok {
				t.Fatal("expected configured primary role")
			}
			if identifier != test.identifier {
				t.Fatalf("expected %s, got %s", test.identifier, identifier)
			}
		})
	}
}
