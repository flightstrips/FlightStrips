import {readFileSync} from "node:fs";
import {resolve} from "node:path";
import {beforeEach, describe, expect, it, vi} from "vitest";
import type {StoreApi} from "zustand/vanilla";

import type {AMANStateEvent} from "@/api/aman";
import {EventType} from "@/api/models";
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
    client._emit(EventType.FrontendAMANState, replacement(7));
  });

  it("adds only command metadata to the matching typed request and tracks it as pending", () => {
    const commandID = store.getState().sendAMANCommand({type: "aman.accept_teta", flight_id: "flight-123"}, true);

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

  it("does not send while disconnected, unauthorized, read-only, non-authoritative, or unready", () => {
    store.getState().setAMANConnectionState("disconnected");
    expect(store.getState().sendAMANCommand({type: "aman.lock_flight", flight_id: "flight-123"}, true)).toBeNull();
    store.getState().setAMANConnectionState("connected");
    expect(store.getState().sendAMANCommand({type: "aman.lock_flight", flight_id: "flight-123"}, false)).toBeNull();
    store.setState({readOnly: true});
    expect(store.getState().sendAMANCommand({type: "aman.lock_flight", flight_id: "flight-123"}, true)).toBeNull();
    store.setState({readOnly: false, amanState: {...store.getState().amanState!, authoritative: false, effective_mode: "read_only"}});
    expect(store.getState().sendAMANCommand({type: "aman.lock_flight", flight_id: "flight-123"}, true)).toBeNull();
    store.setState({amanState: {...replacement(7).data, technical_health: {...replacement(7).data.technical_health, ready: false}}});
    expect(store.getState().sendAMANCommand({type: "aman.lock_flight", flight_id: "flight-123"}, true)).toBeNull();
    expect(client.send).not.toHaveBeenCalled();
  });

  it("keeps pending correlation through reconnect and clears it only on a newer replacement", () => {
    const commandID = store.getState().sendAMANCommand({type: "aman.lock_flight", flight_id: "flight-123"}, true)!;
    store.getState().setAMANConnectionState("disconnected");
    store.getState().setAMANConnectionState("connected");
    client._emit(EventType.FrontendAMANState, replacement(7));
    expect(store.getState().amanPendingCommands[commandID]).toBeDefined();

    client._emit(EventType.FrontendAMANState, replacement(8));
    expect(store.getState().amanPendingCommands[commandID]).toBeUndefined();
    expect(store.getState().amanState?.revision).toBe(8);
  });

  it("turns a correlated rejection into a durable visible result, including conflicts", () => {
    const commandID = store.getState().sendAMANCommand({type: "aman.lock_flight", flight_id: "flight-123"}, true)!;
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
});
