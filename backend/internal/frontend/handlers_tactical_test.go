package frontend

import (
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events/frontend"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTacticalTestHub returns a Hub with no tactical repo (nil is fine for bay
// validation tests because the validation fires before the repo is accessed).
func buildTacticalTestHub() *Hub {
	return &Hub{
		server:       &testutil.MockServer{FrontendHubVal: &testutil.MockFrontendHub{}},
		stripService: &testutil.NoOpStripService{},
	}
}

// TestHandleCreateTacticalStrip_InvalidBay_Rejected verifies that sending an
// unknown bay string is rejected before any database access.
func TestHandleCreateTacticalStrip_InvalidBay_Rejected(t *testing.T) {
	hub := buildTacticalTestHub()
	client := buildFrontendTestClient(hub, 1, "EKCH")

	msg := marshalMessage(t, frontend.CreateTacticalStripAction{
		StripType: "MEMAID",
		Bay:       "DE_ICE", // old incorrect value — must now be rejected
		Label:     "test label",
	})

	err := handleCreateTacticalStrip(context.Background(), client, msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid bay")
}

// TestHandleCreateTacticalStrip_ValidBay_PassesBayValidation verifies that a
// known bay value (TAXI_TWR — the backend constant for the de-ice area) passes
// bay validation. The call will fail later on the nil tactical repo, but that
// is outside the scope of this test.
func TestHandleCreateTacticalStrip_ValidBay_PassesBayValidation(t *testing.T) {
	hub := buildTacticalTestHub()
	client := buildFrontendTestClient(hub, 1, "EKCH")

	msg := marshalMessage(t, frontend.CreateTacticalStripAction{
		StripType: "MEMAID",
		Bay:       shared.BAY_TAXI_TWR, // "TAXI_TWR" — correct value after fix
		Label:     "test label",
	})

	err := handleCreateTacticalStrip(context.Background(), client, msg)
	// Bay validation passes; fails on nil tactical repo — that is expected.
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not available", "should fail on nil repo, not bay validation")
}
