import type {Timestamp} from "@bufbuild/protobuf/wkt";
import {EventType, CommunicationType, type FrontendController, type FrontendInitialEvent, type FrontendStrip, type RunwayConfiguration, type TacticalStrip, type FrontendStandAssignmentEntry, type FrontendStandBlockEntry, type WebSocketEvent} from "./models";
import type {FrontendInitial, FrontendDelta, FrontendObservation} from "./generated/cluster/v1/wire_pb";
import {EntityKind, type EntitySnapshot, type EntityRecord, type Strip, type Session, type Controller, type CdmState, type PdcSequence, type StandAssignment, type StandBlock, type ClientPresence} from "./generated/cluster/v1/storage_pb";
import {amanState} from "./aman-projection";

type Emit = (event: WebSocketEvent) => void;
const number = (value: bigint): number => {
  const result = Number(value);
  if (!Number.isSafeInteger(result)) throw new Error("entity integer exceeds browser range");
  return result;
};
const iso = (value?: Timestamp): string => value ? new Date(Number(value.seconds) * 1000 + Math.floor(value.nanos / 1e6)).toISOString() : "";
const cdmTime = (value?: Timestamp): string => value ? iso(value).slice(11, 16).replace(":", "") : "";
const legacy = (type: EventType, fields: object): WebSocketEvent => ({type, ...fields}) as WebSocketEvent;

function runwaySetup(session?: Session): RunwayConfiguration {
  return {departure: session?.runways.filter(r => r.departure).map(r => r.name) ?? [],
    arrival: session?.runways.filter(r => r.arrival).map(r => r.name) ?? [],
    runway_status: Object.fromEntries(session?.runwayStatuses.map(r => [r.pair, r.status]) ?? [])};
}

function controller(value: Controller): FrontendController {
  return {callsign: value.callsign, position: value.position, identifier: value.position,
    section: value.section, owned_sectors: value.ownedSectors, observer: value.observer};
}

function strip(value: Strip, cdm?: CdmState, pdc?: PdcSequence, stand?: StandAssignment,
  position: (cid: string) => string = cid => cid, positionAltitude?: number | null): FrontendStrip {
  return {
    callsign: value.callsign, origin: value.departure, destination: value.destination,
    alternate: value.alternate, route: value.route, remarks: value.remarks, runway: value.runway,
    squawk: value.squawk, assigned_squawk: value.assignedSquawk, sid: value.sid, star: value.star,
    cleared_altitude: value.clearedAltitude ?? 0, requested_altitude: value.requestedAltitude ?? 0,
    position_altitude: positionAltitude === null ? undefined : positionAltitude ?? value.positionAltitudeFeet,
    heading: value.heading ?? 0, aircraft_type: value.aircraftType, aircraft_category: value.aircraftCategory,
    spoken_callsign: value.spokenCallsign, stand: stand?.stand ?? value.stand,
    capabilities: value.capabilities, communication_type: (value.communicationType || "") as CommunicationType,
    eobt: cdmTime(value.eobt), tobt: cdmTime(cdm?.tobt ?? value.tobt), tsat: cdmTime(cdm?.tsat ?? value.tsat),
    ttot: cdmTime(cdm?.ttot ?? value.ttot), ctot: cdmTime(cdm?.ctot ?? value.ctot), eldt: "", aldt: "",
    aobt: cdmTime(value.aobt), asat: cdmTime(value.asat), asrt: cdmTime(value.asrt), tsac: cdmTime(value.tsac),
    tobt_set_by: value.tobtSetBy, phase: value.phase,
    status: value.operationalStatus, most_penalizing_airspace: value.mostPenalizingAirspace,
    ecfmp_id: value.ecfmpId, ctot_source: value.ctotSource,
    bay: value.bay, release_point: value.releasePoint, version: number(value.revision),
    sequence: number(value.sequence), next_controllers: value.nextControllers.map(position),
    previous_controllers: value.previousControllers.map(position), owner: position(value.ownerCid),
    pdc_state: (pdc?.state ?? value.pdcState ?? "NONE") as FrontendStrip["pdc_state"],
    pdc_request_remarks: pdc?.requestRemarks ?? value.pdcRequestRemarks,
    start_req: value.startRequested, marked: value.marked,
    runway_cleared: value.runwayCleared, runway_confirmed: value.runwayConfirmed,
    registration: value.registration, ob: false,
    unexpected_change_fields: value.unexpectedChangeFields,
    controller_modified_fields: value.controllerModifiedFields,
    is_manual: value.manual, persons_on_board: value.personsOnBoard,
    fpl_type: value.flightPlanType, language: value.language, has_fp: value.hasFlightPlan,
    validation_status: value.validation ? {
      issue_type: value.validation.issueType, message: value.validation.message,
      owning_position: value.validation.owningPosition, active: value.validation.active,
      activation_key: value.validation.activationKey,
      custom_action: value.validation.action?.action.case === "assignHoldingPoint"
        ? {label: value.validation.action.label, action_kind: "assign_holding_point"} : undefined,
    } : undefined,
  };
}

