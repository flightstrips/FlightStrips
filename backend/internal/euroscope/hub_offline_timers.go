package euroscope

import (
	"FlightStrips/internal/shared"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	offlineGracePeriod        = 60 * time.Second
	masterTransferGracePeriod = 45 * time.Second
)

// offlineTimerEntry holds the cancel function and metadata for a pending position offline timer.
type offlineTimerEntry struct {
	cancel       context.CancelFunc
	session      int32
	callsign     string
	positionFreq string
	positionName string
}

type offlineActionDecision int

const (
	offlineActionSkip offlineActionDecision = iota
	offlineActionSilentCleanup
	offlineActionFinalize
)

// sessionUpdatePending batches UpdateSectors/UpdateLayouts/UpdateRoutes calls
// that would otherwise fire concurrently from multiple offline timers.
type sessionUpdatePending struct {
	timer     *time.Timer
	positions []string // offline position names gathered in this debounce window
}

// scheduleOfflineActions starts a goroutine that, after the given grace period,
// deletes the controller from the database, notifies all frontend clients that the
// controller is offline, recalculates sector ownership, and — 5 seconds later —
// broadcasts a specific sector-change notification.
//
// If cancelOfflineTimer is called for the same position key before the grace period
// elapses, the goroutine exits cleanly and none of the above happens.
func (hub *Hub) scheduleOfflineActions(session int32, callsign, positionFreq, positionName string, delay time.Duration) {
	key := fmt.Sprintf("%d:%s", session, positionName)

	ctx, cancel := context.WithCancel(context.Background())

	hub.offlineMu.Lock()
	if existing, ok := hub.offlineTimers[key]; ok {
		existing.cancel()
	}
	hub.offlineTimers[key] = &offlineTimerEntry{
		cancel:       cancel,
		session:      session,
		callsign:     callsign,
		positionFreq: positionFreq,
		positionName: positionName,
	}
	hub.offlineMu.Unlock()

	go func() {
		// Phase 1: grace period
		select {
		case <-ctx.Done():
			slog.Debug("Controller offline timer cancelled (position came back online)",
				slog.String("position", positionName),
				slog.String("callsign", callsign),
				slog.Int("session", int(session)))
			return
		case <-time.After(delay):
		}

		slog.Debug("Controller offline timer fired: processing offline",
			slog.String("position", positionName),
			slog.String("callsign", callsign),
			slog.Int("session", int(session)))

		bgCtx := context.Background()
		entry := &offlineTimerEntry{
			session:      session,
			callsign:     callsign,
			positionFreq: positionFreq,
			positionName: positionName,
		}

		hub.offlineMu.Lock()
		if stored, ok := hub.offlineTimers[key]; ok {
			entry = &offlineTimerEntry{
				session:      stored.session,
				callsign:     stored.callsign,
				positionFreq: stored.positionFreq,
				positionName: stored.positionName,
			}
		}
		delete(hub.offlineTimers, key)
		hub.offlineMu.Unlock()

		s := hub.server
		controllerRepo := s.GetControllerRepository()
		decision, err := hub.classifyOfflineAction(bgCtx, entry)
		if err != nil {
			slog.Error("Failed to classify controller offline timer action",
				slog.String("callsign", entry.callsign),
				slog.String("position", entry.positionName),
				slog.Any("error", err))
			return
		}

		if decision == offlineActionSkip {
			slog.Debug("Skipping stale controller offline timer action",
				slog.String("callsign", entry.callsign),
				slog.String("position", entry.positionName),
				slog.Int("session", int(entry.session)))
			return
		}

		if err := controllerRepo.Delete(bgCtx, entry.session, entry.callsign); err != nil {
			slog.Error("Failed to delete controller record in offline timer",
				slog.String("callsign", entry.callsign),
				slog.Any("error", err))
		}

		if decision == offlineActionSilentCleanup {
			slog.Debug("Silently cleaned stale controller after position was re-covered",
				slog.String("callsign", entry.callsign),
				slog.String("position", entry.positionName),
				slog.Int("session", int(entry.session)))
			return
		}

		s.GetFrontendHub().SendControllerOffline(entry.session, entry.callsign, entry.positionFreq, "")

		// Signal the per-session debouncer to recalculate sectors/layouts/routes.
		// Multiple concurrent offline timers collapse into a single update run.
		hub.scheduleSessionUpdate(entry.session, entry.positionName)
	}()
}

