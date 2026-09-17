package postgres

import (
	"context"
	"sync"
	"time"

	"FlightStrips/internal/shared"
)

type positionBatchKey struct{}
type batchResult struct {
	value any
	err   error
}
type batchCall struct {
	ctx     context.Context
	input   any
	result  chan batchResult
	execute func([]*batchCall)
}
type batchStage struct {
	seen   []bool
	calls  []*batchCall
	closed bool
	timer  *time.Timer
}
type positionBatch struct {
	mu      sync.Mutex
	done    []bool
	stages  map[string]*batchStage
	maxWait time.Duration
}
type positionBatchMember struct {
	batch *positionBatch
	index int
	mu    sync.Mutex
	used  map[string]bool
}

// NewPositionBatch creates an ephemeral rendezvous for distinct aircraft. It
// stores no operational data after the group completes. The timeout is a
// correctness backstop: a participant can be waiting behind a master-change
// writer, a transactional transition, or another lock instead of joining us.
func NewPositionBatch(size int) shared.PositionBatchScope {
	return &positionBatch{done: make([]bool, size), stages: make(map[string]*batchStage), maxWait: 5 * time.Millisecond}
}
func (b *positionBatch) Context(ctx context.Context, index int) context.Context {
	return context.WithValue(ctx, positionBatchKey{}, &positionBatchMember{batch: b, index: index, used: make(map[string]bool)})
}
func (b *positionBatch) flushLocked(stage *batchStage) func() {
	if stage.closed {
		return nil
	}
	stage.closed = true
	if stage.timer != nil {
		stage.timer.Stop()
	}
	calls := stage.calls
	stage.calls = nil
	if len(calls) == 0 {
		return nil
	}
	return func() {
		if err := shared.RunPositionBatch(calls[0].ctx, func() { calls[0].execute(calls) }); err != nil {
			for _, call := range calls {
				call.result <- batchResult{err: err}
			}
		}
	}
}
func (b *positionBatch) ready(stage *batchStage) bool {
	for i := range b.done {
		if !b.done[i] && !stage.seen[i] {
			return false
		}
	}
	return true
}
func (b *positionBatch) Done(index int) {
	b.mu.Lock()
	b.done[index] = true
	var flushes []func()
	for _, stage := range b.stages {
		if b.ready(stage) {
			if flush := b.flushLocked(stage); flush != nil {
				flushes = append(flushes, flush)
			}
		}
	}
	b.mu.Unlock()
	for _, flush := range flushes {
		flush()
	}
}

func joinPositionBatch(ctx context.Context, operation string, input any, execute func([]*batchCall)) (any, error, bool) {
	if shared.PositionBatchingDisabled(ctx) {
		return nil, nil, false
	}
	member, _ := ctx.Value(positionBatchKey{}).(*positionBatchMember)
	if member == nil {
		return nil, nil, false
	}
	member.mu.Lock()
	if member.used[operation] {
		member.mu.Unlock()
		return nil, nil, false
	}
	member.used[operation] = true // retries always obtain a fresh, individual snapshot
	member.mu.Unlock()
	resume := shared.SuspendPositionExecution(ctx)
	defer resume()
	b := member.batch
	b.mu.Lock()
	stage := b.stages[operation]
	if stage == nil {
		stage = &batchStage{seen: make([]bool, len(b.done))}
		b.stages[operation] = stage
		stage.timer = time.AfterFunc(b.maxWait, func() {
			b.mu.Lock()
			flush := b.flushLocked(stage)
			b.mu.Unlock()
			if flush != nil {
				flush()
			}
		})
	}
	if stage.closed {
		b.mu.Unlock()
		return nil, nil, false
	}
	call := &batchCall{ctx: ctx, input: input, result: make(chan batchResult, 1), execute: execute}
	stage.calls = append(stage.calls, call)
	stage.seen[member.index] = true
	var flush func()
	if b.ready(stage) {
		flush = b.flushLocked(stage)
	}
	b.mu.Unlock()
	if flush != nil {
		flush()
	}
	// Await the database result even after cancellation. Never report completion
	// while an accepted write is still executing in another goroutine.
	result := <-call.result
	return result.value, result.err, true
}
