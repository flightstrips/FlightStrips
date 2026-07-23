export const AMAN_WIRE_VERSION = 1 as const;

export type AMANEffectiveMode = "disabled" | "shadow" | "read_only" | "authoritative" | "blocked";
export type AMANLifecycleState = "planned" | "airborne" | "unstable" | "stable" | "landed" | "go_around" | "removed";
export type AMANDataStatus = "fresh" | "stale" | "disconnected";
export type AMANFreezeReason = "none" | "superstable" | "manual";
export type AMANConfidence = "unknown" | "low" | "medium" | "high";
export type AMANHealthStatus = "disabled" | "ready" | "degraded" | "unavailable";

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
  technical_health: AMANTechnicalHealth;
}

export interface AMANFlight {
  flight_id: string;
  callsign: string;
  lifecycle_state: AMANLifecycleState;
  data_status: AMANDataStatus;
  runway_group_id: string | null;
  feeder: string | null;
  holding_fix: string | null;
  holding_fix_eta: string | null;
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
}

export type AMANCommandType =
  | "aman.move_flight"
  | "aman.lock_flight"
  | "aman.unlock_flight"
  | "aman.set_rate"
  | "aman.accept_teta"
  | "aman.keep_fpl_eta"
  | "aman.set_manual_eta"
  | "aman.reset_teta_override"
  | "aman.report_go_around";

export interface AMANCommandMeta {
  command_id: string;
  expected_revision: number;
}

export type AMANCommandIntent =
  | {type: "aman.move_flight"; flight_id: string; runway_group_id: string; before_flight_id: string}
  | {type: "aman.move_flight"; flight_id: string; runway_group_id: string; after_flight_id: string}
  | {type: "aman.lock_flight" | "aman.unlock_flight" | "aman.accept_teta" | "aman.keep_fpl_eta" | "aman.reset_teta_override"; flight_id: string}
  | {type: "aman.set_rate"; runway_group_id: string; arrivals_per_hour: number; effective_at: string}
  | {type: "aman.set_manual_eta"; flight_id: string; manual_eta: string}
  | {type: "aman.report_go_around"; flight_id: string; detected_at: string};

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
const freezeReasons = new Set<AMANFreezeReason>(["none", "superstable", "manual"]);
const confidences = new Set<AMANConfidence>(["unknown", "low", "medium", "high"]);
const healthStatuses = new Set<AMANHealthStatus>(["disabled", "ready", "degraded", "unavailable"]);
const routeFactStates = new Set(["active", "expired"]);

const isObject = (value: unknown): value is Record<string, unknown> => typeof value === "object" && value !== null && !Array.isArray(value);
const isString = (value: unknown): value is string => typeof value === "string";
const isNullableString = (value: unknown): value is string | null => value === null || isString(value);
const isFiniteNumber = (value: unknown): value is number => typeof value === "number" && Number.isFinite(value);
const isNonNegativeInteger = (value: unknown): value is number => Number.isSafeInteger(value) && Number(value) >= 0;
const isNullableFiniteNumber = (value: unknown): value is number | null => value === null || isFiniteNumber(value);
const isStringArray = (value: unknown): value is string[] => Array.isArray(value) && value.every(isString);
const isTimestamp = (value: unknown): value is string => isString(value) && /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/.test(value);
const isNullableTimestamp = (value: unknown): value is string | null => value === null || isTimestamp(value);

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

function isFlight(value: unknown): value is AMANFlight {
  return isObject(value) && isString(value.flight_id) && value.flight_id !== "" && isString(value.callsign)
    && isString(value.lifecycle_state) && lifecycleStates.has(value.lifecycle_state as AMANLifecycleState)
    && isString(value.data_status) && dataStatuses.has(value.data_status as AMANDataStatus)
    && isNullableString(value.runway_group_id) && isNullableString(value.feeder) && isNullableString(value.holding_fix)
    && isNullableTimestamp(value.holding_fix_eta) && (value.route_fact === null || isRouteFact(value.route_fact))
    && isNullableTimestamp(value.raw_teta) && isNullableTimestamp(value.operational_teta)
    && isNullableFiniteNumber(value.gain_loss_seconds) && isString(value.freeze_reason)
    && freezeReasons.has(value.freeze_reason as AMANFreezeReason) && isNullableTimestamp(value.frozen_at)
    && (value.confidence === null || (isString(value.confidence) && confidences.has(value.confidence as AMANConfidence)))
    && (value.provenance === null || isProvenance(value.provenance)) && isNullableFiniteNumber(value.input_age_seconds)
    && isNullableString(value.geometry_version) && isNullableString(value.geometry_digest)
    && isNullableFiniteNumber(value.distance_to_go_nm) && (value.slot === null || isSlot(value.slot))
    && (value.order === null || isNonNegativeInteger(value.order)) && (value.eta_review === null || isETAReview(value.eta_review))
    && Array.isArray(value.queue_offers) && value.queue_offers.every(isQueueOffer);
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
  return isObject(value) && isString(value.id)
    && (value.active_rate_per_hour === undefined || (isNonNegativeInteger(value.active_rate_per_hour) && value.active_rate_per_hour > 0))
    && (value.rate_effective_at === undefined || isTimestamp(value.rate_effective_at));
}

export function isAMANStateEvent(value: unknown): value is AMANStateEvent {
  if (!isObject(value) || value.type !== "aman.state" || value.version !== AMAN_WIRE_VERSION || !isObject(value.data)) return false;
  const data = value.data;
  return isString(data.airport) && data.airport.length === 4 && isNonNegativeInteger(data.revision)
    && isTimestamp(data.generated_at) && isString(data.policy_version) && isString(data.effective_mode)
    && effectiveModes.has(data.effective_mode as AMANEffectiveMode) && typeof data.authoritative === "boolean"
    && Array.isArray(data.flights) && data.flights.every(isFlight)
    && Array.isArray(data.runway_groups) && data.runway_groups.every(isRunwayGroup)
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
  return {state, status: presentationStatus(state), error: null, accepted: true};
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
