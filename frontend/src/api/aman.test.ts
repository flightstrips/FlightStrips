import {readFileSync} from "node:fs";
import {resolve} from "node:path";
import {describe, expect, it} from "vitest";

import {
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
