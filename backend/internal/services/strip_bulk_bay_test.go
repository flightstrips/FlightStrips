package services

import (
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events/frontend"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSendBulkSequenceUpdate_SendsBulkBayEvent verifies that sendBulkSequenceUpdate
// emits a single BulkBayEvent instead of individual BayEvents.
func TestSendBulkSequenceUpdate_SendsBulkBayEvent(t *testing.T) {
	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(&testutil.MockStripRepository{})
	svc.SetFrontendHub(hub)

	callsigns := []string{"SAS100", "SAS200", "SAS300"}
	sequences := []int32{100, 200, 300}
	svc.sendBulkSequenceUpdate(42, callsigns, sequences, "DEP_GATE")

	// Expect exactly one BulkBayEvent, zero individual BayEvents.
	assert.Empty(t, hub.BayEvents, "should not send individual bay events")
	require.Len(t, hub.BulkBayEvents, 1, "should send exactly one bulk bay event")

	evt := hub.BulkBayEvents[0]
	assert.Equal(t, int32(42), evt.Session)
	assert.Equal(t, "DEP_GATE", evt.Bay)
	require.Len(t, evt.Strips, 3)
	assert.Equal(t, frontend.BulkBayEntry{Callsign: "SAS100", Sequence: 100}, evt.Strips[0])
	assert.Equal(t, frontend.BulkBayEntry{Callsign: "SAS200", Sequence: 200}, evt.Strips[1])
	assert.Equal(t, frontend.BulkBayEntry{Callsign: "SAS300", Sequence: 300}, evt.Strips[2])
}

// TestSendBulkSequenceUpdate_LengthMismatch verifies that mismatched slices are silently dropped.
func TestSendBulkSequenceUpdate_LengthMismatch(t *testing.T) {
	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(&testutil.MockStripRepository{})
	svc.SetFrontendHub(hub)

	svc.sendBulkSequenceUpdate(1, []string{"A", "B"}, []int32{10}, "BAY")

	assert.Empty(t, hub.BayEvents)
	assert.Empty(t, hub.BulkBayEvents)
}
