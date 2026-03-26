package cdm

import (
	"sync"
	"time"
)

type recalcDebouncer struct {
	mu       sync.Mutex
	pending  map[string]*recalcDebounceState
	interval time.Duration
}

type recalcDebounceState struct {
	lastScheduled time.Time
	scheduled     bool
	running       bool
	rerun         bool
}

func newRecalcDebouncer(interval time.Duration) *recalcDebouncer {
	return &recalcDebouncer{
		pending:  make(map[string]*recalcDebounceState),
		interval: interval,
	}
}

func (d *recalcDebouncer) Schedule(key string, run func()) {
	if d == nil || key == "" || run == nil {
		return
	}

	now := time.Now()

	d.mu.Lock()
	state, ok := d.pending[key]
	if !ok {
		state = &recalcDebounceState{}
		d.pending[key] = state
	}
	state.lastScheduled = now

	if state.running {
		state.rerun = true
		d.mu.Unlock()
		return
	}

	if state.scheduled {
		d.mu.Unlock()
		return
	}

	state.scheduled = true
	d.mu.Unlock()

	go d.runLoop(key, run)
}

func (d *recalcDebouncer) runLoop(key string, run func()) {
	for {
		wait := d.waitDuration(key)
		if wait > 0 {
			time.Sleep(wait)
		}

		d.mu.Lock()
		state, ok := d.pending[key]
		if !ok {
			d.mu.Unlock()
			return
		}

		if until := state.lastScheduled.Add(d.interval); time.Now().Before(until) {
			d.mu.Unlock()
			continue
		}

		state.scheduled = false
		state.running = true
		state.rerun = false
		d.mu.Unlock()

		run()

		d.mu.Lock()
		state, ok = d.pending[key]
		if !ok {
			d.mu.Unlock()
			return
		}

		state.running = false
		if state.rerun {
			state.scheduled = true
			d.mu.Unlock()
			continue
		}

		delete(d.pending, key)
		d.mu.Unlock()
		return
	}
}

func (d *recalcDebouncer) waitDuration(key string) time.Duration {
	d.mu.Lock()
	defer d.mu.Unlock()

	state, ok := d.pending[key]
	if !ok {
		return 0
	}

	wait := time.Until(state.lastScheduled.Add(d.interval))
	if wait < 0 {
		return 0
	}
	return wait
}
