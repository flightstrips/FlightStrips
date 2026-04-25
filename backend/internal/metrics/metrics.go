package metrics

import (
	"context"
	"strings"
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
	activeConnections       metric.Int64UpDownCounter
	activeClients           metric.Int64UpDownCounter
	messagesReceived        metric.Int64Counter
	messagesSent            metric.Int64Counter
	messageHandledDuration  metric.Float64Histogram
	pdcRequestsReceived     metric.Int64Counter
	pdcRequestOutcomes      metric.Int64Counter
	pdcStateChanges         metric.Int64Counter
	trafficOnStand          metric.Int64Gauge
	trafficTaxiing          metric.Int64Gauge
	trafficArrivalRate15m   metric.Int64Gauge
	trafficDepartureRate15m metric.Int64Gauge
}

func get() *instruments {
	once.Do(func() {
		meter := otel.GetMeterProvider().Meter("flightstrips")

		activeConnections, _ := meter.Int64UpDownCounter(
			"websocket.connections.active",
			metric.WithDescription("Active WebSocket connections"),
			metric.WithUnit("{connection}"),
		)
		activeClients, _ := meter.Int64UpDownCounter(
			"websocket.clients.active",
			metric.WithDescription("Active session-bound client connections by callsign"),
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
		pdcRequestsReceived, _ := meter.Int64Counter(
			"pdc.requests.received",
			metric.WithDescription("PDC requests received"),
			metric.WithUnit("{request}"),
		)
		pdcRequestOutcomes, _ := meter.Int64Counter(
			"pdc.requests.outcomes",
			metric.WithDescription("PDC request processing outcomes"),
			metric.WithUnit("{request}"),
		)
		pdcStateChanges, _ := meter.Int64Counter(
			"pdc.state_changes",
			metric.WithDescription("PDC clearance state transitions"),
			metric.WithUnit("{transition}"),
		)
		trafficOnStand, _ := meter.Int64Gauge(
			"traffic.aircraft.on_stand",
			metric.WithDescription("Aircraft currently on stand or at gate (NOT_CLEARED, CLEARED, STAND)"),
			metric.WithUnit("{aircraft}"),
		)
		trafficTaxiing, _ := meter.Int64Gauge(
			"traffic.aircraft.taxiing",
			metric.WithDescription("Aircraft currently taxiing (PUSH, TAXI, TAXI_LWR, TAXI_TWR)"),
			metric.WithUnit("{aircraft}"),
		)
		trafficArrivalRate15m, _ := meter.Int64Gauge(
			"traffic.arrivals.rate_15m",
			metric.WithDescription("Arrivals (ALDT set) in the rolling last 15 minutes"),
			metric.WithUnit("{aircraft}"),
		)
		trafficDepartureRate15m, _ := meter.Int64Gauge(
			"traffic.departures.rate_15m",
			metric.WithDescription("Departures (AOBT set) in the rolling last 15 minutes"),
			metric.WithUnit("{aircraft}"),
		)

		inst = &instruments{
			activeConnections:       activeConnections,
			activeClients:           activeClients,
			messagesReceived:        messagesReceived,
			messagesSent:            messagesSent,
			messageHandledDuration:  messageHandledDuration,
			pdcRequestsReceived:     pdcRequestsReceived,
			pdcRequestOutcomes:      pdcRequestOutcomes,
			pdcStateChanges:         pdcStateChanges,
			trafficOnStand:          trafficOnStand,
			trafficTaxiing:          trafficTaxiing,
			trafficArrivalRate15m:   trafficArrivalRate15m,
			trafficDepartureRate15m: trafficDepartureRate15m,
		}
	})
	return inst
}

func sessionAttributes(sessionName, airport string, extra ...attribute.KeyValue) metric.MeasurementOption {
	attrs := []attribute.KeyValue{
		attribute.String("session_name", normalizeSessionName(sessionName)),
		attribute.String("airport", normalizeAirport(airport)),
	}
	attrs = append(attrs, extra...)
	return metric.WithAttributes(attrs...)
}

func normalizeSessionName(sessionName string) string {
	sessionName = strings.TrimSpace(sessionName)
	if sessionName == "" {
		return "UNASSIGNED"
	}
	return strings.ToUpper(sessionName)
}

func normalizeAirport(airport string) string {
	airport = strings.TrimSpace(airport)
	if airport == "" {
		return "UNKNOWN"
	}
	return strings.ToUpper(airport)
}

func normalizeChannel(channel string) string {
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return "UNKNOWN"
	}
	return strings.ToUpper(channel)
}

