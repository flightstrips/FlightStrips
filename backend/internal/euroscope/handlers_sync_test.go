package euroscope

import (
	"context"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/assert"
)

// ---- buildMultipleOfflineBroadcastMessage ----

func TestBuildMultipleOfflineBroadcastMessage_NoChanges(t *testing.T) {
	msg := buildMultipleOfflineBroadcastMessage([]string{"EKCH_DEL", "EKCH_GND"}, nil)
	assert.Equal(t, "EKCH_DEL, EKCH_GND went offline.", msg)
}

func TestBuildMultipleOfflineBroadcastMessage_WithChanges(t *testing.T) {
	changes := []shared.SectorChange{
		{SectorName: "CLR", ToPosition: "EKCH_TWR"},
		{SectorName: "GND", ToPosition: ""},
	}
	msg := buildMultipleOfflineBroadcastMessage([]string{"EKCH_DEL", "EKCH_GND"}, changes)
	assert.Equal(t, "EKCH_DEL, EKCH_GND went offline. Sectors: CLR (to EKCH_TWR), GND (no coverage).", msg)
}

// ---- scheduleSessionUpdate debouncer ----

// buildDebouncerHub returns a minimal Hub wired for debouncer tests.
func buildDebouncerHub(server *testutil.MockServer) *Hub {
	return &Hub{
		server:              server,
		sessionUpdateTimers: make(map[int32]*sessionUpdatePending),
	}
}

func TestScheduleSessionUpdate_CollapsesConcurrentCalls(t *testing.T) {
	var mu sync.Mutex
	callCount := 0

	server := &testutil.MockServer{
		FrontendHubVal: &testutil.MockFrontendHub{},
		UpdateSectorsFn: func(_ int32) ([]shared.SectorChange, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			return nil, nil
		},
	}

	hub := buildDebouncerHub(server)

	// Five rapid calls should all collapse into one UpdateSectors invocation.
	for i := 0; i < 5; i++ {
		hub.scheduleSessionUpdate(1, "")
	}

	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	count := callCount
	mu.Unlock()

	assert.Equal(t, 1, count, "five rapid scheduleSessionUpdate calls should collapse into one UpdateSectors call")
}

func TestScheduleSessionUpdate_SeparatesDistinctSessions(t *testing.T) {
	var mu sync.Mutex
	callCount := 0

	server := &testutil.MockServer{
		FrontendHubVal: &testutil.MockFrontendHub{},
		UpdateSectorsFn: func(_ int32) ([]shared.SectorChange, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			return nil, nil
		},
	}

	hub := buildDebouncerHub(server)

	hub.scheduleSessionUpdate(1, "")
	hub.scheduleSessionUpdate(2, "")

	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	count := callCount
	mu.Unlock()

	assert.Equal(t, 2, count, "calls for different sessions must each trigger their own UpdateSectors run")
}

func TestScheduleSessionUpdate_RetriggersWhenCalledDuringProcessing(t *testing.T) {
	var mu sync.Mutex
	callCount := 0

	firstStarted := make(chan struct{})
	firstCanProceed := make(chan struct{})

	server := &testutil.MockServer{
		FrontendHubVal: &testutil.MockFrontendHub{},
		UpdateSectorsFn: func(_ int32) ([]shared.SectorChange, error) {
			mu.Lock()
			n := callCount + 1
			callCount++
			mu.Unlock()

			if n == 1 {
				close(firstStarted)
				<-firstCanProceed
			}
			return nil, nil
		},
	}

	hub := buildDebouncerHub(server)

	// Trigger first debounce run.
	hub.scheduleSessionUpdate(1, "")

	// Wait until the first UpdateSectors is executing.
	<-firstStarted

	// Schedule a second update while the first is still running.
	hub.scheduleSessionUpdate(1, "")

	// Let the first run finish.
	close(firstCanProceed)

	// Give the second debounce timer time to fire and complete.
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	count := callCount
	mu.Unlock()

	assert.Equal(t, 2, count, "a scheduleSessionUpdate call during processing must trigger a second UpdateSectors run")
}

// ---- reconcileStaleControllers ----

// buildReconcileHub returns a Hub with just enough fields for reconcile* tests.
func buildReconcileHub(server *testutil.MockServer) *Hub {
	return &Hub{
		server:                   server,
		offlineTimers:            make(map[string]*offlineTimerEntry),
		aircraftDisconnectTimers: make(map[string]*aircraftDisconnectEntry),
		sessionUpdateTimers:      make(map[int32]*sessionUpdatePending),
	}
}

func TestReconcileStaleControllers_SchedulesTimerForMissingController(t *testing.T) {
	server := &testutil.MockServer{
		FrontendHubVal: &testutil.MockFrontendHub{},
		ControllerRepoVal: &testutil.MockControllerRepository{
			ListFn: func(_ context.Context, _ int32) ([]*models.Controller, error) {
				return []*models.Controller{
					{Callsign: "EKCH_A_GND", Position: "121.900"},
				}, nil
			},
		},
	}

	hub := buildReconcileHub(server)
	client := &Client{hub: hub, session: 1, callsign: "EKCH_TWR"}

	// EKCH_A_GND is in the DB but absent from knownCallsigns → should get a timer.
	reconcileStaleControllers(context.Background(), client, 1, map[string]bool{"EKCH_TWR": true})

	hub.offlineMu.Lock()
	count := len(hub.offlineTimers)
	for _, e := range hub.offlineTimers {
		e.cancel() // clean up goroutines
	}
	hub.offlineMu.Unlock()

	assert.Equal(t, 1, count, "one offline timer must be scheduled for the missing controller")
}

