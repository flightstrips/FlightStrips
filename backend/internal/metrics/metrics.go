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
	messageBytesSent        metric.Int64Counter
	messageSizeBytes        metric.Int64Histogram
	messageHandledDuration  metric.Float64Histogram
	messageDBOperations     metric.Int64Counter
	syncInputStrips         metric.Int64Counter
	syncInputControllers    metric.Int64Counter
	syncChangedStrips       metric.Int64Counter
	syncChangedControllers  metric.Int64Counter
	syncDBOperations        metric.Int64Counter
	syncDuration            metric.Float64Histogram
	syncPhaseDuration       metric.Float64Histogram
	syncOutcomes            metric.Int64Counter
	syncFollowUpWork        metric.Int64Counter
	cdmRecalculations       metric.Int64Counter
	cdmRecalculationTime    metric.Float64Histogram
	cdmRecalculationStrips  metric.Int64Histogram
	hubQueueDepth           metric.Int64Histogram
	hubDispatchDuration     metric.Float64Histogram
	hubBroadcastFanout      metric.Int64Histogram
	hubPublishBlocked       metric.Int64Counter
	hubPublishBlockedTime   metric.Float64Histogram
	hubSlowConsumers        metric.Int64Counter
	hubDispatchAttrs        map[hubDispatchKey]metric.MeasurementOption
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
	satLifecycleEvents      metric.Int64Counter
	satReconciliations      metric.Int64Counter
	satReconciliationTime   metric.Float64Histogram
	satReconciliationPasses metric.Int64Histogram
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
		messageBytesSent, _ := meter.Int64Counter(
			"websocket.message.bytes.sent",
			metric.WithDescription("Serialized WebSocket payload bytes successfully passed to the writer"),
			metric.WithUnit("{byte}"),
		)
		messageSizeBytes, _ := meter.Int64Histogram(
			"websocket.message.size.bytes",
			metric.WithDescription("Serialized WebSocket payload size successfully passed to the writer"),
			metric.WithUnit("{byte}"),
			metric.WithExplicitBucketBoundaries(64, 128, 256, 512, 1024, 2048, 4096, 8192, 16384, 32768, 65536, 131072, 262144, 524288, 1048576),
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
		syncPhaseDuration, _ := meter.Float64Histogram(
			"euroscope.sync.phase.duration",
			metric.WithDescription("EuroScope sync processing duration split by phase"),
			metric.WithUnit("s"),
			metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.0, 5.0),
		)
		syncOutcomes, _ := meter.Int64Counter(
			"euroscope.sync.outcomes",
			metric.WithDescription("EuroScope syncs by whether they changed persisted state"),
			metric.WithUnit("{sync}"),
		)
		syncFollowUpWork, _ := meter.Int64Counter(
			"euroscope.sync.follow_up_work",
			metric.WithDescription("Follow-up work items a EuroScope sync scheduled during finalization"),
			metric.WithUnit("{item}"),
		)
		cdmRecalculations, _ := meter.Int64Counter(
			"cdm.recalculations",
			metric.WithDescription("CDM airport sequence recalculations by outcome"),
			metric.WithUnit("{recalculation}"),
		)
		cdmRecalculationTime, _ := meter.Float64Histogram(
			"cdm.recalculation.duration",
			metric.WithDescription("CDM airport sequence recalculation duration"),
			metric.WithUnit("s"),
			metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.0, 5.0),
		)
		cdmRecalculationStrips, _ := meter.Int64Histogram(
			"cdm.recalculation.strips",
			metric.WithDescription("Strips considered by a CDM airport sequence recalculation"),
			metric.WithUnit("{strip}"),
			metric.WithExplicitBucketBoundaries(1, 5, 10, 25, 50, 100, 200, 400),
		)
		hubQueueDepth, _ := meter.Int64Histogram(
			"websocket.hub.queue.depth",
			metric.WithDescription("Pending messages in the hub dispatch queue, sampled as each message is dispatched"),
			metric.WithUnit("{message}"),
			metric.WithExplicitBucketBoundaries(0, 1, 2, 4, 8, 16, 32, 64, 128, 256),
		)
		hubDispatchDuration, _ := meter.Float64Histogram(
			"websocket.hub.dispatch.duration",
			metric.WithDescription("Time the hub dispatch loop spent fanning one message out to clients"),
			metric.WithUnit("s"),
			metric.WithExplicitBucketBoundaries(0.00005, 0.0001, 0.00025, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1),
		)
		hubBroadcastFanout, _ := meter.Int64Histogram(
			"websocket.hub.broadcast.fanout",
			metric.WithDescription("Clients enqueued for one dispatched hub message"),
			metric.WithUnit("{client}"),
			metric.WithExplicitBucketBoundaries(0, 1, 2, 5, 10, 25, 50, 100, 200),
		)
		hubPublishBlocked, _ := meter.Int64Counter(
			"websocket.hub.publish.blocked",
			metric.WithDescription("Publishes that had to wait because the hub dispatch queue was full"),
			metric.WithUnit("{publish}"),
		)
		hubPublishBlockedTime, _ := meter.Float64Histogram(
			"websocket.hub.publish.blocked.duration",
			metric.WithDescription("Time a publisher was blocked on a full hub dispatch queue"),
			metric.WithUnit("s"),
			metric.WithExplicitBucketBoundaries(0.0001, 0.001, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0),
		)
		hubSlowConsumers, _ := meter.Int64Counter(
			"websocket.clients.slow_disconnects",
			metric.WithDescription("Clients disconnected because their send queue was full"),
			metric.WithUnit("{client}"),
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
		satLifecycleEvents, _ := meter.Int64Counter("sat.lifecycle.events", metric.WithDescription("SAT stage promotions, tier improvements, displacement, takeover, and relocation outcomes"), metric.WithUnit("{event}"))
		satReconciliations, _ := meter.Int64Counter("sat.reconciliation.cycles", metric.WithDescription("Completed SAT reconciliation cycles by outcome"), metric.WithUnit("{cycle}"))
		satReconciliationTime, _ := meter.Float64Histogram("sat.reconciliation.duration", metric.WithDescription("SAT reconciliation cycle duration"), metric.WithUnit("s"), metric.WithExplicitBucketBoundaries(.01, .05, .1, .25, .5, 1, 2, 5, 10, 15))
		satReconciliationPasses, _ := meter.Int64Histogram("sat.reconciliation.passes", metric.WithDescription("Arrival convergence passes used by SAT"), metric.WithUnit("{pass}"), metric.WithExplicitBucketBoundaries(1, 2, 3, 4, 5))
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
			messageBytesSent:        messageBytesSent,
			messageSizeBytes:        messageSizeBytes,
			messageHandledDuration:  messageHandledDuration,
			messageDBOperations:     messageDBOperations,
			syncInputStrips:         syncInputStrips,
			syncInputControllers:    syncInputControllers,
			syncChangedStrips:       syncChangedStrips,
			syncChangedControllers:  syncChangedControllers,
			syncDBOperations:        syncDBOperations,
			syncDuration:            syncDuration,
			syncPhaseDuration:       syncPhaseDuration,
			syncOutcomes:            syncOutcomes,
			syncFollowUpWork:        syncFollowUpWork,
			cdmRecalculations:       cdmRecalculations,
			cdmRecalculationTime:    cdmRecalculationTime,
			cdmRecalculationStrips:  cdmRecalculationStrips,
			hubQueueDepth:           hubQueueDepth,
			hubDispatchDuration:     hubDispatchDuration,
			hubBroadcastFanout:      hubBroadcastFanout,
			hubPublishBlocked:       hubPublishBlocked,
			hubPublishBlockedTime:   hubPublishBlockedTime,
			hubSlowConsumers:        hubSlowConsumers,
			hubDispatchAttrs:        buildHubDispatchAttributes(),
			pdcRequestsReceived:     pdcRequestsReceived,
			pdcRequestOutcomes:      pdcRequestOutcomes,
			pdcStateChanges:         pdcStateChanges,
			trafficOnStand:          trafficOnStand,
			trafficTaxiing:          trafficTaxiing,
			trafficArrivalRate15m:   trafficArrivalRate15m,
			trafficDepartureRate15m: trafficDepartureRate15m,
			satSnapshotAge:          satSnapshotAge, satFeedRecords: satFeedRecords,
			satAssignments: satAssignments, satOutcomes: satOutcomes,
			satConflicts: satConflicts, satExpirations: satExpirations, satLifecycleEvents: satLifecycleEvents,
			satReconciliations: satReconciliations, satReconciliationTime: satReconciliationTime, satReconciliationPasses: satReconciliationPasses,
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
	return fixedLabel(value, allowed...)
}