function tactical(value: EntityRecord["value"] & {case: "tacticalStrip"}, sessionID: number,
  position: (cid: string) => string): TacticalStrip {
  const item = value.value;
  return {id: number(item.id), session_id: sessionID, type: item.kind as TacticalStrip["type"],
    bay: item.bay, label: item.label || item.title, aircraft: item.aircraft || item.body,
    produced_by: position(item.producedBy), owner: position(item.ownerCid), marked: item.marked,
    sequence: number(item.sequence), timer_start: item.timerStartedAt ? iso(item.timerStartedAt) : null,
    confirmed: item.confirmed, confirmed_by: item.confirmedBy, created_at: iso(item.createdAt)};
}

function assignment(value: StandAssignment): FrontendStandAssignmentEntry {
  return {callsign: value.callsign, stand: value.stand, direction: "", stage: value.confirmed ? "confirmed" : "proposed",
    source: value.source, version: number(value.revision), expires_at: iso(value.expiresAt)};
}
function block(value: StandBlock): FrontendStandBlockEntry {
  return {stand: value.stand, block_type: "manual", reason: value.reason,
    created_by: value.actor, expires_at: iso(value.expiresAt), version: number(value.revision)};
}

export class RevisionGapError extends Error {}

// The browser keeps full replacements; legacy store events are view adapters
// produced locally after an atomic entity change has been applied.
export class FrontendProjection {
  private entities = new Map<string, EntitySnapshot>();
  private revisions = new Map<string, bigint>();
  private presence = new Map<string, {value: ClientPresence; seen: number}>();
  private positionAltitudes = new Map<string, number | null>();
  private positionRevisions = new Map<string, {epoch: bigint; revision: bigint}>();
  private presenceRevisions = new Map<string, bigint>();
  private sessionID = 0;
  private airport = "";
  private controllerPosition = "";
  private initialMetadata?: FrontendInitial;

  constructor(private emit: Emit) {}

  get entityRevisions(): ReadonlyMap<string, bigint> {
    const result = new Map<string, bigint>();
    for (const entity of this.entities.values()) {
      if (entity.value?.value.case === "strip") result.set(`strip.${entity.key}`, entity.revision);
      if (entity.value?.value.case === "tacticalStrip") result.set(`tactical.${entity.key}`, entity.revision);
      if (entity.value?.value.case === "pdcSequence") result.set(`pdc.${entity.key}`, entity.revision);
      if (entity.value?.value.case === "cdmState") result.set(`cdm.${entity.key}`, entity.revision);
      if (entity.value?.value.case === "standBlock") result.set(`standBlock.${entity.key}`, entity.revision);
    }
    return result;
  }
  get myPosition(): string { return this.controllerPosition; }
  ownerCid(callsign: string): string | undefined {
    const record = this.entities.get(`strip.${callsign.toUpperCase()}`)?.value?.value;
    return record?.case === "strip" ? record.value.ownerCid : undefined;
  }
  coordinationId(callsign: string): string | undefined {
    for (const record of this.entities.values()) if (record.value?.value.case === "coordination" &&
      record.value.value.value.callsign === callsign.toUpperCase()) return String(record.value.value.value.id);
    return undefined;
  }
  cidForPosition(position: string): string | undefined {
    for (const record of this.entities.values()) if (record.value?.value.case === "controller" &&
      (record.value.value.value.position === position || record.value.value.value.cid === position))
      return record.value.value.value.cid;
    return undefined;
  }

  initial(value: FrontendInitial): void {
    this.entities.clear(); this.presence.clear(); this.revisions.clear(); this.positionAltitudes.clear();
    this.positionRevisions.clear(); this.presenceRevisions.clear();
    this.sessionID = value.sessionId; this.airport = value.airport;
    this.controllerPosition = value.me?.position ?? "";
    this.revisions.set(`session.${value.sessionId}`, value.aggregateRevision);
    this.revisions.set(`airport.${value.airport}`, value.airportAggregateRevision);
    for (const entity of value.entities) this.entities.set(this.key(entity.value, entity.key), entity);
    if (value.taggedObservations.length) {
      for (const observation of value.taggedObservations) this.applyObservation(observation, false);
    } else {
      for (const position of value.positions) this.positionAltitudes.set(position.aircraftKey,
        position.observation.case === "position" ? position.observation.value.altitudeFeet : null);
      for (const client of value.clients) this.presence.set(client.connectionId, {value: client, seen: Date.now()});
    }
    // Keep presentation metadata, without retaining an obsolete entity/feed snapshot.
    this.initialMetadata = {...value, entities: [], taggedObservations: [], positions: [], clients: []};
    this.renderInitial(value);
  }

