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
	activeMasterClients     metric.Int64UpDownCounter
	messagesReceived        metric.Int64Counter
	messagesSent            metric.Int64Counter
	messageHandledDuration  metric.Float64Histogram
	messageDBOperations     metric.Int64Counter
	syncInputStrips         metric.Int64Counter
	syncInputControllers    metric.Int64Counter
	syncChangedStrips       metric.Int64Counter
	syncChangedControllers  metric.Int64Counter
	syncDBOperations        metric.Int64Counter
	syncDuration            metric.Float64Histogram
	pdcRequestsReceived     metric.Int64Counter
	pdcRequestOutcomes      metric.Int64Counter
	pdcStateChanges         metric.Int64Counter
	trafficOnStand          metric.Int64Gauge
	trafficTaxiing          metric.Int64Gauge
	trafficArrivalRate15m   metric.Int64Gauge
	trafficDepartureRate15m metric.Int64Gauge
	satSnapshotAge          metric.Float64Gauge
	satFeedRecords          metric.Int64Gauge
	satAssignments          metric.Int64Counter
	satOutcomes             metric.Int64Counter
	satConflicts            metric.Int64Counter
	satExpirations          metric.Int64Counter
	amanObservationAge      metric.Float64Histogram
	amanGeometryCache       metric.Int64Counter
	amanRouteMaterialized   metric.Int64Counter
	amanRouteDuration       metric.Float64Histogram
	amanPredictorDuration   metric.Float64Histogram
	amanPredictionDrift     metric.Float64Histogram
	amanSequenceRevisions   metric.Int64Counter
	amanSequenceConflicts   metric.Int64Counter
	amanCommandOutcomes     metric.Int64Counter
	amanFlightDegradation   metric.Int64Counter
	amanSourceRefreshes     metric.Int64Counter
	amanPublicationFailures metric.Int64Counter
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
		activeMasterClients, _ := meter.Int64UpDownCounter(
			"euroscope.master_client.active",
			metric.WithDescription("Current master EuroScope client for a session by callsign"),
			metric.WithUnit("{client}"),
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
		messageDBOperations, _ := meter.Int64Counter(
			"websocket.message.db_operations",
			metric.WithDescription("Database operations performed while handling tracked WebSocket messages"),
			metric.WithUnit("{operation}"),
		)
		syncInputStrips, _ := meter.Int64Counter(
			"euroscope.sync.input_strips",
			metric.WithDescription("EuroScope sync strips received"),
			metric.WithUnit("{strip}"),
		)
		syncInputControllers, _ := meter.Int64Counter(
			"euroscope.sync.input_controllers",
			metric.WithDescription("EuroScope sync controllers received"),
			metric.WithUnit("{controller}"),
		)
		syncChangedStrips, _ := meter.Int64Counter(
			"euroscope.sync.changed_strips",
			metric.WithDescription("EuroScope sync strips that changed persisted state"),
			metric.WithUnit("{strip}"),
		)
		syncChangedControllers, _ := meter.Int64Counter(
			"euroscope.sync.changed_controllers",
			metric.WithDescription("EuroScope sync controllers that changed persisted state"),
			metric.WithUnit("{controller}"),
		)
		syncDBOperations, _ := meter.Int64Counter(
			"euroscope.sync.db_operations",
			metric.WithDescription("Database operations performed while handling EuroScope sync"),
			metric.WithUnit("{operation}"),
		)
		syncDuration, _ := meter.Float64Histogram(
			"euroscope.sync.duration",
			metric.WithDescription("EuroScope sync processing duration"),
			metric.WithUnit("s"),
			metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.0, 5.0, 10.0),
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
		satSnapshotAge, _ := meter.Float64Gauge("sat.vatsim.snapshot.age", metric.WithDescription("Age of the VATSIM snapshot used by SAT"), metric.WithUnit("s"))
		satFeedRecords, _ := meter.Int64Gauge("sat.vatsim.records", metric.WithDescription("Relevant VATSIM records observed by SAT"), metric.WithUnit("{flight}"))
		satAssignments, _ := meter.Int64Counter("sat.assignments", metric.WithDescription("Committed SAT assignments and reallocations"), metric.WithUnit("{assignment}"))
		satOutcomes, _ := meter.Int64Counter("sat.allocation.outcomes", metric.WithDescription("SAT allocation outcomes"), metric.WithUnit("{result}"))
		satConflicts, _ := meter.Int64Counter("sat.allocation.conflicts", metric.WithDescription("SAT allocation database and occupancy conflicts"), metric.WithUnit("{conflict}"))
		satExpirations, _ := meter.Int64Counter("sat.assignments.expired", metric.WithDescription("SAT assignments expired or released"), metric.WithUnit("{assignment}"))
		amanObservationAge, _ := meter.Float64Histogram("aman.observation.age", metric.WithDescription("Age of AMAN source observations"), metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(1, 5, 15, 30, 60, 120, 300))
		amanGeometryCache, _ := meter.Int64Counter("aman.geometry.cache", metric.WithDescription("AMAN geometry cache lookups"), metric.WithUnit("{lookup}"))
		amanRouteMaterialized, _ := meter.Int64Counter("aman.route.materialization", metric.WithDescription("Explicit AMAN route materialization attempts"), metric.WithUnit("{route}"))
		amanRouteDuration, _ := meter.Float64Histogram("aman.route.materialization.duration", metric.WithDescription("AMAN route materialization duration"), metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.01, .05, .1, .25, .5, 1, 2, 5))
		amanPredictorDuration, _ := meter.Float64Histogram("aman.predictor.duration", metric.WithDescription("AMAN predictor duration"), metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.001, .005, .01, .05, .1, .5, 1))
		amanPredictionDrift, _ := meter.Float64Histogram("aman.prediction.drift", metric.WithDescription("AMAN raw, operational, and Superstable prediction drift"), metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(1, 5, 15, 30, 60, 120, 300))
		amanSequenceRevisions, _ := meter.Int64Counter("aman.sequence.revisions", metric.WithDescription("Committed AMAN sequence revisions"), metric.WithUnit("{revision}"))
		amanSequenceConflicts, _ := meter.Int64Counter("aman.sequence.conflicts", metric.WithDescription("AMAN sequence revision conflicts"), metric.WithUnit("{conflict}"))
		amanCommandOutcomes, _ := meter.Int64Counter("aman.commands", metric.WithDescription("AMAN command outcomes"), metric.WithUnit("{command}"))
		amanFlightDegradation, _ := meter.Int64Counter("aman.flights.degraded", metric.WithDescription("AMAN stale and disconnected flight transitions"), metric.WithUnit("{flight}"))
		amanSourceRefreshes, _ := meter.Int64Counter("aman.source.refreshes", metric.WithDescription("AMAN source refresh attempts"), metric.WithUnit("{refresh}"))
		amanPublicationFailures, _ := meter.Int64Counter("aman.publication.failures", metric.WithDescription("Post-commit AMAN replacement publication failures"), metric.WithUnit("{publication}"))

		inst = &instruments{
			activeConnections:       activeConnections,
			activeClients:           activeClients,
			activeMasterClients:     activeMasterClients,
			messagesReceived:        messagesReceived,
			messagesSent:            messagesSent,
			messageHandledDuration:  messageHandledDuration,
			messageDBOperations:     messageDBOperations,
			syncInputStrips:         syncInputStrips,
			syncInputControllers:    syncInputControllers,
			syncChangedStrips:       syncChangedStrips,
			syncChangedControllers:  syncChangedControllers,
			syncDBOperations:        syncDBOperations,
			syncDuration:            syncDuration,
			pdcRequestsReceived:     pdcRequestsReceived,
			pdcRequestOutcomes:      pdcRequestOutcomes,
			pdcStateChanges:         pdcStateChanges,
			trafficOnStand:          trafficOnStand,
			trafficTaxiing:          trafficTaxiing,
			trafficArrivalRate15m:   trafficArrivalRate15m,
			trafficDepartureRate15m: trafficDepartureRate15m,
			satSnapshotAge:          satSnapshotAge, satFeedRecords: satFeedRecords,
			satAssignments: satAssignments, satOutcomes: satOutcomes,
			satConflicts: satConflicts, satExpirations: satExpirations,
			amanObservationAge: amanObservationAge, amanGeometryCache: amanGeometryCache,
			amanRouteMaterialized: amanRouteMaterialized, amanRouteDuration: amanRouteDuration,
			amanPredictorDuration: amanPredictorDuration, amanPredictionDrift: amanPredictionDrift,
			amanSequenceRevisions: amanSequenceRevisions, amanSequenceConflicts: amanSequenceConflicts,
			amanCommandOutcomes: amanCommandOutcomes, amanFlightDegradation: amanFlightDegradation,
			amanSourceRefreshes: amanSourceRefreshes, amanPublicationFailures: amanPublicationFailures,
		}
	})
	return inst
}

