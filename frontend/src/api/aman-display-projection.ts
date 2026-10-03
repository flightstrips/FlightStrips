import type * as Pb from "./generated/cluster/v1/storage_pb";
import type * as View from "./aman";
export function displayAMANHeader(v: Pb.AmanDisplayHeader): View.AMANHeader { return {
active_runway_groups: v.activeRunwayGroups.map(displayAMANHeaderRunwayGroup),
readiness: v.readiness ? displayAMANHeaderReadiness(v.readiness) : null,
traffic_summary: v.trafficSummary ? displayAMANHeaderTrafficSummary(v.trafficSummary) : null,
wind: v.wind ? displayAMANHeaderWind(v.wind) : null,
} as View.AMANHeader; }
export function displayAMANHeaderRunwayGroup(v: Pb.AmanDisplayHeaderRunwayGroup): View.AMANHeaderRunwayGroup { return {
id: v.id,
active_rate_per_hour: v.activeRatePerHour ?? null,
rate_effective_at: v.rateEffectiveAt ?? null,
} as View.AMANHeaderRunwayGroup; }
export function displayAMANHeaderReadiness(v: Pb.AmanDisplayHeaderReadiness): View.AMANHeader["readiness"] { return {
status: v.status,
ready: v.ready,
blocked_reasons: v.blockedReasons,
} as View.AMANHeader["readiness"]; }
export function displayAMANHeaderTrafficSummary(v: Pb.AmanDisplayHeaderTrafficSummary): View.AMANHeader["traffic_summary"] { return {
status: v.status,
tma_above_1500_feet_count: v.tmaAbove1500FeetCount,
maestro_horizon_count: v.maestroHorizonCount,
} as View.AMANHeader["traffic_summary"]; }
export function displayAMANHeaderWind(v: Pb.AmanDisplayHeaderWind): View.AMANHeaderWind { return {
surface_direction_degrees: v.surfaceDirectionDegrees,
surface_speed_knots: v.surfaceSpeedKnots,
direction_10000_degrees: v.direction10000Degrees,
speed_10000_knots: v.speed10000Knots,
observed_at: v.observedAt,
source: v.source,
} as View.AMANHeaderWind; }
export function displayAMANTrafficPrediction(v: Pb.AmanDisplayTrafficPrediction): View.AMANTrafficPrediction { return {
generated_at: v.generatedAt,
range_start: v.rangeStart,
range_end: v.rangeEnd,
bucket_minutes: v.bucketMinutes,
source_status: v.sourceStatus,
status: v.status,
degraded_reasons: v.degradedReasons,
buckets: v.buckets.map(displayAMANTrafficBucket),
} as View.AMANTrafficPrediction; }
export function displayAMANTrafficBucket(v: Pb.AmanDisplayTrafficBucket): View.AMANTrafficBucket { return {
start: v.start,
end: v.end,
planned_count: v.plannedCount,
airborne_count: v.airborneCount,
count: v.count,
load_factor: v.loadFactor,
selected_rate: v.selectedRate ? displayAMANTrafficSelectedRate(v.selectedRate) : null,
bucket_high: v.bucketHigh,
window_high: v.windowHigh,
alert: v.alert,
flights: v.flights.map(displayAMANTrafficFlight),
} as View.AMANTrafficBucket; }
export function displayAMANTrafficSelectedRate(v: Pb.AmanDisplayTrafficSelectedRate): View.AMANTrafficSelectedRate { return {
runway_group_id: v.runwayGroupId,
arrivals_per_hour: v.arrivalsPerHour,
effective_at: v.effectiveAt,
} as View.AMANTrafficSelectedRate; }
export function displayAMANTrafficFlight(v: Pb.AmanDisplayTrafficFlight): View.AMANTrafficFlight { return {
callsign: v.callsign,
airborne: v.airborne,
landing_at: v.landingAt,
timing_source: v.timingSource,
data_status: v.dataStatus,
} as View.AMANTrafficFlight; }
export function displayAMANHoldingEntry(v: Pb.AmanDisplayHoldingEntry): View.AMANHoldingEntry { return {
callsign: v.callsign,
holding: v.holding,
eat: v.eat ?? null,
cleared_altitude: v.clearedAltitude ?? null,
source_status: v.sourceStatus,
observed_at: v.observedAt,
} as View.AMANHoldingEntry; }
export function displayAMANWarning(v: Pb.AmanDisplayWarning): View.AMANWarning { return {
id: v.id,
source: v.source,
component: v.component ?? undefined,
severity: v.severity,
code: v.code,
runway_group_id: v.runwayGroupId ?? undefined,
callsign: v.callsign ?? undefined,
related_callsign: v.relatedCallsign ?? undefined,
message: v.message,
} as View.AMANWarning; }
