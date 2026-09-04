package euroscope

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const aircraftDisconnectRetryDelay = 5 * time.Second

// aircraftDisconnectEntry owns the complete lifecycle of one pending disconnect.
// done closes only after the worker has stopped all database writes and frontend events.
type aircraftDisconnectEntry struct {
	cancel   context.CancelFunc
	done     chan struct{}
	reset    chan time.Duration
	session  int32
	callsign string
}

// scheduleAircraftDisconnect starts a worker that removes the strip after the given
// delay, unless cancelAircraftDisconnect stops and joins it first. Duplicate schedules
// leave the existing worker in charge.
func (hub *Hub) scheduleAircraftDisconnect(session int32, callsign string, delay time.Duration) {
	key := fmt.Sprintf("%d:%s", session, callsign)
	ctx, cancel := context.WithCancel(context.Background())
	entry := &aircraftDisconnectEntry{
		cancel:   cancel,
		done:     make(chan struct{}),
		reset:    make(chan time.Duration, 1),
		session:  session,
		callsign: callsign,
	}

	hub.aircraftDisconnectMu.Lock()
	if _, disconnected := hub.aircraftDisconnects[key]; disconnected {
		hub.aircraftDisconnectMu.Unlock()
		cancel()
		return
	}
	if existing := hub.aircraftDisconnectTimers[key]; existing != nil {
		select {
		case existing.reset <- delay:
		default:
			// Keep only the latest requested deadline reset.
			select {
			case <-existing.reset:
			default:
			}
			existing.reset <- delay
		}
		hub.aircraftDisconnectMu.Unlock()
		cancel()
		return
	}
	hub.aircraftDisconnectTimers[key] = entry
	hub.aircraftDisconnectMu.Unlock()

	go hub.runAircraftDisconnect(ctx, key, entry, delay)
}

func (hub *Hub) runAircraftDisconnect(ctx context.Context, key string, entry *aircraftDisconnectEntry, delay time.Duration) {
	defer func() {
		hub.aircraftDisconnectMu.Lock()
		if hub.aircraftDisconnectTimers[key] == entry {
			delete(hub.aircraftDisconnectTimers, key)
		}
		hub.aircraftDisconnectMu.Unlock()
		close(entry.done)
	}()

	wait := delay
	for {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			slog.Debug("Aircraft disconnect timer cancelled (aircraft still alive)",
				slog.String("callsign", entry.callsign),
				slog.Int("session", int(entry.session)))
			return
		case wait = <-entry.reset:
			timer.Stop()
			continue
		case <-timer.C:
		}

		hub.aircraftDisconnectMu.Lock()
		current := hub.aircraftDisconnectTimers[key]
		retainer := hub.aircraftDisconnectRetainer
		hub.aircraftDisconnectMu.Unlock()
		if current != entry {
			return
		}

		if retainer != nil && retainer(ctx, entry.session, entry.callsign) {
			if err := hub.clearEuroscopeSeen(ctx, entry.session, entry.callsign); err != nil {
				if ctx.Err() != nil {
					return
				}
				slog.Warn("Failed to clear EuroScope strip presence after disconnect; retrying",
					slog.String("callsign", entry.callsign),
					slog.Int("session", int(entry.session)),
					slog.Any("error", err))
				wait = aircraftDisconnectRetryDelay
				continue
			}
			hub.aircraftDisconnectMu.Lock()
			if ctx.Err() != nil || hub.aircraftDisconnectTimers[key] != entry {
				hub.aircraftDisconnectMu.Unlock()
				return
			}
			if hub.aircraftDisconnects == nil {
				hub.aircraftDisconnects = make(map[string]struct{})
			}
			hub.aircraftDisconnects[key] = struct{}{}
			hub.aircraftDisconnectMu.Unlock()
			if hub.server != nil && hub.server.GetFrontendHub() != nil {
				hub.server.GetFrontendHub().SendAircraftDisconnect(entry.session, entry.callsign)
			}
			slog.Debug("Aircraft disconnect retained by another source",
				slog.String("callsign", entry.callsign),
				slog.Int("session", int(entry.session)))
			return
		}

		slog.Debug("Aircraft disconnected, removing strip",
			slog.String("callsign", entry.callsign),
			slog.Int("session", int(entry.session)))
		if err := hub.stripService.DeleteStrip(ctx, entry.session, entry.callsign); err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("Failed to delete strip in aircraft disconnect timer; retrying",
				slog.String("callsign", entry.callsign),
				slog.Any("error", err))
			wait = aircraftDisconnectRetryDelay
			continue
		}
		return
	}
}

func (hub *Hub) clearEuroscopeSeen(ctx context.Context, session int32, callsign string) error {
	if hub.server == nil || hub.server.GetStripRepository() == nil {
		return errors.New("strip repository is unavailable")
	}
	return hub.server.GetStripRepository().ClearEuroscopeSeen(ctx, session, callsign)
}

// cancelAircraftDisconnect cancels and joins a pending worker. On return, that
// worker can no longer clear provenance, delete the strip, or emit a disconnect.
func (hub *Hub) cancelAircraftDisconnect(session int32, callsign string) bool {
	key := fmt.Sprintf("%d:%s", session, callsign)
	hub.aircraftDisconnectMu.Lock()
	entry := hub.aircraftDisconnectTimers[key]
	_, disconnected := hub.aircraftDisconnects[key]
	delete(hub.aircraftDisconnects, key)
	if entry != nil {
		entry.cancel()
	}
	hub.aircraftDisconnectMu.Unlock()
	if entry != nil {
		<-entry.done
	}
	return entry != nil || disconnected
}
