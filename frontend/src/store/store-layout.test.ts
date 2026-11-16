import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";

import { ActionType, Bay, CommunicationType, EventType, type FrontendInitialEvent, type FrontendStrip } from "@/api/models";
import type { WebSocketClient } from "@/api/websocket";
import { createWebSocketStore } from "./store";
import { getEstDepartureTransferTarget, isEstDepartureTransferActive, isEstDepartureTransferred } from "@/components/est/transferState";

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
    reset_layout: layout,
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

describe("session master metadata", () => {
  it("loads the snapshot, follows changes and clears on disconnect or a legacy snapshot", () => {
    const client = createMockClient();
    const store = createWebSocketStore(client);
    client._emit(EventType.FrontendInitial, { ...initialEvent("SEQPLN"), master_callsign: "EKCH_A_TWR" });
    expect(store.getState().masterCallsign).toBe("EKCH_A_TWR");
    client._emit(EventType.FrontendMasterInfo, { master_callsign: "EKDK_FMP" });
    expect(store.getState().masterCallsign).toBe("EKDK_FMP");
    client._emit(EventType.FrontendMasterInfo, { master_callsign: "" });
    expect(store.getState().masterCallsign).toBe("");
    client._emit(EventType.FrontendMasterInfo, { master_callsign: "EKDK_FMP" });
    client._emit(EventType.FrontendInitial, initialEvent("SEQPLN"));
    expect(store.getState().masterCallsign).toBe("");
    client._emit(EventType.FrontendMasterInfo, { master_callsign: "EKDK_FMP" });
    client._emit(EventType.FrontendDisconnect, { reason: "ES disconnected" });
    expect(store.getState().masterCallsign).toBe("");
  });

  describe("EST transfer display lifecycle", () => {
    it("stays hidden across acceptance and reconnect but not rejection or cancellation", () => {
      const client = createMockClient();
      const store = createWebSocketStore(client);
      const position = "EKCH_B_GND";
      const strip = { ...foreignStrip(), owner: position, bay: Bay.Cleared, start_req: true, tobt: "1420", tsat: "1425" };
      store.setState({ position, strips: [strip] });
      const hidden = () => {
        const state = store.getState();
        const current = state.strips[0];
        return isEstDepartureTransferActive(state.stripTransfers[current.callsign], position, getEstDepartureTransferTarget(current, position))
          || isEstDepartureTransferred(current, position);
      };
      const transfer = { callsign: strip.callsign, from: position, to: "EKCH_C_GND" };

      expect(hidden()).toBe(false);
      client._emit(EventType.FrontendCoordinationTransferBroadcast, transfer);
      expect(hidden()).toBe(true);
      client._emit(EventType.FrontendCoordinationRejectBroadcast, { callsign: strip.callsign, position: "EKCH_C_GND" });
      expect(hidden()).toBe(false);
      client._emit(EventType.FrontendCoordinationTransferBroadcast, transfer);
      client._emit(EventType.FrontendCoordinationRejectBroadcast, { callsign: strip.callsign, position });
      expect(hidden()).toBe(false);
      client._emit(EventType.FrontendCoordinationTransferBroadcast, transfer);
      client._emit(EventType.FrontendCoordinationAssumeBroadcast, { callsign: strip.callsign, position: "EKCH_C_GND" });
      expect(hidden()).toBe(true);
      expect(store.getState().strips[0]).toMatchObject({ owner: "EKCH_C_GND", previous_controllers: [position], tobt: "1420", tsat: "1425", start_req: true });
      client._emit(EventType.FrontendOwnersUpdate, {
        callsign: strip.callsign, owner: "EKCH_C_GND", next_owners: ["EKCH_TWR"], previous_owners: [position],
      });
      expect(hidden()).toBe(true);

      client._emit(EventType.FrontendInitial, {
        ...initialEvent("AD"),
        me: { ...initialEvent("AD").me, position },
        strips: [store.getState().strips[0]],
      });
      expect(hidden()).toBe(true);
    });

    it("does not treat a tag request or an unrelated assumption as a completed transfer", () => {
      const client = createMockClient();
      const store = createWebSocketStore(client);
      store.setState({ strips: [foreignStrip()] });
      client._emit(EventType.FrontendCoordinationTagRequestBroadcast, { callsign: "SAS123", from: "EKCH_A_GND", to: "EKCH_C_GND" });
      client._emit(EventType.FrontendCoordinationAssumeBroadcast, { callsign: "SAS123", position: "EKCH_C_GND" });
      expect(store.getState().strips[0].owner).toBe("EKCH_A_GND");
      expect(store.getState().strips[0].previous_controllers).toEqual([]);
      client._emit(EventType.FrontendCoordinationTransferBroadcast, { callsign: "SAS123", from: "EKCH_A_GND", to: "EKCH_C_GND" });
      client._emit(EventType.FrontendCoordinationAssumeBroadcast, { callsign: "SAS123", position: "EKCH_TWR" });
      expect(store.getState().strips[0].owner).toBe("EKCH_A_GND");
      expect(store.getState().strips[0].previous_controllers).toEqual([]);
    });

    it("handles a snapshot with null previous owners when the transfer is accepted", () => {
      const client = createMockClient();
      const store = createWebSocketStore(client);
      client._emit(EventType.FrontendInitial, {
        ...initialEvent("AD"),
        strips: [{ ...foreignStrip(), previous_controllers: null }],
      });
      client._emit(EventType.FrontendCoordinationTransferBroadcast, { callsign: "SAS123", from: "EKCH_A_GND", to: "EKCH_C_GND" });
      client._emit(EventType.FrontendCoordinationAssumeBroadcast, { callsign: "SAS123", position: "EKCH_C_GND" });
      expect(store.getState().strips[0].previous_controllers).toEqual(["EKCH_A_GND"]);
    });
  });
});