func normalizeCallsign(callsign string) string {
	return strings.ToUpper(strings.TrimSpace(callsign))
}

func ConnectionOpened(ctx context.Context, sessionName, airport, source, callsign string) {
	get().activeConnections.Add(ctx, 1,
		sessionAttributes(sessionName, airport,
			attribute.String("source", source),
		),
	)

	callsign = normalizeCallsign(callsign)
	if callsign == "" {
		return
	}

	get().activeClients.Add(ctx, 1,
		sessionAttributes(sessionName, airport,
			attribute.String("source", source),
			attribute.String("callsign", callsign),
		),
	)
}

func ConnectionClosed(ctx context.Context, sessionName, airport, source, callsign string) {
	get().activeConnections.Add(ctx, -1,
		sessionAttributes(sessionName, airport,
			attribute.String("source", source),
		),
	)

	callsign = normalizeCallsign(callsign)
	if callsign == "" {
		return
	}

	get().activeClients.Add(ctx, -1,
		sessionAttributes(sessionName, airport,
			attribute.String("source", source),
			attribute.String("callsign", callsign),
		),
	)
}

func MessageReceived(ctx context.Context, sessionName, airport, source, msgType string) {
	get().messagesReceived.Add(ctx, 1,
		sessionAttributes(sessionName, airport,
			attribute.String("source", source),
			attribute.String("type", msgType),
		),
	)
}

func MessageHandled(ctx context.Context, sessionName, airport, source, msgType string, duration time.Duration, success bool) {
	status := "ok"
	if !success {
		status = "error"
	}
	get().messageHandledDuration.Record(ctx, duration.Seconds(),
		sessionAttributes(sessionName, airport,
			attribute.String("source", source),
			attribute.String("type", msgType),
			attribute.String("status", status),
		),
	)
}

func MessageSent(ctx context.Context, sessionName, airport, source, msgType string) {
	get().messagesSent.Add(ctx, 1,
		sessionAttributes(sessionName, airport,
			attribute.String("source", source),
			attribute.String("type", msgType),
		),
	)
}

func PDCRequestReceived(ctx context.Context, sessionName, airport, channel string) {
	get().pdcRequestsReceived.Add(ctx, 1,
		sessionAttributes(sessionName, airport,
			attribute.String("channel", normalizeChannel(channel)),
		),
	)
}

func PDCRequestOutcome(ctx context.Context, sessionName, airport, channel, outcome string) {
	get().pdcRequestOutcomes.Add(ctx, 1,
		sessionAttributes(sessionName, airport,
			attribute.String("channel", normalizeChannel(channel)),
			attribute.String("outcome", outcome),
		),
	)
}

func PDCStateChange(ctx context.Context, sessionName, airport, state string) {
	get().pdcStateChanges.Add(ctx, 1,
		sessionAttributes(sessionName, airport,
			attribute.String("state", state),
		),
	)
}

func RecordTrafficSnapshot(ctx context.Context, sessionName string, airport string, onStand, taxiing, arr15m, dep15m int64) {
	attrs := sessionAttributes(sessionName, airport)
	i := get()
	i.trafficOnStand.Record(ctx, onStand, attrs)
	i.trafficTaxiing.Record(ctx, taxiing, attrs)
	i.trafficArrivalRate15m.Record(ctx, arr15m, attrs)
	i.trafficDepartureRate15m.Record(ctx, dep15m, attrs)
}
