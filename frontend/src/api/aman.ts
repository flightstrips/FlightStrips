export const AMAN_WIRE_VERSION = 1 as const;

export type AMANEffectiveMode = "disabled" | "shadow" | "read_only" | "authoritative" | "blocked";
export type AMANLifecycleState = "planned" | "airborne" | "unstable" | "stable" | "landed" | "go_around" | "removed";
export type AMANDataStatus = "fresh" | "stale" | "disconnected";
export type AMANFreezeReason = "none" | "superstable" | "tma" | "manual";
export type AMANConfidence = "unknown" | "low" | "medium" | "high";
export type AMANFeederETASource = "route" | "holding" | "manual" | "passed";
export type AMANHealthStatus = "disabled" | "ready" | "degraded" | "unavailable";
export type AMANWarningSource = "technical_health" | "sequence";
export type AMANWarningSeverity = "error" | "warning";

export interface AMANStateEvent {
  type: "aman.state";
  version: typeof AMAN_WIRE_VERSION;
  data: AMANState;
}

export interface AMANState {
  airport: string;
  revision: number;
  generated_at: string;
  policy_version: string;
  effective_mode: AMANEffectiveMode;
  authoritative: boolean;
  flights: AMANFlight[];
  runway_groups: AMANRunwayGroup[];
  /** Optional while V1 clients and servers roll through multi-runway support. */
  active_runway_groups?: string[];
  /** Optional while V1 clients and servers roll through configured timelines. */
  timeline_configuration?: AMANTimelineConfiguration;
  /** Optional while V1 clients and servers roll through the TMT extension. */
  traffic_prediction?: AMANTrafficPrediction;
  /** Optional while V1 clients and servers roll through the holding extension. */
  holding_information?: AMANHoldingEntry[];
  /** Complete current replacement; an empty array clears previously published warnings. */
  warnings?: AMANWarning[];
  technical_health: AMANTechnicalHealth;
}

export interface AMANWarning {
  id: string;
  source: AMANWarningSource;
  component?: string;
  severity: AMANWarningSeverity;
  code: string;
  runway_group_id?: string;
  flight_id?: string;
  related_flight_id?: string;
  message: string;
}

export interface AMANTimelineConfiguration {
  version: string;
  mappings: AMANTimelineMapping[];
}

export interface AMANTimelineMapping {
  id: number;
  left: string | null;
  right: string | null;
}

export interface AMANHoldingEntry {
  flight_id: string;
  callsign: string;
  holding: string;
  eat: string | null;
  cleared_altitude: number | null;
  source_status: AMANDataStatus;
  observed_at: string;
}

export type AMANTrafficStatus = "ready" | "degraded" | "disconnected";
export type AMANTrafficAlert = "none" | "yellow" | "red";
export type AMANTrafficTimingSource = "aman" | "vatsim_planned" | "vatsim_airborne";

export interface AMANTrafficPrediction {
  generated_at: string;
  range_start: string;
  range_end: string;
  bucket_minutes: 15;
  source_status: AMANDataStatus;
  status: AMANTrafficStatus;
  degraded_reasons: string[];
  buckets: AMANTrafficBucket[];
}

export interface AMANTrafficBucket {
  start: string;
  end: string;
  planned_count: number;
  airborne_count: number;
  count: number;
  load_factor: number;
  selected_rate: AMANTrafficSelectedRate | null;
  bucket_high: boolean;
  window_high: boolean;
  alert: AMANTrafficAlert;
  flights: AMANTrafficFlight[];
}

export interface AMANTrafficSelectedRate {
  runway_group_id: string;
  arrivals_per_hour: number;
  effective_at: string;
}

export interface AMANTrafficFlight {
  flight_id: string;
  callsign: string;
  airborne: boolean;
  landing_at: string;
  timing_source: AMANTrafficTimingSource;
  data_status: AMANDataStatus;
}

