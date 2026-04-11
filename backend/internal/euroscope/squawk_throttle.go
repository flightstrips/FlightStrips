package euroscope

import (
	euroscopeevents "FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/helpers"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

const defaultSquawkRequestInterval = 5 * time.Second

type queuedSquawkRequest struct {
	cid      string
	callsign string
}

type squawkThrottleState struct {
	lastSent time.Time
	timer    *time.Timer
	pending  []queuedSquawkRequest
}

type squawkThrottle struct {
	mu         sync.Mutex
	interval   time.Duration
	states     map[int32]*squawkThrottleState
	readStrip  func(ctx context.Context, session int32, callsign string) (string, bool, error)
	dispatchFn func(session int32, req queuedSquawkRequest)
}

func newSquawkThrottle(
	interval time.Duration,
	readStrip func(ctx context.Context, session int32, callsign string) (string, bool, error),
	dispatchFn func(session int32, req queuedSquawkRequest),
) *squawkThrottle {
	if interval <= 0 {
		interval = defaultSquawkRequestInterval
	}

	return &squawkThrottle{
		interval:   interval,
		states:     make(map[int32]*squawkThrottleState),
		readStrip:  readStrip,
		dispatchFn: dispatchFn,
	}
}

func (t *squawkThrottle) Enqueue(session int32, req queuedSquawkRequest) {
	now := time.Now()

	t.mu.Lock()
	state := t.getOrCreateState(session)
	if state.timer == nil && len(state.pending) == 0 && (state.lastSent.IsZero() || now.Sub(state.lastSent) >= t.interval) {
		state.lastSent = now
		t.mu.Unlock()
		t.dispatchFn(session, req)
		return
	}

	for i := range state.pending {
		if state.pending[i].callsign != req.callsign {
			continue
		}
		if req.cid != "" {
			state.pending[i].cid = req.cid
		}
		t.mu.Unlock()
		return
	}

	state.pending = append(state.pending, req)
	t.ensureFlushTimerLocked(session, state, now)
	t.mu.Unlock()
}

func (t *squawkThrottle) flush(session int32) {
	t.mu.Lock()
	state, ok := t.states[session]
	if !ok {
		t.mu.Unlock()
		return
	}

	state.timer = nil
	if len(state.pending) == 0 {
		t.mu.Unlock()
		return
	}

	req := state.pending[0]
	state.pending = state.pending[1:]
	state.lastSent = time.Now()
	if len(state.pending) > 0 {
		state.timer = time.AfterFunc(t.interval, func() {
			t.flush(session)
		})
	}
	t.mu.Unlock()

	if !t.shouldDispatch(context.Background(), session, req) {
		return
	}
	t.dispatchFn(session, req)
}

func (t *squawkThrottle) shouldDispatch(ctx context.Context, session int32, req queuedSquawkRequest) bool {
	if t.readStrip == nil {
		return true
	}

	currentAssignedSquawk, found, err := t.readStrip(ctx, session, req.callsign)
	if err != nil {
		slog.Warn("Failed to re-read strip before dispatching queued squawk request",
			slog.Int("session", int(session)),
			slog.String("callsign", req.callsign),
			slog.Any("error", err),
		)
		return true
	}
	if !found {
		slog.Debug("Skipping queued squawk request because strip no longer exists",
			slog.Int("session", int(session)),
			slog.String("callsign", req.callsign),
		)
		return false
	}
	if helpers.IsValidAssignedSquawk(currentAssignedSquawk) {
		slog.Debug("Skipping queued squawk request because assigned squawk is now valid",
			slog.Int("session", int(session)),
			slog.String("callsign", req.callsign),
			slog.String("current_assigned_squawk", currentAssignedSquawk),
		)
		return false
	}

	return true
}

func (t *squawkThrottle) getOrCreateState(session int32) *squawkThrottleState {
	state, ok := t.states[session]
	if !ok {
		state = &squawkThrottleState{}
		t.states[session] = state
	}
	return state
}

func (t *squawkThrottle) ensureFlushTimerLocked(session int32, state *squawkThrottleState, now time.Time) {
	if state.timer != nil {
		return
	}

	delay := t.interval
	if !state.lastSent.IsZero() {
		delay = t.interval - now.Sub(state.lastSent)
		if delay < 0 {
			delay = 0
		}
	}
	state.timer = time.AfterFunc(delay, func() {
		t.flush(session)
	})
}

func (hub *Hub) readAssignedSquawk(ctx context.Context, session int32, callsign string) (string, bool, error) {
	if hub.server == nil {
		return "", true, nil
	}

	stripRepo := hub.server.GetStripRepository()
	if stripRepo == nil {
		return "", true, nil
	}

	strip, err := stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}

	return normalizeAssignedSquawk(strip.AssignedSquawk), true, nil
}

func (hub *Hub) dispatchGenerateSquawkRequest(session int32, req queuedSquawkRequest) {
	event := euroscopeevents.GenerateSquawkEvent{
		Callsign: req.callsign,
	}
	isAutomatic := req.cid == ""
	cid := req.cid
	if cid == "" {
		cid = hub.resolveGenerateSquawkCid(context.Background(), session)
	}
	if cid == "" {
		slog.Warn("No EuroScope client available to generate squawk",
			slog.Int("session", int(session)),
			slog.String("callsign", req.callsign),
		)
		return
	}
	if isAutomatic {
		slog.Info("Auto-generating squawk",
			slog.Int("session", int(session)),
			slog.String("callsign", req.callsign),
			slog.String("cid", cid),
		)
	}
	hub.Send(session, cid, event)
}

func normalizeAssignedSquawk(assignedSquawk *string) string {
	if assignedSquawk == nil {
		return ""
	}
	return strings.TrimSpace(*assignedSquawk)
}
