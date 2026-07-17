package euroscope

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/shared"
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
	return &Hub{
		stripService:             stripService,
		aircraftDisconnectTimers: make(map[string]*aircraftDisconnectEntry),
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
}

func TestAircraftDisconnectTimerRetainsStripOwnedByAnotherSource(t *testing.T) {
	stripService := &aircraftAliveStripService{}
	hub := newAircraftDisconnectTestHub(stripService)
	hub.SetAircraftDisconnectRetainer(func(_ context.Context, session int32, callsign string) bool {
		return session == 42 && callsign == "SAS808"
	})

	hub.scheduleAircraftDisconnect(42, "SAS808", 10*time.Millisecond)
	time.Sleep(30 * time.Millisecond)

	assert.Equal(t, int32(0), stripService.deleteCalls.Load(), "VATSIM or an active SAT assignment must retain the strip")
}
