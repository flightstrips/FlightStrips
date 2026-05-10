package cdm

import "testing"

func TestMovementReasonMarkersFromTraceCollapsesRunwayShiftNoise(t *testing.T) {
	t.Parallel()

	markers := movementReasonMarkersFromTrace([]calculationTraceEntry{
		{
			Kind:            "runway_slot_collision",
			AgainstCallsign: "EZY325X",
			AgainstTtot:     "170000",
			AgainstRunway:   "22L",
			FromTtot:        "170000",
			ToTtot:          "170030",
		},
		{
			Kind:                   "runway_rate_window",
			AgainstCallsign:        "EZY325X",
			AgainstTtot:            "170000",
			AgainstRunway:          "22L",
			RequiredSpacingMinutes: 1.5,
			FromTtot:               "170030",
			ToTtot:                 "170100",
		},
		{
			Kind:                   "runway_rate_window",
			AgainstCallsign:        "EZY325X",
			AgainstTtot:            "170000",
			AgainstRunway:          "22L",
			RequiredSpacingMinutes: 1.5,
			FromTtot:               "170100",
			ToTtot:                 "170100",
		},
	})

	if len(markers) != 1 {
		t.Fatalf("expected 1 compacted marker, got %#v", markers)
	}
	if markers[0].Kind != "runway_spacing" {
		t.Fatalf("expected runway_spacing marker, got %#v", markers[0])
	}
	if valueOrEmpty(markers[0].AgainstCallsign) != "EZY325X" {
		t.Fatalf("expected compacted marker to keep target callsign, got %#v", markers[0])
	}
	if valueOrEmpty(markers[0].FromTtot) != "170000" || valueOrEmpty(markers[0].ToTtot) != "170100" {
		t.Fatalf("expected compacted marker to keep full shift range, got %#v", markers[0])
	}
}

func TestMovementReasonMarkersFromTraceKeepsOnlyLastShift(t *testing.T) {
	t.Parallel()

	markers := movementReasonMarkersFromTrace([]calculationTraceEntry{
		{
			Kind:            "runway_slot_collision",
			AgainstCallsign: "SAS909",
			AgainstTtot:     "170200",
			AgainstRunway:   "22R",
			FromTtot:        "170200",
			ToTtot:          "170300",
		},
		{
			Kind:            "wake_separation",
			AgainstCallsign: "SAS909",
			AgainstRunway:   "22R",
			FromTtot:        "170300",
			ToTtot:          "170300",
		},
		{
			Kind:            "runway_rate_window",
			AgainstCallsign: "NOZ938",
			AgainstTtot:     "170430",
			AgainstRunway:   "22R",
			FromTtot:        "170300",
			ToTtot:          "170500",
		},
	})

	if len(markers) != 1 {
		t.Fatalf("expected 1 final-shift marker, got %#v", markers)
	}
	if valueOrEmpty(markers[0].AgainstCallsign) != "NOZ938" {
		t.Fatalf("expected only the last shifting constraint to remain, got %#v", markers[0])
	}
	if valueOrEmpty(markers[0].FromTtot) != "170300" || valueOrEmpty(markers[0].ToTtot) != "170500" {
		t.Fatalf("expected final shift range to remain, got %#v", markers[0])
	}
}
