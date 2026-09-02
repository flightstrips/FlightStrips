package frontend

import (
	"FlightStrips/internal/models"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"github.com/stretchr/testify/assert"
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

func TestEnrichStandAssignmentBlockingReportsExactStandOverlap(t *testing.T) {
	reason := "observed departure conflicts with confirmed arrival: reserved"
	entries := []frontendEvents.StandAssignmentEntry{
		{Callsign: "CONFIRMED1", Stand: "A17", Direction: "ARRIVAL", Stage: "CONFIRMED"},
		{Callsign: "PHYSICAL1", Stand: "A17", Direction: "DEPARTURE", Stage: "OCCUPIED", ConflictReason: &reason},
	}

	enrichStandAssignmentBlocking(entries, "EKCH")

	assert.Equal(t, []string{"PHYSICAL1"}, entries[0].BlockedBy)
	assert.Equal(t, []string{"CONFIRMED1"}, entries[1].BlockedBy)
}

func TestEnrichStandAssignmentBlockingDoesNotCallExpectedDepartureBlocked(t *testing.T) {
	entries := []frontendEvents.StandAssignmentEntry{
		{Callsign: "CONFIRMED1", Stand: "A17", Direction: "ARRIVAL", Stage: "CONFIRMED"},
		{Callsign: "READY1", Stand: "A17", Direction: "DEPARTURE", Stage: "DEPARTURE_BLOCK"},
	}

	enrichStandAssignmentBlocking(entries, "EKCH")

	assert.Empty(t, entries[0].BlockedBy)
	assert.Empty(t, entries[1].BlockedBy)
}

func TestEnrichStandAssignmentBlockingIgnoresUnassignedAdvisories(t *testing.T) {
	entries := []frontendEvents.StandAssignmentEntry{
		{Callsign: "UNASSIGNED1", Stand: "", Direction: "ARRIVAL", Stage: "ASSIGNED"},
		{Callsign: "UNASSIGNED2", Stand: " ", Direction: "ARRIVAL", Stage: "ESTIMATED"},
	}

	enrichStandAssignmentBlocking(entries, "EKCH")

	assert.Empty(t, entries[0].BlockedBy)
	assert.Empty(t, entries[1].BlockedBy)
}
