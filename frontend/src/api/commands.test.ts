import {describe, expect, it} from "vitest";
import {toBinary, fromBinary} from "@bufbuild/protobuf";
import {ActionType, Bay, type FrontendSendEvent} from "./models";
import {ClientCommandSchema} from "./generated/cluster/v1/wire_pb";
import {encodeAction} from "./commands";

const flight = {callsign: "SAS123"};
const fixtures: FrontendSendEvent[] = [
  {type: ActionType.FrontendMove, ...flight, bay: Bay.Cleared},
  {type: ActionType.FrontendGenerateSquawk, ...flight},
  {type: ActionType.FrontendUpdateStripData, ...flight, sid: "KEMAX", route: "DCT"},
  {type: ActionType.FrontendUpdateOrder, ...flight, insert_after: {kind: "flight", callsign: "SAS124"}},
  {type: ActionType.FrontendSendMessage, text: "hello", recipients: ["123456"]},
  {type: ActionType.FrontendSendPrivateMessage, callsign: "123456", message: "hello"},
  {type: ActionType.FrontendCdmReady, ...flight},
  {type: ActionType.FrontendReleasePoint, ...flight, release_point: "A"},
  {type: ActionType.FrontendStartReq, ...flight, start_req: true},
  {type: ActionType.FrontendMarked, ...flight, marked: true},
  {type: ActionType.FrontendRunwayClearance, ...flight},
  {type: ActionType.FrontendRunwayConfirmation, ...flight},
  {type: ActionType.FrontendIssuePdcClearanceRequest, ...flight, remarks: "cleared"},
  {type: ActionType.FrontendRevertToVoiceRequest, ...flight},
  {type: ActionType.FrontendCoordinationTransferRequest, ...flight, to: "123456"},
  {type: ActionType.FrontendCoordinationAssumeRequest, ...flight},
  {type: ActionType.FrontendCoordinationForceAssumeRequest, ...flight},
  {type: ActionType.FrontendCoordinationFreeRequest, ...flight},
  {type: ActionType.FrontendCoordinationCancelTransferRequest, ...flight},
  {type: ActionType.FrontendCoordinationTagRequest, ...flight},
  {type: ActionType.FrontendCoordinationAcceptTagRequest, ...flight},
  {type: ActionType.FrontendCreateTacticalStrip, strip_type: "START", bay: Bay.Cleared, label: "START", aircraft: "SAS123"},
  {type: ActionType.FrontendDeleteTacticalStrip, id: 1},
  {type: ActionType.FrontendConfirmTacticalStrip, id: 1},
  {type: ActionType.FrontendForceAssumeTacticalStrip, id: 1},
  {type: ActionType.FrontendMarkTacticalStrip, id: 1, marked: true},
  {type: ActionType.FrontendStartTacticalTimer, id: 1},
  {type: ActionType.FrontendMoveTacticalStrip, id: 1, insert_after: null},
  {type: ActionType.FrontendAcknowledgeUnexpectedChange, ...flight, field_name: "sid"},
  {type: ActionType.FrontendAcknowledgeValidationStatus, ...flight, activation_key: "key"},
  {type: ActionType.FrontendClxOverrideValidation, ...flight, override_key: "key"},
  {type: ActionType.FrontendClxUpdateTobt, ...flight},
  {type: ActionType.FrontendCreateManualFPL, ...flight, ades: "EKCH", sid: "KEMAX", ssr: "1000", eobt: "2026-09-29T12:00:00Z", aircraft_type: "A320", fl: "350", route: "DCT", stand: "A1", rwy_dep: "22L"},
  {type: ActionType.FrontendCreateVFRFPL, ...flight, aircraft_type: "C172", persons_on_board: 2, ssr: "7000", fpl_type: "V", language: "EN", remarks: ""},
  {type: ActionType.FrontendMissedApproach, ...flight},
  {type: ActionType.FrontendUpdateRunwayStatus, pair: "22L/04R", status: "OPEN"},
  {type: ActionType.FrontendStandOccupy, stand: "A1", reason: "manual"},
  {type: ActionType.FrontendStandVacate, stand: "A1"},
  {type: ActionType.FrontendStandAssignmentManualRequest, ...flight, stand: "A1", version: 1},
  {type: ActionType.FrontendStandAssignmentAutomaticRequest, ...flight, version: 1},
  {type: ActionType.FrontendStandAssignmentConfirmedOverride, ...flight, stand: "A1", version: 1, reason: "manual"},
  {type: ActionType.FrontendStandAssignmentAcknowledge, ...flight, version: 1},
  {type: ActionType.FrontendStandBlockCreate, stand: "A1", reason: "closed"},
  {type: ActionType.FrontendStandBlockRemove, stand: "A1", block_id: 1, version: 1},
];