export interface AMANFlight {
  flight_id: string;
  callsign: string;
  lifecycle_state: AMANLifecycleState;
  data_status: AMANDataStatus;
  runway_group_id: string | null;
  /** @deprecated Deployed alias for the STAR family; retained for older V1 clients. */
  feeder: string | null;
  /** @deprecated Deployed alias for the STAR family; retained for older V1 clients. */
  star: string | null;
  /** Optional while V1 clients and servers roll through explicit terminal identities. */
  star_family?: string | null;
  /** Optional while V1 clients and servers roll through explicit terminal identities. */
  feeder_fix?: string | null;
  /** Optional while V1 clients and servers roll through feeder-fix timing. */
  feeder_fix_eta?: string | null;
  /** Optional while V1 clients and servers roll through feeder-fix timing. */
  feeder_fix_eta_source?: AMANFeederETASource;
  /** Optional while V1 clients and servers roll through feeder-fix timing. */
  feeder_fix_passed?: boolean;
  holding_fix: string | null;
  holding_fix_eta: string | null;
  holding_entry_time: string | null;
  approach_release_time: string | null;
  expected_holding_seconds: number | null;
  post_holding_transit_seconds: number | null;
  route_fact: AMANRouteFact | null;
  raw_teta: string | null;
  operational_teta: string | null;
  gain_loss_seconds: number | null;
  freeze_reason: AMANFreezeReason;
  frozen_at: string | null;
  confidence: AMANConfidence | null;
  provenance: AMANProvenance | null;
  input_age_seconds: number | null;
  geometry_version: string | null;
  geometry_digest: string | null;
  distance_to_go_nm: number | null;
  slot: AMANSlot | null;
  order: number | null;
  eta_review: AMANETAReview | null;
  queue_offers: AMANQueueOffer[];
  go_around_confirmation: AMANGoAroundConfirmation | null;
}

export interface AMANGoAroundConfirmation {
  episode_id: string;
  reason: string;
  detected_at: string;
  evidence_times: string[];
  status: "pending" | "confirmed" | "rejected";
  decided_at: string | null;
  decided_by: string | null;
  resulting_revision: number | null;
}

export interface AMANRouteFact {
  id: string;
  fix: string;
  observed_at: string;
  state: "active" | "expired";
}

export interface AMANProvenance {
  model_version: string;
  config_version: string;
  performance_profile_id: string | null;
  weather_source: string | null;
  sources: string[];
}

export interface AMANSlot {
  time: string;
  runway_group_id: string;
  sequence: number;
  revision: number;
  reason: string;
}

export interface AMANETAReview {
  status: string;
  created_at: string;
  deadline_at: string;
  resolved_at: string | null;
  actor: string | null;
  note: string | null;
  initial_baseline_teta: string;
  calculated_operational_teta: string;
  selected_teta: string;
  manual_teta: string | null;
}

export interface AMANQueueOffer {
  flight_id: string;
  runway_group_id: string;
  candidate_slot: AMANSlot;
  queue_position: number;
  expires_at: string;
  airport_revision: number;
  reason: string;
}

export interface AMANRunwayGroup {
  id: string;
  selected?: boolean;
  selection_schedule?: string[];
  selection_conflict?: string;
  active_rate_per_hour?: number;
  rate_effective_at?: string;
}

export interface AMANTechnicalHealth {
  status: AMANHealthStatus;
  ready: boolean;
  blocked_reasons: string[];
  vatsim: AMANComponentHealth;
  navigation: AMANComponentHealth;
  weather: AMANComponentHealth;
  repository: AMANComponentHealth;
  predictor: AMANComponentHealth;
  replay_validation: AMANComponentHealth;
}

export interface AMANComponentHealth {
  status: AMANHealthStatus;
  reason: string | null;
  updated_at: string | null;
  age_seconds: number | null;
}

export interface AMANCommandRejectedEvent {
  type: "aman.command_rejected";
  version: typeof AMAN_WIRE_VERSION;
  data: AMANCommandRejection;
}

export interface AMANCommandRejection {
  command_id: string;
  code: string;
  message: string;
  current_revision: number;
  retryable: boolean;
  command_type?: AMANCommandType;
}

export type AMANCommandType =
  | "aman.move_flight"
  | "aman.lock_flight"
  | "aman.unlock_flight"
  | "aman.set_rate"
  | "aman.select_runway_group"
  | "aman.accept_teta"
  | "aman.keep_fpl_eta"
  | "aman.set_manual_eta"
  | "aman.reset_teta_override"
  | "aman.set_manual_feeder_eta"
  | "aman.reset_manual_feeder_eta"
  | "aman.recompute_flight"
  | "aman.change_runway"
  | "aman.report_go_around"
  | "aman.confirm_go_around"
  | "aman.reject_go_around";

