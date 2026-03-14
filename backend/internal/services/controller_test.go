package services

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- ControllerOnline ----

func TestControllerOnline_CreatesIfNotExists(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "EKCH_N_GND"
	const position = "121.900"

	var createdCallsign string
	ctrlRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Controller, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, c *models.Controller) error {
			createdCallsign = c.Callsign
			assert.Equal(t, callsign, c.Callsign)
			assert.Equal(t, position, c.Position)
			assert.Equal(t, session, c.Session)
			return nil
		},
	}

	mockServer := &testutil.MockServer{}
	svc := NewControllerService(ctrlRepo)
	svc.SetServer(mockServer)

	result, err := svc.ControllerOnline(ctx, session, callsign, position, "")
	require.NoError(t, err)
	assert.Equal(t, callsign, createdCallsign)
	// No sector changes since UpdateSectors returns nil
	assert.Empty(t, result.SectorChanges)
}

func TestControllerOnline_UpdatesExisting(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "EKCH_N_GND"
	const oldPosition = "121.900"
	const newPosition = "121.750"

	existingController := &models.Controller{
		Callsign: callsign,
		Session:  session,
		Position: oldPosition,
	}

	var positionUpdated bool
	ctrlRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Controller, error) {
			return existingController, nil
		},
		SetPositionFn: func(_ context.Context, _ int32, cs string, pos string) (int64, error) {
			assert.Equal(t, callsign, cs)
			assert.Equal(t, newPosition, pos)
			positionUpdated = true
			return 1, nil
		},
	}

	mockServer := &testutil.MockServer{}
	svc := NewControllerService(ctrlRepo)
	svc.SetServer(mockServer)

	_, err := svc.ControllerOnline(ctx, session, callsign, newPosition, "")
	require.NoError(t, err)
	assert.True(t, positionUpdated)
}

func TestControllerOnline_SamePosition_IsNoop(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "EKCH_TWR"
	const position = "119.350"

	ctrlRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Controller, error) {
			return &models.Controller{Callsign: callsign, Session: session, Position: position}, nil
		},
	}

	svc := NewControllerService(ctrlRepo)
	result, err := svc.ControllerOnline(ctx, session, callsign, position, "")
	require.NoError(t, err)
	// Same position => heartbeat, no changes
	assert.Empty(t, result.SectorChanges)
	assert.False(t, result.SingleOnPosition)
}

// ---- ControllerOffline ----

func TestControllerOffline_ControllerNotFound(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "GHOST"

	ctrlRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Controller, error) {
			return nil, pgx.ErrNoRows
		},
	}

	hub := &testutil.MockFrontendHub{}
	mockServer := &testutil.MockServer{FrontendHubVal: hub}
	svc := NewControllerService(ctrlRepo)
	svc.SetServer(mockServer)

	result, err := svc.ControllerOffline(ctx, session, callsign)
	require.NoError(t, err)
	assert.False(t, result.ShouldScheduleTimer)
}

func TestControllerOffline_UnknownPosition_DeletesImmediately(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "EKCH_N_GND"

	// Use a position that config.GetPositionBasedOnFrequency won't know
	const unknownPosition = "999.999"

	existingController := &models.Controller{
		Callsign: callsign,
		Session:  session,
		Position: unknownPosition,
	}

	var deleted bool
	ctrlRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Controller, error) {
			return existingController, nil
		},
		DeleteFn: func(_ context.Context, _ int32, _ string) error {
			deleted = true
			return nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	mockServer := &testutil.MockServer{FrontendHubVal: hub}
	svc := NewControllerService(ctrlRepo)
	svc.SetServer(mockServer)

	result, err := svc.ControllerOffline(ctx, session, callsign)
	require.NoError(t, err)
	assert.True(t, deleted)
	assert.False(t, result.ShouldScheduleTimer)
}

