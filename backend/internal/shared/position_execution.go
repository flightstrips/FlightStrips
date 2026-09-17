package shared

import "context"

// Position execution slots bound active work, independently of batch membership.
// A report waiting for its batch contributes no database or lifecycle work.
type positionExecutionKey struct{}
type positionBatchDisabledKey struct{}

// WithoutPositionBatching prevents a retry from joining a rendezvous while
// holding an operational transition lock. Its peers may need that same lock.
func WithoutPositionBatching(ctx context.Context) context.Context {
	return context.WithValue(ctx, positionBatchDisabledKey{}, true)
}

func PositionBatchingDisabled(ctx context.Context) bool {
	disabled, _ := ctx.Value(positionBatchDisabledKey{}).(bool)
	return disabled
}

type positionExecution struct {
	local, global chan struct{}
	root          context.Context
	held          bool
}

func (p *positionExecution) acquire(ctx context.Context) {
	select {
	case p.local <- struct{}{}:
	case <-ctx.Done():
		return
	}
	if p.global != nil {
		select {
		case p.global <- struct{}{}:
		case <-ctx.Done():
			<-p.local
			return
		}
	}
	p.held = true
}
func (p *positionExecution) release() {
	if !p.held {
		return
	}
	p.held = false
	if p.global != nil {
		<-p.global
	}
	<-p.local
}

// SuspendPositionExecution releases the current report's execution slot while
// waiting for a batch or an outer authority/aircraft lock. Resume before doing
// database or lifecycle work. Cancellation still runs completion callbacks.
func SuspendPositionExecution(ctx context.Context) func() {
	p, _ := ctx.Value(positionExecutionKey{}).(*positionExecution)
	if p == nil || !p.held {
		return func() {}
	}
	p.release()
	return func() { p.acquire(ctx) }
}

// RunPositionBatch executes one physical batch inside the same pool budget as
// individual work, without borrowing a suspended participant's mutable lease.
func RunPositionBatch(ctx context.Context, run func()) error {
	p, _ := ctx.Value(positionExecutionKey{}).(*positionExecution)
	if p == nil {
		run()
		return nil
	}
	lease := &positionExecution{local: p.local, global: p.global}
	// Every member belongs to this dispatcher's root cancellation context. A
	// single report's deadline must not cancel acquisition for healthy members,
	// but closing the whole dispatcher must unblock a batch waiting for a slot.
	lease.acquire(p.root)
	if !lease.held {
		return p.root.Err()
	}
	defer lease.release()
	run()
	return nil
}