export interface AMANCommandMeta {
  command_id: string;
  expected_revision: number;
}

export type AMANCommandIntent =
  | {type: "aman.move_flight"; flight_id: string; runway_group_id: string; before_flight_id: string}
  | {type: "aman.move_flight"; flight_id: string; runway_group_id: string; after_flight_id: string}
  | {type: "aman.lock_flight" | "aman.unlock_flight" | "aman.accept_teta" | "aman.keep_fpl_eta" | "aman.reset_teta_override"; flight_id: string}
  | {type: "aman.set_rate"; runway_group_id: string; arrivals_per_hour: number; effective_at: string}
  | {type: "aman.select_runway_group"; runway_group_id: string; effective_at: string}
  | {type: "aman.set_manual_eta"; flight_id: string; manual_eta: string}
  | {type: "aman.set_manual_feeder_eta"; flight_id: string; feeder_eta: string}
  | {type: "aman.reset_manual_feeder_eta"; flight_id: string}
  | {type: "aman.recompute_flight"; flight_id: string}
  | {type: "aman.change_runway"; flight_id: string; runway_group_id: string}
  | {type: "aman.report_go_around"; flight_id: string; detected_at: string}
  | {type: "aman.confirm_go_around" | "aman.reject_go_around"; flight_id: string; episode_id: string};

export type AMANCommandMessage = AMANCommandIntent extends infer Intent
  ? Intent extends {type: AMANCommandType}
    ? {
        type: Intent["type"];
        version: typeof AMAN_WIRE_VERSION;
        data: AMANCommandMeta & Omit<Intent, "type">;
      }
    : never
  : never;

export interface AMANPendingCommand {
  command_id: string;
  type: AMANCommandType;
  expected_revision: number;
  flight_id?: string;
  runway_group_id?: string;
}

export type AMANConnectionState = "connected" | "disconnected";

export type AMANMutationBlockReason =
  | "no_state"
  | "disconnected"
  | "observer"
  | "unauthorized"
  | "not_authoritative"
  | "not_ready";

export interface AMANMutationGateInput {
  state: AMANState | null;
  connection_state: AMANConnectionState;
  read_only: boolean;
  has_fmp_authority: boolean;
}

export function getAMANMutationBlockReason(input: AMANMutationGateInput): AMANMutationBlockReason | null {
  if (input.state === null) return "no_state";
  if (input.connection_state !== "connected") return "disconnected";
  if (input.read_only) return "observer";
  if (!input.has_fmp_authority) return "unauthorized";
  if (!input.state.authoritative || input.state.effective_mode !== "authoritative") return "not_authoritative";
  if (!input.state.technical_health.ready) return "not_ready";
  return null;
}

export function createAMANCommand(intent: AMANCommandIntent, meta: AMANCommandMeta): AMANCommandMessage {
  const {type, ...fields} = intent;
  return {
    type,
    version: AMAN_WIRE_VERSION,
    data: {...meta, ...fields},
  } as AMANCommandMessage;
}

export type AMANPresentationStatus = "empty" | "ready" | "degraded";

export interface AMANReplacementResult {
  state: AMANState | null;
  status: AMANPresentationStatus;
  error: string | null;
  accepted: boolean;
}

const effectiveModes = new Set<AMANEffectiveMode>(["disabled", "shadow", "read_only", "authoritative", "blocked"]);
const lifecycleStates = new Set<AMANLifecycleState>(["planned", "airborne", "unstable", "stable", "landed", "go_around", "removed"]);
const dataStatuses = new Set<AMANDataStatus>(["fresh", "stale", "disconnected"]);
const freezeReasons = new Set<AMANFreezeReason>(["none", "superstable", "tma", "manual"]);
const confidences = new Set<AMANConfidence>(["unknown", "low", "medium", "high"]);
const feederETASources = new Set<AMANFeederETASource>(["route", "holding", "manual", "passed"]);
const healthStatuses = new Set<AMANHealthStatus>(["disabled", "ready", "degraded", "unavailable"]);
const routeFactStates = new Set(["active", "expired"]);
const trafficStatuses = new Set<AMANTrafficStatus>(["ready", "degraded", "disconnected"]);
const trafficAlerts = new Set<AMANTrafficAlert>(["none", "yellow", "red"]);
const trafficSources = new Set<AMANTrafficTimingSource>(["aman", "vatsim_planned", "vatsim_airborne"]);
const warningSources = new Set<AMANWarningSource>(["technical_health", "sequence"]);
const warningSeverities = new Set<AMANWarningSeverity>(["error", "warning"]);

