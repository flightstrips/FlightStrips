package euroscope

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	eventseuroscope "FlightStrips/pkg/events/euroscope"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type aircraftAliveStripService struct {
	noOpStripService
	syncCalls     atomic.Int32
	positionCalls atomic.Int32
	deleteCalls   atomic.Int32
}

func (s *aircraftAliveStripService) SyncStrip(_ context.Context, _ int32, _ string, _ interface{}, _ string) error {
	s.syncCalls.Add(1)
	return nil
}

func (s *aircraftAliveStripService) UpdateAircraftPosition(_ context.Context, _ int32, _ string, _, _ float64, _ int32, _ string) error {
	s.positionCalls.Add(1)
	return nil
}

func (s *aircraftAliveStripService) DeleteStrip(_ context.Context, _ int32, _ string) error {
	s.deleteCalls.Add(1)
	return nil
}

func newAircraftDisconnectTestHub(stripService shared.StripService) *Hub {
	frontendHub := &testutil.MockFrontendHub{}
	return &Hub{
		stripService:             stripService,
		aircraftDisconnectTimers: make(map[string]*aircraftDisconnectEntry),
		server: &testutil.MockServer{
			FrontendHubVal: frontendHub,
			StripRepoVal:   &testutil.MockStripRepository{},
		},
	}
}

func mustMarshalMessage(t *testing.T, payload interface{}) []byte {
	t.Helper()

	data, err := json.Marshal(payload)
	require.NoError(t, err)

	return data
}

func TestHandleStripUpdateEvent_CancelsPendingAircraftDisconnect(t *testing.T) {
	stripService := &aircraftAliveStripService{}
	hub := newAircraftDisconnectTestHub(stripService)
	client := &Client{hub: hub, session: 42, airport: "EKCH"}

	hub.scheduleAircraftDisconnect(client.session, "BAW819K", 25*time.Millisecond)

	err := handleStripUpdateEvent(context.Background(), client, Message{
		Type: eventseuroscope.StripUpdate,
		Message: mustMarshalMessage(t, eventseuroscope.StripUpdateEvent{
			Type: eventseuroscope.StripUpdate,
			Strip: eventseuroscope.Strip{
				Callsign: "BAW819K",
			},
		}),
	})
	require.NoError(t, err)

	hub.aircraftDisconnectMu.Lock()
	timerCount := len(hub.aircraftDisconnectTimers)
	hub.aircraftDisconnectMu.Unlock()

	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 0, timerCount, "strip update should cancel a pending disconnect timer")
	assert.Equal(t, int32(1), stripService.syncCalls.Load(), "strip update should still be processed")
	assert.Equal(t, int32(0), stripService.deleteCalls.Load(), "cancelled disconnect timer must not delete the strip")
	frontendHub := hub.server.GetFrontendHub().(*testutil.MockFrontendHub)
	require.Len(t, frontendHub.StripUpdates, 1)
	assert.Equal(t, "BAW819K", frontendHub.StripUpdates[0].Callsign)
}

func TestHandlePositionUpdate_CancelsPendingAircraftDisconnect(t *testing.T) {
	stripService := &aircraftAliveStripService{}
	hub := newAircraftDisconnectTestHub(stripService)
	client := &Client{hub: hub, session: 42, airport: "EKCH"}

	hub.scheduleAircraftDisconnect(client.session, "DLH9HV", 25*time.Millisecond)

	err := handlePositionUpdate(context.Background(), client, Message{
		Type: eventseuroscope.PositionUpdate,
		Message: mustMarshalMessage(t, eventseuroscope.AircraftPositionUpdateEvent{
			Type:     eventseuroscope.PositionUpdate,
			Callsign: "DLH9HV",
			Lat:      55.62583,
			Lon:      12.64562,
			Altitude: 19,
		}),
	})
	require.NoError(t, err)

	hub.aircraftDisconnectMu.Lock()
	timerCount := len(hub.aircraftDisconnectTimers)
	hub.aircraftDisconnectMu.Unlock()

	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 0, timerCount, "position update should cancel a pending disconnect timer")
	assert.Equal(t, int32(1), stripService.positionCalls.Load(), "position update should still be processed")
	assert.Equal(t, int32(0), stripService.deleteCalls.Load(), "cancelled disconnect timer must not delete the strip")
	frontendHub := hub.server.GetFrontendHub().(*testutil.MockFrontendHub)
	require.Len(t, frontendHub.StripUpdates, 1)
	assert.Equal(t, "DLH9HV", frontendHub.StripUpdates[0].Callsign)
}