func (hub *Hub) classifyOfflineAction(ctx context.Context, entry *offlineTimerEntry) (offlineActionDecision, error) {
	controllerRepo := hub.server.GetControllerRepository()
	controller, err := controllerRepo.GetByCallsign(ctx, entry.session, entry.callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return offlineActionSkip, nil
		}
		return offlineActionSkip, err
	}

	if controller.Position != entry.positionFreq {
		return offlineActionSkip, nil
	}

	controllersOnPosition, err := controllerRepo.GetByPosition(ctx, entry.session, entry.positionFreq)
	if err != nil {
		return offlineActionSkip, err
	}

	for _, other := range controllersOnPosition {
		if other.Callsign != entry.callsign && shared.IsOperationalPositionController(other) {
			return offlineActionSilentCleanup, nil
		}
	}

	return offlineActionFinalize, nil
}

// cancelOfflineTimer cancels a pending offline timer for the given position.
// Returns true if a timer was found and cancelled, false if none was pending.
func (hub *Hub) cancelOfflineTimer(session int32, positionName string) bool {
	key := fmt.Sprintf("%d:%s", session, positionName)
	hub.offlineMu.Lock()
	defer hub.offlineMu.Unlock()
	if entry, ok := hub.offlineTimers[key]; ok {
		entry.cancel()
		delete(hub.offlineTimers, key)
		slog.Debug("Offline timer cancelled — position came back online",
			slog.String("position", positionName),
			slog.Int("session", int(session)))
		return true
	}
	return false
}

// extendSessionTimers extends all pending position offline timers and aircraft disconnect
// timers for the session to masterTransferGracePeriod. Called when the master role is
// transferred so that slaves still on VATSIM have time to sync and cancel the timers.
func (hub *Hub) extendSessionTimers(session int32) {
	hub.offlineMu.Lock()
	posEntries := make([]*offlineTimerEntry, 0)
	for _, e := range hub.offlineTimers {
		if e.session == session {
			posEntries = append(posEntries, e)
		}
	}
	hub.offlineMu.Unlock()

	for _, e := range posEntries {
		hub.scheduleOfflineActions(e.session, e.callsign, e.positionFreq, e.positionName, masterTransferGracePeriod)
	}

	hub.aircraftDisconnectMu.Lock()
	acEntries := make([]*aircraftDisconnectEntry, 0)
	for _, e := range hub.aircraftDisconnectTimers {
		if e.session == session {
			acEntries = append(acEntries, e)
		}
	}
	hub.aircraftDisconnectMu.Unlock()

	for _, e := range acEntries {
		hub.scheduleAircraftDisconnect(e.session, e.callsign, masterTransferGracePeriod)
	}
}

// scheduleOnlineBroadcast fires a broadcast notification 5 seconds after a position
// first comes online, giving the sector update enough time to propagate to all
// clients before the message arrives.
func (hub *Hub) scheduleOnlineBroadcast(session int32, positionName string, changes []shared.SectorChange) {
	go func() {
		time.Sleep(5 * time.Second)
		msg := buildOnlineBroadcastMessage(positionName, changes)
		slog.Info("Sending online broadcast message",
			slog.String("position", positionName),
			slog.String("message", msg),
			slog.Int("session", int(session)))
		hub.server.GetFrontendHub().SendBroadcast(session, msg, "SYSTEM")
	}()
}

// scheduleSessionUpdate debounces calls to UpdateSectors/UpdateLayouts/UpdateRoutes
// for a session. Multiple calls within the debounce window (300 ms) are collapsed
// into a single run. If new offline events arrive while the run is in progress, a
// second run is automatically scheduled.
//
// positionName is the human-readable position name that went offline; pass "" when
// triggering an update that is not tied to a specific position going offline.
func (hub *Hub) scheduleSessionUpdate(session int32, positionName string) {
	hub.sessionUpdateMu.Lock()
	defer hub.sessionUpdateMu.Unlock()

	if pending, ok := hub.sessionUpdateTimers[session]; ok {
		// Timer is still pending — append the position and reset the window.
		if positionName != "" {
			pending.positions = append(pending.positions, positionName)
		}
		pending.timer.Reset(300 * time.Millisecond)
		return
	}

	positions := make([]string, 0, 1)
	if positionName != "" {
		positions = append(positions, positionName)
	}
	pending := &sessionUpdatePending{positions: positions}
	pending.timer = time.AfterFunc(300*time.Millisecond, func() {
		hub.sessionUpdateMu.Lock()
		// Guard against the AfterFunc-Reset race: if this pending has already
		// been consumed or replaced, skip this run.
		cur, ok := hub.sessionUpdateTimers[session]
		if !ok || cur != pending {
			hub.sessionUpdateMu.Unlock()
			return
		}
		pos := pending.positions
		delete(hub.sessionUpdateTimers, session)
		hub.sessionUpdateMu.Unlock()

		hub.runSessionUpdate(session, pos)
	})
	hub.sessionUpdateTimers[session] = pending
}

