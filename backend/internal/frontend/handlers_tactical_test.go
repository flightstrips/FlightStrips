package frontend

import (
	"FlightStrips/internal/models"
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

func buildTacticalRepoHub(repo *testutil.MockTacticalStripRepository) *Hub {
	return &Hub{
		server: &testutil.MockServer{
			FrontendHubVal:       &testutil.MockFrontendHub{},
			TacticalStripRepoVal: repo,
		},
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

func TestHandleConfirmTacticalStrip_InvalidTypeRejectedBeforeMutation(t *testing.T) {
	confirmCalled := false
	hub := buildTacticalRepoHub(&testutil.MockTacticalStripRepository{
		GetByIDFn: func(_ context.Context, id int64, sessionID int32) (*models.TacticalStrip, error) {
			assert.Equal(t, int64(42), id)
			assert.Equal(t, int32(1), sessionID)
			return &models.TacticalStrip{
				ID:        id,
				SessionID: sessionID,
				Type:      models.TacticalStripTypeStart,
			}, nil
		},
		ConfirmFn: func(_ context.Context, _ int64, _ int32, _ string) (*models.TacticalStrip, error) {
			confirmCalled = true
			return nil, nil
		},
	})
	client := buildFrontendTestClient(hub, 1, "EKCH")
	client.position = "EKCH_TWR"

	msg := marshalMessage(t, frontend.ConfirmTacticalStripAction{ID: 42})

	err := handleConfirmTacticalStrip(context.Background(), client, msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "confirm is only valid")
	assert.False(t, confirmCalled, "Confirm must not be called when the tactical strip type is invalid")
}

func TestHandleConfirmTacticalStrip_ProducerRejectedBeforeMutation(t *testing.T) {
	confirmCalled := false
	hub := buildTacticalRepoHub(&testutil.MockTacticalStripRepository{
		GetByIDFn: func(_ context.Context, id int64, sessionID int32) (*models.TacticalStrip, error) {
			return &models.TacticalStrip{
				ID:         id,
				SessionID:  sessionID,
				Type:       models.TacticalStripTypeCrossing,
				ProducedBy: "EKCH_TWR",
			}, nil
		},
		ConfirmFn: func(_ context.Context, _ int64, _ int32, _ string) (*models.TacticalStrip, error) {
			confirmCalled = true
			return nil, nil
		},
	})
	client := buildFrontendTestClient(hub, 1, "EKCH")
	client.position = "EKCH_TWR"

	msg := marshalMessage(t, frontend.ConfirmTacticalStripAction{ID: 43})

	err := handleConfirmTacticalStrip(context.Background(), client, msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "producer cannot confirm")
	assert.False(t, confirmCalled, "Confirm must not be called when the producer is not allowed to confirm")
}

func TestHandleStartTacticalTimer_InvalidTypeRejectedBeforeMutation(t *testing.T) {
	startTimerCalled := false
	hub := buildTacticalRepoHub(&testutil.MockTacticalStripRepository{
		GetByIDFn: func(_ context.Context, id int64, sessionID int32) (*models.TacticalStrip, error) {
			return &models.TacticalStrip{
				ID:        id,
				SessionID: sessionID,
				Type:      models.TacticalStripTypeMemaid,
			}, nil
		},
		StartTimerFn: func(_ context.Context, _ int64, _ int32) (*models.TacticalStrip, error) {
			startTimerCalled = true
			return nil, nil
		},
	})
	client := buildFrontendTestClient(hub, 1, "EKCH")

	msg := marshalMessage(t, frontend.StartTacticalTimerAction{ID: 99})

	err := handleStartTacticalTimer(context.Background(), client, msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start timer is only valid")
	assert.False(t, startTimerCalled, "StartTimer must not be called when the tactical strip type is invalid")
}
