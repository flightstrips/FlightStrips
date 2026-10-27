import {readFileSync} from "node:fs";
import {act, cleanup, fireEvent, render, screen, within} from "@testing-library/react";
import {afterEach, beforeEach, describe, expect, it, vi} from "vitest";

import {isAMANStateEvent} from "@/api/aman";
import {EventType} from "@/api/models";
import type {WebSocketClient} from "@/api/websocket";
import AMAN from "@/routes/ekch/AMAN";
import {createWebSocketStore} from "@/store/store";
import {WebSocketStoreContext} from "@/store/store-context";

const path = process.env.AMAN_CONTRACT_OUTPUT;
if (!path) throw new Error("Run npm run test:contract:aman first.");
const scenarios = JSON.parse(readFileSync(path, "utf8")) as {name: string; event: unknown}[];
function scenario(name: string) {
  const event = structuredClone(scenarios.find((item) => item.name === name)?.event);
  if (!isAMANStateEvent(event)) throw new Error(`Invalid publisher scenario: ${name}`);
  return event;
}

// Only replace the network boundary. Use the actual event handlers, Zustand
// store, subscriptions, route and child components with serialized Go output.
function workspace(name: string, fmp = true) {
  const handlers = new Map<string, ((event: unknown) => void)[]>();
  const send = vi.fn();
  const client = {
    on: (type: string, handler: (event: unknown) => void) => handlers.set(type, [...(handlers.get(type) ?? []), handler]),
    send, reconnect: vi.fn(), setReadOnly: vi.fn(),
  } as unknown as WebSocketClient;
  const store = createWebSocketStore(client);
  const emit = (type: string, event: unknown) => act(() => handlers.get(type)?.forEach((handler) => handler(event)));
  const event = scenario(name);
  emit(EventType.FrontendInitial, {
    type: EventType.FrontendInitial, session_id: event.data.session_id,
    airport: "EKCH", layout: "AA", callsign: "EKDK_FMP", controllers: [], strips: [], tactical_strips: [],
    me: {callsign: "EKDK_FMP", position: "131.040", identifier: "FMP", section: "", owned_sectors: []},
    runway_setup: {departure: [], arrival: [], runway_status: {}}, coordinations: [], messages: [],
    available_sids: [], initial_cfl_by_runway: {}, transition_altitude: 7000, read_only: false,
    position_available: true, stand_assignment_enabled: false, stand_assignments: [], stand_blocks: [],
    capabilities: {aman_fmp: fmp},
  });
  act(() => store.getState().setAMANConnectionState("connected"));
  emit(EventType.FrontendAMANState, event);
  render(<WebSocketStoreContext.Provider value={store}><AMAN /></WebSocketStoreContext.Provider>);
  return {store, send, emit, event};
}

