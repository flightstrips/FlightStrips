import type {Timestamp} from "@bufbuild/protobuf/wkt";
import type {AmanAirport, AmanCoordination, AmanFlight, EntitySnapshot} from "./generated/cluster/v1/storage_pb";
import type {AMANComponentHealth, AMANFlight, AMANRunwayGroup, AMANSlot, AMANStateEvent, AMANTechnicalHealth} from "./aman";

const iso = (value?: Timestamp): string | null => value
  ? new Date(Number(value.seconds) * 1000 + Math.floor(value.nanos / 1e6)).toISOString() : null;
const num = (value: bigint): number => Number(value);

function slot(value: NonNullable<AmanFlight["slot"]>): AMANSlot {
  return {time: iso(value.time) ?? "", runway_group_id: value.runwayGroupId,
    sequence: value.sequence, revision: num(value.revision), reason: value.reason};
}
function flight(value: AmanFlight): AMANFlight {
  const prediction = value.prediction;
  const holdingPlan = prediction?.holdingPlan;
  const review = value.etaReview;
  const goAround = value.goAroundConfirmation;
  return {
    callsign: value.callsign, aircraft_type: value.latestObservation?.aircraftType || undefined,
    wake_category: value.latestObservation?.wakeCategory || undefined,
    lifecycle_state: value.state as AMANFlight["lifecycle_state"],
    sequence_disposition: value.sequenceDisposition as AMANFlight["sequence_disposition"],
    data_status: value.dataStatus as AMANFlight["data_status"],
    runway_group_id: value.selectedRunwayGroup || null,
    feeder: value.selectedStarFamily || null, star: value.selectedStarFamily || null,
    star_family: value.selectedStarFamily || null, feeder_fix: value.selectedFeederFix || null,
    feeder_fix_eta: value.feederEta ? iso(value.feederEta.eta) : undefined,
    feeder_fix_eta_source: value.feederEta?.source as AMANFlight["feeder_fix_eta_source"],
    feeder_fix_passed: value.feederEta?.passed,
    holding_fix: value.selectedHolding || null, holding_fix_eta: iso(prediction?.holdingFixEta),
    holding_entry_time: iso(holdingPlan?.holdingEntryTime),
    approach_release_time: iso(holdingPlan?.approachReleaseTime),
    expected_holding_seconds: holdingPlan?.expectedHoldingDuration ? Number(holdingPlan.expectedHoldingDuration.seconds) : null,
    post_holding_transit_seconds: holdingPlan?.postHoldingTransit ? Number(holdingPlan.postHoldingTransit.seconds) : null,
    route_fact: value.activeRouteFact ? {id: value.activeRouteFact.id, fix: value.activeRouteFact.fix,
      observed_at: iso(value.activeRouteFact.observedAt) ?? "", state: value.activeRouteFact.state as "active" | "cleared" | "expired"} : null,
    raw_teta: iso(prediction?.rawTeta), operational_teta: iso(prediction?.operationalTeta),
    gain_loss_seconds: prediction?.rawTeta && prediction.operationalTeta
      ? Number(prediction.operationalTeta.seconds - prediction.rawTeta.seconds) : null,
    freeze_reason: value.freezeReason as AMANFlight["freeze_reason"], frozen_at: iso(value.frozenAt),
    confidence: (prediction?.confidence || null) as AMANFlight["confidence"],
    provenance: prediction ? {model_version: prediction.modelVersion, config_version: prediction.configVersion,
      performance_profile_id: prediction.performanceProfileId ?? null,
      weather_source: prediction.weatherSource ?? null, sources: prediction.sources} : null,
    input_age_seconds: prediction?.inputObservedAt && prediction.generatedAt
      ? Number(prediction.generatedAt.seconds - prediction.inputObservedAt.seconds) : null,
    geometry_version: prediction?.datasetVersion ?? null, geometry_digest: prediction?.geometryDigest ?? null,
    distance_to_go_nm: prediction?.distanceToGoNm ?? null,
    slot: value.slot ? slot(value.slot) : null, order: value.order ?? null,
    eta_review: review ? {status: review.status, created_at: iso(review.createdAt) ?? "",
      deadline_at: iso(review.deadlineAt) ?? "", resolved_at: iso(review.resolvedAt),
      actor: review.actor ?? null, note: review.note ?? null,
      initial_baseline_teta: iso(review.initialBaselineTeta) ?? "",
      calculated_operational_teta: iso(review.calculatedOperationalTeta) ?? "",
      selected_teta: iso(review.selectedTeta) ?? "", manual_teta: iso(review.manualTeta)} : null,
    queue_offers: value.queueOffers.filter(offer => offer.candidateSlot).map(offer => ({
      callsign: offer.callsign, runway_group_id: offer.runwayGroupId,
      candidate_slot: slot(offer.candidateSlot!), queue_position: offer.queuePosition,
      expires_at: iso(offer.expiresAt) ?? "", airport_revision: num(offer.airportRevision), reason: offer.reason,
    })),
    go_around_confirmation: goAround ? {episode_id: goAround.episodeId, reason: goAround.reason,
      detected_at: iso(goAround.detectedAt) ?? "", evidence_times: goAround.evidenceTimes.map(t => iso(t) ?? ""),
      status: goAround.status as "pending" | "confirmed" | "rejected", decided_at: iso(goAround.decidedAt),
      decided_by: goAround.decidedBy ?? null, resulting_revision: goAround.resultingRevision === undefined ? null : num(goAround.resultingRevision)} : null,
    runway_gap_exception: value.gapException ? {gap_id: value.gapException.gapId,
      runway_group_id: value.gapException.runwayGroupId, opportunity: iso(value.gapException.opportunity) ?? "",
      command_id: value.gapException.commandId} : undefined,
  };
}

