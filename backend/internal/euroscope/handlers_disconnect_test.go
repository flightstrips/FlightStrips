package euroscope

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
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
	assignedCalls atomic.Int32
	deleteCalls   atomic.Int32
	positionMu    sync.Mutex
	lastPosition  cachedAircraftPosition
}

type blockingPositionStripService struct {
	noOpStripService
	firstEntered chan struct{}
	releaseFirst chan struct{}
}

func (s *blockingPositionStripService) UpdateAircraftPosition(_ context.Context, _ int32, callsign string, _, _ float64, _ int32, _ string) error {
	if callsign == "SAS123" {
		close(s.firstEntered)
		<-s.releaseFirst
	}
	return nil
}

func (s *aircraftAliveStripService) SyncStrip(_ context.Context, _ int32, _ string, _ interface{}, _ string) error {
	s.syncCalls.Add(1)
	return nil
}

func (s *aircraftAliveStripService) UpdateAircraftPosition(_ context.Context, _ int32, _ string, lat, lon float64, altitude int32, _ string) error {
	s.positionMu.Lock()
	s.lastPosition = cachedAircraftPosition{lat: lat, lon: lon, altitude: altitude}
	s.positionMu.Unlock()
	s.positionCalls.Add(1)
	return nil
}

func (s *aircraftAliveStripService) position() cachedAircraftPosition {
	s.positionMu.Lock()
	defer s.positionMu.Unlock()
	return s.lastPosition
}

func (s *aircraftAliveStripService) UpdateAssignedSquawk(_ context.Context, _ int32, _ string, _ string) error {
	s.assignedCalls.Add(1)
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
		master:                   make(map[int32]*Client),
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

func TestHandleStripUpdateEvent_DeduplicatesPositionOnlyCallbacks(t *testing.T) {
	stripService := &aircraftAliveStripService{}
	hub := newAircraftDisconnectTestHub(stripService)
	client := &Client{hub: hub, session: 42, airport: "EKCH", positionCoalesceDelay: 10 * time.Millisecond}
	hub.master[client.session] = client
	t.Cleanup(client.stopPendingPositionUpdates)

	strip := eventseuroscope.Strip{
		Callsign:       "SAS123",
		Origin:         "EKCH",
		Destination:    "ESSA",
		Route:          "NEXEN",
		AssignedSquawk: "1234",
	}
	strip.Position.Lat = 55.6
	strip.Position.Lon = 12.6
	strip.Position.Altitude = 1000

	sendStrip := func(value eventseuroscope.Strip) {
		t.Helper()
		require.NoError(t, handleStripUpdateEvent(context.Background(), client, Message{
			Type: eventseuroscope.StripUpdate,
			Message: mustMarshalMessage(t, eventseuroscope.StripUpdateEvent{
				Type:  eventseuroscope.StripUpdate,
				Strip: value,
			}),
		}))
	}

	sendStrip(strip)
	moved := strip
	moved.Position.Lat = 55.7
	moved.Position.Lon = 12.7
	moved.Position.Altitude = 2000
	sendStrip(moved)
	movedAgain := moved
	movedAgain.Position.Lat = 55.8
	movedAgain.Position.Lon = 12.8
	movedAgain.Position.Altitude = 3000
	sendStrip(movedAgain)
	require.Eventually(t, func() bool {
		return stripService.positionCalls.Load() == 1
	}, 100*time.Millisecond, time.Millisecond)
	assert.Equal(t, cachedAircraftPosition{lat: 55.8, lon: 12.8, altitude: 3000}, stripService.position(),
		"coalescing should retain the newest position")
	slave := &Client{hub: hub, session: client.session, airport: client.airport, positionCoalesceDelay: time.Millisecond}
	slave.rememberOperationalStrip(movedAgain)
	slavePosition := movedAgain
	slavePosition.Position.Lat = 55.9
	require.NoError(t, handleStripUpdateEvent(context.Background(), slave, Message{
		Type: eventseuroscope.StripUpdate,
		Message: mustMarshalMessage(t, eventseuroscope.StripUpdateEvent{
			Type: eventseuroscope.StripUpdate, Strip: slavePosition,
		}),
	}))
	time.Sleep(5 * time.Millisecond)
	assert.Equal(t, int32(1), stripService.positionCalls.Load(), "slave position-only callbacks must be ignored")

	changed := movedAgain
	changed.Route = "NEXEN DCT"
	sendStrip(changed)

	assert.Equal(t, int32(2), stripService.syncCalls.Load(),
		"position-only callbacks should be dropped while operational changes still sync")

	sendAssigned := func(squawk string) {
		t.Helper()
		require.NoError(t, handleAssignedSquawk(context.Background(), client, Message{
			Type: eventseuroscope.AssignedSquawk,
			Message: mustMarshalMessage(t, eventseuroscope.AssignedSquawkEvent{
				Type:     eventseuroscope.AssignedSquawk,
				Callsign: strip.Callsign,
				Squawk:   squawk,
			}),
		}))
	}

	sendAssigned("1234")
	sendAssigned("5670")
	sendAssigned("5670")
	assert.Equal(t, int32(1), stripService.assignedCalls.Load(),
		"the full-strip snapshot should seed assigned-squawk deduplication")
}

func TestProcessAircraftPosition_DifferentCallsignsDoNotBlockEachOther(t *testing.T) {
	stripService := &blockingPositionStripService{
		firstEntered: make(chan struct{}),
		releaseFirst: make(chan struct{}),
	}
	client := &Client{hub: newAircraftDisconnectTestHub(stripService), session: 42, airport: "EKCH"}
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	released := false
	defer func() {
		if !released {
			close(stripService.releaseFirst)
		}
	}()

	go func() {
		firstDone <- client.processAircraftPosition(context.Background(), "SAS123", cachedAircraftPosition{lat: 55.6})
	}()
	select {
	case <-stripService.firstEntered:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("first callsign did not enter position processing")
	}

	go func() {
		secondDone <- client.processAircraftPosition(context.Background(), "SAS456", cachedAircraftPosition{lat: 55.7})
	}()
	select {
	case err := <-secondDone:
		require.NoError(t, err)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("second callsign was blocked by unrelated position processing")
	}

	close(stripService.releaseFirst)
	released = true
	require.NoError(t, <-firstDone)
}

func TestHandlePositionUpdate_CancelsPendingAircraftDisconnect(t *testing.T) {
	stripService := &aircraftAliveStripService{}
	hub := newAircraftDisconnectTestHub(stripService)
	client := &Client{hub: hub, session: 42, airport: "EKCH"}
	hub.master[client.session] = client

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

	slave := &Client{hub: hub, session: 42, airport: "EKCH"}
	err = handlePositionUpdate(context.Background(), slave, Message{
		Type: eventseuroscope.PositionUpdate,
		Message: mustMarshalMessage(t, eventseuroscope.AircraftPositionUpdateEvent{
			Type: eventseuroscope.PositionUpdate, Callsign: "DLH9HV", Lat: 56, Lon: 13, Altitude: 100,
		}),
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), stripService.positionCalls.Load(), "slave position updates must be ignored")
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
