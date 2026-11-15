import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ActionType, EventType, type FrontendVacsRequest } from "@/api/models";
import type { WebSocketClient } from "@/api/websocket";
import { VacsClient } from "./vacs-client";
import type { VacsRelayResult, VacsRelaySnapshot } from "./types";

function fixture() {
  let receive: (result: VacsRelayResult) => void = () => {};
  const off = vi.fn();
  const send = vi.fn((_event: FrontendVacsRequest) => true);
  const on = vi.fn((type, handler) => { expect(type).toBe(EventType.FrontendVacsResult); receive = handler; return off; });
  const client = new VacsClient({send, on} as unknown as Pick<WebSocketClient, "send" | "on">);
  let sequence = 0;
  const reply = (snapshot: Partial<VacsRelaySnapshot> = {}, index = send.mock.calls.length - 1) => {
    receive({request_id: send.mock.calls[index][0].request_id, state: {status: "idle", actionable: true,
      own_client_id: "self", own_position_id: "EKCH_TWR", sequence: ++sequence, ...snapshot}});
  };
  return {client, send, off, reply, receive: (result: VacsRelayResult) => receive(result)};
}
beforeEach(() => vi.useFakeTimers());
afterEach(() => {vi.clearAllTimers(); vi.useRealTimers();});

describe("VACS plugin relay", () => {
  it("uses the backend socket and restores native call state without ending calls on stop", () => {
    const f = fixture(); f.client.start();
    expect(f.send).toHaveBeenCalledWith(expect.objectContaining({type: ActionType.FrontendVacsRequest, action: "status"}));
    f.reply({status: "connected", call_id: "active", label: "EKCH_APP"});
    expect(f.client.getState()).toMatchObject({status: "connected", callId: "active", peer: {displayName: "EKCH_APP"}});
    f.client.stop(); expect(f.send).toHaveBeenCalledTimes(1); expect(f.off).toHaveBeenCalledOnce();
  });
  it("bounds status requests while the plugin is unresponsive", async () => {
    const f = fixture(); f.client.start(); await vi.advanceTimersByTimeAsync(7000);
    expect(f.send).toHaveBeenCalledTimes(1); f.client.stop(); await Promise.resolve();
  });
  it("cancels the native outgoing call and prevents duplicate actions", async () => {
    const f = fixture(); f.client.start(); f.reply({status: "outgoing", call_id: "ringing"});
    const action = f.client.endCall("ringing");
    expect(f.send).toHaveBeenLastCalledWith(expect.objectContaining({action: "cancel", call_id: "ringing"}));
    await expect(f.client.endCall("ringing")).rejects.toThrow("unavailable");
    f.reply({status: "outgoing", pending: true, actionable: false, call_id: "ringing"}); await action;
    expect(f.client.getState().actionable).toBe(false); f.client.stop();
  });
  it("does not retry unconfirmed actions and recovers from native state", async () => {
    const f = fixture(); f.client.start(); f.reply();
    const action = f.client.dialClient({id: "peer", displayName: "Approach", frequency: "119.8"});
    const rejection = expect(action).rejects.toThrow("timed out");
    await vi.advanceTimersByTimeAsync(8000); await rejection;
    expect(f.send.mock.calls.filter(call => call[0].action === "dial")).toHaveLength(1);
    expect(f.client.getState().actionable).toBe(false);
    await vi.advanceTimersByTimeAsync(1000); f.reply({status: "connected", call_id: "recovered", sequence: 20});
    expect(f.client.getState()).toMatchObject({status: "connected", callId: "recovered"}); f.client.stop();
  });
  it("ignores reordered status replies after a newer action result", async () => {
    const f = fixture(); f.client.start(); f.reply(); await vi.advanceTimersByTimeAsync(1000);
    const action = f.client.dialClient({id: "peer", displayName: "Approach", frequency: "119.8"});
    f.reply({status: "outgoing", call_id: "new", sequence: 3}); await action;
    f.reply({status: "idle", sequence: 2}, 1);
    expect(f.client.getState()).toMatchObject({status: "outgoing", callId: "new"}); f.client.stop();
  });
  it("clears pending actions on invalidation and ignores late replies", async () => {
    const f = fixture(); f.client.start(); f.reply({status: "incoming", call_id: "old", waiting: 3});
    const action = f.client.acceptCall("old"), rejection = expect(action).rejects.toThrow("disconnected");
    f.client.invalidate(); await rejection; f.reply({status: "connected", call_id: "old"});
    expect(f.client.getState().status).toBe("unavailable");
    await vi.advanceTimersByTimeAsync(1000); f.reply({sequence: 1});
    expect(f.client.getState().status).toBe("idle"); f.client.stop();
  });
  it("reports unsupported plugins and gates actions during native recovery", async () => {
    const f = fixture(); f.client.start();
    f.receive({request_id: f.send.mock.calls[0][0].request_id, error: "Update the EuroScope plugin"});
    await Promise.resolve(); expect(f.client.getState()).toMatchObject({status: "unavailable", tooltip: "Update the EuroScope plugin"});
    await vi.advanceTimersByTimeAsync(1000); f.reply({status: "synchronizing", actionable: false});
    await expect(f.client.rejectCall("old")).rejects.toThrow("unavailable"); f.client.stop();
  });
  it("rejects replies belonging to another board or browser", () => {
    const a = fixture(), b = fixture(); a.client.start(); b.client.start();
    b.receive({request_id: a.send.mock.calls[0][0].request_id, state: {status: "connected", sequence: 99, call_id: "foreign"}});
    expect(b.client.getState().status).toBe("unavailable"); a.client.stop(); b.client.stop();
  });
});
