import { beforeEach, describe, expect, it, vi } from "vitest";
import { ActionType, Bay, type FrontendStrip, type TacticalStrip } from "@/api/models";
import type { WebSocketClient } from "@/api/websocket";
import { createWebSocketStore, type WebSocketState } from "./store";

describe("runway clearance actions", () => {
  const client = {
    send: vi.fn(),
    on: vi.fn(),
  };
  const store = createWebSocketStore(client as unknown as WebSocketClient);

  const makeStrip = (callsign: string, bay: Bay, cleared = false, confirmed = false) =>
    ({ callsign, bay, owner: "TWR", sequence: 1, runway: "22L", runway_cleared: cleared, runway_confirmed: confirmed }) as FrontendStrip;

  beforeEach(() => {
    client.send.mockClear();
    store.setState({
      strips: [],
      tacticalStrips: [],
      readOnly: false,
      position: "TWR",
      stripTransfers: {},
    } as Partial<WebSocketState>);
  });

  it("marks entry green when RWY ARR is empty", () => {
    store.setState({ strips: [makeStrip("FIRST", Bay.Final)] });

    store.getState().move("FIRST", Bay.RwyArr);

    expect(store.getState().strips[0]).toMatchObject({
      bay: Bay.RwyArr,
      runway_cleared: true,
      runway_confirmed: true,
    });
  });

  it("marks entry red when a flight or tactical strip already occupies RWY ARR", () => {
    store.setState({
      strips: [makeStrip("FIRST", Bay.RwyArr, true, true), makeStrip("SECOND", Bay.Final)],
    });

    store.getState().move("SECOND", Bay.RwyArr);
    expect(store.getState().strips[1]).toMatchObject({ runway_cleared: true, runway_confirmed: false });

    store.setState({
      strips: [makeStrip("THIRD", Bay.Final)],
      tacticalStrips: [{ id: 42, bay: Bay.RwyArr } as TacticalStrip],
    });
    store.getState().move("THIRD", Bay.RwyArr);
    expect(store.getState().strips[0]).toMatchObject({ runway_cleared: true, runway_confirmed: false });
  });

  it("blocks red-to-green while another strip is green and allows it after clearance is removed", () => {
    store.setState({
      strips: [makeStrip("FIRST", Bay.RwyArr, true, true), makeStrip("SECOND", Bay.RwyArr, true)],
    });

    store.getState().runwayConfirmation("SECOND");
    expect(client.send).not.toHaveBeenCalled();
    expect(store.getState().strips[1].runway_confirmed).toBe(false);

    store.setState({
      strips: [makeStrip("FIRST", Bay.RwyArr), makeStrip("SECOND", Bay.RwyArr, true)],
    });
    store.getState().runwayConfirmation("SECOND");
    expect(client.send).toHaveBeenCalledWith({
      type: ActionType.FrontendRunwayConfirmation,
      callsign: "SECOND",
    });
    expect(store.getState().strips[1].runway_confirmed).toBe(true);
  });

  it("clears green status when the runway field is toggled off", () => {
    store.setState({ strips: [makeStrip("FIRST", Bay.RwyArr, true, true)] });

    store.getState().resetRunwayClearance("FIRST");

    expect(store.getState().strips[0]).toMatchObject({
      runway_cleared: false,
      runway_confirmed: false,
    });
  });

  it("blocks RWY ARR clearance changes for non-owning controllers", () => {
    store.setState({
      position: "APN",
      strips: [makeStrip("FIRST", Bay.RwyArr, true, true)],
    });

    store.getState().resetRunwayClearance("FIRST");
    store.getState().runwayConfirmation("FIRST");

    expect(client.send).not.toHaveBeenCalled();
    expect(store.getState().strips[0]).toMatchObject({ runway_cleared: true, runway_confirmed: true });
  });

  it("blocks moves through bay actions and pickup during a non-tag transfer", () => {
    store.setState({
      strips: [makeStrip("HANDOVER", Bay.Final)],
      stripTransfers: { HANDOVER: { from: "TWR", to: "APP", isTagRequest: false } },
    });

    store.getState().move("HANDOVER", Bay.RwyArr);
    store.getState().moveToControlzone("HANDOVER");
    store.getState().pickupStrip("HANDOVER", Bay.Push);

    expect(client.send).not.toHaveBeenCalled();
    expect(store.getState().strips[0].bay).toBe(Bay.Final);
  });

  it("keeps movement available during a tag request", () => {
    store.setState({
      strips: [makeStrip("TAG", Bay.Final)],
      stripTransfers: { TAG: { from: "TWR", to: "APP", isTagRequest: true } },
    });

    store.getState().move("TAG", Bay.RwyArr);

    expect(client.send).toHaveBeenCalled();
    expect(store.getState().strips[0].bay).toBe(Bay.RwyArr);
  });

  it("restores landing clearance after toggling green off", () => {
    store.setState({ strips: [makeStrip("FIRST", Bay.RwyArr, true, true)] });

    store.getState().resetRunwayClearance("FIRST");
    store.getState().runwayConfirmation("FIRST");

    expect(client.send).toHaveBeenLastCalledWith({
      type: ActionType.FrontendRunwayConfirmation,
      callsign: "FIRST",
    });
    expect(store.getState().strips[0]).toMatchObject({
      bay: Bay.RwyArr,
      runway_cleared: true,
      runway_confirmed: true,
    });
  });

  it("does not restore landing clearance while another RWY ARR strip is green", () => {
    store.setState({
      strips: [makeStrip("FIRST", Bay.RwyArr), makeStrip("SECOND", Bay.RwyArr, true, true)],
    });

    store.getState().runwayConfirmation("FIRST");

    expect(client.send).not.toHaveBeenCalled();
    expect(store.getState().strips[0]).toMatchObject({
      runway_cleared: false,
      runway_confirmed: false,
    });
  });
});
