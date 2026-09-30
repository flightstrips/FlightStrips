import {create} from "@bufbuild/protobuf";
import {TimestampSchema} from "@bufbuild/protobuf/wkt";
import {ActionType, type FrontendSendEvent, type StripRef} from "./models";
import {ClientCommandSchema, type ClientCommand} from "./generated/cluster/v1/wire_pb";
import type {AMANCommandMessage} from "./aman";

type Domain = Exclude<ClientCommand["action"]["case"], undefined>;
type Payload = Record<string, unknown>;

// create() hydrates every nested generated message and oneof. Legacy UI
// intents are translated at this boundary; the socket only sees generated
// ClientCommand messages.
const command = (domain: Domain, value: Payload): ClientCommand =>
  create(ClientCommandSchema, {action: {case: domain, value}} as never);
const change = (name: string, value: Payload = {}) => ({change: {case: name, value}});
const send = (name: string, value: Payload = {}) => ({send: {case: name, value}});
const createCase = (name: string, value: Payload = {}) => ({create: {case: name, value}});
const bigint = (value: number) => BigInt(value);

function timestamp(value: string) {
  const millis = Date.parse(value);
  if (!Number.isFinite(millis)) throw new Error("invalid timestamp");
  return create(TimestampSchema, {
    seconds: BigInt(Math.floor(millis / 1000)),
    nanos: Math.round((millis % 1000) * 1e6),
  });
}

function stripRef(ref: StripRef | null) {
  if (!ref) return undefined;
  if (ref.kind === "flight" && ref.callsign) return {identity: {case: "flightCallsign", value: ref.callsign}};
  if (ref.kind === "tactical" && ref.id !== undefined) return {identity: {case: "tacticalId", value: bigint(ref.id)}};
  throw new Error("invalid strip reference");
}

export interface EncodedAction {
  requestId: string;
  action: ClientCommand;
  expectedEntityRevision?: bigint;
}

export interface ActionContext {
  ownerCid(callsign: string): string | undefined;
  coordinationId(callsign: string): string | undefined;
  cidForPosition(position: string): string | undefined;
}

