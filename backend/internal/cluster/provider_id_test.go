package cluster

import "testing"

func TestProviderEventCommandID(t *testing.T) {
	first, err := ProviderEventCommandID("aman-observation", "VATSIM", "evt-42")
	if err != nil {
		t.Fatal(err)
	}
	retry, err := ProviderEventCommandID("aman-observation", "vatsim", "evt-42")
	if err != nil || first != retry {
		t.Fatalf("provider retry changed identity: %s %s %v", first, retry, err)
	}
	if old, err := AmanObservationCommandID("vatsim", "evt-42"); err != nil || old != first {
		t.Fatalf("AMAN identity changed: %s %v", old, err)
	}
	other, _ := ProviderEventCommandID("pdc-response", "vatsim", "evt-42")
	if other == first {
		t.Fatal("unrelated provider families share an ID")
	}
	if _, err := ProviderEventCommandID("pdc-response", "hoppie", ""); err == nil {
		t.Fatal("missing event ID accepted")
	}
}