func TestScheduleAircraftDisconnectResetsExistingWorker(t *testing.T) {
	stripService := &aircraftAliveStripService{}
	hub := newAircraftDisconnectTestHub(stripService)
	const callsign = "SAS811"
	const session = int32(42)
	key := fmt.Sprintf("%d:%s", session, callsign)

	hub.scheduleAircraftDisconnect(session, callsign, 20*time.Millisecond)
	hub.aircraftDisconnectMu.Lock()
	original := hub.aircraftDisconnectTimers[key]
	hub.aircraftDisconnectMu.Unlock()
	require.NotNil(t, original)

	hub.scheduleAircraftDisconnect(session, callsign, time.Second)
	hub.aircraftDisconnectMu.Lock()
	current := hub.aircraftDisconnectTimers[key]
	hub.aircraftDisconnectMu.Unlock()

	assert.Same(t, original, current)
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(0), stripService.deleteCalls.Load(), "reset deadline must replace the original short deadline")
	hub.cancelAircraftDisconnect(session, callsign)
}

func TestAircraftDisconnectTimerRetainsStripOwnedByAnotherSource(t *testing.T) {
	stripService := &aircraftAliveStripService{}
	hub := newAircraftDisconnectTestHub(stripService)
	hub.server = &testutil.MockServer{
		FrontendHubVal: &testutil.MockFrontendHub{},
		StripRepoVal: &testutil.MockStripRepository{
			ClearEuroscopeSeenFn: func(context.Context, int32, string) error { return nil },
		},
	}
	hub.SetAircraftDisconnectRetainer(func(_ context.Context, session int32, callsign string) bool {
		return session == 42 && callsign == "SAS808"
	})

	hub.scheduleAircraftDisconnect(42, "SAS808", 10*time.Millisecond)
	time.Sleep(30 * time.Millisecond)

	assert.Equal(t, int32(0), stripService.deleteCalls.Load(), "VATSIM or an active SAT assignment must retain the strip")
	assert.True(t, hub.IsAircraftDisconnectPending(42, "SAS808"), "the completed disconnect remains tombstoned until EuroScope reports the strip again")
	assert.True(t, hub.cancelAircraftDisconnect(42, "SAS808"))
	assert.False(t, hub.IsAircraftDisconnectPending(42, "SAS808"))
}

func TestAircraftDisconnectTimerKeepsPendingStateWhenClearingProvenanceFails(t *testing.T) {
	stripService := &aircraftAliveStripService{}
	frontendHub := &testutil.MockFrontendHub{}
	var clearAttempts atomic.Int32
	hub := newAircraftDisconnectTestHub(stripService)
	hub.server = &testutil.MockServer{
		FrontendHubVal: frontendHub,
		StripRepoVal: &testutil.MockStripRepository{
			ClearEuroscopeSeenFn: func(context.Context, int32, string) error {
				clearAttempts.Add(1)
				return errors.New("database unavailable")
			},
		},
	}
	hub.SetAircraftDisconnectRetainer(func(context.Context, int32, string) bool { return true })

	hub.scheduleAircraftDisconnect(42, "SAS809", time.Millisecond)
	t.Cleanup(func() { hub.cancelAircraftDisconnect(42, "SAS809") })
	require.Eventually(t, func() bool {
		return clearAttempts.Load() > 0
	}, 100*time.Millisecond, time.Millisecond)

	assert.True(t, hub.IsAircraftDisconnectPending(42, "SAS809"), "failed provenance clearing must remain pending for retry")
	assert.Empty(t, frontendHub.AircraftDisconnects)
	assert.Equal(t, int32(0), stripService.deleteCalls.Load())
}

func TestCancelAircraftDisconnectJoinsInFlightProvenanceClear(t *testing.T) {
	stripService := &aircraftAliveStripService{}
	frontendHub := &testutil.MockFrontendHub{}
	clearStarted := make(chan struct{})
	clearStopped := make(chan struct{})
	hub := newAircraftDisconnectTestHub(stripService)
	hub.server = &testutil.MockServer{
		FrontendHubVal: frontendHub,
		StripRepoVal: &testutil.MockStripRepository{
			ClearEuroscopeSeenFn: func(ctx context.Context, _ int32, _ string) error {
				close(clearStarted)
				<-ctx.Done()
				close(clearStopped)
				return ctx.Err()
			},
		},
	}
	hub.SetAircraftDisconnectRetainer(func(context.Context, int32, string) bool { return true })
	hub.scheduleAircraftDisconnect(42, "SAS810", time.Millisecond)

	select {
	case <-clearStarted:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("disconnect worker did not start provenance clearing")
	}

	assert.True(t, hub.cancelAircraftDisconnect(42, "SAS810"))
	select {
	case <-clearStopped:
	default:
		t.Fatal("cancellation returned before the provenance clear stopped")
	}
	assert.False(t, hub.IsAircraftDisconnectPending(42, "SAS810"))
	assert.Empty(t, frontendHub.AircraftDisconnects)
}
