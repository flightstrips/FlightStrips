import {afterEach, describe, expect, it, vi} from "vitest";
import {WebSocketClient} from "./websocket";
import {ActionType, EventType} from "./models";
import type {BoardContext, OwnMembership} from "./sessions";

class Socket {
  static OPEN = 1;
  static instances: Socket[] = [];
  readyState = 1;
  onopen?: () => void;
  onmessage?: (event: {data: string}) => void;
  onclose?: (event: {code: number; reason: string}) => void;
  sent: Array<Record<string, unknown>> = [];
  constructor() {Socket.instances.push(this);}
  send(raw: string) {this.sent.push(JSON.parse(raw));}
  close() {this.readyState = 3;}
  emit(event: unknown) {this.onmessage?.({data: JSON.stringify(event)});}
}
const own: OwnMembership = {session_id: 1, airport: "EKCH", environment: "LIVE", instance_id: "own", membership_id: "m", transport_generation: "18446744073709551615", generation: "18446744073709551614", master_epoch: "1", role: "slave", read_only: false, ready: true, definition_revision: "revision", view: "ekch", actions: ["strip.move"], capabilities: {cdm: true, stands: true, text_pilot: true, pdc: true, aman: true}, callsign: "OWN", position: "118.000"};
afterEach(() => {vi.unstubAllGlobals(); Socket.instances = [];});
async function connect() {
  vi.stubGlobal("WebSocket", Socket);
  const client = new WebSocketClient("ws://board", undefined, own);
  client.setToken("token");
  const connected = client.connect();
  const socket = Socket.instances.at(-1)!;
  socket.onopen!();
  await connected;
  return {client, socket, board: socket.sent[0].board as BoardContext};
}

describe("exact board subscription transport", () => {
  it("authenticates and sends every action through the same exact own incarnation", async () => {
    const {client, socket, board} = await connect();
    expect(board).toMatchObject({session_id: 1, instance_id: "own", generation: "18446744073709551614", transport_generation: "18446744073709551615", environment: "LIVE"});
    expect(board.subscription_id).not.toBe("");
    expect(client.send({type: ActionType.FrontendMarked, callsign: "SAS123", marked: true})).toBe(false);
    client.setReadOnly(false);
    expect(client.send({type: ActionType.FrontendMarked, callsign: "SAS123", marked: true})).toBe(true);
    expect(socket.sent.at(-1)?.board).toEqual(board);
    client.disconnect();
  });
  it("rejects foreign context for all event and result families", async () => {
    const {client, socket, board} = await connect();
    let received = 0;
    for (const type of Object.values(EventType)) {
      client.on(type, () => received++);
      socket.emit({type, board: {...board, environment: "SWEATBOX_test"}});
      socket.emit({type, board: {...board, generation: "old"}});
      socket.emit({type, board: {...board, subscription_id: "old"}});
      socket.emit({type});
    }
    expect(received).toBe(0);
    socket.emit({type: EventType.FrontendMarked, board, callsign: "SAS123", marked: true});
    expect(received).toBe(1);
    client.disconnect();
  });
  it("accepts snapshots only for the selected session, definition and compiled view", async () => {
    const {client, socket, board} = await connect();
    const receive = vi.fn();
    client.on(EventType.FrontendInitial, receive);
    const initial = {type: EventType.FrontendInitial, board, session_id: 1, airport: "EKCH", definition: {definition_revision: "revision", view: "ekch"}};
    socket.emit({...initial, session_id: 2});
    socket.emit({...initial, airport: "AAAA"});
    socket.emit({...initial, definition: {...initial.definition, view: "unknown"}});
    socket.emit({...initial, definition: {...initial.definition, definition_revision: "old"}});
    expect(receive).not.toHaveBeenCalled();
    socket.emit(initial);
    expect(receive).toHaveBeenCalledOnce();
    client.disconnect();
  });
});
