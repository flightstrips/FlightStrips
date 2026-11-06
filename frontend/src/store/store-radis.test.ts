import {expect, it, vi} from "vitest";
import {createWebSocketStore} from "./store";
import type {WebSocketClient} from "@/api/websocket";
import {EventType, type FrontendStrip} from "@/api/models";

it("sends revisioned RADIS actions without local authority and blocks every action on own disconnect", () => {
  const handlers = new Map<EventType, (event: unknown) => void>();
  const client = {on: vi.fn((type, fn) => {handlers.set(type, fn); return () => handlers.delete(type);}), send: vi.fn(() => true), setReadOnly: vi.fn(), isConnected: () => true};
  const store = createWebSocketStore(client as unknown as WebSocketClient);
  store.setState({isInitialized: true, readOnly: false, ownPluginReady: true, airport: "EKBI", boardDefinition: {view: "radis",board_state:{revisioned:true,recovery_seconds:123}, definition_revision: "EKBI-fixture", actions: ["strip.move", "strip.annotate"], capabilities: {cdm: true, stands: true, text_pilot: true, pdc: false, aman: false}}, strips: [{id: 11, version: 7, callsign: "LOCAL1", bay: "PASSIVE_TOP", sequence: 1000}] as FrontendStrip[]});
  const before = store.getState().strips;
  expect(store.getState().annotateStrip("LOCAL1", "notes", "shared")).toBe(true);
  expect(store.getState().moveBoardStrip("LOCAL1", "ACTIVE", null)).toBe(true);
  expect(store.getState().updateBoardStrip("LOCAL1", {altitude: 2300, assigned_squawk: "2401"})).toBe(true);
  expect(client.send.mock.calls[0]).toEqual([{type: "strip.annotate", callsign: "LOCAL1", strip_id: 11, version: 7, definition_revision: "EKBI-fixture", field: "notes", value: "shared", enabled: false}]);
  expect(store.getState().strips).toBe(before);
  handlers.get(EventType.FrontendDisconnect)!({board:{session_id:1},read_only: true});
  client.send.mockClear();
  expect(store.getState().annotateStrip("LOCAL1", "deicing", "", true)).toBe(false);
  expect(store.getState().moveBoardStrip("LOCAL1", "RUNWAY", null)).toBe(false);
  expect(store.getState().updateBoardStrip("LOCAL1", {heading: 100})).toBe(false);
  expect(client.send).not.toHaveBeenCalled();
  expect(store.getState().strips).toBe(before);
  store.dispose();
});
