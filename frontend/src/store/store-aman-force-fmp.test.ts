import {describe, expect, it, vi} from "vitest";

import {EventType, type FrontendInitialEvent} from "@/api/models";
import type {WebSocketClient} from "@/api/websocket";
import {createWebSocketStore} from "./store";

function initialSnapshot(): FrontendInitialEvent {
  return {
    type: EventType.FrontendInitial,
    controllers: [], strips: [], tactical_strips: [],
    me: {callsign: "EKCH_W_APP", position: "119.805", identifier: "W", section: "", owned_sectors: []},
    airport: "EKCH", layout: "AA", callsign: "EKCH_W_APP",
    runway_setup: {departure: [], arrival: [], runway_status: {}},
    coordinations: [], messages: [], available_sids: [], initial_cfl_by_runway: {}, transition_altitude: 7000,
    read_only: false, position_available: true, stand_assignment_enabled: false, stand_assignments: [], stand_blocks: [],
    capabilities: {aman_fmp: false},
  };
}

function createMockClient() {
  const handlers = new Map<string, Array<(data: unknown) => void>>();
  return {
    on: vi.fn((eventType: string, handler: (data: unknown) => void) => {
      handlers.set(eventType, [...(handlers.get(eventType) ?? []), handler]);
    }),
    onInvalidated: () => () => undefined,
    send: vi.fn(),
    reconnect: vi.fn(),
    setReadOnly: vi.fn(),
    _emit: (eventType: string, data: unknown) => handlers.get(eventType)?.forEach((handler) => handler(data)),
  } as unknown as WebSocketClient & {_emit: (eventType: string, data: unknown) => void; send: ReturnType<typeof vi.fn>};
}

const authority = (data: Record<string, unknown>) => ({
  type: "aman.fmp_authority", version: 1,
  data: {has_fmp_authority: false, forced: false, forced_holder: null, fmp_online: false, can_force: true, ...data},
});

describe("FORCE FMP store", () => {
  it("sends force and release commands and takes authority from the server", () => {
    const client = createMockClient();
    const store = createWebSocketStore(client);
    store.getState().setAMANConnectionState("connected");
    client._emit(EventType.FrontendInitial, initialSnapshot());
    client._emit(EventType.FrontendAMANFMPAuthority, authority({}));
    expect(store.getState().amanFMPAuthority).toBe(false);

    expect(store.getState().setAMANForcedFMP(true)).toBe(true);
    const forced = client.send.mock.lastCall![0];
    expect(forced).toMatchObject({type: "aman.force_fmp", version: 1, data: {command_id: expect.any(String)}});
    expect(store.getState().amanForceFMPCommandID).toBe(forced.data.command_id);

    client._emit(EventType.FrontendAMANFMPAuthority, authority({has_fmp_authority: true, forced: true, forced_holder: "EKCH_W_APP", can_force: false}));
    expect(store.getState().amanFMPAuthority).toBe(true);
    expect(store.getState().amanForcedFMP?.forced_holder).toBe("EKCH_W_APP");
    expect(store.getState().amanForceFMPCommandID).toBeNull();

    store.getState().setAMANForcedFMP(false);
    expect(client.send.mock.lastCall![0]).toMatchObject({type: "aman.release_forced_fmp"});
  });

  it("shows a rejected FORCE FMP without adding it to AMAN command rejections", () => {
    const client = createMockClient();
    const store = createWebSocketStore(client);
    store.getState().setAMANConnectionState("connected");
    client._emit(EventType.FrontendInitial, initialSnapshot());
    store.getState().setAMANForcedFMP(true);
    const commandID = store.getState().amanForceFMPCommandID;

    client._emit(EventType.FrontendAMANCommandRejected, {type: "aman.command_rejected", version: 1, data: {command_id: commandID, code: "invalid_transition", message: "FMP is online", current_revision: 0, retryable: false}});

    expect(store.getState().amanForceFMPRejection).toBe("FMP is online");
    expect(store.getState().amanForceFMPCommandID).toBeNull();
    expect(store.getState().amanCommandRejections).toEqual({});
  });

  it("loses forced FMP status on disconnect", () => {
    const client = createMockClient();
    const store = createWebSocketStore(client);
    store.getState().setAMANConnectionState("connected");
    client._emit(EventType.FrontendInitial, initialSnapshot());
    client._emit(EventType.FrontendAMANFMPAuthority, authority({has_fmp_authority: true, forced: true, forced_holder: "EKCH_W_APP"}));

    store.getState().setAMANConnectionState("disconnected");

    expect(store.getState().amanFMPAuthority).toBe(false);
    expect(store.getState().amanForcedFMP).toBeNull();
    expect(store.getState().setAMANForcedFMP(true)).toBe(false);
  });
});