func TestControllerOnline_New_SendsOnlineEvent(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "EKCH_N_GND"
	const position = "121.900"

	ctrlRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Controller, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, _ *models.Controller) error { return nil },
	}

	hub := &testutil.MockFrontendHub{}
	mockServer := &testutil.MockServer{FrontendHubVal: hub}
	svc := NewControllerService(ctrlRepo)
	svc.SetServer(mockServer)

	result, err := svc.ControllerOnline(ctx, session, callsign, position, "")
	require.NoError(t, err)
	assert.True(t, result.NotifyOnline, "new controller should trigger online notification")
}

func TestControllerOnline_SamePosition_DoesNotSendOnlineEvent(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "EKCH_TWR"
	const position = "119.350"

	ctrlRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Controller, error) {
			return &models.Controller{Callsign: callsign, Session: session, Position: position}, nil
		},
	}

	svc := NewControllerService(ctrlRepo)
	result, err := svc.ControllerOnline(ctx, session, callsign, position, "")
	require.NoError(t, err)
	assert.False(t, result.NotifyOnline, "heartbeat should not trigger online notification")
}

func TestControllerOnline_PositionChanged_SendsOnlineEvent(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "EKCH_N_GND"
	const oldPosition = "121.900"
	const newPosition = "121.750"

	ctrlRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Controller, error) {
			return &models.Controller{Callsign: callsign, Session: session, Position: oldPosition}, nil
		},
		SetPositionFn: func(_ context.Context, _ int32, _ string, _ string) (int64, error) {
			return 1, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	mockServer := &testutil.MockServer{FrontendHubVal: hub}
	svc := NewControllerService(ctrlRepo)
	svc.SetServer(mockServer)

	result, err := svc.ControllerOnline(ctx, session, callsign, newPosition, "")
	require.NoError(t, err)
	assert.True(t, result.NotifyOnline, "position change should trigger online notification")
}

func TestControllerOffline_NotFound_SendsOfflineEvent(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "GHOST"

	ctrlRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Controller, error) {
			return nil, pgx.ErrNoRows
		},
	}

	hub := &testutil.MockFrontendHub{}
	mockServer := &testutil.MockServer{FrontendHubVal: hub}
	svc := NewControllerService(ctrlRepo)
	svc.SetServer(mockServer)

	result, err := svc.ControllerOffline(ctx, session, callsign)
	require.NoError(t, err)
	assert.False(t, result.ShouldScheduleTimer)
	require.Len(t, hub.ControllerOfflines, 1, "frontend should receive controller_offline event even for unknown controller")
	assert.Equal(t, callsign, hub.ControllerOfflines[0].Callsign)
}

// ---- UpsertController ----

func TestUpsertController_CreatesIfNotExists(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "EKCH_DEL"
	const position = "121.600"

	var created bool
	ctrlRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Controller, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, c *models.Controller) error {
			created = true
			assert.Equal(t, callsign, c.Callsign)
			assert.Equal(t, position, c.Position)
			return nil
		},
	}

	svc := NewControllerService(ctrlRepo)
	err := svc.UpsertController(ctx, session, callsign, position)
	require.NoError(t, err)
	assert.True(t, created)
}

func TestUpsertController_UpdatesExisting(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "EKCH_DEL"
	const newPosition = "121.750"

	existingController := &models.Controller{Callsign: callsign, Session: session, Position: "121.600"}

	var positionSet bool
	ctrlRepo := &testutil.MockControllerRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Controller, error) {
			return existingController, nil
		},
		SetPositionFn: func(_ context.Context, _ int32, cs string, pos string) (int64, error) {
			assert.Equal(t, callsign, cs)
			assert.Equal(t, newPosition, pos)
			positionSet = true
			return 1, nil
		},
	}

	svc := NewControllerService(ctrlRepo)
	err := svc.UpsertController(ctx, session, callsign, newPosition)
	require.NoError(t, err)
	assert.True(t, positionSet)
}