function health(value: AmanAirport): AMANTechnicalHealth {
  const unavailable: AMANComponentHealth = {status: "unavailable", reason: null, updated_at: null, age_seconds: null};
  const components = new Map(value.health?.components.map(c => [c.component, {
    status: c.status as AMANComponentHealth["status"], reason: c.reason || null,
    updated_at: iso(c.updatedAt), age_seconds: c.ageSeconds ?? null,
  }]) ?? []);
  const component = (name: string) => components.get(name) ?? unavailable;
  return {status: (value.health?.status || "unavailable") as AMANTechnicalHealth["status"],
    ready: value.health?.ready ?? false, blocked_reasons: value.health?.blockedReasons ?? [],
    vatsim: component("vatsim"), navigation: component("navigation"), weather: component("weather"),
    repository: component("repository"), predictor: component("predictor"), replay_validation: component("replay_validation")};
}

function runway(value: AmanAirport["runwayGroups"][number]): AMANRunwayGroup {
  return {id: value.id, selected: value.selected,
    selection_schedule: value.selectionSchedule.map(point => iso(point.effectiveAt) ?? ""),
    selection_conflict: value.selectionConflict,
    active_rate_per_hour: value.activeRatePerHour || undefined,
    rate_effective_at: iso(value.rateEffectiveAt) ?? undefined,
    gaps: value.gaps.map(g => ({id: g.id, start: iso(g.start) ?? "", end: iso(g.end) ?? "",
      label: g.label, created_at: iso(g.createdAt) ?? "", created_by: g.createdBy})),
    closures: value.closures.map(c => ({id: c.id, start: iso(c.start) ?? "", end: iso(c.end),
      reason: c.reason, created_at: iso(c.createdAt) ?? "", created_by: c.createdBy})),
    capacity_reservations: value.reservations.map(r => ({id: r.id, start: iso(r.start) ?? "", end: iso(r.end) ?? "",
      label: r.label, created_at: iso(r.createdAt) ?? "", created_by: r.createdBy})),
  };
}

function coordination(value: AmanCoordination) {
  const kind = value.request.case === "speed" ? "speed" as const : "route_direct" as const;
  return {id: value.id, callsign: value.callsign, recipient_controller: value.recipientController,
    recipient_status: value.recipientStatus as "assigned" | "unassigned", kind,
    state: value.state as "pending" | "accepted" | "rejected" | "superseded" | "expired",
    payload: value.request.case === "speed" ? {speed: {requested: value.request.value.requested}}
      : value.request.case === "routeDirect" ? {route_direct: {route: value.request.value.route,
        direct_to: value.request.value.directTo}} : {},
    created_at: iso(value.createdAt) ?? "", updated_at: iso(value.updatedAt) ?? "",
    supersedes: value.supersedes, superseded_by: value.supersededBy,
    clearance: value.clearance ? {fact_id: value.clearance.factId,
      kind: value.clearance.kind as "route_direct" | "speed", value: value.clearance.value,
      issuer: value.clearance.issuer, observed_at: iso(value.clearance.observedAt) ?? ""} : undefined};
}

export function amanState(airport: string, entities: Iterable<EntitySnapshot>): AMANStateEvent | null {
  let state: AmanAirport | undefined;
  const flights: AmanFlight[] = [];
  const coordinations: AmanCoordination[] = [];
  for (const entity of entities) {
    if (entity.value?.value.case === "amanAirport") state = entity.value.value.value;
    if (entity.value?.value.case === "amanFlight") flights.push(entity.value.value.value);
    if (entity.value?.value.case === "amanCoordination") coordinations.push(entity.value.value.value);
  }
  if (!state) return null;
  const groups = state.runwayGroups.map(runway);
  return {type: "aman.state", version: 1, data: {airport, revision: num(state.revision),
    generated_at: iso(state.generatedAt) ?? new Date(0).toISOString(), policy_version: state.policyVersion,
    effective_mode: state.effectiveMode as AMANStateEvent["data"]["effective_mode"], authoritative: state.authoritative,
    flights: flights.map(flight), runway_groups: groups,
    active_runway_groups: state.activeRunwayGroupIds.length ? state.activeRunwayGroupIds : undefined,
    timeline_configuration: state.timelineMappings.length ? {version: state.policyVersion,
      mappings: state.timelineMappings.map(m => ({id: m.id, left: m.left ?? null, right: m.right ?? null}))}
      : undefined,
    technical_health: health(state), coordination_requests: coordinations.map(coordination),
    coordination_revision: num(state.revision)}};
}
