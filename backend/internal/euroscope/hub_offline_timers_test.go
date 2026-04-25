package euroscope

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stringPtr(value string) *string {
	return &value
}

func TestClassifyOfflineAction_SkipsWhenOriginalControllerIsAlreadyGone(t *testing.T) {
	const session = int32(1)
	const callsign = "EKCH_TWR"
	const position = "118.700"

	server := &testutil.MockServer{
		FrontendHubVal: &testutil.MockFrontendHub{},
		ControllerRepoVal: &testutil.MockControllerRepository{
			GetByCallsignFn: func(_ context.Context, sess int32, cs string) (*models.Controller, error) {
				assert.Equal(t, session, sess)
				assert.Equal(t, callsign, cs)
				return nil, pgx.ErrNoRows
			},
		},
	}

	hub := buildReconcileHub(server)
	decision, err := hub.classifyOfflineAction(context.Background(), &offlineTimerEntry{
		session:      session,
		callsign:     callsign,
		positionFreq: position,
		positionName: "EKCH_TWR",
	})

	require.NoError(t, err)
	assert.Equal(t, offlineActionSkip, decision)
}

func TestClassifyOfflineAction_SkipsWhenOriginalControllerMovedToAnotherPosition(t *testing.T) {
	const session = int32(1)
	const callsign = "EKCH_TWR"
	const originalPosition = "118.700"
	const newPosition = "121.900"

	server := &testutil.MockServer{
		FrontendHubVal: &testutil.MockFrontendHub{},
		ControllerRepoVal: &testutil.MockControllerRepository{
			GetByCallsignFn: func(_ context.Context, sess int32, cs string) (*models.Controller, error) {
				assert.Equal(t, session, sess)
				assert.Equal(t, callsign, cs)
				return &models.Controller{
					Callsign: callsign,
					Position: newPosition,
				}, nil
			},
		},
	}

	hub := buildReconcileHub(server)
	decision, err := hub.classifyOfflineAction(context.Background(), &offlineTimerEntry{
		session:      session,
		callsign:     callsign,
		positionFreq: originalPosition,
		positionName: "EKCH_TWR",
	})

	require.NoError(t, err)
	assert.Equal(t, offlineActionSkip, decision)
}

func TestClassifyOfflineAction_UsesSilentCleanupWhenPositionAlreadyCovered(t *testing.T) {
	const session = int32(1)
	const callsign = "EKCH_TWR"
	const position = "118.700"
	t.Cleanup(config.SetOwnerCallsignPrefixesForTest([]string{"EKCH", "EKDK"}))

	server := &testutil.MockServer{
		FrontendHubVal: &testutil.MockFrontendHub{},
		ControllerRepoVal: &testutil.MockControllerRepository{
			GetByCallsignFn: func(_ context.Context, sess int32, cs string) (*models.Controller, error) {
				assert.Equal(t, session, sess)
				assert.Equal(t, callsign, cs)
				return &models.Controller{
					Callsign: callsign,
					Position: position,
				}, nil
			},
			GetByPositionFn: func(_ context.Context, sess int32, pos string) ([]*models.Controller, error) {
				assert.Equal(t, session, sess)
				assert.Equal(t, position, pos)
				return []*models.Controller{
					{Callsign: callsign, Position: position},
					{Callsign: "EKCH_A_TWR", Position: position},
				}, nil
			},
		},
	}

	hub := buildReconcileHub(server)
	decision, err := hub.classifyOfflineAction(context.Background(), &offlineTimerEntry{
		session:      session,
		callsign:     callsign,
		positionFreq: position,
		positionName: "EKCH_TWR",
	})

	require.NoError(t, err)
	assert.Equal(t, offlineActionSilentCleanup, decision)
}

func TestClassifyOfflineAction_IgnoresObserverCoverage(t *testing.T) {
	const session = int32(1)
	const callsign = "EKCH_TWR"
	const position = "118.700"
	t.Cleanup(config.SetOwnerCallsignPrefixesForTest([]string{"EKCH", "EKDK"}))

	server := &testutil.MockServer{
		FrontendHubVal: &testutil.MockFrontendHub{},
		ControllerRepoVal: &testutil.MockControllerRepository{
			GetByCallsignFn: func(_ context.Context, sess int32, cs string) (*models.Controller, error) {
				assert.Equal(t, session, sess)
				assert.Equal(t, callsign, cs)
				return &models.Controller{
					Callsign: callsign,
					Position: position,
				}, nil
			},
			GetByPositionFn: func(_ context.Context, sess int32, pos string) ([]*models.Controller, error) {
				assert.Equal(t, session, sess)
				assert.Equal(t, position, pos)
				return []*models.Controller{
					{Callsign: callsign, Position: position},
					{Callsign: "FR_OBS", Position: position, Observer: true},
				}, nil
			},
		},
	}

	hub := buildReconcileHub(server)
	decision, err := hub.classifyOfflineAction(context.Background(), &offlineTimerEntry{
		session:      session,
		callsign:     callsign,
		positionFreq: position,
		positionName: "EKCH_TWR",
	})

	require.NoError(t, err)
	assert.Equal(t, offlineActionFinalize, decision)
}