// AMAN metrics intentionally accept only fixed vocabulary labels. In
// particular, callsigns, route text, command IDs, and provider payloads never
// become metric dimensions.
func RecordAMANObservation(ctx context.Context, age time.Duration, state string) {
	get().amanObservationAge.Record(ctx, max(age.Seconds(), 0), metric.WithAttributes(attribute.String("state", amanStateLabel(state))))
}

func RecordAMANGeometryCache(ctx context.Context, outcome string) {
	get().amanGeometryCache.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", fixedAMANLabel(outcome, "hit", "miss", "error"))))
}

func RecordAMANRouteMaterialization(ctx context.Context, duration time.Duration, outcome string) {
	attrs := metric.WithAttributes(attribute.String("outcome", fixedAMANLabel(outcome, "success", "failure")))
	i := get()
	i.amanRouteMaterialized.Add(ctx, 1, attrs)
	i.amanRouteDuration.Record(ctx, max(duration.Seconds(), 0), attrs)
}

func RecordAMANPredictor(ctx context.Context, duration time.Duration, confidence, degradation string) {
	get().amanPredictorDuration.Record(ctx, max(duration.Seconds(), 0), metric.WithAttributes(
		attribute.String("confidence", fixedAMANLabel(confidence, "unknown", "low", "medium", "high")),
		attribute.String("degradation", fixedAMANLabel(degradation, "none", "weather", "geometry", "performance", "source")),
	))
}

