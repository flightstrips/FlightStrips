import {afterEach, beforeEach, describe, expect, it, vi} from "vitest";
import {create, fromBinary, toBinary} from "@bufbuild/protobuf";
import {WebSocketClient} from "./websocket";
import {ActionType, Bay} from "./models";
import {createWebSocketStore} from "@/store/store";
import {FrontendFrameSchema} from "./generated/cluster/v1/wire_pb";
import {CommandOutcome_Status} from "./generated/cluster/v1/storage_pb";

class FakeSocket {
  static OPEN = 1;
  static sockets: FakeSocket[] = [];
  protocol = "flightstrips.frontend.pb.v2";
  readyState = FakeSocket.OPEN;
  binaryType = "";
  sent: Uint8Array[] = [];
  onopen: (() => void) | null = null;
  onclose: ((event: {code: number; reason: string}) => void) | null = null;
  onmessage: ((event: {data: ArrayBuffer}) => void) | null = null;
  onerror: (() => void) | null = null;
  constructor() { FakeSocket.sockets.push(this); }
  send(bytes: Uint8Array) { this.sent.push(new Uint8Array(bytes)); }
  close() { this.readyState = 3; this.onclose?.({code: 1000, reason: ""}); }
  open() { this.onopen?.(); }
  receive(frame: object) {
    const bytes = toBinary(FrontendFrameSchema, create(FrontendFrameSchema, frame as never));
    this.onmessage?.({data: new Uint8Array(bytes).buffer});
  }
  frames() { return this.sent.map(bytes => fromBinary(FrontendFrameSchema, bytes)); }
}

const initial = {protocolRevision: 2, frame: {case: "initial", value: {
  sessionId: 7, airport: "EKCH", me: {cid: "123456", callsign: "EKCH_TWR", position: "TWR"},
}}};

beforeEach(() => {
  FakeSocket.sockets = [];
  sessionStorage.clear();
  vi.stubGlobal("WebSocket", FakeSocket);
});
afterEach(() => vi.unstubAllGlobals());

