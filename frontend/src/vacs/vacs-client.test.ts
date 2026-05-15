import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MockWebSocket } from "@/test/mock-websocket";
import { VacsClient, resetVacsClientForTest } from "./vacs-client";
import type { SessionStateSnapshot } from "./types";

function idleSnapshot(overrides: Partial<SessionStateSnapshot> = {}): SessionStateSnapshot {
  return {
    connectionState: "connected",
    sessionInfo: {
      client: {
        id: "1000001",
        displayName: "EKCH_TWR",
        frequency: "118.300",
        positionId: "EKCH_TWR",
      },
      profile: { type: "Unchanged" },
    },
    stations: [],
    clients: [
      {
        id: "1000002",
        displayName: "EKCH_APP",
        frequency: "120.200",
        positionId: "EKCH_APP",
      },
    ],
    clientId: "1000001",
    callConfig: {},
    clientPageSettings: {},
    capabilities: {},
    incomingCalls: [],
    outgoingCall: null,
    ...overrides,
  };
}

function response(id: string, data: unknown) {
  return JSON.stringify({ type: "response", id, ok: true, data });
}

async function flushBootstrap(ws: MockWebSocket, snapshot = idleSnapshot()): Promise<void> {
  await vi.waitFor(() => {
    expect(ws.sent.some((m) => m.includes("app_frontend_ready"))).toBe(true);
  });
  const ready = JSON.parse(
    ws.sent.find((m) => m.includes("app_frontend_ready"))!,
  ) as { id: string };
  ws.emit("message", { data: response(ready.id, null) });

  await vi.waitFor(() => {
    expect(ws.sent.some((m) => m.includes("remote_get_session_state"))).toBe(true);
  });
  const session = JSON.parse(
    ws.sent.find((m) => m.includes("remote_get_session_state"))!,
  ) as { id: string };
  ws.emit("message", { data: response(session.id, snapshot) });
  await Promise.resolve();
}