// runSessionUpdate executes the combined UpdateSectors/UpdateLayouts/UpdateRoutes
// recalculation for a session and sends the broadcast notification.
// positions holds the names of any positions that went offline in this window.
func (hub *Hub) runSessionUpdate(session int32, positions []string) {
	s := hub.server
	changes, err := s.UpdateSectors(session)
	if err != nil {
		slog.Error("Failed to update sectors in session update",
			slog.Int("session", int(session)), slog.Any("error", err))
	}
	if err := s.UpdateLayouts(session); err != nil {
		slog.Error("Failed to update layouts in session update",
			slog.Int("session", int(session)), slog.Any("error", err))
	}
	if err := s.UpdateRoutesForSession(session, true); err != nil {
		slog.Error("Failed to update routes in session update",
			slog.Int("session", int(session)), slog.Any("error", err))
	}

	if len(positions) == 0 {
		return
	}

	// Broadcast after a short delay so clients have received the sector update first.
	go func() {
		time.Sleep(5 * time.Second)
		var msg string
		if len(positions) == 1 {
			msg = buildOfflineBroadcastMessage(positions[0], changes)
		} else {
			msg = buildMultipleOfflineBroadcastMessage(positions, changes)
		}
		slog.Info("Sending session update broadcast",
			slog.Int("session", int(session)),
			slog.String("message", msg))
		s.GetFrontendHub().SendBroadcast(session, msg, "SYSTEM")
	}()
}

// buildOnlineBroadcastMessage constructs the human-readable broadcast message for a
// position coming online, listing each sector that transferred responsibility.
func buildOnlineBroadcastMessage(positionName string, changes []shared.SectorChange) string {
	if len(changes) == 0 {
		return fmt.Sprintf("%s is now online.", positionName)
	}
	if len(changes) == 1 {
		c := changes[0]
		if c.FromPosition == "" {
			return fmt.Sprintf("%s is now online. Sector %s now has coverage.", positionName, c.SectorName)
		}
		return fmt.Sprintf("%s is now online. Sector %s transferred from %s.", positionName, c.SectorName, c.FromPosition)
	}
	parts := make([]string, len(changes))
	for i, c := range changes {
		if c.FromPosition == "" {
			parts[i] = fmt.Sprintf("%s (no previous coverage)", c.SectorName)
		} else {
			parts[i] = fmt.Sprintf("%s (from %s)", c.SectorName, c.FromPosition)
		}
	}
	return fmt.Sprintf("%s is now online. Sectors: %s.", positionName, strings.Join(parts, ", "))
}

// buildMultipleOfflineBroadcastMessage constructs a combined offline message when
// several positions went offline in the same debounce window.
func buildMultipleOfflineBroadcastMessage(positions []string, changes []shared.SectorChange) string {
	names := strings.Join(positions, ", ")
	if len(changes) == 0 {
		return fmt.Sprintf("%s went offline.", names)
	}
	parts := make([]string, len(changes))
	for i, c := range changes {
		if c.ToPosition == "" {
			parts[i] = fmt.Sprintf("%s (no coverage)", c.SectorName)
		} else {
			parts[i] = fmt.Sprintf("%s (to %s)", c.SectorName, c.ToPosition)
		}
	}
	return fmt.Sprintf("%s went offline. Sectors: %s.", names, strings.Join(parts, ", "))
}

// buildOfflineBroadcastMessage constructs the human-readable broadcast message for a
// position going offline, listing each sector that transferred responsibility.
func buildOfflineBroadcastMessage(positionName string, changes []shared.SectorChange) string {
	if len(changes) == 0 {
		return fmt.Sprintf("%s went offline.", positionName)
	}
	if len(changes) == 1 {
		c := changes[0]
		if c.ToPosition == "" {
			return fmt.Sprintf("%s went offline. Sector %s has no coverage.", positionName, c.SectorName)
		}
		return fmt.Sprintf("%s went offline. Sector %s transferred to %s.", positionName, c.SectorName, c.ToPosition)
	}
	parts := make([]string, len(changes))
	for i, c := range changes {
		if c.ToPosition == "" {
			parts[i] = fmt.Sprintf("%s (no coverage)", c.SectorName)
		} else {
			parts[i] = fmt.Sprintf("%s (to %s)", c.SectorName, c.ToPosition)
		}
	}
	return fmt.Sprintf("%s went offline. Sectors: %s.", positionName, strings.Join(parts, ", "))
}
