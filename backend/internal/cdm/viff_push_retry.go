package cdm

import (
	"sync"
	"time"
)

var viffPushRetryDelays = [...]time.Duration{time.Second, time.Second, 3 * time.Second, 5 * time.Second, 10 * time.Second}

// A flight has at most one asynchronous export in progress. Each proposal gets
// one initial attempt and five retries; a changed proposal starts a fresh budget.
type viffPushRetryTracker struct {
	mu      sync.Mutex
	entries map[string]*viffPushRetryEntry
	removed map[int32]bool
}

type viffPushRetryEntry struct {
	session     int32
	inFlight    *viffPushAttempt
	confirmed   *viffPushState
	proposal    *viffPushState
	pending     *viffPushState
	failures    uint
	nextAttempt time.Time
}

type viffPushAttempt struct {
	key   string
	entry *viffPushRetryEntry
	state viffPushState
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
	if entry.proposal == nil || *entry.proposal != state {
		proposal := state
		entry.proposal = &proposal
		entry.failures = 0
		entry.nextAttempt = time.Time{}
	}
	if entry.failures > uint(len(viffPushRetryDelays)) || now.Before(entry.nextAttempt) {
		return nil
	}
	attempt := &viffPushAttempt{key: key, entry: entry, state: state}
	entry.inFlight = attempt
	return attempt
}

func (t *viffPushRetryTracker) complete(attempt *viffPushAttempt, success bool, now time.Time) *viffPushAttempt {
	t.mu.Lock()
	defer t.mu.Unlock()
	entry := t.entries[attempt.key]
	// Removal/recreation of a session must invalidate old goroutine completions.
	if entry != attempt.entry || entry.inFlight != attempt {
		return nil
	}
	entry.inFlight = nil
	if success {
		state := attempt.state
		entry.confirmed = &state
		entry.failures = 0
		entry.nextAttempt = time.Time{}
	} else {
		entry.failures++
		if entry.failures > uint(len(viffPushRetryDelays)) {
			entry.nextAttempt = time.Time{}
		} else {
			entry.nextAttempt = now.Add(viffPushRetryDelays[entry.failures-1])
		}
	}
	pending := entry.pending
	entry.pending = nil
	if pending == nil || *pending == attempt.state || (entry.confirmed != nil && *entry.confirmed == *pending) {
		return nil
	}
	// Reserve the latest proposal before releasing the lock, so another caller
	// cannot race the export that the completing worker will dispatch.
	entry.proposal = pending
	entry.failures = 0
	entry.nextAttempt = time.Time{}
	next := &viffPushAttempt{key: attempt.key, entry: entry, state: *pending}
	entry.inFlight = next
	return next
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