const isObject = (value: unknown): value is Record<string, unknown> => typeof value === "object" && value !== null && !Array.isArray(value);
const isString = (value: unknown): value is string => typeof value === "string";
const isNullableString = (value: unknown): value is string | null => value === null || isString(value);
const isIdentity = (value: unknown): value is string => isString(value) && value !== "" && value.trim() === value;
const isNullableIdentity = (value: unknown): value is string | null => value === null || isIdentity(value);
const isOptionalNullableIdentity = (value: unknown): value is string | null | undefined => value === undefined || isNullableIdentity(value);
const isFiniteNumber = (value: unknown): value is number => typeof value === "number" && Number.isFinite(value);
const isNonNegativeInteger = (value: unknown): value is number => Number.isSafeInteger(value) && Number(value) >= 0;
const isNullableFiniteNumber = (value: unknown): value is number | null => value === null || isFiniteNumber(value);
const isStringArray = (value: unknown): value is string[] => Array.isArray(value) && value.every(isString);
const isTimestamp = (value: unknown): value is string => isString(value) && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/.test(value);
const isNullableTimestamp = (value: unknown): value is string | null => value === null || isTimestamp(value);
const isOptionalNullableTimestamp = (value: unknown): value is string | null | undefined => value === undefined || isNullableTimestamp(value);
const isOptionalIdentity = (value: unknown): value is string | undefined => value === undefined || isIdentity(value);

function hasValidFeederETA(value: Record<string, unknown>): boolean {
  if (value.feeder_fix_eta === undefined && value.feeder_fix_eta_source === undefined && value.feeder_fix_passed === undefined) return true;
  if (!isOptionalNullableTimestamp(value.feeder_fix_eta) || !isString(value.feeder_fix_eta_source)
    || !feederETASources.has(value.feeder_fix_eta_source as AMANFeederETASource) || typeof value.feeder_fix_passed !== "boolean") return false;
  return value.feeder_fix_passed
    ? value.feeder_fix_eta_source === "passed" && value.feeder_fix_eta == null
    : value.feeder_fix_eta_source !== "passed" && isTimestamp(value.feeder_fix_eta);
}

function isSlot(value: unknown): value is AMANSlot {
  return isObject(value) && isTimestamp(value.time) && isString(value.runway_group_id)
    && isNonNegativeInteger(value.sequence) && isNonNegativeInteger(value.revision) && isString(value.reason);
}

function isRouteFact(value: unknown): value is AMANRouteFact {
  return isObject(value) && isString(value.id) && isString(value.fix) && isTimestamp(value.observed_at)
    && isString(value.state) && routeFactStates.has(value.state);
}

function isProvenance(value: unknown): value is AMANProvenance {
  return isObject(value) && isString(value.model_version) && isString(value.config_version)
    && isNullableString(value.performance_profile_id) && isNullableString(value.weather_source) && isStringArray(value.sources);
}

function isETAReview(value: unknown): value is AMANETAReview {
  return isObject(value) && isString(value.status) && isTimestamp(value.created_at) && isTimestamp(value.deadline_at)
    && isNullableTimestamp(value.resolved_at) && isNullableString(value.actor) && isNullableString(value.note)
    && isTimestamp(value.initial_baseline_teta) && isTimestamp(value.calculated_operational_teta)
    && isTimestamp(value.selected_teta) && isNullableTimestamp(value.manual_teta);
}

function isQueueOffer(value: unknown): value is AMANQueueOffer {
  return isObject(value) && isString(value.flight_id) && isString(value.runway_group_id) && isSlot(value.candidate_slot)
    && isNonNegativeInteger(value.queue_position) && isTimestamp(value.expires_at)
    && isNonNegativeInteger(value.airport_revision) && isString(value.reason);
}