describe("durable browser action results", () => {
  it("restores rejected optimistic edits on the same socket and retains delta cursors", async () => {
    const client = new WebSocketClient("ws://example/frontEndEvents");
    const store = createWebSocketStore(client);
    client.setToken("token");
    const connected = client.connect();
    const socket = FakeSocket.sockets[0]; socket.open(); await connected;
    socket.receive({protocolRevision: 2, frame: {case: "initial", value: {
      ...initial.frame.value, aggregateRevision: 1n,
      entities: [{key: "SAS123", revision: 1n, value: {value: {case: "strip", value: {
        id: 1n, callsign: "SAS123", bay: "CLEARED", sequence: 1000n, pdcState: "NONE",
      }}}}],
    }}});
    store.getState().move("SAS123", Bay.Taxi);
    expect(store.getState().strips[0].bay).toBe(Bay.Taxi);
    const command = socket.frames().at(-1)!.frame;
    if (command.case !== "command") throw new Error("fixture");
    expect(command.value.expectedEntityRevision).toBeUndefined();
    socket.receive({protocolRevision: 2, frame: {case: "actionResult", value: {
      requestId: command.value.requestId, status: CommandOutcome_Status.FAILED,
      reasonCode: "INVALID_ARGUMENT", detail: "strip is locked by validation",
    }}});
    expect(FakeSocket.sockets).toHaveLength(1);
    expect(client.isConnected()).toBe(true);
    expect(store.getState().strips[0].bay).toBe(Bay.Cleared);
    socket.receive({protocolRevision: 2, frame: {case: "delta", value: {
      aggregate: {target: {case: "session", value: {id: 7}}}, aggregateRevision: 2n,
      changes: [{key: "SAS123", revision: 2n, operation: {case: "upsert", value: {
        value: {case: "strip", value: {id: 1n, callsign: "SAS123", bay: "TAXI", sequence: 2000n, pdcState: "NONE"}},
      }}}],
    }}});
    expect(store.getState().strips[0].bay).toBe(Bay.Taxi);
    client.disconnect();
  });
  it("queries after a lost reply without resending a missing action or storing private text", async () => {
    const client = new WebSocketClient("ws://example/frontEndEvents");
    client.setToken("token");
    const firstConnect = client.connect();
    const first = FakeSocket.sockets[0];
    first.open(); await firstConnect;
    first.receive(initial);
    client.send({type: ActionType.FrontendSendPrivateMessage, callsign: "123456", message: "secret message"});
    const commandFrame = first.frames().at(-1)!.frame;
    if (commandFrame.case !== "command") throw new Error("expected command");
    const id = commandFrame.value.requestId;
    expect(sessionStorage.getItem("flightstrips.pending-actions.v2.123456.7")).toContain(id);
    expect(JSON.stringify(sessionStorage)).not.toContain("secret message");

    client.reconnect();
    const second = FakeSocket.sockets[1];
    second.open(); second.receive(initial);
    expect(second.frames().at(-1)!.frame).toMatchObject({case: "statusQuery", value: {requestIds: [id]}});
    const beforeMissing = second.sent.length;
    second.receive({protocolRevision: 2, frame: {case: "statusMissing", value: {requestId: id}}});
    expect(second.sent).toHaveLength(beforeMissing);
    second.receive({protocolRevision: 2, frame: {case: "actionResult", value: {requestId: id, status: CommandOutcome_Status.ACCEPTED}}});
    expect(sessionStorage.getItem("flightstrips.pending-actions.v2.123456.7")).toContain(id);
    second.receive({protocolRevision: 2, frame: {case: "actionResult", value: {requestId: id, status: CommandOutcome_Status.EXPIRED}}});
    expect(sessionStorage.getItem("flightstrips.pending-actions.v2.123456.7")).not.toContain(id);
    client.disconnect();
  });

  it("shows manual retry after reload when an outcome is missing", async () => {
    const id = crypto.randomUUID();
    sessionStorage.setItem("flightstrips.pending-actions.v2.123456.7", JSON.stringify([{requestId: id, label: "send_private_message"}]));
    const client = new WebSocketClient("ws://example/frontEndEvents");
    const statuses: string[] = [];
    client.onActionStatus(status => { if (status) statuses.push(String(status.status)); });
    client.setToken("token");
    const connected = client.connect();
    const socket = FakeSocket.sockets[0];
    socket.open(); await connected;
    socket.receive(initial);
    expect(socket.frames().at(-1)!.frame).toMatchObject({case: "statusQuery", value: {requestIds: [id]}});
    const before = socket.sent.length;
    socket.receive({protocolRevision: 2, frame: {case: "statusMissing", value: {requestId: id}}});
    expect(socket.sent).toHaveLength(before);
    expect(statuses).toContain("not-confirmed");
    client.disconnect();
  });

  it("keeps an accepted effect pending and queries for its terminal outcome", async () => {
    vi.useFakeTimers();
    try {
      const client = new WebSocketClient("ws://example/frontEndEvents");
      client.setToken("token");
      const connected = client.connect();
      const socket = FakeSocket.sockets[0];
      socket.open(); await connected;
      socket.receive(initial);
      client.send({type: ActionType.FrontendSendPrivateMessage, callsign: "123456", message: "secret"});
      const frame = socket.frames().at(-1)!.frame;
      if (frame.case !== "command") throw new Error("expected command");
      const id = frame.value.requestId;
      socket.receive({protocolRevision: 2, frame: {case: "actionResult", value: {requestId: id, status: CommandOutcome_Status.ACCEPTED}}});
      vi.advanceTimersByTime(5000);
      expect(socket.frames().at(-1)!.frame).toMatchObject({case: "statusQuery", value: {requestIds: [id]}});
      socket.receive({protocolRevision: 2, frame: {case: "actionResult", value: {requestId: id, status: CommandOutcome_Status.UNKNOWN}}});
      const count = socket.sent.length;
      vi.advanceTimersByTime(5000);
      expect(socket.sent).toHaveLength(count);
      client.disconnect();
    } finally {
      vi.useRealTimers();
    }
  });
});
