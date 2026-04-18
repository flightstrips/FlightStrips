package euroscope

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// aircraftDisconnectEntry holds the cancel function and metadata for a pending aircraft disconnect timer.
type aircraftDisconnectEntry struct {
	cancel   context.CancelFunc
	session  int32
	callsign string
}

// scheduleAircraftDisconnect starts a goroutine that removes the strip after the given
// delay, unless cancelAircraftDisconnect is called first (e.g. because the new master's
// SyncEvent confirms the aircraft is still alive).
func (hub *Hub) scheduleAircraftDisconnect(session int32, callsign string, delay time.Duration) {
	key := fmt.Sprintf("%d:%s", session, callsign)
	ctx, cancel := context.WithCancel(context.Background())

	hub.aircraftDisconnectMu.Lock()
	if existing, ok := hub.aircraftDisconnectTimers[key]; ok {
		existing.cancel()
	}
	hub.aircraftDisconnectTimers[key] = &aircraftDisconnectEntry{cancel: cancel, session: session, callsign: callsign}
	hub.aircraftDisconnectMu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
			slog.Debug("Aircraft disconnect timer cancelled (aircraft still alive)",
				slog.String("callsign", callsign),
				slog.Int("session", int(session)))
			return
		case <-time.After(delay):
		}

		hub.aircraftDisconnectMu.Lock()
		delete(hub.aircraftDisconnectTimers, key)
		hub.aircraftDisconnectMu.Unlock()

		slog.Debug("Aircraft disconnected, removing strip",
			slog.String("callsign", callsign),
			slog.Int("session", int(session)))
		if err := hub.stripService.DeleteStrip(context.Background(), session, callsign); err != nil {
			slog.Error("Failed to delete strip in aircraft disconnect timer",
				slog.String("callsign", callsign),
				slog.Any("error", err))
		}
	}()
}

// cancelAircraftDisconnect cancels a pending aircraft disconnect timer.
func (hub *Hub) cancelAircraftDisconnect(session int32, callsign string) {
	key := fmt.Sprintf("%d:%s", session, callsign)
	hub.aircraftDisconnectMu.Lock()
	defer hub.aircraftDisconnectMu.Unlock()
	if entry, ok := hub.aircraftDisconnectTimers[key]; ok {
		entry.cancel()
		delete(hub.aircraftDisconnectTimers, key)
	}
}