func TestClassifyOfflineAction_IgnoresMismatchedPrefixCoverage(t *testing.T) {
	const session = int32(1)
	const callsign = "EKCH_TWR"
	const position = "118.700"

	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_TWR", Frequency: position},
	}))
	t.Cleanup(config.SetOwnerCallsignPrefixesForTest([]string{"EKCH", "EKDK"}))

	server := &testutil.MockServer{
		FrontendHubVal: &testutil.MockFrontendHub{},
		ControllerRepoVal: &testutil.MockControllerRepository{
			GetByCallsignFn: func(_ context.Context, sess int32, cs string) (*models.Controller, error) {
				assert.Equal(t, session, sess)
				assert.Equal(t, callsign, cs)
				return &models.Controller{
					Callsign: callsign,
					Position: position,
				}, nil
			},
			GetByPositionFn: func(_ context.Context, sess int32, pos string) ([]*models.Controller, error) {
				assert.Equal(t, session, sess)
				assert.Equal(t, position, pos)
				return []*models.Controller{
					{Callsign: callsign, Position: position},
					{Callsign: "ESMS_TWR", Position: position},
				}, nil
			},
		},
	}

	hub := buildReconcileHub(server)
	decision, err := hub.classifyOfflineAction(context.Background(), &offlineTimerEntry{
		session:      session,
		callsign:     callsign,
		positionFreq: position,
		positionName: "EKCH_TWR",
	})

	require.NoError(t, err)
	assert.Equal(t, offlineActionFinalize, decision)
}

func TestClassifyOfflineAction_FinalizesWhenOriginalControllerStillOwnsPosition(t *testing.T) {
	const session = int32(1)
	const callsign = "EKCH_TWR"
	const position = "118.700"
	t.Cleanup(config.SetOwnerCallsignPrefixesForTest([]string{"EKCH", "EKDK"}))

	server := &testutil.MockServer{
		FrontendHubVal: &testutil.MockFrontendHub{},
		ControllerRepoVal: &testutil.MockControllerRepository{
			GetByCallsignFn: func(_ context.Context, sess int32, cs string) (*models.Controller, error) {
				assert.Equal(t, session, sess)
				assert.Equal(t, callsign, cs)
				return &models.Controller{
					Callsign: callsign,
					Position: position,
				}, nil
			},
			GetByPositionFn: func(_ context.Context, sess int32, pos string) ([]*models.Controller, error) {
				assert.Equal(t, session, sess)
				assert.Equal(t, position, pos)
				return []*models.Controller{
					{Callsign: callsign, Position: position},
				}, nil
			},
		},
	}

	hub := buildReconcileHub(server)
	decision, err := hub.classifyOfflineAction(context.Background(), &offlineTimerEntry{
		session:      session,
		callsign:     callsign,
		positionFreq: position,
		positionName: "EKCH_TWR",
	})

	require.NoError(t, err)
	assert.Equal(t, offlineActionFinalize, decision)
}

