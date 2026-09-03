import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand/vanilla";

import { ActionType, Bay, type FrontendStrip } from "@/api/models";
import type { WebSocketClient } from "@/api/websocket";
import { createWebSocketStore, type WebSocketState } from "./store";

describe("strip move actions", () => {
  let client: { send: ReturnType<typeof vi.fn>; on: ReturnType<typeof vi.fn> };
  let store: StoreApi<WebSocketState>;

  beforeEach(() => {
    client = { send: vi.fn(), on: vi.fn() };
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
});
