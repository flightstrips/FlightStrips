import {readFileSync} from "node:fs";
import {resolve} from "node:path";
import {describe, expect, it} from "vitest";

import {
  getActiveAMANRunwayGroups,
  isAMANCommandRejectedEvent,
  isAMANStateEvent,
  replaceAMANState,
  type AMANStateEvent,
} from "./aman";

const golden = JSON.parse(readFileSync(
  resolve(process.cwd(), "../backend/pkg/events/frontend/testdata/aman-state-v1.json"),
  "utf8",
)) as unknown;

function replacement(revision: number, callsign = "SAS123"): AMANStateEvent {
  const event = structuredClone(golden) as AMANStateEvent;
  event.data.revision = revision;
  event.data.flights[0].callsign = callsign;
  event.data.flights[0].slot!.revision = revision;
  return event;
}

describe("AMAN V1 full replacement contract", () => {
  it("accepts the shared Go/TypeScript golden fixture", () => {
    expect(isAMANStateEvent(golden)).toBe(true);
  });

  it.each(["none", "superstable", "manual", "tma"] as const)("accepts the V1 freeze reason %s", (reason) => {
    const event = replacement(8);
    event.data.flights[0].freeze_reason = reason;
    expect(isAMANStateEvent(event)).toBe(true);
    expect(replaceAMANState(null, event).state?.flights[0].freeze_reason).toBe(reason);
  });

  it("rejects an unknown freeze reason without retaining stale AMAN state", () => {
    const current = replaceAMANState(null, replacement(7)).state;
    const event = replacement(8) as unknown as {data: {flights: Array<{freeze_reason: string}>}};
    event.data.flights[0].freeze_reason = "future";

    expect(replaceAMANState(current, event as unknown as AMANStateEvent)).toEqual({
      state: null,
      status: "degraded",
      error: "invalid_aman_state",
      accepted: false,
    });
  });

  it("accepts a legacy V1 replacement without the TMT extension", () => {
    const legacy = replacement(8);
    delete legacy.data.traffic_prediction;
    delete legacy.data.holding_information;
    delete legacy.data.flights[0].star_family;
    delete legacy.data.flights[0].feeder_fix;
    delete legacy.data.flights[0].feeder_fix_eta;
    delete legacy.data.flights[0].feeder_fix_eta_source;
    delete legacy.data.flights[0].feeder_fix_passed;
    delete legacy.data.active_runway_groups;
    delete legacy.data.warnings;

    expect(isAMANStateEvent(legacy)).toBe(true);
    const accepted = replaceAMANState(null, legacy);
    expect(accepted).toMatchObject({accepted: true, error: null});
    expect(accepted.state?.active_runway_groups).toEqual(["ARRIVAL-22"]);
  });

  it("accepts complete warning snapshots and clears them with an empty replacement", () => {
    const warned = replacement(8);
    warned.data.warnings = [
      {
        id: 'warning:"sequence"/-/"protected_same_star_spacing"/"ARRIVAL-22"/"TRAIL"/"LEAD"',
        source: "sequence", severity: "error", code: "protected_same_star_spacing",
        runway_group_id: "ARRIVAL-22", flight_id: "TRAIL", related_flight_id: "LEAD",
        message: "Flights TRAIL and LEAD conflict with protected MONAK spacing on runway group ARRIVAL-22",
      },
      {
        id: 'warning:"technical_health"/"navigation"/"airac_expired"/-/-/-',
        source: "technical_health", component: "navigation", severity: "warning", code: "airac_expired",
        message: "AMAN navigation is degraded: airac_expired",
      },
    ];
    const accepted = replaceAMANState(null, warned);
    expect(accepted.state?.warnings).toEqual(warned.data.warnings);

    const cleared = replacement(9);
    cleared.data.warnings = [];
    expect(replaceAMANState(accepted.state, cleared).state?.warnings).toEqual([]);
  });

  it.each([
    ["non-array snapshot", "invalid"],
    ["unknown source", [{id: "warning-1", source: "future", severity: "error", code: "blocked", message: "Blocked"}]],
    ["unknown severity", [{id: "warning-1", source: "sequence", severity: "fatal", code: "blocked", message: "Blocked"}]],
    ["malformed optional identity", [{id: "warning-1", source: "sequence", severity: "error", code: "blocked", flight_id: " PADDED ", message: "Blocked"}]],
    ["mismatched stable ID", [{id: "warning-1", source: "technical_health", severity: "error", code: "authority_blocked", message: "Blocked"}]],
    ["inconsistent source scope", [{
      id: 'warning:"technical_health"/-/"blocked"/"ARRIVAL-22"/-/-', source: "technical_health", severity: "error",
      code: "blocked", runway_group_id: "ARRIVAL-22", message: "Blocked",
    }]],
  ])("rejects a %s in the optional warning field", (_name, warnings) => {
    const malformed = replacement(8) as unknown as {data: Record<string, unknown>};
    malformed.data.warnings = warnings;
    expect(isAMANStateEvent(malformed)).toBe(false);
  });

  it("accepts structurally valid duplicate warning identities for defensive store deduplication", () => {
    const duplicated = replacement(8);
    const warning = {
      id: 'warning:"technical_health"/"weather"/"weather_stale"/-/-/-',
      source: "technical_health" as const, component: "weather", severity: "warning" as const,
      code: "weather_stale", message: "AMAN weather is degraded: weather_stale",
    };
    duplicated.data.warnings = [warning, {...warning}];
    expect(isAMANStateEvent(duplicated)).toBe(true);
  });

  it("accepts optional ordered timeline mappings and rejects ambiguous mappings", () => {
    const event = replacement(8);
    event.data.timeline_configuration = {
      version: "EKCH-AIP-2609-V4",
      mappings: [{id: 1, left: "TESPI", right: "TUDLO"}, {id: 3, left: "ERNOV", right: null}],
    };
    expect(isAMANStateEvent(event)).toBe(true);

    event.data.timeline_configuration.mappings[1].id = 1;
    expect(isAMANStateEvent(event)).toBe(false);
    event.data.timeline_configuration.mappings[1] = {id: 3, left: "TESPI", right: null};
    expect(isAMANStateEvent(event)).toBe(false);
    event.data.timeline_configuration.mappings[1] = {id: 3, left: null, right: null};
    expect(isAMANStateEvent(event)).toBe(false);
  });

  it("accepts and orders a multi-runway active set by configured runway groups", () => {
    const event = replacement(8);
    event.data.runway_groups.unshift({id: "ARRIVAL-04", selected: false, selection_schedule: []});
    event.data.active_runway_groups = ["ARRIVAL-22", "ARRIVAL-04"];

    expect(isAMANStateEvent(event)).toBe(true);
    expect(getActiveAMANRunwayGroups(event.data).map((group) => group.id)).toEqual(["ARRIVAL-04", "ARRIVAL-22"]);
  });

  it.each([
    ["empty", []],
    ["duplicate", ["ARRIVAL-22", "ARRIVAL-22"]],
    ["unknown", ["ARRIVAL-04"]],
    ["malformed", [" PADDED "]],
  ])("rejects an %s active runway set", (_name, activeRunwayGroups) => {
    const event = replacement(8);
    event.data.active_runway_groups = activeRunwayGroups;
    expect(isAMANStateEvent(event)).toBe(false);
  });

  it("rejects a non-array active runway field", () => {
    const event = replacement(8) as unknown as {data: Record<string, unknown>};
    event.data.active_runway_groups = "ARRIVAL-22";
    expect(isAMANStateEvent(event)).toBe(false);
  });

  it("validates new feeder ETA provenance and passed state", () => {
    const route = replacement(8);
    expect(isAMANStateEvent(route)).toBe(true);

    const passed = replacement(9);
    passed.data.flights[0].feeder_fix_eta = null;
    passed.data.flights[0].feeder_fix_eta_source = "passed";
    passed.data.flights[0].feeder_fix_passed = true;
    expect(isAMANStateEvent(passed)).toBe(true);

    passed.data.flights[0].feeder_fix_eta = "2026-07-22T10:12:00.000Z";
    expect(isAMANStateEvent(passed)).toBe(false);
    passed.data.flights[0].feeder_fix_eta = null;
    passed.data.flights[0].feeder_fix_eta_source = "landing" as "route";
    expect(isAMANStateEvent(passed)).toBe(false);
  });

  it("accepts manual feeder ETA projection but rejects partial optional fields", () => {
    const manual = replacement(8);
    manual.data.flights[0].feeder_fix_eta = "2026-07-22T10:12:00.000Z";
    manual.data.flights[0].feeder_fix_eta_source = "manual";
    manual.data.flights[0].feeder_fix_passed = false;
    expect(isAMANStateEvent(manual)).toBe(true);

    delete manual.data.flights[0].feeder_fix_eta_source;
    expect(isAMANStateEvent(manual)).toBe(false);
  });

  it("accepts holding information with missing EAT and CFL", () => {
    const event = replacement(8);
    event.data.holding_information = [{
      flight_id: "flight-123", callsign: "SAS123", holding: "OLPIB", eat: null,
      cleared_altitude: null, source_status: "stale", observed_at: "2026-07-22T10:00:00.000Z",
    }];

    expect(isAMANStateEvent(event)).toBe(true);
    event.data.holding_information[0].source_status = "unknown" as "fresh";
    expect(isAMANStateEvent(event)).toBe(false);
  });

  it.each(["star_family", "feeder_fix", "holding_fix"] as const)("rejects a malformed %s identity", (field) => {
    const malformed = replacement(8) as unknown as {data: {flights: Array<Record<string, unknown>>}};
    malformed.data.flights[0][field] = 123;

    expect(isAMANStateEvent(malformed)).toBe(false);

    malformed.data.flights[0][field] = " PADDED ";
    expect(isAMANStateEvent(malformed)).toBe(false);
  });

  it("ignores duplicate and older revisions, then atomically accepts any newer revision", () => {
    const initial = replaceAMANState(null, replacement(7));
    expect(initial.accepted).toBe(true);
    expect(initial.state?.revision).toBe(7);

    const duplicate = replaceAMANState(initial.state, replacement(7, "DUPLICATE"));
    expect(duplicate.accepted).toBe(false);
    expect(duplicate.state).toBe(initial.state);
    expect(duplicate.state?.flights[0].callsign).toBe("SAS123");

    const older = replaceAMANState(initial.state, replacement(4, "OLDER"));
    expect(older.accepted).toBe(false);
    expect(older.state).toBe(initial.state);

    const newer = replacement(10, "SAS999");
    newer.data.authoritative = false;
    newer.data.effective_mode = "read_only";
    newer.data.technical_health.status = "degraded";
    newer.data.technical_health.ready = false;
    newer.data.technical_health.blocked_reasons = ["predictor:stale"];
    const accepted = replaceAMANState(initial.state, newer);

    expect(accepted).toMatchObject({accepted: true, status: "degraded", error: null});
    expect(accepted.state).not.toBe(newer.data);
    expect(accepted.state).toMatchObject({
      revision: 10,
      authoritative: false,
      effective_mode: "read_only",
      flights: [{callsign: "SAS999"}],
      technical_health: {status: "degraded", ready: false, blocked_reasons: ["predictor:stale"]},
    });
    expect(initial.state).toMatchObject({revision: 7, authoritative: true, flights: [{callsign: "SAS123"}]});
  });

  it("clears the whole presentation on an invalid payload", () => {
    const initial = replaceAMANState(null, replacement(7));
    const invalid = replacement(8) as unknown as {data: {flights: Array<{slot: {time: unknown}}>} };
    invalid.data.flights[0].slot.time = 123;

    expect(replaceAMANState(initial.state, invalid)).toEqual({
      state: null,
      status: "degraded",
      error: "invalid_aman_state",
      accepted: false,
    });
  });

  it("rejects traffic buckets whose authoritative totals or boundaries were changed", () => {
    const wrongTotal = replacement(8);
    wrongTotal.data.traffic_prediction!.buckets[1].count = 2;
    expect(isAMANStateEvent(wrongTotal)).toBe(false);

    const wrongBoundary = replacement(8);
    wrongBoundary.data.traffic_prediction!.buckets[1].start = "2026-07-22T10:14:00.000Z";
    expect(isAMANStateEvent(wrongBoundary)).toBe(false);
  });

  it("validates persisted go-around confirmation evidence", () => {
    const pending = replacement(8);
    pending.data.flights[0].go_around_confirmation = {
      episode_id: "flight-123/go-around/1", reason: "climb", detected_at: "2026-07-22T10:00:00.000Z",
      evidence_times: ["2026-07-22T09:59:58.000Z", "2026-07-22T09:59:59.000Z"], status: "pending",
      decided_at: null, decided_by: null, resulting_revision: null,
    };
    expect(isAMANStateEvent(pending)).toBe(true);

    pending.data.flights[0].go_around_confirmation.evidence_times = [];
    expect(isAMANStateEvent(pending)).toBe(false);
  });

  it("accepts an explicit disabled-mode health replacement", () => {
    const disabled = replacement(8);
    disabled.data.effective_mode = "disabled";
    disabled.data.authoritative = false;
    disabled.data.flights = [];
    disabled.data.runway_groups = [];
    disabled.data.technical_health.status = "disabled";
    disabled.data.technical_health.ready = false;
    for (const component of [
      disabled.data.technical_health.vatsim,
      disabled.data.technical_health.navigation,
      disabled.data.technical_health.weather,
      disabled.data.technical_health.repository,
      disabled.data.technical_health.predictor,
      disabled.data.technical_health.replay_validation,
    ]) {
      component.status = "disabled";
    }

    expect(isAMANStateEvent(disabled)).toBe(true);
    expect(replaceAMANState(null, disabled)).toMatchObject({accepted: true, status: "ready", error: null});
  });

  it.each(["degraded", "unavailable"] as const)("marks valid %s health as degraded presentation", (healthStatus) => {
    const event = replacement(8);
    event.data.technical_health.status = healthStatus;
    event.data.technical_health.ready = false;

    const accepted = replaceAMANState(null, event);
    expect(accepted).toMatchObject({accepted: true, status: "degraded", error: null});
    expect(replaceAMANState(accepted.state, event)).toMatchObject({accepted: false, status: "degraded", error: null});
  });

  it("validates command rejection correlation fields", () => {
    expect(isAMANCommandRejectedEvent({
      type: "aman.command_rejected",
      version: 1,
      data: {command_id: "command-7", code: "revision_conflict", message: "revision changed", current_revision: 9, retryable: true},
    })).toBe(true);
    expect(isAMANCommandRejectedEvent({
      type: "aman.command_rejected",
      version: 1,
      data: {command_id: "", code: "revision_conflict", message: "revision changed", current_revision: 9, retryable: true},
    })).toBe(false);
  });
});