function isGoAroundConfirmation(value: unknown): value is AMANGoAroundConfirmation {
  return isObject(value) && isString(value.episode_id) && value.episode_id !== "" && isString(value.reason)
    && isTimestamp(value.detected_at) && Array.isArray(value.evidence_times) && value.evidence_times.length > 0
    && value.evidence_times.every(isTimestamp) && (value.status === "pending" || value.status === "confirmed" || value.status === "rejected")
    && isNullableTimestamp(value.decided_at) && isNullableString(value.decided_by)
    && (value.resulting_revision === null || isNonNegativeInteger(value.resulting_revision));
}

function isFlight(value: unknown): value is AMANFlight {
  return isObject(value) && isString(value.flight_id) && value.flight_id !== "" && isString(value.callsign)
    && isString(value.lifecycle_state) && lifecycleStates.has(value.lifecycle_state as AMANLifecycleState)
    && isString(value.data_status) && dataStatuses.has(value.data_status as AMANDataStatus)
    && isNullableString(value.runway_group_id) && isNullableString(value.feeder) && isNullableString(value.star)
    && isOptionalNullableIdentity(value.star_family) && isOptionalNullableIdentity(value.feeder_fix) && isNullableIdentity(value.holding_fix)
    && hasValidFeederETA(value)
    && isNullableTimestamp(value.holding_fix_eta) && isNullableTimestamp(value.holding_entry_time) && isNullableTimestamp(value.approach_release_time)
    && isNullableFiniteNumber(value.expected_holding_seconds) && isNullableFiniteNumber(value.post_holding_transit_seconds) && (value.route_fact === null || isRouteFact(value.route_fact))
    && isNullableTimestamp(value.raw_teta) && isNullableTimestamp(value.operational_teta)
    && isNullableFiniteNumber(value.gain_loss_seconds) && isString(value.freeze_reason)
    && freezeReasons.has(value.freeze_reason as AMANFreezeReason) && isNullableTimestamp(value.frozen_at)
    && (value.confidence === null || (isString(value.confidence) && confidences.has(value.confidence as AMANConfidence)))
    && (value.provenance === null || isProvenance(value.provenance)) && isNullableFiniteNumber(value.input_age_seconds)
    && isNullableString(value.geometry_version) && isNullableString(value.geometry_digest)
    && isNullableFiniteNumber(value.distance_to_go_nm) && (value.slot === null || isSlot(value.slot))
    && (value.order === null || isNonNegativeInteger(value.order)) && (value.eta_review === null || isETAReview(value.eta_review))
    && Array.isArray(value.queue_offers) && value.queue_offers.every(isQueueOffer)
    && (value.go_around_confirmation === null || isGoAroundConfirmation(value.go_around_confirmation));
}

function isComponentHealth(value: unknown): value is AMANComponentHealth {
  return isObject(value) && isString(value.status) && healthStatuses.has(value.status as AMANHealthStatus)
    && isNullableString(value.reason) && isNullableTimestamp(value.updated_at) && isNullableFiniteNumber(value.age_seconds);
}

function isTechnicalHealth(value: unknown): value is AMANTechnicalHealth {
  return isObject(value) && isString(value.status) && healthStatuses.has(value.status as AMANHealthStatus)
    && typeof value.ready === "boolean" && isStringArray(value.blocked_reasons) && isComponentHealth(value.vatsim)
    && isComponentHealth(value.navigation) && isComponentHealth(value.weather) && isComponentHealth(value.repository)
    && isComponentHealth(value.predictor) && isComponentHealth(value.replay_validation);
}

function isRunwayGroup(value: unknown): value is AMANRunwayGroup {
  return isObject(value) && isIdentity(value.id)
    && (value.selected === undefined || typeof value.selected === "boolean")
    && (value.selection_schedule === undefined || (Array.isArray(value.selection_schedule) && value.selection_schedule.every(isTimestamp)))
    && (value.selection_conflict === undefined || isString(value.selection_conflict))
    && (value.active_rate_per_hour === undefined || (isNonNegativeInteger(value.active_rate_per_hour) && value.active_rate_per_hour > 0))
    && (value.rate_effective_at === undefined || isTimestamp(value.rate_effective_at));
}