export function encodeAction(event: FrontendSendEvent, revisions: ReadonlyMap<string, bigint>, context?: ActionContext): EncodedAction {
  if (event.type.startsWith("aman.")) return encodeAMAN(event as AMANCommandMessage);
  if (event.type === ActionType.FrontendToken) throw new Error("authentication is a separate frame");
  const requestId = crypto.randomUUID();
  let action: ClientCommand;
  let revisionKey = "";
  switch (event.type) {
    case ActionType.FrontendMove:
      action = command("strip", {callsign: event.callsign, ...change("move", {bay: event.bay, clearance: event.clearance, confirmedRemoval: event.confirmed_removal})}); break;
    case ActionType.FrontendGenerateSquawk:
      action = command("strip", {callsign: event.callsign, ...change("generateSquawk")}); break;
    case ActionType.FrontendUpdateStripData:
      action = command("strip", {callsign: event.callsign, ...change("updateData", {
        sid: event.sid, eobt: event.eobt ? timestamp(event.eobt) : undefined, route: event.route,
        heading: event.heading, altitudeFeet: event.altitude, stand: event.stand, runway: event.runway,
        onBlock: event.ob, remarks: event.remarks, aircraftType: event.aircraft_type,
      })}); break;
    case ActionType.FrontendUpdateOrder:
      action = command("strip", {callsign: event.callsign, ...change("setOrder", {insertAfter: stripRef(event.insert_after)})}); break;
    case ActionType.FrontendSendMessage:
      action = command("message", {...send("broadcast", {text: event.text, recipients: event.recipients})}); break;
    case ActionType.FrontendSendPrivateMessage:
      action = command("message", {...send("privateMessage", {targetCid: event.callsign, text: event.message})}); break;
    case ActionType.FrontendCdmReady:
      action = command("cdm", {callsign: event.callsign, ...change("setReady", {ready: true})}); break;
    case ActionType.FrontendReleasePoint:
      action = command("strip", {callsign: event.callsign, ...change("setReleasePoint", {point: event.release_point})}); break;
    case ActionType.FrontendStartReq:
      action = command("strip", {callsign: event.callsign, ...change("setStartRequested", {requested: event.start_req})}); break;
    case ActionType.FrontendMarked:
      action = command("strip", {callsign: event.callsign, ...change("setMarked", {marked: event.marked})}); break;
    case ActionType.FrontendRunwayClearance:
      action = command("strip", {callsign: event.callsign, ...change("setRunwayCleared", {value: true})}); break;
    case ActionType.FrontendRunwayConfirmation:
      action = command("strip", {callsign: event.callsign, ...change("setRunwayConfirmed", {value: true})}); break;
    case ActionType.FrontendIssuePdcClearanceRequest:
      action = command("pdc", {callsign: event.callsign, ...change("issue", {requestRemarks: event.remarks ?? ""})}); break;
    case ActionType.FrontendRevertToVoiceRequest:
      action = command("pdc", {callsign: event.callsign, ...change("revertToVoice")}); break;
    case ActionType.FrontendCoordinationTransferRequest:
      action = command("coordination", {callsign: event.callsign, ...change("transfer", {toCid: event.to ? context?.cidForPosition(event.to) ?? event.to : ""})}); break;
    case ActionType.FrontendCoordinationAssumeRequest:
      action = command("coordination", {callsign: event.callsign, ...change("assume")}); break;
    case ActionType.FrontendCoordinationForceAssumeRequest:
      action = command("coordination", {callsign: event.callsign, ...change("forceAssume", {fromCid: context?.ownerCid(event.callsign) ?? ""})}); break;
    case ActionType.FrontendCoordinationFreeRequest:
      action = command("coordination", {callsign: event.callsign, ...change("free")}); break;
    case ActionType.FrontendCoordinationCancelTransferRequest:
      action = command("coordination", {callsign: event.callsign, ...change("cancel", {transferId: context?.coordinationId(event.callsign) ?? ""})}); break;
    case ActionType.FrontendCoordinationTagRequest:
      action = command("coordination", {callsign: event.callsign, ...change("tag", {toCid: context?.ownerCid(event.callsign) ?? "", tag: ""})}); break;
    case ActionType.FrontendCoordinationAcceptTagRequest:
      action = command("coordination", {callsign: event.callsign, ...change("acceptTag", {requestId: context?.coordinationId(event.callsign) ?? ""})}); break;
    case ActionType.FrontendCreateTacticalStrip:
      action = command("tactical", {stripId: 0n, ...change("create", {kind: event.strip_type,
        title: event.label, body: event.aircraft, bay: event.bay, label: event.label, aircraft: event.aircraft})}); break;
    case ActionType.FrontendDeleteTacticalStrip:
      action = command("tactical", {stripId: bigint(event.id), ...change("delete")}); break;
    case ActionType.FrontendConfirmTacticalStrip:
      action = command("tactical", {stripId: bigint(event.id), ...change("confirm")}); break;
    case ActionType.FrontendForceAssumeTacticalStrip:
      action = command("tactical", {stripId: bigint(event.id), ...change("forceAssume")}); break;
    case ActionType.FrontendMarkTacticalStrip:
      action = command("tactical", {stripId: bigint(event.id), ...change("mark", {marked: event.marked})}); break;
    case ActionType.FrontendStartTacticalTimer:
      action = command("tactical", {stripId: bigint(event.id), ...change("startTimer")}); break;
    case ActionType.FrontendMoveTacticalStrip:
      action = command("tactical", {stripId: bigint(event.id), ...change("move", {bay: event.bay, insertAfter: stripRef(event.insert_after)})}); break;
    case ActionType.FrontendAcknowledgeUnexpectedChange:
      action = command("validation", {callsign: event.callsign, ...change("acknowledgeUnexpectedChange", {fieldName: event.field_name})}); break;
    case ActionType.FrontendAcknowledgeValidationStatus:
      action = command("validation", {callsign: event.callsign, ...change("acknowledge", {activationKey: event.activation_key})}); break;
    case ActionType.FrontendClxOverrideValidation:
      action = command("validation", {callsign: event.callsign, ...change("clxOverride", {overrideKey: event.override_key})}); break;
    case ActionType.FrontendClxUpdateTobt:
      action = command("validation", {callsign: event.callsign, ...change("updateTobt", {value: roundedClxTobt()})}); break;
    case ActionType.FrontendCreateManualFPL:
      action = command("flightPlan", {callsign: event.callsign, ...createCase("manual", {
        destination: event.ades, sid: event.sid, squawk: event.ssr, eobt: timestamp(event.eobt),
        aircraftType: event.aircraft_type, flightLevel: event.fl, route: event.route,
        stand: event.stand, departureRunway: event.rwy_dep,
      })}); break;
    case ActionType.FrontendCreateVFRFPL:
      action = command("flightPlan", {callsign: event.callsign, ...createCase("vfr", {
        aircraftType: event.aircraft_type, personsOnBoard: event.persons_on_board, squawk: event.ssr,
        flightPlanType: event.fpl_type, language: event.language, remarks: event.remarks,
      })}); break;
    case ActionType.FrontendMissedApproach:
      action = command("strip", {callsign: event.callsign, ...change("missedApproach")}); break;
    case ActionType.FrontendUpdateRunwayStatus:
      action = command("session", {...change("updateRunwayStatus", {pair: event.pair, status: event.status})}); break;
    case ActionType.FrontendStandOccupy:
      action = command("stand", {stand: event.stand, ...change("createBlock", {reason: event.reason})}); break;
    case ActionType.FrontendStandVacate:
      action = command("stand", {stand: event.stand, ...change("removeBlock")}); break;
    case ActionType.FrontendStandAssignmentManualRequest:
      action = command("stand", {callsign: event.callsign, stand: event.stand, ...change("manual", {reason: ""})}); break;
    case ActionType.FrontendStandAssignmentAutomaticRequest:
      action = command("stand", {callsign: event.callsign, ...change("automatic")}); break;
    case ActionType.FrontendStandAssignmentConfirmedOverride:
      action = command("stand", {callsign: event.callsign, stand: event.stand, ...change("confirmOverride")}); break;
    case ActionType.FrontendStandAssignmentAcknowledge:
      action = command("stand", {callsign: event.callsign, ...change("acknowledge")}); break;
    case ActionType.FrontendStandBlockCreate:
      action = command("stand", {stand: event.stand, ...change("createBlock", {reason: event.reason})}); break;
    case ActionType.FrontendStandBlockRemove:
      action = command("stand", {stand: event.stand, ...change("removeBlock")}); break;
    default:
      throw new Error(`unsupported action: ${(event as {type: string}).type}`);
  }
  if ("callsign" in event && typeof event.callsign === "string") revisionKey = `strip.${event.callsign.toUpperCase()}`;
  if ("id" in event && typeof event.id === "number") revisionKey = `tactical.${event.id}`;
  const expectedEntityRevision = action.action.case === "stand"
    ? ("version" in event && typeof event.version === "number" ? BigInt(event.version)
      : "stand" in event && typeof event.stand === "string"
        ? revisions.get(`standBlock.${event.stand.toUpperCase()}`) ?? 0n : 0n)
    : action.action.case === "pdc" && "callsign" in event && typeof event.callsign === "string"
      ? revisions.get(`pdc.${event.callsign.toUpperCase()}`) ?? 0n
    : action.action.case === "cdm" && "callsign" in event && typeof event.callsign === "string"
      ? revisions.get(`cdm.${event.callsign.toUpperCase()}`) ?? 0n
    : action.action.case === "tactical" ? revisions.get(revisionKey) ?? 0n
    : revisions.get(revisionKey);
  return {requestId, action, expectedEntityRevision};
}

