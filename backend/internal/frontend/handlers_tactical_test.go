package frontend

import (
	"FlightStrips/internal/config"
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
		stripService: &noOpStripService{},
	}
}

func buildTacticalRepoHub(repo *testutil.MockTacticalStripRepository) *Hub {
	return &Hub{
		server: &testutil.MockServer{
			FrontendHubVal:       &testutil.MockFrontendHub{},
			TacticalStripRepoVal: repo,
		},
		stripService: &noOpStripService{},
		send:         make(chan internalMessage, 10),
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

func TestHandleConfirmTacticalStrip_OwnerRejectedBeforeMutation(t *testing.T) {
	confirmCalled := false
	hub := buildTacticalRepoHub(&testutil.MockTacticalStripRepository{
		GetByIDFn: func(_ context.Context, id int64, sessionID int32) (*models.TacticalStrip, error) {
			return &models.TacticalStrip{
				ID:        id,
				SessionID: sessionID,
				Type:      models.TacticalStripTypeMemaid,
				Owner:     "EKCH_TWR",
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
	assert.Contains(t, err.Error(), "owner cannot confirm")
	assert.False(t, confirmCalled)
}

func TestHandleCreateTacticalStrip_RunwayRules(t *testing.T) {
	t.Cleanup(config.SetRunwaysForTest([]string{"04R", "04L", "12", "22R", "22L", "30"}))

	tests := []struct {
		name    string
		bay     string
		label   string
		wantErr string
	}{
		{name: "taxi requires runway", bay: shared.BAY_TAXI, wantErr: "runway label is required"},
		{name: "twy arr requires valid runway", bay: shared.BAY_TWY_ARR, label: "99", wantErr: "invalid runway"},
		{name: "rwy arr rejects runway", bay: shared.BAY_RWY_ARR, label: "22L", wantErr: "runway label is not allowed"},
		{name: "rwy dep accepts no runway", bay: shared.BAY_DEPART},
		{name: "taxi lower requires runway", bay: shared.BAY_TAXI_LWR, wantErr: "runway label is required"},
		{name: "taxi lower accepts valid runway", bay: shared.BAY_TAXI_LWR, label: "22L"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created := false
			repo := &testutil.MockTacticalStripRepository{
				GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
					return 0, nil
				},
				CreateFn: func(_ context.Context, sessionID int32, stripType, bay, label string, _ *string, producedBy string, sequence int32) (*models.TacticalStrip, error) {
					created = true
					assert.Equal(t, "EKCH_TWR", producedBy)
					return &models.TacticalStrip{
						ID:         1,
						SessionID:  sessionID,
						Type:       stripType,
						Bay:        bay,
						Label:      label,
						ProducedBy: producedBy,
						Owner:      producedBy,
						Sequence:   sequence,
					}, nil
				},
			}
			hub := buildTacticalRepoHub(repo)
			client := buildFrontendTestClient(hub, 1, "EKCH")
			client.position = "EKCH_TWR"

			err := handleCreateTacticalStrip(context.Background(), client, marshalMessage(t, frontend.CreateTacticalStripAction{
				StripType: models.TacticalStripTypeStart,
				Bay:       tt.bay,
				Label:     tt.label,
			}))

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.False(t, created)
				return
			}
			require.NoError(t, err)
			assert.True(t, created)
		})
	}
}

func TestHandleForceAssumeTacticalStrip_TransfersOwnership(t *testing.T) {
	var assumedBy string
	hub := buildTacticalRepoHub(&testutil.MockTacticalStripRepository{
		ForceAssumeFn: func(_ context.Context, id int64, sessionID int32, owner string) (*models.TacticalStrip, error) {
			assumedBy = owner
			return &models.TacticalStrip{
				ID:        id,
				SessionID: sessionID,
				Type:      models.TacticalStripTypeCrossing,
				Owner:     owner,
				Marked:    false,
			}, nil
		},
	})
	client := buildFrontendTestClient(hub, 1, "EKCH")
	client.position = "EKCH_TWR"

	err := handleForceAssumeTacticalStrip(context.Background(), client, marshalMessage(t, frontend.ForceAssumeTacticalStripAction{ID: 99}))
	require.NoError(t, err)
	assert.Equal(t, "EKCH_TWR", assumedBy)
}

func TestHandleMarkTacticalStrip_NonOwnerRejected(t *testing.T) {
	updated := false
	hub := buildTacticalRepoHub(&testutil.MockTacticalStripRepository{
		GetByIDFn: func(_ context.Context, id int64, sessionID int32) (*models.TacticalStrip, error) {
			return &models.TacticalStrip{ID: id, SessionID: sessionID, Owner: "EKCH_GND"}, nil
		},
		UpdateMarkedFn: func(_ context.Context, _ int64, _ int32, _ bool) (*models.TacticalStrip, error) {
			updated = true
			return nil, nil
		},
	})
	client := buildFrontendTestClient(hub, 1, "EKCH")
	client.position = "EKCH_TWR"

	err := handleMarkTacticalStrip(context.Background(), client, marshalMessage(t, frontend.MarkTacticalStripAction{ID: 42, Marked: true}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the tactical strip owner")
	assert.False(t, updated)
}

func TestHandleDeleteTacticalStrip_NonOwnerRejected(t *testing.T) {
	deleted := false
	hub := buildTacticalRepoHub(&testutil.MockTacticalStripRepository{
		GetByIDFn: func(_ context.Context, id int64, sessionID int32) (*models.TacticalStrip, error) {
			return &models.TacticalStrip{ID: id, SessionID: sessionID, Owner: "EKCH_GND"}, nil
		},
		DeleteFn: func(_ context.Context, _ int64, _ int32) error {
			deleted = true
			return nil
		},
	})
	client := buildFrontendTestClient(hub, 1, "EKCH")
	client.position = "EKCH_TWR"

	err := handleDeleteTacticalStrip(context.Background(), client, marshalMessage(t, frontend.DeleteTacticalStripAction{ID: 42}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the tactical strip owner")
	assert.False(t, deleted)
}

func TestHandleMoveTacticalStrip_NonOwnerRejected(t *testing.T) {
	hub := buildTacticalRepoHub(&testutil.MockTacticalStripRepository{
		GetByIDFn: func(_ context.Context, id int64, sessionID int32) (*models.TacticalStrip, error) {
			return &models.TacticalStrip{ID: id, SessionID: sessionID, Owner: "EKCH_GND"}, nil
		},
	})
	client := buildFrontendTestClient(hub, 1, "EKCH")
	client.position = "EKCH_TWR"

	err := handleMoveTacticalStrip(context.Background(), client, marshalMessage(t, frontend.MoveTacticalStripAction{ID: 42}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only the tactical strip owner")
}

func TestMapTacticalStripToPayload_IncludesOwnershipState(t *testing.T) {
	payload := MapTacticalStripToPayload(&models.TacticalStrip{
		ID:         42,
		SessionID:  1,
		Type:       models.TacticalStripTypeCrossing,
		ProducedBy: "EKCH_GND",
		Owner:      "EKCH_TWR",
		Marked:     true,
	})

	assert.Equal(t, "EKCH_GND", payload.ProducedBy)
	assert.Equal(t, "EKCH_TWR", payload.Owner)
	assert.True(t, payload.Marked)
}
