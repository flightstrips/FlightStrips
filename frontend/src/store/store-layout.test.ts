import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand/vanilla";

import { ActionType, Bay, CommunicationType, EventType, type FrontendInitialEvent, type FrontendStrip } from "@/api/models";
import type { WebSocketClient } from "@/api/websocket";
import { createWebSocketStore, type WebSocketState } from "./store";

function createMockClient() {
  const handlers = new Map<string, Array<(data: unknown) => void>>();

  return {
    on: vi.fn((eventType: string, handler: (data: unknown) => void) => {
      handlers.set(eventType, [...(handlers.get(eventType) ?? []), handler]);
    }),
    send: vi.fn(),
    reconnect: vi.fn(),
    setReadOnly: vi.fn(),
    _emit: (eventType: string, data: unknown) => {
      handlers.get(eventType)?.forEach((handler) => handler(data));
    },
  } as unknown as WebSocketClient & {
    _emit: (eventType: string, data: unknown) => void;
  };
}

function initialEvent(layout: string): FrontendInitialEvent {
  return {
    type: EventType.FrontendInitial,
    controllers: [],
    strips: [],
    tactical_strips: [],
    me: { callsign: "EKCH_B_GND", position: "EKCH_B_GND", identifier: "SQ", section: "GND", owned_sectors: [] },
    airport: "EKCH",
    layout,
    callsign: "EKCH_B_GND",
    runway_setup: { departure: [], arrival: [] },
    coordinations: [],
    messages: [],
    available_sids: [],
    initial_cfl_by_runway: {},
    transition_altitude: 5000,
    read_only: false,
    position_available: true,
    stand_assignment_enabled: false,
    stand_assignments: [],
    stand_blocks: [],
  };
}

function foreignStrip(): FrontendStrip {
  return {
    callsign: "SAS123",
    origin: "EKCH",
    destination: "ENGM",
    alternate: "",
    route: "",
    remarks: "",
    runway: "22R",
    squawk: "1000",
    assigned_squawk: "1000",
    sid: "",
    star: "",
    cleared_altitude: 0,
    requested_altitude: 0,
    heading: 0,
    aircraft_type: "A320",
    aircraft_category: "",
    stand: "A18",
    capabilities: "",
    communication_type: CommunicationType.Unknown,
    eobt: "",
    tobt: "",
    tsat: "",
    ctot: "",
    eldt: "",
    bay: Bay.Stand,
    release_point: "",
    version: 1,
    sequence: 0,
    next_controllers: ["EKCH_C_GND"],
    previous_controllers: [],
    owner: "EKCH_A_GND",
    pdc_state: "NONE",
    start_req: false,
    marked: false,
    runway_cleared: false,
    runway_confirmed: false,
    registration: "",
  };
}

describe("EST layout behavior", () => {
  let client: ReturnType<typeof createMockClient>;
  let store: StoreApi<WebSocketState>;

  beforeEach(() => {
    client = createMockClient();
    store = createWebSocketStore(client);
  });

  it("does not automatically open EST from the server recommendation", () => {
    client._emit(EventType.FrontendInitial, initialEvent("SEQPLN"));

    expect(store.getState().displayedLayout).toBe("");
    expect(store.getState().followRecommendedLayout).toBe(true);
  });

  it("keeps a manually opened EST board after reconnecting", () => {
    client._emit(EventType.FrontendInitial, initialEvent("AD"));
    store.getState().setDisplayedLayout("EST");
    client._emit(EventType.FrontendLayoutUpdate, {
      type: EventType.FrontendLayoutUpdate,
      layout: "AAAD",
    });
    client._emit(EventType.FrontendInitial, initialEvent("AD"));

    expect(store.getState().displayedLayout).toBe("EST");
    expect(store.getState().followRecommendedLayout).toBe(false);
  });

  it("waits for the force-assume route before sending EST ready and transfer", async () => {
    store.setState({
      displayedLayout: "EST",
      position: "EKCH_B_GND",
      strips: [foreignStrip()],
    });

    const transfer = store.getState().startRequestAndTransfer("SAS123");

    expect(client.send).toHaveBeenNthCalledWith(1, {
      type: ActionType.FrontendCoordinationForceAssumeRequest,
      callsign: "SAS123",
      request_id: "SAS123-1",
    });
    client._emit(EventType.FrontendCoordinationForceAssumeResult, {
      type: EventType.FrontendCoordinationForceAssumeResult,
      callsign: "SAS123",
      request_id: "SAS123-1",
      owner: "EKCH_B_GND",
      next_owners: ["EKCH_C_GND"],
    });

    await expect(transfer).resolves.toBe(true);
    expect(client.send).toHaveBeenNthCalledWith(2, {
      type: ActionType.FrontendStartReq,
      callsign: "SAS123",
      start_req: true,
    });
    expect(client.send).toHaveBeenNthCalledWith(3, {
      type: ActionType.FrontendCoordinationTransferRequest,
      callsign: "SAS123",
      to: "EKCH_C_GND",
    });
  });

  it("only rejects the matching pending force-assume transfer", async () => {
    const secondStrip = { ...foreignStrip(), callsign: "SAS456" };
    store.setState({
      displayedLayout: "EST",
      position: "EKCH_B_GND",
      strips: [foreignStrip(), secondStrip],
    });

    const firstTransfer = store.getState().startRequestAndTransfer("SAS123");
    const secondTransfer = store.getState().startRequestAndTransfer("SAS456");

    client._emit(EventType.FrontendActionRejected, {
      type: EventType.FrontendActionRejected,
      action: ActionType.FrontendCoordinationForceAssumeRequest,
      reason: "not allowed",
      request_id: "SAS123-1",
    });
    await expect(firstTransfer).resolves.toBe(false);

    client._emit(EventType.FrontendCoordinationForceAssumeResult, {
      type: EventType.FrontendCoordinationForceAssumeResult,
      callsign: "SAS456",
      request_id: "SAS456-2",
      owner: "EKCH_B_GND",
      next_owners: ["EKCH_C_GND"],
    });

    await expect(secondTransfer).resolves.toBe(true);
    expect(client.send).toHaveBeenLastCalledWith({
      type: ActionType.FrontendCoordinationTransferRequest,
      callsign: "SAS456",
      to: "EKCH_C_GND",
    });
  });
});
