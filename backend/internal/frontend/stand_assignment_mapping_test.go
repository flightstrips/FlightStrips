package frontend

import (
	"FlightStrips/internal/models"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestPublishedStandBlockingMatchesFullSnapshot(t *testing.T) {
	reason := "observed departure conflicts with confirmed arrival: reserved"
	entries := []frontendEvents.StandAssignmentEntry{
		{Callsign: "CONFIRMED", Stand: "TEST-A", Direction: "ARRIVAL", Stage: "CONFIRMED"},
		{Callsign: "EXPECTED", Stand: "TEST-A", Direction: "DEPARTURE", Stage: "DEPARTURE_BLOCK"},
		{Callsign: "PHYSICAL", Stand: "TEST-A", Direction: "DEPARTURE", Stage: "OCCUPIED", ConflictReason: &reason},
		{Callsign: "BLOCKING", Stand: "TEST-B", Blocks: []string{"TEST-A"}, Direction: "ARRIVAL", Stage: "ASSIGNED"},
		{Callsign: "UNASSIGNED", Stand: "", Direction: "ARRIVAL", Stage: "ESTIMATED"},
	}
	want := append([]frontendEvents.StandAssignmentEntry(nil), entries...)
	enrichStandAssignmentBlocking(want, "XXXX")
	for i, entry := range entries {
		assert.Equal(t, want[i], enrichPublishedStandAssignment(entry, entries, "XXXX"))
		assert.Nil(t, entries[i].BlockedBy, "single publication must not mutate shared input")
	}
}

func BenchmarkSingleStandPublication(b *testing.B) {
	entries := make([]frontendEvents.StandAssignmentEntry, 200)
	for i := range entries {
		entries[i] = frontendEvents.StandAssignmentEntry{Callsign: fmt.Sprintf("TEST%d", i), Stand: fmt.Sprintf("STAND%d", i), Direction: "DEPARTURE", Stage: "RESERVED"}
	}
	b.Run("all_pairs", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			enrichStandAssignmentBlocking(entries, "XXXX")
		}
	})
	b.Run("single_strip", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = enrichPublishedStandAssignment(entries[0], entries, "XXXX")
		}
	})
}

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