describe("manual companion layout behavior", () => {
  let client: ReturnType<typeof createMockClient>;
  let store: ReturnType<typeof createWebSocketStore>;

  beforeEach(() => {
    client = createMockClient();
    store = createWebSocketStore(client);
  });
  afterEach(() => { store.dispose(); vi.useRealTimers(); vi.restoreAllMocks(); });

  it("automatically opens the SEQ PLN scope from the server recommendation", () => {
    client._emit(EventType.FrontendInitial, initialEvent("SEQPLN"));

    expect(store.getState().displayedLayout).toBe("SEQPLN");
    expect(store.getState().followRecommendedLayout).toBe(true);
  });

  it("keeps raw EST as a manual-only layout", () => {
    client._emit(EventType.FrontendInitial, initialEvent("EST"));

    expect(store.getState().displayedLayout).toBe("");
    expect(store.getState().followRecommendedLayout).toBe(true);
  });

  it("follows a live SEQ PLN server recommendation", () => {
    client._emit(EventType.FrontendInitial, initialEvent("AD"));
    client._emit(EventType.FrontendLayoutUpdate, {
      type: EventType.FrontendLayoutUpdate,
      layout: "SEQPLN",
    });

    expect(store.getState().displayedLayout).toBe("SEQPLN");
    expect(store.getState().followRecommendedLayout).toBe(true);
  });

  it("resets a manually selected scope to the frequency-based recommendation", () => {
    client._emit(EventType.FrontendInitial, {
      ...initialEvent("TWTE"),
      reset_layout: "SEQPLN",
    });
    store.getState().setDisplayedLayout("TWTE");

    store.getState().resetLayout();

    expect(store.getState().displayedLayout).toBe("SEQPLN");
    expect(store.getState().followRecommendedLayout).toBe(false);
  });

  it("uses the latest frequency-based recommendation on reset without switching immediately", () => {
    client._emit(EventType.FrontendInitial, initialEvent("AAAD"));
    store.getState().setDisplayedLayout("TWTE");
    client._emit(EventType.FrontendLayoutUpdate, {
      type: EventType.FrontendLayoutUpdate,
      layout: "AAAD",
      reset_layout: "SEQPLN",
    });

    expect(store.getState().displayedLayout).toBe("TWTE");
    store.getState().resetLayout();
    expect(store.getState().displayedLayout).toBe("SEQPLN");
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

  it("automatically opens AMAN from the server recommendation", () => {
    client._emit(EventType.FrontendInitial, initialEvent("AMAN"));
    expect(store.getState().displayedLayout).toBe("AMAN");
    expect(store.getState().followRecommendedLayout).toBe(true);
  });

  it("follows a live AMAN server recommendation", () => {
    client._emit(EventType.FrontendInitial, initialEvent("AD"));
    client._emit(EventType.FrontendLayoutUpdate, {type: EventType.FrontendLayoutUpdate, layout: "AMAN"});

    expect(store.getState().displayedLayout).toBe("AMAN");
    expect(store.getState().followRecommendedLayout).toBe(true);
  });

  it("keeps a manually opened AMAN board after reconnecting", () => {
    client._emit(EventType.FrontendInitial, initialEvent("AD"));
    store.getState().setDisplayedLayout("AMAN");
    client._emit(EventType.FrontendLayoutUpdate, {type: EventType.FrontendLayoutUpdate, layout: "AAAD"});
    client._emit(EventType.FrontendInitial, initialEvent("AD"));

    expect(store.getState().displayedLayout).toBe("AMAN");
    expect(store.getState().followRecommendedLayout).toBe(false);
  });

  it("sends SI selection without creating coordination or changing the strip locally", () => {
    const strip = { ...foreignStrip(), owner: "EKCH_B_GND" };
    store.setState({ position: "EKCH_B_GND", strips: [strip] });

    store.getState().selectNextController("SAS123", "GW");

    expect(client.send).toHaveBeenCalledTimes(1);
    expect(client.send).toHaveBeenCalledWith({
      type: ActionType.FrontendNextControllerSelect,
      callsign: "SAS123",
      sector: "GW",
    });
    expect(store.getState().strips[0]).toEqual(strip);
    expect(store.getState().stripTransfers).toEqual({});
  });

  it("sends a combined startup request and transfer for an owned SEQ PLN strip", async () => {
    store.setState({
      displayedLayout: "SEQPLN",
      position: "EKCH_B_GND",
      strips: [{ ...foreignStrip(), owner: "EKCH_B_GND", bay: Bay.Cleared }],
    });

    await expect(store.getState().startRequestAndTransfer("SAS123")).resolves.toBe(true);
    expect(client.send).toHaveBeenCalledWith({
      type: ActionType.FrontendCoordinationTransferRequest,
      callsign: "SAS123",
      to: "EKCH_C_GND",
      start_req_transfer: true,
    });
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
      type: ActionType.FrontendCoordinationTransferRequest,
      callsign: "SAS123",
      to: "EKCH_C_GND",
      start_req_transfer: true,
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
    expect(client.reconnect).not.toHaveBeenCalled();

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
      start_req_transfer: true,
    });
  });

  it.each([
    [ActionType.FrontendUpdateStripData, "non-owner cannot modify strip fields"],
    [ActionType.FrontendMove, "pushback timing validation"],
    [ActionType.FrontendReleasePoint, "strip is owned by another controller"],
  ])("reconciles %s without resetting the board", (action, reason) => {
    client._emit(EventType.FrontendInitial, initialEvent("AD"));
    const serverStrip = { ...foreignStrip(), route: "DCT LANGO", bay: Bay.Cleared };
    store.setState({ strips: [{ ...serverStrip, route: "STALE", bay: Bay.Push }],
      displayedLayout: "GND", followRecommendedLayout: false, selectedCallsign: "SAS123",
      validationDialogCallsign: "SAS123", contextMenu: { callsign: "SAS123", x: 1, y: 2 } });
    client._emit(EventType.FrontendActionRejected, {
      type: EventType.FrontendActionRejected,
      action, reason,
    });

    expect(client.reconnect).not.toHaveBeenCalled();
    expect(store.getState().reconciling).toBe(true);
    expect(client.send).toHaveBeenCalledExactlyOnceWith({ type: ActionType.FrontendActionReconciliation, request_id: "refresh-1" });
    store.getState().move("SAS123", Bay.Taxi);
    expect(client.send).toHaveBeenCalledTimes(1);
    client._emit(EventType.FrontendActionReconciliation, {
      ...initialEvent("AD"), type: EventType.FrontendActionReconciliation, request_id: "refresh-1",
      strips: [serverStrip], coordinations: [{ callsign: "SAS123", from: "EKCH_A_GND", to: "EKCH_C_GND", is_tag_request: false }],
    });
    expect(store.getState()).toMatchObject({ reconciling: false, readOnly: false,
      displayedLayout: "GND", followRecommendedLayout: false, selectedCallsign: "SAS123",
      validationDialogCallsign: "SAS123", contextMenu: { callsign: "SAS123", x: 1, y: 2 },
      stripTransfers: { SAS123: { from: "EKCH_A_GND", to: "EKCH_C_GND" } } });
    expect(store.getState().strips).toEqual([serverStrip]);
    expect(client.send).toHaveBeenCalledTimes(1); // Never replay a rejected command.
    expect(client.setReadOnly).toHaveBeenLastCalledWith(false);
  });

  it("coalesces rejections and keeps writes fenced if refresh fails", () => {
    vi.useFakeTimers();
    client._emit(EventType.FrontendActionRejected, { action: "move", reason: "rejected" });
    client._emit(EventType.FrontendActionRejected, { action: "update_strip_data", reason: "rejected" });
    expect(client.send).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(5_000);
    expect(store.getState().reconciling).toBe(true);
    store.getState().setStartReq("SAS123", true);
    expect(client.send).toHaveBeenCalledTimes(1);
    // A late response to this same request can still recover the state.
    client._emit(EventType.FrontendActionReconciliation, { ...initialEvent("AD"), request_id: "refresh-1" });
    expect(store.getState().reconciling).toBe(false);
    expect(client.reconnect).not.toHaveBeenCalled();
  });

  it("does not loop on a rejected refresh or accept an unrelated response", () => {
    const errors = vi.spyOn(toast, "error");
    client._emit(EventType.FrontendActionRejected, { action: "move", reason: "rejected" });
    client._emit(EventType.FrontendActionReconciliation, { ...initialEvent("AD"), request_id: "old-request" });
    expect(store.getState().reconciling).toBe(true);
    client._emit(EventType.FrontendActionRejected, { action: ActionType.FrontendActionReconciliation, reason: "unavailable" });
    expect(client.send).toHaveBeenCalledTimes(1);
    expect(store.getState().reconciling).toBe(true);
    const retry = errors.mock.calls.at(-1)?.[1]?.action;
    if (!retry || typeof retry !== "object" || !("onClick" in retry)) throw new Error("Missing refresh retry action");
    retry.onClick({} as React.MouseEvent<HTMLButtonElement>);
    expect(client.send).toHaveBeenLastCalledWith({ type: ActionType.FrontendActionReconciliation, request_id: "refresh-2" });
    client._emit(EventType.FrontendActionReconciliation, { ...initialEvent("AD"), request_id: "refresh-1" });
    client._emit(EventType.FrontendActionRejected, { action: ActionType.FrontendActionReconciliation, request_id: "refresh-1", reason: "old failure" });
    expect(store.getState().reconciling).toBe(true);
    client._emit(EventType.FrontendActionReconciliation, { ...initialEvent("AD"), request_id: "refresh-2" });
    expect(store.getState().reconciling).toBe(false);
    expect(client.reconnect).not.toHaveBeenCalled();
  });

  it("sends one assumption until confirmed and leaves newer transfers alone", () => {
    vi.useFakeTimers();
    store.setState({ position: "EKCH_B_GND", strips: [foreignStrip()] });
    for (let i = 0; i < 9; i++) store.getState().assumeStrip("SAS123");
    expect(client.send).toHaveBeenCalledExactlyOnceWith({ type: ActionType.FrontendCoordinationAssumeRequest, callsign: "SAS123" });
    expect(store.getState().pendingAssumptions.SAS123).toBe(true);
    client._emit(EventType.FrontendCoordinationTransferBroadcast, { callsign: "SAS123", from: "EKCH_A_GND", to: "EKCH_B_GND" });
    client._emit(EventType.FrontendCoordinationAssumeBroadcast, { callsign: "SAS123", position: "EKCH_B_GND" });
    client._emit(EventType.FrontendCoordinationTransferBroadcast, { callsign: "SAS123", from: "EKCH_B_GND", to: "EKCH_C_GND" });
    store.getState().assumeStrip("SAS123");
    expect(store.getState().pendingAssumptions.SAS123).toBeUndefined();
    vi.advanceTimersByTime(5_000);
    expect(client.send).toHaveBeenCalledTimes(1);
    expect(store.getState().stripTransfers.SAS123.to).toBe("EKCH_C_GND");
  });

  it("refreshes an unconfirmed assumption without resending it", () => {
    vi.useFakeTimers();
    store.setState({ strips: [foreignStrip()] });
    store.getState().assumeStrip("SAS123");
    vi.advanceTimersByTime(5_000);
    expect(client.send).toHaveBeenLastCalledWith({ type: ActionType.FrontendActionReconciliation, request_id: "refresh-1" });
    expect(client.send).toHaveBeenCalledTimes(2);
    client._emit(EventType.FrontendActionReconciliation, { ...initialEvent("AD"), request_id: "refresh-1", strips: [foreignStrip()] });
    expect(store.getState().pendingAssumptions).toEqual({});
  });

  it("skips unchanged edits and checks current ownership before any field edit", () => {
    const strip = { ...foreignStrip(), owner: "EKCH_B_GND", route: "DCT LANGO", eobt: "1450" };
    store.setState({ position: "EKCH_B_GND", strips: [strip] });
    store.getState().updateStrip("SAS123", { route: strip.route, eobt: " 1450 " });
    store.getState().assignRunway("SAS123", strip.runway);
    expect(client.send).not.toHaveBeenCalled();
    client._emit(EventType.FrontendOwnersUpdate, { callsign: "SAS123", owner: "EKCH_C_GND", next_owners: [], previous_owners: [] });
    store.getState().updateStrip("SAS123", { route: "OTHER", heading: 180 });
    store.getState().assignRunway("SAS123", "04L");
    store.getState().setReleasePoint("SAS123", "J4");
    expect(client.send).not.toHaveBeenCalled();
    expect(store.getState().strips[0].route).toBe(strip.route);
    expect(store.getState().strips[0].runway).toBe(strip.runway);
  });
});