  restoreOptimisticState(): void {
    if (this.initialMetadata) this.renderInitial(this.initialMetadata);
  }

  private renderInitial(value: FrontendInitial): void {
    const all = [...this.entities.values()];
    const records = (kind: EntityRecord["value"]["case"]) => all.filter(e => e.value?.value.case === kind).map(e => e.value!.value);
    const session = records("session")[0];
    const s = session?.case === "session" ? session.value : undefined;
    const cdm = new Map(records("cdmState").flatMap(e => e.case === "cdmState" ? [[e.value.callsign, e.value] as const] : []));
    const pdc = new Map(records("pdcSequence").flatMap(e => e.case === "pdcSequence" ? [[e.value.callsign, e.value] as const] : []));
    const stands = new Map(records("standAssignment").flatMap(e => e.case === "standAssignment" ? [[e.value.callsign, e.value] as const] : []));
    const controllers = records("controller").flatMap(e => e.case === "controller" && this.online(e.value.cid) ? [controller(e.value)] : []);
    const strips = records("strip").flatMap(e => e.case === "strip" ? [strip(e.value, cdm.get(e.value.callsign), pdc.get(e.value.callsign), stands.get(e.value.callsign), cid => this.position(cid), this.positionAltitudes.get(e.value.callsign))] : []);
    const tacticalStrips = records("tacticalStrip").flatMap(e => e.case === "tacticalStrip" ? [tactical(e, value.sessionId, cid => this.position(cid))] : []);
    const coordinations = records("coordination").flatMap(e => e.case === "coordination" ? [{callsign: e.value.callsign,
      from: this.position(e.value.fromCid), to: this.position(e.value.toCid), is_tag_request: e.value.status === "TAG"}] : []);
    const messages = records("frontendMessage").flatMap(e => e.case === "frontendMessage" ? [{id: number(e.value.id), sender: e.value.sender, text: e.value.text, is_broadcast: e.value.broadcast, recipients: e.value.recipients}] : []);
    const standAssignments = records("standAssignment").flatMap(e => e.case === "standAssignment" ? [assignment(e.value)] : []);
    const standBlocks = records("standBlock").flatMap(e => e.case === "standBlock" ? [block(e.value)] : []);
    const me = value.me ? controller(value.me) : {callsign: "", position: "", identifier: "", section: "", owned_sectors: []};
    this.emit({type: EventType.FrontendInitial, controllers, strips, tactical_strips: tacticalStrips,
      me, airport: value.airport, layout: value.layoutId || s?.layoutId || "", callsign: me.callsign,
      runway_setup: runwaySetup(s), coordinations, messages,
      available_sids: value.availableSids.map(item => ({name: item.name, runway: item.runway})),
      initial_cfl_by_runway: Object.fromEntries(value.initialCflByRunway.map(item => [item.runway, item.altitudeFeet])),
      transition_altitude: value.transitionAltitudeFeet, read_only: value.readOnly,
      position_available: value.positionAvailable, stand_assignment_enabled: value.standAssignmentEnabled,
      stand_assignments: standAssignments, stand_blocks: standBlocks,
      capabilities: {aman_fmp: value.amanFmp}} satisfies FrontendInitialEvent);
    const aman = amanState(value.airport, this.entities.values());
    if (aman) this.emit(aman as WebSocketEvent);
  }

