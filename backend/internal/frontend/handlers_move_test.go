package frontend

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events/frontend"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildFrontendTestHub creates a minimal Hub suitable for handler tests.
func buildFrontendTestHub(server *testutil.MockServer, ss shared.StripService) *Hub {
	return &Hub{
		server:       server,
		stripService: ss,
	}
}

// buildFrontendTestClient creates a Client attached to the hub.
func buildFrontendTestClient(hub *Hub, session int32, airport string) *Client {
	return &Client{
		hub:     hub,
		session: session,
		airport: airport,
	}
}

// marshalMessage packs an event struct into a shared.Message for handler calls.
func marshalMessage(t *testing.T, v interface{}) Message {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return shared.Message[frontend.EventType]{Message: data}
}

// spyStripService wraps NoOpStripService and spies on AutoTransferAirborneStrip calls.
type spyStripService struct {
	testutil.NoOpStripService
	autoTransferCalled bool
	moveToBayCalled    bool
}

func (s *spyStripService) AutoTransferAirborneStrip(_ context.Context, _ int32, _ string) error {
	s.autoTransferCalled = true
	return nil
}

func (s *spyStripService) MoveToBay(_ context.Context, _ int32, _ string, _ string, _ bool) error {
	s.moveToBayCalled = true
	return nil
}

func (s *spyStripService) UpdateGroundStateForMove(_ context.Context, _ int32, _ string, _ string, _ string, _ string) error {
	return nil
}

func (s *spyStripService) UpdateClearedFlagForMove(_ context.Context, _ int32, _ string, _ bool, _ string, _ string) error {
	return nil
}

// mockServerWithStripRepo returns a MockServer wired with the given strip repo.
func mockServerWithStripRepo(repo *testutil.MockStripRepository) *testutil.MockServer {
	ms := &testutil.MockServer{
		FrontendHubVal: &testutil.MockFrontendHub{},
	}
	ms.StripRepoVal = repo
	return ms
}

// TestHandleMove_AirborneStrip_NoAutoTransfer verifies that moving a strip to
// AIRBORNE bay does NOT trigger AutoTransferAirborneStrip (task 044).
func TestHandleMove_AirborneStrip_NoAutoTransfer(t *testing.T) {
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, callsign string) (*models.Strip, error) {
			return &models.Strip{Callsign: callsign, Bay: shared.BAY_DEPART}, nil
		},
	}

	spy := &spyStripService{}
	hub := buildFrontendTestHub(mockServerWithStripRepo(stripRepo), spy)
	client := buildFrontendTestClient(hub, 1, "EKCH")

	msg := marshalMessage(t, frontend.MoveEvent{
		Callsign: "EZY123",
		Bay:      shared.BAY_AIRBORNE,
	})

	err := handleMove(context.Background(), client, msg)
	require.NoError(t, err)
	assert.True(t, spy.moveToBayCalled, "MoveToBay should be called to move the strip")
	assert.False(t, spy.autoTransferCalled, "AutoTransferAirborneStrip must NOT be called from handleMove after task 044")
}

// TestHandleMove_InvalidBay_Rejected verifies that invalid bay strings are rejected.
func TestHandleMove_InvalidBay_Rejected(t *testing.T) {
	spy := &spyStripService{}
	hub := buildFrontendTestHub(&testutil.MockServer{FrontendHubVal: &testutil.MockFrontendHub{}}, spy)
	client := buildFrontendTestClient(hub, 1, "EKCH")

	msg := marshalMessage(t, frontend.MoveEvent{
		Callsign: "EZY123",
		Bay:      "INVALID_BAY",
	})

	err := handleMove(context.Background(), client, msg)
	assert.Error(t, err)
	assert.False(t, spy.moveToBayCalled, "MoveToBay must not be called for invalid bay")
}

// TestHandleMove_OwnedByOther_Rejectedverifies that handleMove rejects a move
// when the strip is owned by another controller and no coordination exists (task 048).
func TestHandleMove_OwnedByOther_Rejected(t *testing.T) {
	ownerPos := "EKCH_D_GND"
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, callsign string) (*models.Strip, error) {
			return &models.Strip{Callsign: callsign, Bay: shared.BAY_NOT_CLEARED, Owner: &ownerPos}, nil
		},
	}
	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Coordination, error) {
			return nil, nil // no coordination
		},
	}
	ms := mockServerWithStripRepo(stripRepo)
	ms.CoordRepoVal = coordRepo

	spy := &spyStripService{}
	hub := buildFrontendTestHub(ms, spy)
	client := buildFrontendTestClient(hub, 1, "EKCH")
	client.position = "EKCH_A_GND" // different from owner

	msg := marshalMessage(t, frontend.MoveEvent{
		Callsign: "SAS100",
		Bay:      shared.BAY_PUSH,
	})

	err := handleMove(context.Background(), client, msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not authorized")
	assert.False(t, spy.moveToBayCalled)
}

// TestHandleMove_OwnedByOther_AllowedToArrivalBay verifies that a strip owned by
// another controller can still be moved into FINAL/RWY_ARR/TWY_ARR (task 048).
func TestHandleMove_OwnedByOther_AllowedToArrivalBay(t *testing.T) {
	ownerPos := "EKCH_D_GND"
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, callsign string) (*models.Strip, error) {
			return &models.Strip{Callsign: callsign, Bay: shared.BAY_DEPART, Owner: &ownerPos}, nil
		},
	}

	ms := mockServerWithStripRepo(stripRepo)

	spy := &spyStripService{}
	hub := buildFrontendTestHub(ms, spy)
	client := buildFrontendTestClient(hub, 1, "EKCH")
	client.position = "EKCH_A_TWR"

	msg := marshalMessage(t, frontend.MoveEvent{
		Callsign: "BEL123",
		Bay:      shared.BAY_FINAL,
	})

	err := handleMove(context.Background(), client, msg)
	require.NoError(t, err)
	assert.True(t, spy.moveToBayCalled, "Move to arrival bay should be allowed regardless of ownership")
}

// TestHandleMove_OwnedByOther_AllowedWithCoordination verifies that a strip owned by
// another controller can be moved if there is an active coordination to this position (task 048).
func TestHandleMove_OwnedByOther_AllowedWithCoordination(t *testing.T) {
	ownerPos := "EKCH_D_GND"
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, callsign string) (*models.Strip, error) {
			return &models.Strip{Callsign: callsign, Bay: shared.BAY_TAXI, Owner: &ownerPos}, nil
		},
	}
	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Coordination, error) {
			return &models.Coordination{ToPosition: "EKCH_A_GND"}, nil
		},
	}
	ms := mockServerWithStripRepo(stripRepo)
	ms.CoordRepoVal = coordRepo

	spy := &spyStripService{}
	hub := buildFrontendTestHub(ms, spy)
	client := buildFrontendTestClient(hub, 1, "EKCH")
	client.position = "EKCH_A_GND"

	msg := marshalMessage(t, frontend.MoveEvent{
		Callsign: "SAS200",
		Bay:      shared.BAY_TAXI_TWR,
	})

	err := handleMove(context.Background(), client, msg)
	require.NoError(t, err)
	assert.True(t, spy.moveToBayCalled, "Move should succeed when coordination targets this position")
}