describe("VacsClient", () => {
  let client: VacsClient;

  beforeEach(() => {
    MockWebSocket.reset();
    vi.useFakeTimers();
    client = new VacsClient({
      createWebSocket: (url) => new MockWebSocket(url) as unknown as WebSocket,
    });
  });

  afterEach(() => {
    client.stop();
    resetVacsClientForTest();
    vi.useRealTimers();
  });

  it("connects, subscribes, and reaches idle from snapshot", async () => {
    client.start();
    const ws = MockWebSocket.instances[0]!;
    ws.simulateOpen();
    await flushBootstrap(ws);

    expect(ws.url).toContain("9600");
    expect(ws.sent.filter((m) => m.includes('"type":"subscribe"')).length).toBeGreaterThan(0);
    expect(client.getState()).toEqual({
      status: "idle",
      clients: idleSnapshot().clients,
      ownPositionId: "EKCH_TWR",
      ownClientId: "1000001",
    });
  });

  it("handles incoming call accept flow", async () => {
    client.simulateOpenForTest();
    const invite = {
      callId: "call-in-1",
      source: { clientId: "1000002", positionId: "EKCH_APP" },
      target: { client: "1000001" },
      prio: false,
    };
    client.handleMessageForTest(
      JSON.stringify({ type: "event", name: "signaling:call-invite", payload: invite }),
    );
    expect(client.getState().status).toBe("incoming");

    client.start();
    const ws = MockWebSocket.instances[0]!;
    const acceptPromise = client.acceptCall("call-in-1");
    await vi.waitFor(() => expect(ws.sent.some((m) => m.includes("signaling_accept_call"))).toBe(true));
    const invoke = ws.sent
      .map((m) => JSON.parse(m) as { type: string; id?: string })
      .find((m) => m.type === "invoke" && m.id);
    ws.emit("message", { data: response(invoke!.id!, null) });
    await acceptPromise;

    client.handleMessageForTest(
      JSON.stringify({
        type: "event",
        name: "webrtc:call-connected",
        payload: "call-in-1",
      }),
    );
    const state = client.getState();
    expect(state.status).toBe("connected");
    if (state.status === "connected") {
      expect(state.callId).toBe("call-in-1");
      expect(state.peer?.id).toBe("1000002");
    }
  });

  it("handles outgoing dial flow", async () => {
    client.simulateOpenForTest();
    client.start();
    const ws = MockWebSocket.instances[0]!;
    const dialPromise = client.dialClient({
      id: "1000002",
      displayName: "EKCH_APP",
      frequency: "120.200",
      positionId: "EKCH_APP",
    });
    await vi.waitFor(() => expect(ws.sent.some((m) => m.includes("signaling_start_call"))).toBe(true));
    const invokes = ws.sent.map((m) => JSON.parse(m) as { type: string; id?: string });
    const invoke = [...invokes].reverse().find((m) => m.type === "invoke");
    ws.emit("message", { data: response(invoke!.id!, "call-out-1") });
    await dialPromise;
    expect(client.getState().status).toBe("idle");

    client.handleMessageForTest(
      JSON.stringify({
        type: "event",
        name: "webrtc:call-connected",
        payload: "call-out-1",
      }),
    );
    expect(client.getState().status).toBe("connected");
  });

  it("restores idle state from snapshot after reconnect", async () => {
    client.start();
    const ws1 = MockWebSocket.instances[0]!;
    ws1.simulateOpen();
    await flushBootstrap(ws1);
    client.handleMessageForTest(
      JSON.stringify({
        type: "event",
        name: "webrtc:call-connected",
        payload: "old-call",
      }),
    );
    expect(client.getState().status).toBe("connected");

    ws1.close();
    expect(client.getState().status).toBe("unavailable");

    await vi.advanceTimersByTimeAsync(1_000);
    const ws2 = MockWebSocket.instances[1]!;
    ws2.simulateOpen();
    await flushBootstrap(ws2);
    expect(client.getState().status).toBe("idle");
  });

  it("accepts FIFO for multiple incoming calls", async () => {
    client.simulateOpenForTest();
    client.handleMessageForTest(
      JSON.stringify({
        type: "event",
        name: "signaling:call-invite",
        payload: {
          callId: "c1",
          source: { clientId: "2", positionId: "P2" },
          target: {},
          prio: false,
        },
      }),
    );
    client.handleMessageForTest(
      JSON.stringify({
        type: "event",
        name: "signaling:call-invite",
        payload: {
          callId: "c2",
          source: { clientId: "3", positionId: "P3" },
          target: {},
          prio: false,
        },
      }),
    );
    const state = client.getState();
    expect(state.status).toBe("incoming");
    if (state.status === "incoming") {
      expect(state.calls[0]!.callId).toBe("c1");
    }

    client.start();
    const ws = MockWebSocket.instances[0]!;
    const p = client.acceptCall("c1");
    await vi.waitFor(() => expect(ws.sent.length).toBeGreaterThan(0));
    const invokes = ws.sent.map((m) => JSON.parse(m) as { type: string; id?: string });
    const invoke = [...invokes].reverse().find((m) => m.type === "invoke");
    ws.emit("message", { data: response(invoke!.id!, null) });
    await p;
  });

  it("rejects invoke on timeout", async () => {
    client.simulateOpenForTest();
    client.start();
    const p = client.dialClient({
      id: "999",
      displayName: "UNKNOWN",
      frequency: "118.000",
    });
    const assertion = expect(p).rejects.toThrow("VACS invoke timeout");
    await vi.advanceTimersByTimeAsync(50_000);
    await assertion;
  });

  it("transitions to unauthenticated on auth event", () => {
    client.simulateOpenForTest();
    client.handleMessageForTest(
      JSON.stringify({ type: "event", name: "auth:unauthenticated", payload: null }),
    );
    expect(client.getState().status).toBe("unauthenticated");
  });
});