  delta(value: FrontendDelta): void {
    const target = value.aggregate?.target;
    const key = target?.case === "session" ? `session.${target.value.id}` : target?.case === "airport" ? `airport.${target.value.icao}` : "";
    if (!key || (key.startsWith("session.") && key !== `session.${this.sessionID}`) || (key.startsWith("airport.") && key !== `airport.${this.airport}`)) throw new RevisionGapError("unexpected aggregate");
    const previous = this.revisions.get(key);
    if (previous === undefined || value.aggregateRevision > previous + 1n) throw new RevisionGapError("frontend revision gap");
    if (value.aggregateRevision <= previous) return;
    for (const item of value.changes) {
      if (item.operation.case === "upsert") {
        const entity = {key: item.key, revision: item.revision, value: item.operation.value} as EntitySnapshot;
        const slot = this.key(entity.value, entity.key);
        const created = !this.entities.has(slot);
        this.entities.set(slot, entity);
        this.publish(item.operation.value, created);
      } else if (item.operation.case === "delete") {
        this.remove(item.operation.value.kind, item.key);
      } else throw new RevisionGapError("missing entity operation");
    }
    this.revisions.set(key, value.aggregateRevision);
    if (key.startsWith("airport.") && value.changes.some(c => c.operation.case === "upsert" &&
      (c.operation.value.value.case === "amanAirport" || c.operation.value.value.case === "amanFlight" || c.operation.value.value.case === "amanCoordination"))) {
      const aman = amanState(this.airport, this.entities.values());
      if (aman) this.emit(aman as WebSocketEvent);
    }
  }

  observation(value: FrontendObservation): void {
    this.applyObservation(value, true);
  }

  private applyObservation(value: FrontendObservation, emit: boolean): void {
    if (value.value.case === "position") {
      const position = value.value.value;
      const previous = this.positionRevisions.get(position.aircraftKey);
      if (previous && (position.ownerEpoch < previous.epoch ||
        (position.ownerEpoch === previous.epoch && value.sourceRevision <= previous.revision))) return;
      this.positionRevisions.set(position.aircraftKey, {epoch: position.ownerEpoch, revision: value.sourceRevision});
      const altitude = !value.stale && !value.removed && position.observation.case === "position"
        ? position.observation.value.altitudeFeet : null;
      const currentStrip = this.entities.get(`strip.${position.aircraftKey}`)?.value?.value;
      const previousAltitude = this.positionAltitudes.has(position.aircraftKey)
        ? this.positionAltitudes.get(position.aircraftKey) ?? null
        : currentStrip?.case === "strip" ? currentStrip.value.positionAltitudeFeet ?? null : null;
      this.positionAltitudes.set(position.aircraftKey, altitude);
      // Observations must not replay persisted bay/order over optimistic moves.
      // A tombstone clears only the altitude overlay, without removing the strip.
      if (emit && altitude !== previousAltitude && currentStrip?.case === "strip") {
        this.emit(legacy(EventType.FrontendPositionAltitude,
          {callsign: position.aircraftKey, position_altitude: altitude}));
      }
    } else if (value.value.case === "presence") {
      const client = value.value.value.present;
      if (client.case !== "client") return;
      const prior = this.presenceRevisions.get(client.value.connectionId);
      if (prior !== undefined && value.sourceRevision <= prior) return;
      this.presenceRevisions.set(client.value.connectionId, value.sourceRevision);
      if (value.removed || value.stale) {
        this.presence.delete(client.value.connectionId);
        if (emit && !this.online(client.value.cid)) this.emit(legacy(EventType.FrontendControllerOffline,
          {callsign: client.value.callsign, position: client.value.position, identifier: client.value.position}));
        return;
      }
      this.presence.set(client.value.connectionId, {value: client.value, seen: Date.now()});
      if (!emit) return;
      const entity = [...this.entities.values()].find(e => e.value?.value.case === "controller" && e.value.value.value.cid === client.value.cid);
      if (entity?.value?.value.case === "controller") this.emit(legacy(EventType.FrontendControllerOnline, controller(entity.value.value.value)));
    }
  }

  expirePresence(now = Date.now()): void {
    for (const [id, item] of this.presence) if (now - item.seen > 10000) {
      this.presence.delete(id);
      if (![...this.presence.values()].some(other => other.value.cid === item.value.cid))
        this.emit(legacy(EventType.FrontendControllerOffline, {callsign: item.value.callsign, position: item.value.position, identifier: item.value.position}));
    }
  }

  private online(cid: string): boolean { return [...this.presence.values()].some(item => item.value.cid === cid); }
  private position(cid: string): string {
    if (!cid) return "";
    const controller = this.entities.get(`controller.${cid}`)?.value?.value;
    return controller?.case === "controller" ? controller.value.position : cid;
  }
  private key(value: EntityRecord | undefined, key: string): string { return `${value?.value.case ?? "unknown"}.${key}`; }

