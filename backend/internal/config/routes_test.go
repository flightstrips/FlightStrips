package config

import "testing"

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