describe("frontend command coverage", () => {
  it.each(fixtures)("encodes $type as a generated binary oneof", event => {
    const {action, requestId} = encodeAction(event, new Map());
    const decoded = fromBinary(ClientCommandSchema, toBinary(ClientCommandSchema, action));
    expect(requestId).toMatch(/^[0-9a-f-]{36}$/);
    expect(decoded.action.case).not.toBeUndefined();
  });
  it("uses typed ownership and entity revisions for coordination, PDC and tactical actions", () => {
    const context = {ownerCid: () => "CID-OWNER", coordinationId: () => "42", cidForPosition: () => "CID-TARGET"};
    const revisions = new Map([["strip.SAS123", 3n], ["pdc.SAS123", 4n], ["tactical.1", 5n]]);
    const tag = encodeAction({type: ActionType.FrontendCoordinationTagRequest, ...flight}, revisions, context);
    expect(tag.expectedEntityRevision).toBe(3n);
    expect(tag.action.action.case === "coordination" && tag.action.action.value.change.case === "tag"
      ? tag.action.action.value.change.value.toCid : "").toBe("CID-OWNER");
    const cancel = encodeAction({type: ActionType.FrontendCoordinationCancelTransferRequest, ...flight}, revisions, context);
    expect(cancel.action.action.case === "coordination" && cancel.action.action.value.change.case === "cancel"
      ? cancel.action.action.value.change.value.transferId : "").toBe("42");
    const pdc = encodeAction({type: ActionType.FrontendRevertToVoiceRequest, ...flight}, revisions);
    expect(pdc.expectedEntityRevision).toBe(4n);
    const tactical = encodeAction({type: ActionType.FrontendDeleteTacticalStrip, id: 1}, revisions);
    expect(tactical.expectedEntityRevision).toBe(5n);
    const block = encodeAction({type: ActionType.FrontendStandBlockCreate, stand: "A1", reason: "closed"}, revisions);
    expect(block.expectedEntityRevision).toBe(0n);
    const unblock = encodeAction({type: ActionType.FrontendStandBlockRemove, stand: "A1", block_id: 1, version: 6}, revisions);
    expect(unblock.expectedEntityRevision).toBe(6n);
  });

  const at = "2026-09-29T12:00:00Z";
  const amanFixtures: Array<[string, Record<string, unknown>]> = [
    ["move_flight", {callsign: "SAS123", runway_group_id: "22L", before_callsign: "SAS124"}],
    ...["lock_flight", "unlock_flight", "desequence_flight", "resume_flight", "remove_flight",
      "accept_teta", "keep_fpl_eta", "reset_teta_override", "reset_manual_feeder_eta", "recompute_flight"]
      .map(name => [name, {callsign: "SAS123"}] as [string, Record<string, unknown>]),
    ["set_rate", {runway_group_id: "22L", arrivals_per_hour: 30, effective_at: at}],
    ["select_runway_group", {runway_group_id: "22L", effective_at: at}],
    ["set_active_runway_groups", {runway_group_ids: ["22L"]}],
    ["set_manual_eta", {callsign: "SAS123", manual_eta: at}],
    ["set_manual_feeder_eta", {callsign: "SAS123", feeder_eta: at}],
    ["change_runway", {callsign: "SAS123", runway_group_id: "22L"}],
    ["report_go_around", {callsign: "SAS123", detected_at: at}],
    ["confirm_go_around", {callsign: "SAS123", episode_id: "episode"}],
    ["reject_go_around", {callsign: "SAS123", episode_id: "episode"}],
    ["create_gap", {runway_group_id: "22L", start: at, end: "2026-09-29T12:05:00Z", label: "gap"}],
    ["remove_gap", {runway_group_id: "22L", gap_id: "gap"}],
    ["create_runway_closure", {runway_group_id: "22L", start: at, reason: "closed"}],
    ["remove_runway_closure", {runway_group_id: "22L", closure_id: "closure", reason: "open"}],
    ["create_capacity_reservation", {runway_group_id: "22L", after_callsign: "SAS123", reason: "reserve"}],
    ["remove_capacity_reservation", {runway_group_id: "22L", reservation_id: "reservation", reason: "remove"}],
    ["place_flight_at_time", {callsign: "SAS123", runway_group_id: "22L", slot_time: at, allow_gap: false}],
    ["submit_coordination_request", {callsign: "SAS123", kind: "speed", requested: "180"}],
    ["accept_coordination_request", {request_id: "nested"}],
    ["reject_coordination_request", {request_id: "nested", reason: "unable"}],
  ];
  it.each(amanFixtures)("encodes aman.%s with its distinct nested oneof", (name, data) => {
    const event = {type: `aman.${name}`, version: 1, data: {
      command_id: crypto.randomUUID(), expected_revision: 7, ...data,
    }} as unknown as FrontendSendEvent;
    const encoded = encodeAction(event, new Map());
    const decoded = fromBinary(ClientCommandSchema, toBinary(ClientCommandSchema, encoded.action));
    expect(decoded.action.case).toBe("aman");
    if (decoded.action.case !== "aman") return;
    expect(decoded.action.value.change.case).toBe(name.replace(/_([a-z])/g, (_, letter: string) => letter.toUpperCase()));
    if (name === "submit_coordination_request" && decoded.action.value.change.case === "submitCoordinationRequest") {
      expect(decoded.action.value.change.value.coordinationRequestId).not.toBe(encoded.requestId);
    }
  });
});