function hasValidActiveRunwayGroups(data: Record<string, unknown>): boolean {
  if (data.active_runway_groups === undefined) return true;
  if (!Array.isArray(data.active_runway_groups) || data.active_runway_groups.length === 0
    || !Array.isArray(data.runway_groups)) return false;
  const configured = new Set(data.runway_groups.map((group) => isObject(group) ? group.id : undefined));
  const active = new Set<string>();
  for (const id of data.active_runway_groups) {
    if (!isIdentity(id) || active.has(id) || !configured.has(id)) return false;
    active.add(id);
  }
  return data.runway_groups.every((group) => !isObject(group) || group.selected !== true || active.has(group.id as string));
}

function isTimelineConfiguration(value: unknown): value is AMANTimelineConfiguration {
  if (!isObject(value) || !isIdentity(value.version) || !Array.isArray(value.mappings) || value.mappings.length === 0) return false;
  const families = new Set<string>();
  let previousID = 0;
  return value.mappings.every((mapping) => {
    if (!isObject(mapping) || !Number.isSafeInteger(mapping.id) || Number(mapping.id) <= previousID
      || !isNullableIdentity(mapping.left) || !isNullableIdentity(mapping.right)
      || (mapping.left === null && mapping.right === null)) return false;
    previousID = Number(mapping.id);
    for (const family of [mapping.left, mapping.right]) {
      if (family === null) continue;
      if (families.has(family)) return false;
      families.add(family);
    }
    return true;
  });
}

function isTrafficPrediction(value: unknown): value is AMANTrafficPrediction {
  if (!isObject(value) || !isTimestamp(value.generated_at) || !isTimestamp(value.range_start) || !isTimestamp(value.range_end)
    || value.bucket_minutes !== 15 || !isString(value.source_status) || !dataStatuses.has(value.source_status as AMANDataStatus)
    || !isString(value.status) || !trafficStatuses.has(value.status as AMANTrafficStatus)
    || !isStringArray(value.degraded_reasons) || !Array.isArray(value.buckets) || value.buckets.length !== 12) return false;
  const rangeStart = value.range_start;
  return value.buckets.every((bucket, index) => {
    if (!isObject(bucket) || !isTimestamp(bucket.start) || !isTimestamp(bucket.end)
      || !isNonNegativeInteger(bucket.planned_count) || !isNonNegativeInteger(bucket.airborne_count)
      || !isNonNegativeInteger(bucket.count) || bucket.count !== bucket.planned_count + bucket.airborne_count
      || !isNonNegativeInteger(bucket.load_factor) || bucket.load_factor !== bucket.count * 4
      || typeof bucket.bucket_high !== "boolean" || typeof bucket.window_high !== "boolean"
      || !isString(bucket.alert) || !trafficAlerts.has(bucket.alert as AMANTrafficAlert) || !Array.isArray(bucket.flights)) return false;
    const expectedStart = Date.parse(rangeStart) + index * 15 * 60_000;
    if (Date.parse(bucket.start) !== expectedStart || Date.parse(bucket.end) !== expectedStart + 15 * 60_000) return false;
    const rate = bucket.selected_rate;
    if (rate !== null && (!isObject(rate) || !isString(rate.runway_group_id) || !isNonNegativeInteger(rate.arrivals_per_hour)
      || rate.arrivals_per_hour === 0 || !isTimestamp(rate.effective_at))) return false;
    return bucket.flights.every((flight) => isObject(flight) && isString(flight.flight_id) && isString(flight.callsign)
      && typeof flight.airborne === "boolean" && isTimestamp(flight.landing_at) && isString(flight.timing_source)
      && trafficSources.has(flight.timing_source as AMANTrafficTimingSource) && isString(flight.data_status)
      && dataStatuses.has(flight.data_status as AMANDataStatus));
  }) && Date.parse(value.range_end) === Date.parse(value.range_start) + 3 * 60 * 60_000;
}

function isHoldingEntry(value: unknown): value is AMANHoldingEntry {
  return isObject(value) && isString(value.flight_id) && value.flight_id.length > 0
    && isString(value.callsign) && isString(value.holding) && value.holding.length > 0
    && isNullableTimestamp(value.eat) && isNullableFiniteNumber(value.cleared_altitude)
    && isString(value.source_status) && dataStatuses.has(value.source_status as AMANDataStatus)
    && isTimestamp(value.observed_at);
}

function warningIdentity(warning: AMANWarning): string {
  const optional = (value: string | undefined) => value === undefined ? "-" : JSON.stringify(value);
  return `warning:${[
    JSON.stringify(warning.source), optional(warning.component), JSON.stringify(warning.code),
    optional(warning.runway_group_id), optional(warning.flight_id), optional(warning.related_flight_id),
  ].join("/")}`;
}