func RecordAMANPredictionDrift(ctx context.Context, kind string, drift time.Duration) {
	get().amanPredictionDrift.Record(ctx, max(drift.Seconds(), 0), metric.WithAttributes(attribute.String("kind", fixedAMANLabel(kind, "raw_operational", "superstable"))))
}

func RecordAMANSequenceRevision(ctx context.Context) { get().amanSequenceRevisions.Add(ctx, 1) }
func RecordAMANSequenceConflict(ctx context.Context, kind string) {
	get().amanSequenceConflicts.Add(ctx, 1, metric.WithAttributes(attribute.String("kind", fixedAMANLabel(kind, "revision", "command", "policy"))))
}
func RecordAMANCommand(ctx context.Context, outcome string) {
	get().amanCommandOutcomes.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", fixedAMANLabel(outcome, "accepted", "rejected", "duplicate", "failed"))))
}
func RecordAMANFlightDegradation(ctx context.Context, state string) {
	get().amanFlightDegradation.Add(ctx, 1, metric.WithAttributes(attribute.String("state", amanStateLabel(state))))
}
func RecordAMANSourceRefresh(ctx context.Context, source, outcome string) {
	get().amanSourceRefreshes.Add(ctx, 1, metric.WithAttributes(attribute.String("source", fixedAMANLabel(source, "vatsim", "navigation", "weather")), attribute.String("outcome", fixedAMANLabel(outcome, "success", "failure", "stale", "disconnected"))))
}
func RecordAMANPublicationFailure(ctx context.Context, destination string) {
	get().amanPublicationFailures.Add(ctx, 1, metric.WithAttributes(attribute.String("destination", fixedAMANLabel(destination, "frontend", "euroscope"))))
}

func amanStateLabel(value string) string {
	return fixedAMANLabel(value, "fresh", "stale", "disconnected")
}
func fixedAMANLabel(value string, allowed ...string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return "other"
}

// RecordSATFeedSnapshot records only operational dimensions. Callsigns, CIDs,
// names, and other pilot data are deliberately excluded from metric labels.
func RecordSATFeedSnapshot(ctx context.Context, age time.Duration, pilots, prefiles int) {
	i := get()
	i.satSnapshotAge.Record(ctx, max(age.Seconds(), 0))
	i.satFeedRecords.Record(ctx, int64(pilots), metric.WithAttributes(attribute.String("state", "online")))
	i.satFeedRecords.Record(ctx, int64(prefiles), metric.WithAttributes(attribute.String("state", "prefile")))
}

func RecordSATRelevantFlights(ctx context.Context, sessionName, airport string, pilots, prefiles int) {
	i := get()
	i.satFeedRecords.Record(ctx, int64(pilots), sessionAttributes(sessionName, airport, attribute.String("state", "online")))
	i.satFeedRecords.Record(ctx, int64(prefiles), sessionAttributes(sessionName, airport, attribute.String("state", "prefile")))
}

func RecordSATAssignment(ctx context.Context, stage, source, category string, tier int) {
	attrs := []attribute.KeyValue{attribute.String("stage", stage), attribute.String("source", source), attribute.String("category", category)}
	if tier > 0 {
		attrs = append(attrs, attribute.Int("tier", tier))
	}
	get().satAssignments.Add(ctx, 1, metric.WithAttributes(attrs...))
}

