package cdm

import (
	"errors"
	"log/slog"
	"sync"
	"time"
)

var viffPushRetryDelays = [...]time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second}

const viffPushRetryTimeout = 5 * time.Minute

// One worker owns a flight's export through requests and retry waits. Retries
// continue for up to five minutes, with the latest proposal replacing queued data.
type viffPushRetryTracker struct {
	mu      sync.Mutex
	entries map[string]*viffPushRetryEntry
	removed map[int32]bool
}

type viffPushRetryEntry struct {
	session         int32
	inFlight        *viffPushAttempt
	confirmed       *viffPushState
	exhausted       *viffPushState
	pending         *viffPushState
	failures        uint
	failureSince    time.Time
	refreshRequired bool
}

type viffPushAttempt struct {
	key             string
	entry           *viffPushRetryEntry
	state           viffPushState
	startedAt       time.Time
	notBefore       time.Time
	refreshRequired bool
}

type viffPushResult struct {
	next       *viffPushAttempt
	level      slog.Level
	failingFor time.Duration
	failures   uint
}

func (t *viffPushRetryTracker) begin(session int32, callsign string, state viffPushState, now time.Time) *viffPushAttempt {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.removed[session] {
		return nil
	}
	if t.entries == nil {
		t.entries = make(map[string]*viffPushRetryEntry)
	}
	key := viffPushKey(session, callsign)
	entry := t.entries[key]
	if entry == nil {
		entry = &viffPushRetryEntry{session: session}
		t.entries[key] = entry
	}
	if entry.inFlight != nil {
		pending := state
		entry.pending = &pending
		return nil
	}
	if entry.confirmed != nil && *entry.confirmed == state {
		return nil
	}
	if entry.exhausted != nil {
		if *entry.exhausted == state {
			return nil
		}
		// As before, genuinely changed data may start a fresh retry window.
		entry.exhausted = nil
		entry.failures = 0
		entry.failureSince = time.Time{}
	}
	attempt := &viffPushAttempt{key: key, entry: entry, state: state, startedAt: now}
	entry.inFlight = attempt
	return attempt
}

// prepare selects the newest proposal after a retry wait, before sending it.
func (t *viffPushRetryTracker) prepare(attempt *viffPushAttempt, now time.Time) *viffPushAttempt {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry := t.entries[attempt.key]
	if entry != attempt.entry || entry.inFlight != attempt {
		return nil
	}
	next := *attempt
	next.startedAt = now
	next.refreshRequired = attempt.refreshRequired || entry.refreshRequired
	entry.refreshRequired = false
	if entry.pending != nil {
		next.state = *entry.pending
		entry.pending = nil
	}
	entry.inFlight = &next
	return &next
}

// A TOBT-only acknowledgement does not acknowledge the pending TSAT/TTOT.
// Keep that export queued, but make even an unsent initial attempt reload it.
func (t *viffPushRetryTracker) requireRefresh(session int32, callsign string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if entry := t.entries[viffPushKey(session, callsign)]; entry != nil && entry.inFlight != nil {
		entry.refreshRequired = true
	}
}

// refresh records the actual persisted payload about to be sent, without
// resetting the failure deadline or dropping proposals queued during the read.
func (t *viffPushRetryTracker) refresh(attempt *viffPushAttempt, state viffPushState) *viffPushAttempt {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry := t.entries[attempt.key]
	if entry != attempt.entry || entry.inFlight != attempt {
		return nil
	}
	next := *attempt
	next.state = state
	entry.inFlight = &next
	return &next
}

func (t *viffPushRetryTracker) complete(attempt *viffPushAttempt, err error, now time.Time) viffPushResult {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry := t.entries[attempt.key]
	// Removal/recreation invalidates old worker completions.
	if entry != attempt.entry || entry.inFlight != attempt {
		return viffPushResult{}
	}
	entry.inFlight = nil
	pending := entry.pending
	entry.pending = nil
	result := viffPushResult{level: slog.LevelDebug}
	state := attempt.state
	delay := time.Duration(0)
	if err == nil {
		confirmed := state
		entry.confirmed = &confirmed
		entry.failures = 0
		entry.failureSince = time.Time{}
		entry.exhausted = nil
		if pending == nil || *pending == state {
			return result
		}
		state = *pending
	} else {
		if entry.failureSince.IsZero() {
			entry.failureSince = attempt.startedAt
		}
		entry.failures++
		result.failures = entry.failures
		result.failingFor = now.Sub(entry.failureSince)
		// Feed lag is expected until the retry window expires. Other failures
		// remain warnings, and all errors stop retrying at the same deadline.
		if !errors.Is(err, errViffUpdatePending) || result.failingFor >= viffPushRetryTimeout {
			result.level = slog.LevelWarn
		}
		if result.failingFor >= viffPushRetryTimeout {
			if pending != nil {
				state = *pending
			}
			entry.exhausted = &state
			return result
		}
		index := min(entry.failures-1, uint(len(viffPushRetryDelays)-1))
		delay = viffPushRetryDelays[index]
		if pending != nil && *pending != state {
			state = *pending
			delay = 0
		}
		delay = min(delay, viffPushRetryTimeout-result.failingFor)
	}
	// Reserve the next request while waiting, preventing a second worker from
	// racing this one. Proposal changes do not reset an ongoing failure period.
	next := &viffPushAttempt{
		key: attempt.key, entry: entry, state: state,
		notBefore: now.Add(delay), refreshRequired: true,
	}
	entry.inFlight = next
	result.next = next
	return result
}

// confirmExport supersedes outstanding asynchronous work only after an
// authoritative export succeeds. Replacing the entry invalidates stale workers.
func (t *viffPushRetryTracker) confirmExport(session int32, callsign string, state *viffPushState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.removed[session] {
		return
	}
	if t.entries == nil {
		t.entries = make(map[string]*viffPushRetryEntry)
	}
	entry := &viffPushRetryEntry{session: session}
	if state != nil {
		confirmed := *state
		entry.confirmed = &confirmed
	}
	t.entries[viffPushKey(session, callsign)] = entry
}

// discard releases an unfinished worker without retaining an obsolete proposal.
func (t *viffPushRetryTracker) discard(attempt *viffPushAttempt) {
	if attempt == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if entry := t.entries[attempt.key]; entry == attempt.entry && entry.inFlight == attempt {
		delete(t.entries, attempt.key)
	}
}

func (t *viffPushRetryTracker) removeSession(session int32) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.removed == nil {
		t.removed = make(map[int32]bool)
	}
	t.removed[session] = true
	for key, entry := range t.entries {
		if entry.session == session {
			delete(t.entries, key)
		}
	}
}
