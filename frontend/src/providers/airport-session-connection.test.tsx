import {readFileSync} from "node:fs";
import {resolve} from "node:path";
import {StrictMode} from "react";
import {act, cleanup, render, screen} from "@testing-library/react";
import {afterEach, beforeEach, describe, expect, it, vi} from "vitest";
import type {AMANStateEvent} from "@/api/aman";
import {EventType} from "@/api/models";
import type {BoardContext, OwnMembership} from "@/api/sessions";
import {useWebSocketStore} from "@/store/store-hooks";
import {AirportSessionProvider} from "./airport-session-provider";

const mocks = vi.hoisted(() => ({discover: vi.fn(), getToken: vi.fn(), logout: vi.fn()}));
vi.mock("@auth0/auth0-react", () => ({useAuth0: () => ({getAccessTokenSilently: mocks.getToken, logout: mocks.logout, isAuthenticated: true, isLoading: false})}));
vi.mock("@/api/sessions", async original => ({...await original<typeof import("@/api/sessions")>(), discoverSessions: mocks.discover}));

class Socket {
  static OPEN = 1;
  static instances: Socket[] = [];
  readyState = 0;
  onopen?: () => void;
  onclose?: (event: {code: number; reason: string}) => void;
  onmessage?: (event: {data: string}) => void;
  sent: Array<{board: BoardContext}> = [];
  constructor() {Socket.instances.push(this);}
  send(raw: string) {this.sent.push(JSON.parse(raw));}
  close() {this.readyState = 3; this.onclose?.({code: 1000, reason: "closed"});}
  open() {this.readyState = 1; this.onopen?.();}
  emit(data: unknown) {this.onmessage?.({data: JSON.stringify(data)});}
}

const member: OwnMembership = {session_id: 1, airport: "EKCH", environment: "LIVE", instance_id: "own", membership_id: "m", transport_generation: "1", generation: "2", master_epoch: "1", role: "master", read_only: false, ready: true, definition_revision: "r", view: "ekch", actions: [], capabilities: {cdm: false, stands: false, text_pilot: false, pdc: false, aman: true}, callsign: "EKDK_FMP", position: "131.040"};
function discovery(...members: OwnMembership[]) {
  const sessions = new Map<number, {session_id: number; airport: string; environment: string; instances: OwnMembership[]}>();
  for (const m of members) {
    if (!sessions.has(m.session_id)) sessions.set(m.session_id, {session_id: m.session_id, airport: m.airport, environment: m.environment, instances: []});
    sessions.get(m.session_id)!.instances.push(m);
  }
  mocks.discover.mockResolvedValue([...sessions.values()]);
}
const aman = JSON.parse(readFileSync(resolve(process.cwd(), "../backend/pkg/events/frontend/testdata/aman-state-v1.json"), "utf8")) as AMANStateEvent;

function snapshot(socket: Socket, own = member, ready = true) {
  const board = socket.sent[0].board;
  socket.emit({type: EventType.FrontendInitial, board, session_id: own.session_id, airport: own.airport,
    definition: {definition_revision: own.definition_revision, view: own.view, actions: own.actions, capabilities: own.capabilities},
    controllers: [], strips: [], tactical_strips: [], coordinations: [], messages: [], available_sids: [],
    me: {callsign: own.callsign, position: own.position, identifier: "FMP", section: "", owned_sectors: []},
    layout: "AA", callsign: own.callsign, runway_setup: {departure: [], arrival: [], runway_status: {}},
    read_only: !ready, observer: false, own_plugin_ready: ready, capabilities: {aman_fmp: ready},
  });
  socket.emit({...aman, board, data: {...aman.data, session_id: own.session_id}});
}
function Board() {
  const value = useWebSocketStore(s => s);
  return <output data-testid="state">{JSON.stringify({session: value.amanSessionID, instance: value.boardContext?.instance_id, membership: value.boardContext?.membership_id, connection: value.amanConnectionState, ready: value.ownPluginReady, readOnly: value.readOnly, awaiting: value.amanAwaitingSnapshot, fmp: value.amanFMPAuthority, revision: value.amanState?.revision})}</output>;
}
function state() {return JSON.parse(screen.getByTestId("state").textContent!);}
async function mount() {
  await act(async () => {render(<StrictMode><AirportSessionProvider url="ws://board"><Board/></AirportSessionProvider></StrictMode>);});
}
async function poll() {await act(async () => {await vi.advanceTimersByTimeAsync(5000);});}
const latest = () => Socket.instances.at(-1)!;

beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
  sessionStorage.clear();
  window.history.replaceState(null, "", "/");
  Socket.instances = [];
  vi.stubGlobal("WebSocket", Socket);
  mocks.getToken.mockResolvedValue("token");
  discovery();
  vi.spyOn(console, "log").mockImplementation(() => {});
  vi.spyOn(console, "error").mockImplementation(() => {});
});
afterEach(async () => {cleanup(); await Promise.resolve(); vi.useRealTimers(); vi.unstubAllGlobals(); vi.restoreAllMocks();});

describe("discovery through authorized board connection", () => {
  it("waits for discovery and accepts a fast initial snapshot during socket open", async () => {
    await mount();
    expect(Socket.instances).toHaveLength(0);
    discovery(member);
    await poll();
    await act(async () => {latest().open(); snapshot(latest());});
    expect(state()).toMatchObject({session: 1, instance: "own", connection: "connected", ready: true, readOnly: false, awaiting: false, fmp: true, revision: aman.data.revision});
  });

  it("recovers from rejected admission without requiring another discovery change", async () => {
    discovery(member);
    await mount();
    await act(async () => {latest().open(); latest().close();});
    await act(async () => {await vi.advanceTimersByTimeAsync(2000);});
    await act(async () => {latest().open(); snapshot(latest());});
    expect(state()).toMatchObject({connection: "connected", ready: true, readOnly: false});
  });

  it("recovers the same membership from a not-ready snapshot after discovery reports readiness", async () => {
    discovery({...member, ready: false, read_only: true});
    await mount();
    await act(async () => {latest().open(); snapshot(latest(), member, false);});
    expect(state()).toMatchObject({ready: false, readOnly: true});
    const old = latest();
    discovery(member);
    await poll();
    expect(latest()).not.toBe(old);
    await act(async () => {latest().open(); snapshot(latest());});
    expect(state()).toMatchObject({connection: "connected", ready: true, readOnly: false, awaiting: false, fmp: true});
  });

  it("follows only the selected tab's same physical instance when its incarnation changes", async () => {
    sessionStorage.setItem("flightstrips.airport-session.v1", JSON.stringify({session: 1, instance: "own"}));
    discovery(member, {...member, instance_id: "other"}, {...member, session_id: 2, environment: "SWEATBOX"});
    await mount();
    const old = latest();
    await act(async () => {old.open(); snapshot(old);});
    discovery({...member, instance_id: "other"}, {...member, session_id: 2, environment: "SWEATBOX"});
    await poll();
    expect(latest()).toBe(old);
    expect(state()).toMatchObject({session: 1, instance: "own", ready: false, readOnly: true});
    const rejoined = {...member, membership_id: "new", generation: "3", transport_generation: "4"};
    discovery(rejoined, {...member, instance_id: "other"}, {...member, session_id: 2, environment: "SWEATBOX"});
    await poll();
    const next = latest();
    expect(next).not.toBe(old);
    await act(async () => {next.open(); snapshot(next, rejoined); old.open(); snapshot(old); old.close();});
    expect(next.readyState).toBe(Socket.OPEN);
    expect(state()).toMatchObject({session: 1, instance: "own", membership: "new", connection: "connected", ready: true, readOnly: false});
  });
});
