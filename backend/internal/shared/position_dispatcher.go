package shared

import (
	"context"
	"sync"
)

type positionJob struct {
	key string
	run func(context.Context)
}

// PositionDispatcher bounds work, preserves FIFO per aircraft and lets ready
// aircraft overlap database waits. Jobs must honor cancellation and not panic.
type PositionDispatcher struct {
	mu      sync.Mutex
	changed *sync.Cond
	queue   []positionJob
	active  map[string]bool
	running int
	limit   int
	closing bool
	paused  bool
	ctx     context.Context
	cancel  context.CancelFunc
	workers sync.WaitGroup
	budget  chan struct{}
}

func NewPositionDispatcher(workers, pending int, budget chan struct{}) *PositionDispatcher {
	if workers < 1 || pending < 1 {
		panic("invalid position dispatcher capacity")
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &PositionDispatcher{active: make(map[string]bool), limit: pending, ctx: ctx, cancel: cancel, budget: budget}
	d.changed = sync.NewCond(&d.mu)
	for i := 0; i < workers; i++ {
		d.workers.Add(1)
		go d.worker()
	}
	return d
}

func (d *PositionDispatcher) Submit(ctx context.Context, key string, run func(context.Context)) error {
	stop := context.AfterFunc(ctx, func() { d.mu.Lock(); d.changed.Broadcast(); d.mu.Unlock() })
	defer stop()
	d.mu.Lock()
	defer d.mu.Unlock()
	for len(d.queue) >= d.limit && !d.closing && ctx.Err() == nil {
		d.changed.Wait()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if d.closing {
		return context.Canceled
	}
	d.queue = append(d.queue, positionJob{key: key, run: run})
	d.changed.Broadcast()
	return nil
}

func (d *PositionDispatcher) Barrier(ctx context.Context) error {
	stop := context.AfterFunc(ctx, func() { d.mu.Lock(); d.changed.Broadcast(); d.mu.Unlock() })
	defer stop()
	d.mu.Lock()
	defer d.mu.Unlock()
	for (len(d.queue) != 0 || d.running != 0) && ctx.Err() == nil {
		d.changed.Wait()
	}
	return ctx.Err()
}

func (d *PositionDispatcher) Depth() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.queue)
}

// Close drains accepted jobs until the deadline. Remaining jobs still invoke
// their completion callback with cancellation, so none disappear from metrics.
func (d *PositionDispatcher) Close(ctx context.Context) error {
	d.mu.Lock()
	d.closing = true
	d.changed.Broadcast()
	d.mu.Unlock()
	err := d.Barrier(ctx)
	d.cancel()
	d.mu.Lock()
	d.changed.Broadcast()
	d.mu.Unlock()
	d.workers.Wait()
	return err
}

func (d *PositionDispatcher) worker() {
	defer d.workers.Done()
	for {
		d.mu.Lock()
		index := -1
		for index < 0 {
			for i, job := range d.queue {
				if !d.paused && !d.active[job.key] {
					index = i
					break
				}
			}
			if index >= 0 {
				break
			}
			if d.closing && len(d.queue) == 0 {
				d.mu.Unlock()
				return
			}
			d.changed.Wait()
		}
		job := d.queue[index]
		copy(d.queue[index:], d.queue[index+1:])
		d.queue[len(d.queue)-1] = positionJob{}
		d.queue = d.queue[:len(d.queue)-1]
		d.active[job.key] = true
		d.running++
		d.changed.Broadcast()
		d.mu.Unlock()
		acquired := false
		if d.budget != nil {
			select {
			case d.budget <- struct{}{}:
				acquired = true
			case <-d.ctx.Done():
			}
		}
		job.run(d.ctx)
		if acquired {
			<-d.budget
		}
		d.mu.Lock()
		delete(d.active, job.key)
		d.running--
		d.changed.Broadcast()
		d.mu.Unlock()
	}
}

// Cancel fences accepted work and stops idle workers without waiting for the
// caller (which may itself be a worker). Close joins workers during shutdown.
func (d *PositionDispatcher) Cancel() {
	d.mu.Lock()
	d.closing = true
	d.cancel()
	d.changed.Broadcast()
	d.mu.Unlock()
}

// RunBarrier also pauses timer-submitted jobs while the operational handler is
// executing. Only the socket reader calls this method.
func (d *PositionDispatcher) RunBarrier(ctx context.Context, run func()) error {
	stop := context.AfterFunc(ctx, func() { d.mu.Lock(); d.changed.Broadcast(); d.mu.Unlock() })
	defer stop()
	d.mu.Lock()
	for (len(d.queue) != 0 || d.running != 0) && ctx.Err() == nil {
		d.changed.Wait()
	}
	if err := ctx.Err(); err != nil {
		d.mu.Unlock()
		return err
	}
	d.paused = true
	d.mu.Unlock()
	defer func() { d.mu.Lock(); d.paused = false; d.changed.Broadcast(); d.mu.Unlock() }()
	run()
	return nil
}
