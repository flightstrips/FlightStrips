import {readFileSync} from "node:fs";
import {resolve} from "node:path";
import {beforeEach, describe, expect, it, vi} from "vitest";
import type {StoreApi} from "zustand/vanilla";

import type {AMANStateEvent} from "@/api/aman";
import {EventType, type FrontendInitialEvent} from "@/api/models";
import type {WebSocketClient} from "@/api/websocket";
import {createWebSocketStore, type WebSocketState} from "./store";

const golden = JSON.parse(readFileSync(
  resolve(process.cwd(), "../backend/pkg/events/frontend/testdata/aman-state-v1.json"),
  "utf8",
)) as AMANStateEvent;

function replacement(revision: number): AMANStateEvent {
  const event = structuredClone(golden);
  event.data.revision = revision;
  event.data.flights[0].slot!.revision = revision;
  return event;
}

function initialSnapshot(amanFMP: boolean, readOnly = false): FrontendInitialEvent {
  return {
    type: EventType.FrontendInitial,
    controllers: [],
    strips: [],
    tactical_strips: [],
    me: {callsign: "EKCH_FMH", position: "120.500", identifier: "FMH", section: "", owned_sectors: []},
    airport: "EKCH",
    layout: "AA",
    callsign: "EKCH_FMH",
    runway_setup: {departure: [], arrival: [], runway_status: {}},
    coordinations: [],
    messages: [],
    available_sids: [],
    initial_cfl_by_runway: {},
    transition_altitude: 7000,
    read_only: readOnly,
    position_available: true,
    stand_assignment_enabled: false,
    stand_assignments: [],
    stand_blocks: [],
    capabilities: {aman_fmp: amanFMP},
  };
}

function createMockClient() {
  const handlers = new Map<string, Array<(data: unknown) => void>>();
  return {
    on: vi.fn((eventType: string, handler: (data: unknown) => void) => {
      handlers.set(eventType, [...(handlers.get(eventType) ?? []), handler]);
    }),
    send: vi.fn(),
    reconnect: vi.fn(),
    setReadOnly: vi.fn(),
    _emit: (eventType: string, data: unknown) => handlers.get(eventType)?.forEach((handler) => handler(data)),
  } as unknown as WebSocketClient & {_emit: (eventType: string, data: unknown) => void};
}