function encodeAMAN(event: AMANCommandMessage): EncodedAction {
  const data = event.data as unknown as Record<string, unknown>;
  const kind = event.type.slice(5).replace(/_([a-z])/g, (_, letter: string) => letter.toUpperCase());
  const expectedAirportRevision = BigInt(Number(data.expected_revision));
  const camel = (key: string) => key.replace(/_([a-z])/g, (_, letter: string) => letter.toUpperCase());
  const field = Object.fromEntries(Object.entries(data).map(([key, value]) => [camel(key), value]));
  delete field.commandId;
  delete field.expectedRevision;
  const timestampFields = ["effective_at", "manual_eta", "feeder_eta", "detected_at", "start", "end", "slot_time"];
  for (const key of timestampFields) if (typeof field[camel(key)] === "string") field[camel(key)] = timestamp(field[camel(key)] as string);
  if (event.type === "aman.set_manual_eta") {field.value = field.manualEta; delete field.manualEta;}
  if (event.type === "aman.set_manual_feeder_eta") {field.value = field.feederEta; delete field.feederEta;}
  if (event.type === "aman.move_flight") field.neighbor = "before_callsign" in data
    ? {case: "beforeCallsign", value: data.before_callsign} : {case: "afterCallsign", value: data.after_callsign};
  if (event.type === "aman.submit_coordination_request") {
    field.coordinationRequestId = crypto.randomUUID();
    field.request = data.kind === "speed" ? {case: "speed", value: {requested: data.requested}}
      : {case: "routeDirect", value: {route: data.route, directTo: data.direct_to}};
  }
  if (event.type === "aman.create_gap") field.extent = data.end
    ? {case: "end", value: timestamp(data.end as string)} : {case: "slotCount", value: data.slot_count};
  if (event.type === "aman.create_runway_closure") field.placement = data.start
    ? {case: "start", value: timestamp(data.start as string)} : {case: "afterCallsign", value: data.after_callsign};
  if (event.type === "aman.accept_coordination_request" || event.type === "aman.reject_coordination_request") {
    field.coordinationRequestId = field.requestId;
    delete field.requestId;
  }
  const action = command("aman", {expectedAirportRevision, ...change(kind, field)});
  return {requestId: String(data.command_id), action};
}

function roundedClxTobt() {
  const target = new Date(Date.now() + 15 * 60_000);
  target.setUTCSeconds(0, 0);
  target.setUTCMinutes(Math.ceil(target.getUTCMinutes() / 5) * 5);
  return timestamp(target.toISOString());
}
