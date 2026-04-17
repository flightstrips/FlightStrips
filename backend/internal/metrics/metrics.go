package metrics

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	once sync.Once
	inst *instruments
)

type instruments struct {
	activeConnections   metric.Int64UpDownCounter
	messagesReceived    metric.Int64Counter
	messagesSent        metric.Int64Counter
	messageHandledDuration metric.Float64Histogram
	pdcRequests         metric.Int64Counter
	pdcStateChanges     metric.Int64Counter
}

func get() *instruments {
	once.Do(func() {
		meter := otel.GetMeterProvider().Meter("flightstrips")

		activeConnections, _ := meter.Int64UpDownCounter(
			"websocket.connections.active",
			metric.WithDescription("Active WebSocket connections"),
			metric.WithUnit("{connection}"),
		)
		messagesReceived, _ := meter.Int64Counter(
			"websocket.messages.received",
			metric.WithDescription("WebSocket messages received"),
			metric.WithUnit("{message}"),
		)
		messagesSent, _ := meter.Int64Counter(
			"websocket.messages.sent",
			metric.WithDescription("WebSocket messages sent"),
			metric.WithUnit("{message}"),
		)
		messageHandledDuration, _ := meter.Float64Histogram(
			"websocket.message.duration",
			metric.WithDescription("WebSocket message handler processing duration"),
			metric.WithUnit("s"),
			metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0),
		)
		pdcRequests, _ := meter.Int64Counter(
			"pdc.requests",
			metric.WithDescription("PDC requests received"),
			metric.WithUnit("{request}"),
		)
		pdcStateChanges, _ := meter.Int64Counter(
			"pdc.state_changes",
			metric.WithDescription("PDC clearance state transitions"),
			metric.WithUnit("{transition}"),
		)

		inst = &instruments{
			activeConnections:      activeConnections,
			messagesReceived:       messagesReceived,
			messagesSent:           messagesSent,
			messageHandledDuration: messageHandledDuration,
			pdcRequests:            pdcRequests,
			pdcStateChanges:        pdcStateChanges,
		}
	})
	return inst
}

func ConnectionOpened(ctx context.Context, session int32, source string) {
	get().activeConnections.Add(ctx, 1,
		metric.WithAttributes(
			attribute.Int("session", int(session)),
			attribute.String("source", source),
		),
	)
}

func ConnectionClosed(ctx context.Context, session int32, source string) {
	get().activeConnections.Add(ctx, -1,
		metric.WithAttributes(
			attribute.Int("session", int(session)),
			attribute.String("source", source),
		),
	)
}

func MessageReceived(ctx context.Context, session int32, source string, msgType string) {
	get().messagesReceived.Add(ctx, 1,
		metric.WithAttributes(
			attribute.Int("session", int(session)),
			attribute.String("source", source),
			attribute.String("type", msgType),
		),
	)
}

func MessageHandled(ctx context.Context, session int32, source string, msgType string, duration time.Duration, success bool) {
	status := "ok"
	if !success {
		status = "error"
	}
	get().messageHandledDuration.Record(ctx, duration.Seconds(),
		metric.WithAttributes(
			attribute.Int("session", int(session)),
			attribute.String("source", source),
			attribute.String("type", msgType),
			attribute.String("status", status),
		),
	)
}

func MessageSent(ctx context.Context, session int32, msgType string) {
	get().messagesSent.Add(ctx, 1,
		metric.WithAttributes(
			attribute.Int("session", int(session)),
			attribute.String("type", msgType),
		),
	)
}

func PDCRequest(ctx context.Context, session int32, result string) {
	get().pdcRequests.Add(ctx, 1,
		metric.WithAttributes(
			attribute.Int("session", int(session)),
			attribute.String("result", result),
		),
	)
}

func PDCStateChange(ctx context.Context, session int32, state string) {
	get().pdcStateChanges.Add(ctx, 1,
		metric.WithAttributes(
			attribute.Int("session", int(session)),
			attribute.String("state", state),
		),
	)
}