function hasValidWarningScope(warning: AMANWarning): boolean {
  if (warning.source === "technical_health") {
    return warning.runway_group_id === undefined && warning.flight_id === undefined && warning.related_flight_id === undefined;
  }
  return warning.component === undefined && warning.runway_group_id !== undefined
    && warning.flight_id !== undefined && warning.related_flight_id !== undefined;
}

function hasValidWarnings(value: unknown): value is AMANWarning[] {
  if (!Array.isArray(value)) return false;
  return value.every((warning) => {
    if (!isObject(warning) || !isIdentity(warning.id)
      || !isString(warning.source) || !warningSources.has(warning.source as AMANWarningSource)
      || !isString(warning.severity) || !warningSeverities.has(warning.severity as AMANWarningSeverity)
      || !isIdentity(warning.code) || !isOptionalIdentity(warning.component)
      || !isOptionalIdentity(warning.runway_group_id) || !isOptionalIdentity(warning.flight_id)
      || !isOptionalIdentity(warning.related_flight_id) || !isIdentity(warning.message)) return false;
    const typed = warning as unknown as AMANWarning;
    if (!hasValidWarningScope(typed) || typed.id !== warningIdentity(typed)) return false;
    return true;
  });
}

export function isAMANStateEvent(value: unknown): value is AMANStateEvent {
  if (!isObject(value) || value.type !== "aman.state" || value.version !== AMAN_WIRE_VERSION || !isObject(value.data)) return false;
  const data = value.data;
  return isString(data.airport) && data.airport.length === 4 && isNonNegativeInteger(data.revision)
    && isTimestamp(data.generated_at) && isString(data.policy_version) && isString(data.effective_mode)
    && effectiveModes.has(data.effective_mode as AMANEffectiveMode) && typeof data.authoritative === "boolean"
    && Array.isArray(data.flights) && data.flights.every(isFlight)
    && Array.isArray(data.runway_groups) && data.runway_groups.every(isRunwayGroup)
    && hasValidActiveRunwayGroups(data)
    && (data.timeline_configuration === undefined || isTimelineConfiguration(data.timeline_configuration))
    && (data.traffic_prediction === undefined || isTrafficPrediction(data.traffic_prediction))
    && (data.holding_information === undefined || (Array.isArray(data.holding_information) && data.holding_information.every(isHoldingEntry)))
    && (data.warnings === undefined || hasValidWarnings(data.warnings))
    && isTechnicalHealth(data.technical_health);
}

export function replaceAMANState(current: AMANState | null, event: unknown): AMANReplacementResult {
  if (!isAMANStateEvent(event)) {
    return {state: null, status: "degraded", error: "invalid_aman_state", accepted: false};
  }
  if (current !== null && event.data.revision <= current.revision) {
    return {state: current, status: presentationStatus(current), error: null, accepted: false};
  }
  const state = structuredClone(event.data);
  const active = new Set(state.active_runway_groups
    ?? state.runway_groups.filter((group) => group.selected).map((group) => group.id));
  state.active_runway_groups = state.runway_groups.filter((group) => active.has(group.id)).map((group) => group.id);
  return {state, status: presentationStatus(state), error: null, accepted: true};
}

export function getActiveAMANRunwayGroups(state: AMANState): AMANRunwayGroup[] {
  const active = new Set(state.active_runway_groups
    ?? state.runway_groups.filter((group) => group.selected).map((group) => group.id));
  return state.runway_groups.filter((group) => active.has(group.id));
}

function presentationStatus(state: AMANState): AMANPresentationStatus {
  return state.technical_health.status === "degraded" || state.technical_health.status === "unavailable"
    ? "degraded"
    : "ready";
}

export function isAMANCommandRejectedEvent(value: unknown): value is AMANCommandRejectedEvent {
  return isObject(value) && value.type === "aman.command_rejected" && value.version === AMAN_WIRE_VERSION
    && isObject(value.data) && isString(value.data.command_id) && value.data.command_id !== ""
    && isString(value.data.code) && isString(value.data.message)
    && isNonNegativeInteger(value.data.current_revision) && typeof value.data.retryable === "boolean";
}
