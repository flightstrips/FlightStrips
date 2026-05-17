package frontend

import (
	"encoding/json"
	"testing"
)

func TestCdmDataEventMarshal_IncludesEmptyPhase(t *testing.T) {
	t.Parallel()

	payload, err := (CdmDataEvent{
		Callsign: "SAS123",
		Eobt:     "1015",
		Tobt:     "1020",
		Tsat:     "1030",
		Ctot:     "1045",
		Phase:    "",
	}).Marshal()
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}

	phase, ok := decoded["phase"]
	if !ok {
		t.Fatal("expected marshaled CDM event to include phase field")
	}
	if phase != "" {
		t.Fatalf("expected empty phase, got %#v", phase)
	}
}
