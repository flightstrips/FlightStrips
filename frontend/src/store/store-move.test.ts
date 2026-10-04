import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand/vanilla";

import { create } from "@bufbuild/protobuf";
import { FrontendProjection } from "@/api/projection";
import { FrontendInitialSchema, FrontendObservationSchema, FrontendDeltaSchema } from "@/api/generated/cluster/v1/wire_pb";
import { ActionType, Bay, EventType, type FrontendStrip, type WebSocketEvent } from "@/api/models";
import type { WebSocketClient } from "@/api/websocket";
import { createWebSocketStore, type WebSocketState } from "./store";

describe("strip move actions", () => {
  let client: { send: ReturnType<typeof vi.fn>; on: ReturnType<typeof vi.fn>; setReadOnly: ReturnType<typeof vi.fn> };
  let store: StoreApi<WebSocketState>;

  beforeEach(() => {
    client = { send: vi.fn(), on: vi.fn(), setReadOnly: vi.fn() };
    store = createWebSocketStore(client as unknown as WebSocketClient);
    store.setState({
      strips: [{ callsign: "OYABC", bay: Bay.Taxi, sequence: 1 } as FrontendStrip],
      tacticalStrips: [],
      readOnly: false,
    });
  });

  it("sends and applies moves into CONTROLZONE", () => {
    store.getState().move("OYABC", Bay.Controlzone);

    expect(client.send).toHaveBeenCalledWith({
      type: ActionType.FrontendMove,
      callsign: "OYABC",
      bay: Bay.Controlzone,
      clearance: false,
      confirmed_removal: false,
    });
    expect(store.getState().strips[0].bay).toBe(Bay.Controlzone);
  });

  it("marks deliberate clearance moves", () => {
    store.getState().move("OYABC", Bay.Cleared, true);

    expect(client.send).toHaveBeenCalledWith({
      type: ActionType.FrontendMove,
      callsign: "OYABC",
      bay: Bay.Cleared,
      clearance: true,
      confirmed_removal: false,
    });
  });

  it("marks confirmed EST removals explicitly", () => {
    store.getState().move("OYABC", Bay.Hidden, false, true);

    expect(client.send).toHaveBeenCalledWith({
      type: ActionType.FrontendMove,
      callsign: "OYABC",
      bay: Bay.Hidden,
      clearance: false,
      confirmed_removal: true,
    });
  });
  it("preserves immediate moves and reorders during position traffic, then accepts the authoritative move", () => {
    const projection = new FrontendProjection((event: WebSocketEvent) => {
      const subscription = client.on.mock.calls.find(([type]) => type === event.type);
      subscription?.[1](event);
    });
    projection.initial(create(FrontendInitialSchema, {
      sessionId: 7, airport: "EKCH", sessionName: "TEST", aggregateRevision: 1n,
      me: {cid: "123456", callsign: "EKCH_TWR", position: "TWR"},
      entities: [{key: "OYABC", revision: 1n, value: {value: {case: "strip", value: {
        id: 1n, callsign: "OYABC", bay: Bay.Taxi, sequence: 100n, pdcState: "NONE",
      }}}}],
    }));
    store.getState().move("OYABC", Bay.Cleared, true);
    const moved = store.getState().strips[0];
    const report = (revision: bigint, altitude: number, removed = false) => projection.observation(
      create(FrontendObservationSchema, {sourceRevision: revision, removed,
        value: {case: "position", value: {sessionId: 7, aircraftKey: "OYABC", ownerEpoch: 1n,
          observation: {case: "position", value: {altitudeFeet: altitude}}}}}));
    report(1n, 1200);
    expect(store.getState().strips[0]).toMatchObject({bay: Bay.Cleared, sequence: moved.sequence,
      runway_cleared: moved.runway_cleared, position_altitude: 1200});
    const withAltitude = store.getState().strips[0];
    report(2n, 1200);
    expect(store.getState().strips[0]).toBe(withAltitude);
    store.getState().updateOrder("OYABC", null);
    const reorderedSequence = store.getState().strips[0].sequence;
    report(3n, 1300);
    expect(store.getState().strips[0]).toMatchObject({bay: Bay.Cleared, sequence: reorderedSequence});
    report(4n, 1300, true);
    expect(store.getState().strips[0]).toMatchObject({bay: Bay.Cleared, sequence: reorderedSequence});
    expect(store.getState().strips[0].position_altitude).toBeUndefined();
    projection.delta(create(FrontendDeltaSchema, {
      aggregate: {target: {case: "session", value: {id: 7}}}, aggregateRevision: 2n,
      changes: [{key: "OYABC", revision: 2n, operation: {case: "upsert", value: {
        value: {case: "strip", value: {id: 1n, callsign: "OYABC", bay: Bay.Cleared,
          sequence: 200n, pdcState: "NONE"}},
      }}}],
    }));
    expect(store.getState().strips[0]).toMatchObject({bay: Bay.Cleared, sequence: 200});
    expect(client.on.mock.calls.some(([type]) => type === EventType.FrontendPositionAltitude)).toBe(true);
  });

});