func TestReconcileStaleControllers_SkipsKnownControllers(t *testing.T) {
	server := &testutil.MockServer{
		FrontendHubVal: &testutil.MockFrontendHub{},
		ControllerRepoVal: &testutil.MockControllerRepository{
			ListFn: func(_ context.Context, _ int32) ([]*models.Controller, error) {
				return []*models.Controller{
					{Callsign: "EKCH_A_GND", Position: "121.900"},
				}, nil
			},
		},
	}

	hub := buildReconcileHub(server)
	client := &Client{hub: hub, session: 1, callsign: "EKCH_TWR"}

	// EKCH_A_GND IS in knownCallsigns → no timer.
	reconcileStaleControllers(context.Background(), client, 1, map[string]bool{
		"EKCH_TWR":   true,
		"EKCH_A_GND": true,
	})

	hub.offlineMu.Lock()
	count := len(hub.offlineTimers)
	hub.offlineMu.Unlock()

	assert.Equal(t, 0, count, "no offline timer must be scheduled for a controller present in the sync")
}

func TestReconcileStaleControllers_KnownControllerIsNotScheduled(t *testing.T) {
	// Verify that a controller whose callsign is in knownCallsigns is never
	// passed to scheduleOfflineActions, even when other stale controllers are present.
	server := &testutil.MockServer{
		FrontendHubVal: &testutil.MockFrontendHub{},
		ControllerRepoVal: &testutil.MockControllerRepository{
			ListFn: func(_ context.Context, _ int32) ([]*models.Controller, error) {
				return []*models.Controller{
					{Callsign: "EKCH_A_GND", Position: "121.900"},
					{Callsign: "EKCH_TWR", Position: "118.700"},
				}, nil
			},
		},
	}

	hub := buildReconcileHub(server)
	client := &Client{hub: hub, session: 1, callsign: "EKCH_TWR"}

	// Only EKCH_TWR is in the sync; EKCH_A_GND is stale.
	reconcileStaleControllers(context.Background(), client, 1, map[string]bool{"EKCH_TWR": true})

	hub.offlineMu.Lock()
	count := len(hub.offlineTimers)
	for _, e := range hub.offlineTimers {
		e.cancel()
	}
	hub.offlineMu.Unlock()

	// Exactly 1 timer for the one stale controller; the known EKCH_TWR must not be scheduled.
	assert.Equal(t, 1, count, "only the stale controller should have an offline timer")
}

// ---- reconcileStaleStrips ----

func TestReconcileStaleStrips_SchedulesTimerForMissingStrip(t *testing.T) {
	server := &testutil.MockServer{
		FrontendHubVal: &testutil.MockFrontendHub{},
		StripRepoVal: &testutil.MockStripRepository{
			ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
				return []*models.Strip{
					{Callsign: "SAS123"},
				}, nil
			},
		},
	}

	hub := buildReconcileHub(server)
	client := &Client{hub: hub, session: 1}

	// SAS123 is in the DB but absent from the sync → should get a disconnect timer.
	reconcileStaleStrips(context.Background(), client, 1, map[string]bool{})

	hub.aircraftDisconnectMu.Lock()
	count := len(hub.aircraftDisconnectTimers)
	for _, e := range hub.aircraftDisconnectTimers {
		e.cancel()
	}
	hub.aircraftDisconnectMu.Unlock()

	assert.Equal(t, 1, count, "one disconnect timer must be scheduled for the missing strip")
}

func TestReconcileStaleStrips_SkipsKnownStrips(t *testing.T) {
	server := &testutil.MockServer{
		FrontendHubVal: &testutil.MockFrontendHub{},
		StripRepoVal: &testutil.MockStripRepository{
			ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
				return []*models.Strip{
					{Callsign: "SAS123"},
				}, nil
			},
		},
	}

	hub := buildReconcileHub(server)
	client := &Client{hub: hub, session: 1}

	// SAS123 IS in the sync → no timer.
	reconcileStaleStrips(context.Background(), client, 1, map[string]bool{"SAS123": true})

	hub.aircraftDisconnectMu.Lock()
	count := len(hub.aircraftDisconnectTimers)
	hub.aircraftDisconnectMu.Unlock()

	assert.Equal(t, 0, count, "no disconnect timer must be scheduled for a strip present in the sync")
}

func TestReconcileStaleStrips_MixedKnownAndStale(t *testing.T) {
	server := &testutil.MockServer{
		FrontendHubVal: &testutil.MockFrontendHub{},
		StripRepoVal: &testutil.MockStripRepository{
			ListFn: func(_ context.Context, _ int32) ([]*models.Strip, error) {
				return []*models.Strip{
					{Callsign: "SAS123"},
					{Callsign: "SAS456"},
					{Callsign: "SAS789"},
				}, nil
			},
		},
	}

	hub := buildReconcileHub(server)
	client := &Client{hub: hub, session: 1}

	// SAS123 is in the sync; SAS456 and SAS789 are stale.
	reconcileStaleStrips(context.Background(), client, 1, map[string]bool{"SAS123": true})

	hub.aircraftDisconnectMu.Lock()
	count := len(hub.aircraftDisconnectTimers)
	for _, e := range hub.aircraftDisconnectTimers {
		e.cancel()
	}
	hub.aircraftDisconnectMu.Unlock()

	assert.Equal(t, 2, count, "one disconnect timer per stale strip")
}
