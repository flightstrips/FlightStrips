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

func TestCdmDataEventMarshal_IncludesEmptyTobtSourceFields(t *testing.T) {
	t.Parallel()

	payload, err := (CdmDataEvent{
		Callsign:    "SAS123",
		Eobt:        "1015",
		Tobt:        "1020",
		ReqTobtType: "",
		TobtSetBy:   "",
		Tsat:        "1030",
		Ctot:        "1045",
		Phase:       "",
	}).Marshal()
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}

	reqTobtType, ok := decoded["req_tobt_type"]
	if !ok {
		t.Fatal("expected marshaled CDM event to include req_tobt_type field")
	}
	if reqTobtType != "" {
		t.Fatalf("expected empty req_tobt_type, got %#v", reqTobtType)
	}

	tobtSetBy, ok := decoded["tobt_set_by"]
	if !ok {
		t.Fatal("expected marshaled CDM event to include tobt_set_by field")
	}
	if tobtSetBy != "" {
		t.Fatalf("expected empty tobt_set_by, got %#v", tobtSetBy)
	}
}
