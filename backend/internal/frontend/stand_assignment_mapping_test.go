package frontend

import (
	"FlightStrips/internal/models"
	"testing"
)

func TestMapStandAssignmentEntryPreservesSnapshotAndIncrementalMetadata(t *testing.T) {
	rule, conflict := "fallback", "controller override"
	tier := int32(3)
	entry := mapStandAssignmentEntry(&models.StandAssignment{ID: 9, Callsign: "SAS901", Stand: "A12",
		Direction: "ARRIVAL", Stage: "CONFIRMED", Source: "MANUAL_OVERRIDE", Manual: true,
		RuleID: &rule, Tier: &tier, ConflictReason: &conflict, Acknowledged: false, Version: 6})
	if entry.ID != 9 || entry.Version != 6 || !entry.Manual || !entry.PendingAcknowledgement {
		t.Fatalf("metadata lost: %#v", entry)
	}
}