describe("AMAN command store", () => {
  let client: ReturnType<typeof createMockClient>;
  let store: StoreApi<WebSocketState>;

  beforeEach(() => {
    client = createMockClient();
    store = createWebSocketStore(client);
    store.getState().setAMANConnectionState("connected");
    client._emit(EventType.FrontendInitial, initialSnapshot(true));
    client._emit(EventType.FrontendAMANState, replacement(7));
  });

  it("adds only command metadata to the matching typed request and tracks it as pending", () => {
    const commandID = store.getState().sendAMANCommand({type: "aman.accept_teta", flight_id: "flight-123"});

    expect(commandID).toEqual(expect.any(String));
    expect(client.send).toHaveBeenCalledWith({
      type: "aman.accept_teta",
      version: 1,
      data: {command_id: commandID, expected_revision: 7, flight_id: "flight-123"},
    });
    expect(store.getState().amanPendingCommands[commandID!]).toEqual({
      command_id: commandID,
      type: "aman.accept_teta",
      expected_revision: 7,
      flight_id: "flight-123",
    });
    expect(store.getState().amanState?.revision).toBe(7);
  });

  it("tracks recompute until a server replacement confirms it", () => {
    const commandID = store.getState().sendAMANCommand({type: "aman.recompute_flight", flight_id: "flight-123"})!;

    expect(client.send).toHaveBeenCalledWith({
      type: "aman.recompute_flight", version: 1,
      data: {command_id: commandID, expected_revision: 7, flight_id: "flight-123"},
    });
    expect(store.getState().amanPendingCommands[commandID]?.type).toBe("aman.recompute_flight");
    client._emit(EventType.FrontendAMANState, replacement(8));
    expect(store.getState().amanPendingCommands[commandID]).toBeUndefined();
  });

  it("sends typed manual feeder ETA set and reset commands", () => {
    const setID = store.getState().sendAMANCommand({type: "aman.set_manual_feeder_eta", flight_id: "flight-123", feeder_eta: "2026-07-22T12:10:00.000Z"})!;
    const resetID = store.getState().sendAMANCommand({type: "aman.reset_manual_feeder_eta", flight_id: "flight-123"})!;

    expect(client.send).toHaveBeenNthCalledWith(1, {
      type: "aman.set_manual_feeder_eta", version: 1,
      data: {command_id: setID, expected_revision: 7, flight_id: "flight-123", feeder_eta: "2026-07-22T12:10:00.000Z"},
    });
    expect(client.send).toHaveBeenNthCalledWith(2, {
      type: "aman.reset_manual_feeder_eta", version: 1,
      data: {command_id: resetID, expected_revision: 7, flight_id: "flight-123"},
    });
  });

  it("does not send while disconnected, unauthorized, read-only, non-authoritative, or unready", () => {
    store.getState().setAMANConnectionState("disconnected");
    expect(store.getState().sendAMANCommand({type: "aman.lock_flight", flight_id: "flight-123"})).toBeNull();
    store.getState().setAMANConnectionState("connected");
    expect(store.getState().sendAMANCommand({type: "aman.lock_flight", flight_id: "flight-123"})).toBeNull();
    client._emit(EventType.FrontendInitial, initialSnapshot(false));
    expect(store.getState().sendAMANCommand({type: "aman.lock_flight", flight_id: "flight-123"})).toBeNull();
    client._emit(EventType.FrontendInitial, initialSnapshot(true, true));
    expect(store.getState().sendAMANCommand({type: "aman.lock_flight", flight_id: "flight-123"})).toBeNull();
    store.setState({readOnly: false, amanState: {...store.getState().amanState!, authoritative: false, effective_mode: "read_only"}});
    expect(store.getState().sendAMANCommand({type: "aman.lock_flight", flight_id: "flight-123"})).toBeNull();
    store.setState({amanState: {...replacement(7).data, technical_health: {...replacement(7).data.technical_health, ready: false}}});
    expect(store.getState().sendAMANCommand({type: "aman.lock_flight", flight_id: "flight-123"})).toBeNull();
    expect(client.send).not.toHaveBeenCalled();
  });

  it("replaces authority from server snapshots and fails closed across reconnects", () => {
    expect(store.getState().amanFMPAuthority).toBe(true);

    client._emit(EventType.FrontendInitial, initialSnapshot(false));
    expect(store.getState().amanFMPAuthority).toBe(false);

    client._emit(EventType.FrontendInitial, initialSnapshot(true));
    store.getState().setAMANConnectionState("disconnected");
    expect(store.getState().amanFMPAuthority).toBe(false);

    store.getState().setAMANConnectionState("connected");
    expect(store.getState().amanFMPAuthority).toBe(false);
    client._emit(EventType.FrontendInitial, initialSnapshot(true));
    expect(store.getState().amanFMPAuthority).toBe(true);
  });

  it("keeps pending correlation through reconnect and clears it only on a newer replacement", () => {
    const commandID = store.getState().sendAMANCommand({type: "aman.lock_flight", flight_id: "flight-123"})!;
    store.getState().setAMANConnectionState("disconnected");
    store.getState().setAMANConnectionState("connected");
    client._emit(EventType.FrontendAMANState, replacement(7));
    expect(store.getState().amanPendingCommands[commandID]).toBeDefined();

    client._emit(EventType.FrontendAMANState, replacement(8));
    expect(store.getState().amanPendingCommands[commandID]).toBeUndefined();
    expect(store.getState().amanState?.revision).toBe(8);
  });

  it("stores the complete active runway set and derives it from legacy selection", () => {
    expect(store.getState().amanState?.active_runway_groups).toEqual(["ARRIVAL-22"]);

    const multiRunway = replacement(8);
    multiRunway.data.runway_groups.push({id: "ARRIVAL-04", selected: false, selection_schedule: []});
    multiRunway.data.active_runway_groups = ["ARRIVAL-04", "ARRIVAL-22"];
    client._emit(EventType.FrontendAMANState, multiRunway);

    expect(store.getState().amanState?.active_runway_groups).toEqual(["ARRIVAL-22", "ARRIVAL-04"]);
  });

  it("turns a correlated rejection into a durable visible result, including conflicts", () => {
    const commandID = store.getState().sendAMANCommand({type: "aman.lock_flight", flight_id: "flight-123"})!;
    client._emit(EventType.FrontendAMANCommandRejected, {
      type: "aman.command_rejected",
      version: 1,
      data: {command_id: commandID, code: "revision_conflict", message: "revision changed", current_revision: 8, retryable: true},
    });

    expect(store.getState().amanPendingCommands[commandID]).toBeUndefined();
    expect(store.getState().amanCommandRejections[commandID]).toMatchObject({code: "revision_conflict", current_revision: 8});

    store.getState().setAMANConnectionState("disconnected");
    store.getState().setAMANConnectionState("connected");
    client._emit(EventType.FrontendAMANState, replacement(8));
    expect(store.getState().amanCommandRejections[commandID]).toMatchObject({code: "revision_conflict"});

    store.getState().dismissAMANCommandRejection(commandID);
    expect(store.getState().amanCommandRejections[commandID]).toBeUndefined();
  });

  it("retains command type correlation after a newer state clears pending", () => {
    const commandID = store.getState().sendAMANCommand({
      type: "aman.select_runway_group", runway_group_id: "ARRIVAL-22", effective_at: golden.data.generated_at,
    })!;
    client._emit(EventType.FrontendAMANState, replacement(8));
    expect(store.getState().amanPendingCommands[commandID]).toBeUndefined();

    client._emit(EventType.FrontendAMANCommandRejected, {
      type: "aman.command_rejected",
      version: 1,
      data: {command_id: commandID, code: "revision_conflict", message: "revision changed", current_revision: 8, retryable: true},
    });

    expect(store.getState().amanCommandRejections[commandID].command_type).toBe("aman.select_runway_group");
  });
});