// fixedLabel collapses a value to a closed vocabulary so an unexpected input can
// never introduce unbounded metric cardinality.
func fixedLabel(value string, allowed ...string) string {
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

func RecordSATLifecycleEvent(ctx context.Context, event, reason, source string) {
	get().satLifecycleEvents.Add(ctx, 1, metric.WithAttributes(
		attribute.String("event", event),
		attribute.String("reason", reason),
		attribute.String("source", source),
	))
}

func RecordSATReconciliation(ctx context.Context, duration time.Duration, outcome string) {
	i := get()
	i.satReconciliations.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome)))
	i.satReconciliationTime.Record(ctx, max(duration.Seconds(), 0), metric.WithAttributes(attribute.String("outcome", outcome)))
}

func RecordSATReconciliationPasses(ctx context.Context, passes int, capped bool) {
	get().satReconciliationPasses.Record(ctx, int64(passes), metric.WithAttributes(attribute.Bool("capped", capped)))
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

// Sync phases and follow-up work items use a fixed vocabulary so a slow or
// repeating sync can be attributed to one stage without callsigns or positions
// ever becoming metric dimensions.
const (
	SyncPhaseBuildState  = "build_state"
	SyncPhaseControllers = "controllers"
	SyncPhaseRunways     = "runways"
	SyncPhaseSession     = "session"
	SyncPhaseSectors     = "sectors"
	SyncPhaseStrips      = "strips"
	SyncPhaseFinalize    = "finalize"
	SyncPhaseAutoAssume  = "auto_assume"
	SyncPhaseReconcile   = "reconcile"
	SyncPhaseSids        = "sids"
)

const (
	SyncWorkRouteRecalc       = "route_recalculation"
	SyncWorkBayUpdate         = "bay_update"
	SyncWorkPdcValidation     = "pdc_validation"
	SyncWorkSquawkValidation  = "squawk_validation"
	SyncWorkLandingValidation = "landing_validation"
	SyncWorkCdmRecalculation  = "cdm_recalculation"
	SyncWorkStripUpdate       = "strip_update"
)

var syncPhases = []string{
	SyncPhaseBuildState, SyncPhaseControllers, SyncPhaseRunways, SyncPhaseSession,
	SyncPhaseSectors, SyncPhaseStrips, SyncPhaseFinalize, SyncPhaseAutoAssume,
	SyncPhaseReconcile, SyncPhaseSids,
}

var syncWorkKinds = []string{
	SyncWorkRouteRecalc, SyncWorkBayUpdate, SyncWorkPdcValidation, SyncWorkSquawkValidation,
	SyncWorkLandingValidation, SyncWorkCdmRecalculation, SyncWorkStripUpdate,
}

// RecordEuroscopeSyncPhase attributes part of a sync to one processing phase.
// euroscope.sync.duration says a sync was slow; this says which stage was slow.
func RecordEuroscopeSyncPhase(ctx context.Context, sessionName, airport, phase string, duration time.Duration) {
	get().syncPhaseDuration.Record(ctx, max(duration.Seconds(), 0),
		sessionAttributes(sessionName, airport, attribute.String("phase", fixedLabel(phase, syncPhases...))),
	)
}

// RecordEuroscopeSyncOutcome separates syncs that changed persisted state from
// heartbeats that did not. A rising unchanged rate alongside follow-up work or
// CDM recalculations is the signature of a sync repeating work for no reason.
func RecordEuroscopeSyncOutcome(ctx context.Context, sessionName, airport string, changed bool) {
	outcome := "unchanged"
	if changed {
		outcome = "changed"
	}
	get().syncOutcomes.Add(ctx, 1, sessionAttributes(sessionName, airport, attribute.String("outcome", outcome)))
}

// RecordEuroscopeSyncFollowUpWork counts the work a sync scheduled for its
// finalization phase, so the cost of a sync can be traced to what it fanned out
// into rather than only to how long it took.
func RecordEuroscopeSyncFollowUpWork(ctx context.Context, sessionName, airport, kind string, count int) {
	if count <= 0 {
		return
	}
	get().syncFollowUpWork.Add(ctx, int64(count),
		sessionAttributes(sessionName, airport, attribute.String("kind", fixedLabel(kind, syncWorkKinds...))),
	)
}

// RecordCDMRecalculation records one full airport sequence recalculation. The
// strip count exposes how much work each recalculation performed, which turns a
// CPU spike into a question of rate versus size.
func RecordCDMRecalculation(ctx context.Context, airport string, strips int, duration time.Duration, notify, success bool) {
	outcome := "success"
	if !success {
		outcome = "failure"
	}
	attrs := metric.WithAttributes(
		attribute.String("airport", normalizeAirport(airport)),
		attribute.String("outcome", outcome),
		attribute.Bool("notify", notify),
	)

	i := get()
	i.cdmRecalculations.Add(ctx, 1, attrs)
	i.cdmRecalculationTime.Record(ctx, max(duration.Seconds(), 0), attrs)
	if strips >= 0 {
		i.cdmRecalculationStrips.Record(ctx, int64(strips), metric.WithAttributes(attribute.String("airport", normalizeAirport(airport))))
	}
}

// RecordHubDispatch records one message leaving a hub dispatch loop. Queue depth
// shows the backlog the loop is working through, fanout shows how many clients
// each message reaches, and duration shows how long the single-threaded loop was
// occupied — together they explain a hub that has become the bottleneck.
//
// This runs on the single goroutine every broadcast is serialised behind, so the
// attribute set is looked up from the table built at startup rather than being
// sorted and allocated per message.
func RecordHubDispatch(ctx context.Context, source, kind string, queueDepth, fanout int, duration time.Duration) {
	i := get()
	attrs := i.hubDispatchAttrs[hubDispatchKey{source: normalizeHubSource(source), kind: normalizeHubKind(kind)}]

	i.hubQueueDepth.Record(ctx, int64(max(queueDepth, 0)), attrs)
	i.hubBroadcastFanout.Record(ctx, int64(max(fanout, 0)), attrs)
	i.hubDispatchDuration.Record(ctx, max(duration.Seconds(), 0), attrs)
}

// RecordHubPublishBlocked records a publisher that had to wait for room in the
// hub dispatch queue. Any sustained rate here means hub dispatch has fallen
// behind and is now stalling the goroutines producing the events.
func RecordHubPublishBlocked(ctx context.Context, source string, duration time.Duration) {
	attrs := metric.WithAttributes(attribute.String("source", normalizeHubSource(source)))
	i := get()
	i.hubPublishBlocked.Add(ctx, 1, attrs)
	i.hubPublishBlockedTime.Record(ctx, max(duration.Seconds(), 0), attrs)
}

// RecordSlowConsumerDisconnect records a client dropped for failing to drain its
// send queue.
func RecordSlowConsumerDisconnect(ctx context.Context, sessionName, airport, source string) {
	get().hubSlowConsumers.Add(ctx, 1, sessionAttributes(sessionName, airport, attribute.String("source", normalizeHubSource(source))))
}

// hubDispatchKey identifies one precomputed hub dispatch attribute set.
type hubDispatchKey struct {
	source string
	kind   string
}

var (
	hubSourceLabels = []string{"frontend", "euroscope", "alb", "other"}
	hubKindLabels   = []string{"broadcast", "direct", "airport", "layout", "other"}
)

// buildHubDispatchAttributes materialises every source and kind combination once
// so the dispatch loop only performs a map lookup per message.
func buildHubDispatchAttributes() map[hubDispatchKey]metric.MeasurementOption {
	attrs := make(map[hubDispatchKey]metric.MeasurementOption, len(hubSourceLabels)*len(hubKindLabels))
	for _, source := range hubSourceLabels {
		for _, kind := range hubKindLabels {
			attrs[hubDispatchKey{source: source, kind: kind}] = metric.WithAttributes(
				attribute.String("source", source),
				attribute.String("kind", kind),
			)
		}
	}
	return attrs
}

func normalizeHubSource(source string) string {
	return fixedLabel(source, hubSourceLabels...)
}

func normalizeHubKind(kind string) string {
	return fixedLabel(kind, hubKindLabels...)
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

// RecordOutboundPayload records the serialized application payload accepted by
// the WebSocket writer. It excludes WebSocket framing, TLS overhead, and any
// change in size caused by transport compression. Only source and message type
// are dimensions, keeping the metric independent of clients and sessions.
func RecordOutboundPayload(ctx context.Context, source, msgType string, sizeBytes int) {
	attrs := metric.WithAttributes(
		attribute.String("source", normalizeHubSource(source)),
		attribute.String("type", normalizeOutboundMessageType(msgType)),
	)
	size := int64(max(sizeBytes, 0))
	i := get()
	i.messageBytesSent.Add(ctx, size, attrs)
	i.messageSizeBytes.Record(ctx, size, attrs)
}

func normalizeOutboundMessageType(msgType string) string {
	msgType = strings.TrimSpace(msgType)
	if msgType == "" {
		return "unknown"
	}
	return msgType
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