  private publish(record: EntityRecord, created = false): void {
    const item = record.value;
    switch (item.case) {
      case "strip": this.publishStrip(item.value.callsign); return;
      case "session": this.emit(legacy(EventType.FrontendRunWayConfiguration, {runway_setup: runwaySetup(item.value)})); return;
      case "controller": if (this.online(item.value.cid)) this.emit(legacy(EventType.FrontendControllerUpdate, controller(item.value))); return;
      case "coordination": this.emit(legacy(item.value.status === "TAG" ? EventType.FrontendCoordinationTagRequestBroadcast : EventType.FrontendCoordinationTransferBroadcast,
        {callsign: item.value.callsign, from: this.position(item.value.fromCid), to: this.position(item.value.toCid)})); return;
      case "tacticalStrip": this.emit(legacy(created ? EventType.FrontendTacticalStripCreated : EventType.FrontendTacticalStripUpdated, {strip: tactical(item, this.sessionID, cid => this.position(cid))})); return;
      case "standAssignment": this.emit(legacy(EventType.FrontendStandAssignmentUpdate, {assignment: assignment(item.value)})); return;
      case "standBlock": this.emit(legacy(EventType.FrontendStandBlockUpdate, {stand: item.value.stand, block: block(item.value)})); return;
      case "pdcSequence": this.emit(legacy(EventType.FrontendPdcStateChange, {callsign: item.value.callsign, state: item.value.state, pdc_request_remarks: item.value.requestRemarks})); return;
      case "cdmState": {
        const currentStrip = this.entities.get(`strip.${item.value.callsign}`)?.value?.value;
        this.emit(legacy(EventType.FrontendCdmData, {callsign: item.value.callsign,
          eobt: currentStrip?.case === "strip" ? cdmTime(currentStrip.value.eobt) : "",
          tobt: cdmTime(item.value.tobt), tsat: cdmTime(item.value.tsat),
          ttot: cdmTime(item.value.ttot), ctot: cdmTime(item.value.ctot),
          ...(currentStrip?.case === "strip" ? {
            tobt_set_by: currentStrip.value.tobtSetBy,
            aobt: cdmTime(currentStrip.value.aobt), asat: cdmTime(currentStrip.value.asat),
            asrt: cdmTime(currentStrip.value.asrt), tsac: cdmTime(currentStrip.value.tsac),
            status: currentStrip.value.operationalStatus,
            most_penalizing_airspace: currentStrip.value.mostPenalizingAirspace,
            ecfmp_id: currentStrip.value.ecfmpId, ctot_source: currentStrip.value.ctotSource,
            phase: currentStrip.value.phase,
          } : {})})); return;
      }
      case "frontendMessage": this.emit(legacy(EventType.FrontendMessageReceived, {id: number(item.value.id), sender: item.value.sender, text: item.value.text, is_broadcast: item.value.broadcast, recipients: item.value.recipients})); return;
      case "atis": this.emit(legacy(EventType.FrontendAtisUpdate, {metar: item.value.text, arr_atis_code: item.value.code, dep_atis_code: item.value.code})); return;
      default: return;
    }
  }

  private publishStrip(callsign: string): void {
    const item = this.entities.get(`strip.${callsign}`)?.value?.value;
    if (item?.case !== "strip") return;
    const cdm = this.entities.get(`cdmState.${callsign}`)?.value?.value;
    const pdc = this.entities.get(`pdcSequence.${callsign}`)?.value?.value;
    const stand = this.entities.get(`standAssignment.${callsign}`)?.value?.value;
    this.emit(legacy(EventType.FrontendStripUpdate, strip(item.value,
      cdm?.case === "cdmState" ? cdm.value : undefined,
      pdc?.case === "pdcSequence" ? pdc.value : undefined,
      stand?.case === "standAssignment" ? stand.value : undefined,
      cid => this.position(cid), this.positionAltitudes.get(callsign))));
  }

  private remove(kind: EntityKind, key: string): void {
    const name = EntityKind[kind].toLowerCase().replace(/_([a-z])/g, (_, letter: string) => letter.toUpperCase());
    const removed = this.entities.get(`${name}.${key}`)?.value?.value;
    this.entities.delete(`${name}.${key}`);
    switch (kind) {
      case EntityKind.STRIP: this.emit(legacy(EventType.FrontendAircraftDisconnect, {callsign: key})); break;
      case EntityKind.TACTICAL_STRIP: this.emit(legacy(EventType.FrontendTacticalStripDeleted, {id: Number(key), bay: ""})); break;
      case EntityKind.STAND_ASSIGNMENT: this.emit(legacy(EventType.FrontendStandAssignmentRemoved, {callsign: key})); break;
      case EntityKind.STAND_BLOCK: this.emit(legacy(EventType.FrontendStandBlockUpdate, {stand: key, block: null})); break;
      case EntityKind.COORDINATION: if (removed?.case === "coordination") this.emit(legacy(EventType.FrontendCoordinationFreeBroadcast,
        {callsign: removed.value.callsign})); break;
    }
  }
}
