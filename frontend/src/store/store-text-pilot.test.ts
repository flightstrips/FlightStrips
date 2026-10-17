import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ActionType, Bay, CommunicationType, EventType, type FrontendStrip } from "@/api/models";
import type { WebSocketClient } from "@/api/websocket";
import { createWebSocketStore } from "./store";

describe("frequency message requests", () => {
  const handlers = new Map<string, (event: unknown) => void>();
  const client = { send: vi.fn((_event: unknown) => true), isConnected: vi.fn(() => true), on: vi.fn((type, handler) => handlers.set(type, handler)) };
  let store: ReturnType<typeof createWebSocketStore>;
  beforeEach(() => {
    vi.useFakeTimers(); vi.clearAllMocks(); handlers.clear(); client.send.mockReturnValue(true); client.isConnected.mockReturnValue(true);
    store = createWebSocketStore(client as unknown as WebSocketClient);
    store.setState({ readOnly: false, strips: [{ callsign: "SAS123", communication_type: CommunicationType.Text, bay: Bay.NotCleared }] as FrontendStrip[] });
  });
  afterEach(() => vi.useRealTimers());
  const result = (extra: Record<string, unknown> = {}) => {
    const req = client.send.mock.calls.at(-1)?.[0] as unknown as { request_id: string; callsign: string };
    handlers.get(EventType.FrontendFrequencyMessageResult)?.({ type: EventType.FrontendFrequencyMessageResult, request_id: req.request_id, callsign: req.callsign, submitted: true, applied_actions: 0, ...extra });
  };
  it("waits for the matching aircraft and request acknowledgement", async () => {
    const pending = store.getState().sendFrequencyMessage("SAS123", "Hold position.");
    let resolved = false; void pending.then(() => { resolved = true; });
    result({ callsign: "BAW456" }); await Promise.resolve(); expect(resolved).toBe(false);
    result(); expect((await pending).submitted).toBe(true);
  });
  it("does not send twice while an aircraft request is pending", async () => {
    const pending = store.getState().sendFrequencyMessage("SAS123", "Hold position.");
    expect((await store.getState().sendFrequencyMessage("SAS123", "Hold position.")).submitted).toBe(false);
    expect(client.send).toHaveBeenCalledTimes(1); result(); await pending;
  });
  it("rejects a late result after controller context changes", async () => {
    store.setState({ position: "121.905", amanSessionID: 42 });
    const pending = store.getState().sendFrequencyMessage("SAS123", "Hold position.");
    store.setState({ position: "118.105" }); result();
    expect(await pending).toMatchObject({ submitted: false, error: expect.stringContaining("context changed") });
  });
  it("retains uncertain actions and never automatically replays after timeout", async () => {
    const action = { type: ActionType.FrontendMove, callsign: "SAS123", bay: Bay.Push } as const;
    const pending = store.getState().sendFrequencyMessage("SAS123", "Push approved.", [action]);
    await vi.advanceTimersByTimeAsync(12000);
    expect(await pending).toMatchObject({ submitted: false, applied_actions: 1 });
    expect(client.send).toHaveBeenCalledTimes(1);
    expect(store.getState().strips[0].bay).toBe(Bay.NotCleared);
  });
  it("rejects observers and disconnected clients", async () => {
    store.setState({ readOnly: true }); expect((await store.getState().sendFrequencyMessage("SAS123", "Hold.")).submitted).toBe(false);
    store.setState({ readOnly: false }); client.isConnected.mockReturnValue(false);
    expect((await store.getState().sendFrequencyMessage("SAS123", "Hold.")).submitted).toBe(false); expect(client.send).not.toHaveBeenCalled();
  });
  it("issues a text clearance without an optimistic move or PDC request", async () => {
    store.getState().move("SAS123", Bay.Cleared, true);
    expect(store.getState().strips[0].bay).toBe(Bay.NotCleared);
    expect(client.send).toHaveBeenCalledWith(expect.objectContaining({ type: ActionType.FrontendSendFrequencyMessage, clearance: true, actions: [{ type: ActionType.FrontendMove, callsign: "SAS123", bay: Bay.Cleared, clearance: true }] }));
    result({ applied_actions: 1 }); await Promise.resolve(); expect(client.send).toHaveBeenCalledTimes(1);
  });
});