func RecordSATOutcome(ctx context.Context, outcome, category string) {
	get().satOutcomes.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome), attribute.String("category", category)))
}

func RecordSATConflict(ctx context.Context, kind string) {
	get().satConflicts.Add(ctx, 1, metric.WithAttributes(attribute.String("kind", kind)))
}

func RecordSATExpiration(ctx context.Context, direction, stage string) {
	get().satExpirations.Add(ctx, 1, metric.WithAttributes(attribute.String("direction", direction), attribute.String("stage", stage)))
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

func normalizeVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "UNKNOWN"
	}
	return version
}

func ConnectionOpened(ctx context.Context, sessionName, airport, source, callsign, version string) {
	get().activeConnections.Add(ctx, 1,
		sessionAttributes(sessionName, airport,
			attribute.String("source", source),
			attribute.String("client_version", normalizeVersion(version)),
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
			attribute.String("client_version", normalizeVersion(version)),
		),
	)
}

func ConnectionClosed(ctx context.Context, sessionName, airport, source, callsign, version string) {
	get().activeConnections.Add(ctx, -1,
		sessionAttributes(sessionName, airport,
			attribute.String("source", source),
			attribute.String("client_version", normalizeVersion(version)),
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
			attribute.String("client_version", normalizeVersion(version)),
		),
	)
}

func MasterClientAssigned(ctx context.Context, sessionName, airport, callsign, version string) {
	callsign = normalizeCallsign(callsign)
	if callsign == "" {
		return
	}

	get().activeMasterClients.Add(ctx, 1,
		sessionAttributes(sessionName, airport,
			attribute.String("callsign", callsign),
			attribute.String("client_version", normalizeVersion(version)),
		),
	)
}

func MasterClientCleared(ctx context.Context, sessionName, airport, callsign, version string) {
	callsign = normalizeCallsign(callsign)
	if callsign == "" {
		return
	}

	get().activeMasterClients.Add(ctx, -1,
		sessionAttributes(sessionName, airport,
			attribute.String("callsign", callsign),
			attribute.String("client_version", normalizeVersion(version)),
		),
	)
}

func MessageReceived(ctx context.Context, sessionName, airport, source, msgType, version string) {
	get().messagesReceived.Add(ctx, 1,
		sessionAttributes(sessionName, airport,
			attribute.String("source", source),
			attribute.String("type", msgType),
			attribute.String("client_version", normalizeVersion(version)),
		),
	)
}

func MessageHandled(ctx context.Context, sessionName, airport, source, msgType, version string, duration time.Duration, success bool) {
	status := "ok"
	if !success {
		status = "error"
	}
	get().messageHandledDuration.Record(ctx, duration.Seconds(),
		sessionAttributes(sessionName, airport,
			attribute.String("source", source),
			attribute.String("type", msgType),
			attribute.String("status", status),
			attribute.String("client_version", normalizeVersion(version)),
		),
	)
}

func MessageDBOperations(ctx context.Context, sessionName, airport, source, msgType, version string, dbOperations int) {
	if dbOperations <= 0 {
		return
	}
	get().messageDBOperations.Add(ctx, int64(dbOperations),
		sessionAttributes(sessionName, airport,
			attribute.String("source", source),
			attribute.String("type", msgType),
			attribute.String("client_version", normalizeVersion(version)),
		),
	)
}

func RecordEuroscopeSync(ctx context.Context, sessionName, airport, version string, inputStrips, inputControllers, changedStrips, changedControllers, dbOperations int, duration time.Duration) {
	attrs := sessionAttributes(sessionName, airport, attribute.String("client_version", normalizeVersion(version)))
	i := get()
	i.syncInputStrips.Add(ctx, int64(inputStrips), attrs)
	i.syncInputControllers.Add(ctx, int64(inputControllers), attrs)
	i.syncChangedStrips.Add(ctx, int64(changedStrips), attrs)
	i.syncChangedControllers.Add(ctx, int64(changedControllers), attrs)
	i.syncDBOperations.Add(ctx, int64(dbOperations), attrs)
	i.syncDuration.Record(ctx, duration.Seconds(), attrs)
}

func MessageSent(ctx context.Context, sessionName, airport, source, msgType, version string) {
	get().messagesSent.Add(ctx, 1,
		sessionAttributes(sessionName, airport,
			attribute.String("source", source),
			attribute.String("type", msgType),
			attribute.String("client_version", normalizeVersion(version)),
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