func TestScheduleOfflineActions_SilentCleanupSuppressesOfflineNotification(t *testing.T) {
	const session = int32(1)
	const callsign = "EKCH_TWR"
	const position = "118.700"
	const positionName = "EKCH_TWR"
	t.Cleanup(config.SetOwnerCallsignPrefixesForTest([]string{"EKCH", "EKDK"}))

	var deleteCalls atomic.Int32
	var updateSectorsCalls atomic.Int32
	var updateLayoutsCalls atomic.Int32
	var updateRoutesCalls atomic.Int32

	frontendHub := &testutil.MockFrontendHub{}
	server := &testutil.MockServer{
		FrontendHubVal: frontendHub,
		ControllerRepoVal: &testutil.MockControllerRepository{
			GetByCallsignFn: func(_ context.Context, sess int32, cs string) (*models.Controller, error) {
				assert.Equal(t, session, sess)
				assert.Equal(t, callsign, cs)
				return &models.Controller{
					Callsign: callsign,
					Position: position,
				}, nil
			},
			GetByPositionFn: func(_ context.Context, sess int32, pos string) ([]*models.Controller, error) {
				assert.Equal(t, session, sess)
				assert.Equal(t, position, pos)
				return []*models.Controller{
					{Callsign: callsign, Position: position},
					{Callsign: "EKCH_A_TWR", Position: position},
				}, nil
			},
			DeleteFn: func(_ context.Context, sess int32, cs string) error {
				assert.Equal(t, session, sess)
				assert.Equal(t, callsign, cs)
				deleteCalls.Add(1)
				return nil
			},
		},
		UpdateSectorsFn: func(_ int32) ([]shared.SectorChange, error) {
			updateSectorsCalls.Add(1)
			return nil, nil
		},
		UpdateLayoutsFn: func(_ int32) error {
			updateLayoutsCalls.Add(1)
			return nil
		},
		UpdateRoutesForSessionFn: func(_ int32, _ bool) error {
			updateRoutesCalls.Add(1)
			return nil
		},
	}

	hub := buildReconcileHub(server)
	hub.scheduleOfflineActions(session, callsign, position, positionName, 10*time.Millisecond)

	time.Sleep(120 * time.Millisecond)

	assert.Equal(t, int32(1), deleteCalls.Load(), "stale controller row should be cleaned up")
	assert.Empty(t, frontendHub.ControllerOfflines, "replacement coverage should suppress stale offline notification")
	assert.Equal(t, int32(0), updateSectorsCalls.Load(), "silent cleanup must not recalculate sectors")
	assert.Equal(t, int32(0), updateLayoutsCalls.Load(), "silent cleanup must not recalculate layouts")
	assert.Equal(t, int32(0), updateRoutesCalls.Load(), "silent cleanup must not recalculate routes")

	hub.offlineMu.Lock()
	defer hub.offlineMu.Unlock()
	assert.Empty(t, hub.offlineTimers, "timer entry should be removed after execution")
}

func TestScheduleOfflineActions_UsesStoredTimerEntryMetadata(t *testing.T) {
	const session = int32(1)
	const originalCallsign = "EKCH_TWR"
	const replacementCallsign = "EKCH_A_TWR"
	const originalPosition = "118.700"
	const replacementPosition = "118.105"
	const positionName = "EKCH_TWR"

	var deleteCallsign string

	frontendHub := &testutil.MockFrontendHub{}
	server := &testutil.MockServer{
		FrontendHubVal: frontendHub,
		ControllerRepoVal: &testutil.MockControllerRepository{
			GetByCallsignFn: func(_ context.Context, sess int32, cs string) (*models.Controller, error) {
				assert.Equal(t, session, sess)
				assert.Equal(t, replacementCallsign, cs)
				return &models.Controller{
					Callsign: replacementCallsign,
					Position: replacementPosition,
				}, nil
			},
			GetByPositionFn: func(_ context.Context, sess int32, pos string) ([]*models.Controller, error) {
				assert.Equal(t, session, sess)
				assert.Equal(t, replacementPosition, pos)
				return []*models.Controller{
					{Callsign: replacementCallsign, Position: replacementPosition},
				}, nil
			},
			DeleteFn: func(_ context.Context, sess int32, cs string) error {
				assert.Equal(t, session, sess)
				deleteCallsign = cs
				return nil
			},
		},
	}

	hub := buildReconcileHub(server)
	hub.scheduleOfflineActions(session, originalCallsign, originalPosition, positionName, 10*time.Millisecond)

	time.Sleep(5 * time.Millisecond)

	key := "1:" + positionName
	hub.offlineMu.Lock()
	hub.offlineTimers[key] = &offlineTimerEntry{
		session:      session,
		callsign:     replacementCallsign,
		positionFreq: replacementPosition,
		positionName: positionName,
	}
	hub.offlineMu.Unlock()

	time.Sleep(120 * time.Millisecond)

	require.Equal(t, replacementCallsign, deleteCallsign)
	require.Len(t, frontendHub.ControllerOfflines, 1)
	assert.Equal(t, replacementCallsign, frontendHub.ControllerOfflines[0].Callsign)
	assert.Equal(t, replacementPosition, frontendHub.ControllerOfflines[0].Position)
}
