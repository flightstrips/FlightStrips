import {describe, expect, it, vi} from "vitest";
import {EventType, type FrontendInitialEvent} from "@/api/models";
import type {InboundDeparturesEvent} from "@/api/inbound-departures";
import type {WebSocketClient} from "@/api/websocket";
import {createWebSocketStore} from "./store";

const snapshot: InboundDeparturesEvent = {
  type: EventType.FrontendInboundDepartures, session_id: 1, airport: "EKCH", enabled: true,
  updated_at: "2026-10-13T20:43:00Z", stale: false, flights: [{
    callsign: "FBT08F", departure: "LFLL", arrival: "EKCH", ctot: "1942", atot: "1924", eta: "2120",
    status: "early", deviation_seconds: -1080, outside_window_seconds: 780,
  }],
};

function setup() {
  const handlers = new Map<string, (data: unknown) => void>();
  const client = {on: (event: string, callback: (data: unknown) => void) => handlers.set(event, callback), setReadOnly: vi.fn()} as unknown as WebSocketClient;
  const store = createWebSocketStore(client);
  const initial = {
    type: EventType.FrontendInitial, session_id: 1, controllers: [], strips: [], tactical_strips: [],
    me: {callsign: "EKCH_APP", position: "APP", identifier: "APP", section: "", owned_sectors: []},
    airport: "EKCH", layout: "AA", callsign: "EKCH_APP", runway_setup: {departure: [], arrival: [], runway_status: {}},
    coordinations: [], messages: [], available_sids: [], initial_cfl_by_runway: {}, transition_altitude: 7000,
    read_only: false, position_available: true,
    stand_assignment_enabled: false, stand_assignments: [], stand_blocks: [], inbound_departures: snapshot,
  } satisfies FrontendInitialEvent;
  const emit = (event: string, data: unknown) => handlers.get(event)?.(data);
  emit(EventType.FrontendInitial, initial);
  return {store, emit, initial};
}

describe("inbound departure snapshots", () => {
  it("loads cached findings and replaces them with subsequent updates", () => {
    const {store, emit} = setup();
    expect(store.getState().inboundDepartures).toEqual(snapshot);
    const staleSnapshot = {...snapshot, stale: true};
    emit(EventType.FrontendInboundDepartures, staleSnapshot);
    expect(store.getState().inboundDepartures).toEqual(staleSnapshot);
    emit(EventType.FrontendInboundDepartures, {...snapshot, flights: []});
    expect(store.getState().inboundDepartures?.flights).toEqual([]);
  });

  it("rejects other-session and other-airport events and resets on a new initial snapshot", () => {
    const {store, emit, initial} = setup();
    emit(EventType.FrontendInboundDepartures, {...snapshot, session_id: 2, flights: []});
    emit(EventType.FrontendInboundDepartures, {...snapshot, airport: "EKBI", flights: []});
    expect(store.getState().inboundDepartures?.flights).toHaveLength(1);
    emit(EventType.FrontendInitial, {...initial, session_id: 2, inbound_departures: undefined});
    expect(store.getState().inboundDepartures).toBeNull();
  });
});
