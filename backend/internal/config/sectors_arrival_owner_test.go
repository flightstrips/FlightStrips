package config

import "testing"

func TestIsArrivalTowerOwner_MatchesApplicableTwTeOwners(t *testing.T) {
	t.Cleanup(SetSectorsForTest([]Sector{
		{Name: "TE", Active: []string{"22L", "22R"}, Owner: []string{"EKCH_A_TWR", "EKCH_D_TWR"}},
		{Name: "TW", Active: []string{"22L", "22R"}, Owner: []string{"EKCH_D_TWR", "EKCH_A_TWR"}},
		{Name: "TE", Active: []string{"04L", "04R"}, Owner: []string{"EKCH_D_TWR", "EKCH_A_TWR"}},
		{Name: "TW", Active: []string{"04L", "04R"}, Owner: []string{"EKCH_A_TWR", "EKCH_D_TWR"}},
		{Name: "GW", Active: []string{"22L", "22R"}, Owner: []string{"EKCH_C_TWR"}},
	}))

	if !IsArrivalTowerOwner("EKCH_A_TWR", []string{"22L", "22R"}) {
		t.Fatal("expected EKCH_A_TWR to match applicable TE/TW owners")
	}
	if !IsArrivalTowerOwner("EKCH_D_TWR", []string{"22L", "22R"}) {
		t.Fatal("expected EKCH_D_TWR to match applicable TE/TW owners")
	}
	if IsArrivalTowerOwner("EKCH_C_TWR", []string{"22L", "22R"}) {
		t.Fatal("did not expect EKCH_C_TWR to match TE/TW owners")
	}
}
