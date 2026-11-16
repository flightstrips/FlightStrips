package shared

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

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

// TracePositionWait records a lock/gate wait without changing the parent
// context. End it before resuming execution so the two waits stay distinct.
func TracePositionWait(ctx context.Context, reason string) func() {
	if ctx.Value(positionExecutionKey{}) == nil {
		return func() {}
	}
	_, span := otel.Tracer("position").Start(ctx, "position.wait.lock",
		trace.WithAttributes(attribute.String("position.wait.reason", reason)))
	return func() { span.End() }
}

func (p *positionExecution) acquire(ctx context.Context) {
	p.acquireFor(ctx, "dispatch")
}

func (p *positionExecution) acquireFor(ctx context.Context, reason string) {
	// The initial dispatch has no message span yet. Resumptions do: measure the
	// admission wait separately from the batch/lock for which we yielded.
	started := time.Now()
	localDone := started
	if trace.SpanFromContext(ctx).IsRecording() {
		_, span := otel.Tracer("position").Start(ctx, "position.wait.execution",
			trace.WithAttributes(attribute.String("position.wait.reason", reason)))
		defer func() {
			span.SetAttributes(
				attribute.Float64("position.wait.local_ms", float64(localDone.Sub(started))/float64(time.Millisecond)),
				attribute.Float64("position.wait.global_ms", float64(time.Since(localDone))/float64(time.Millisecond)),
				attribute.Bool("position.execution.acquired", p.held))
			span.End()
		}()
	}
	select {
	case p.local <- struct{}{}:
	case <-ctx.Done():
		localDone = time.Now()
		return
	}
	localDone = time.Now()
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
	return SuspendPositionExecutionFor(ctx, "outer_lock")
}

// SuspendPositionExecutionFor attributes readmission to a bounded stage name.
// It does not change the message's wall-clock processing or completion timers.
func SuspendPositionExecutionFor(ctx context.Context, reason string) func() {
	p, _ := ctx.Value(positionExecutionKey{}).(*positionExecution)
	if p == nil || !p.held {
		return func() {}
	}
	p.release()
	return func() { p.acquireFor(ctx, reason) }
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
	// Keep root cancellation semantics and the initiating report's trace.
	acquireCtx := trace.ContextWithSpan(p.root, trace.SpanFromContext(ctx))
	lease.acquireFor(acquireCtx, "batch.execute")
	if !lease.held {
		return p.root.Err()
	}
	defer lease.release()
	run()
	return nil
}