describe("AMAN pre-merge smoke: publisher → event handler → store → workspace", () => {
  beforeEach(() => localStorage.clear());
  afterEach(cleanup);

  it.each(scenarios)("renders MAESTRO, TMT and current diagnostics for $name", ({name}) => {
    const {store, event} = workspace(name);
    expect(store.getState().amanError).toBeNull();
    expect(store.getState().amanState).not.toBeNull();
    expect(screen.getByRole("main", {name: "Arrival management workspace"})).toBeInTheDocument();
    expect(screen.queryByText("AMAN timeline unavailable")).not.toBeInTheDocument();
    expect(screen.queryByRole("region", {name: "AMAN timeline configuration unavailable"})).not.toBeInTheDocument();
    const tmt = screen.getByRole("region", {name: "TMT traffic prediction"});
    expect(within(tmt).getAllByRole("listitem")).toHaveLength(12);
    const start = Date.parse(event.data.generated_at);
    for (const flight of event.data.flights) {
      const slot = flight.slot && Date.parse(flight.slot.time);
      if (slot && slot >= start && slot <= start + 90 * 60_000) {
        expect(screen.getByTestId(`operational-marker-${flight.callsign}`)).toBeInTheDocument();
      }
    }
    fireEvent.click(screen.getByRole("tab", {name: `Warnings (${event.data.warnings?.length ?? 0})`}));
    for (const warning of event.data.warnings ?? []) {
      expect(screen.getByText(warning.message)).toBeInTheDocument();
    }
  });

  it("keeps healthy targets visible while an unreliable flight recovers", () => {
    const {store, emit} = workspace("unreliable_with_healthy_neighbour");
    expect(screen.getByRole("button", {name: /Select SAS123/})).toBeInTheDocument();
    const warned = store.getState().amanState!;
    expect(warned.flights.find((flight) => flight.callsign === "CCA877Q")?.gain_loss_seconds).toBeNull();
    emit(EventType.FrontendAMANState, scenario("recovered"));
    expect(store.getState().amanError).toBeNull();
    expect(store.getState().amanWarnings.items.some((warning) => warning.code === "prediction_unreliable")).toBe(false);
    expect(screen.queryByText("AMAN timeline unavailable")).not.toBeInTheDocument();
    expect(screen.getByRole("region", {name: "TMT traffic prediction"})).toBeInTheDocument();
  });

  it("sends commands with the published revision and refuses them while disconnected", () => {
    const {store, send, event} = workspace("healthy");
    act(() => store.getState().sendAMANCommand({type: "aman.lock_flight", callsign: "SAS123"}));
    expect(send).toHaveBeenCalledWith(expect.objectContaining({
      type: "aman.lock_flight", version: 1,
      data: expect.objectContaining({callsign: "SAS123", expected_revision: event.data.revision}),
    }));
    act(() => store.getState().setAMANConnectionState("disconnected"));
    send.mockClear();
    expect(store.getState().sendAMANCommand({type: "aman.unlock_flight", callsign: "SAS123"})).toBeNull();
    expect(send).not.toHaveBeenCalled();
  });

  it("keeps observers read-only", () => {
    const {store, send} = workspace("healthy", false);
    expect(store.getState().sendAMANCommand({type: "aman.lock_flight", callsign: "SAS123"})).toBeNull();
    expect(send).not.toHaveBeenCalled();
  });

  it("keeps AMAN operational when only one aircraft has degraded input", () => {
    const {store, send} = workspace("single_flight_input_degraded");
    expect(store.getState().amanState?.technical_health.ready).toBe(true);
    expect(store.getState().amanState?.flights[0].data_status).toBe("stale");
    act(() => store.getState().sendAMANCommand({type: "aman.lock_flight", callsign: "SAS123"}));
    expect(send).toHaveBeenCalledOnce();
    expect(screen.getByRole("region", {name: "TMT traffic prediction"})).toBeInTheDocument();
    expect(screen.queryByText("AMAN timeline unavailable")).not.toBeInTheDocument();
  });

  it("restricts commands for an actual airport source outage while retaining the workspace", () => {
    const {store, send} = workspace("airport_source_outage");
    expect(store.getState().amanState?.technical_health.ready).toBe(false);
    expect(store.getState().sendAMANCommand({type: "aman.lock_flight", callsign: "SAS123"})).toBeNull();
    expect(send).not.toHaveBeenCalled();
    expect(screen.queryByText("AMAN timeline unavailable")).not.toBeInTheDocument();
    expect(screen.getByRole("region", {name: "TMT traffic prediction"})).toBeInTheDocument();
  });

  it("ignores old replacements but accepts the first complete snapshot after reconnect", () => {
    const {store, emit, event} = workspace("healthy");
    const newer = structuredClone(event);
    newer.data.revision++;
    newer.data.flights[0].slot!.revision++;
    emit(EventType.FrontendAMANState, newer);
    emit(EventType.FrontendAMANState, event);
    expect(store.getState().amanState?.revision).toBe(newer.data.revision);
    act(() => {
      store.getState().setAMANConnectionState("disconnected");
      store.getState().setAMANConnectionState("connected");
    });
    emit(EventType.FrontendAMANState, event);
    expect(store.getState().amanState?.revision).toBe(event.data.revision);
    expect(store.getState().amanAwaitingSnapshot).toBe(false);
    expect(screen.getByRole("button", {name: /Select SAS123/})).toBeInTheDocument();
  });

  it("shows a rejected state explicitly and restores the workspace on a valid replacement", () => {
    const {store, emit, event} = workspace("healthy");
    const malformed = structuredClone(event) as unknown as {data: {flights: unknown}};
    malformed.data.flights = null;
    emit(EventType.FrontendAMANState, malformed);
    expect(screen.getByText("State rejected: invalid_aman_state")).toBeInTheDocument();
    expect(store.getState().amanState).toBeNull();
    emit(EventType.FrontendAMANState, event);
    expect(store.getState().amanError).toBeNull();
    expect(screen.queryByText("AMAN timeline unavailable")).not.toBeInTheDocument();
    expect(screen.getByRole("button", {name: /Select SAS123/})).toBeInTheDocument();
    expect(screen.getByRole("region", {name: "TMT traffic prediction"})).toBeInTheDocument();
  });
});
