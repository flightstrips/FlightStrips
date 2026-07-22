import {readFileSync} from "node:fs";
import {resolve} from "node:path";
import {fireEvent, render, screen} from "@testing-library/react";
import {describe, expect, it, vi} from "vitest";

import type {AMANCommandIntent, AMANStateEvent} from "@/api/aman";
import {AMANControlsView, type AMANControlsViewProps} from "./AMANControls";

const golden = JSON.parse(readFileSync(
  resolve(process.cwd(), "../backend/pkg/events/frontend/testdata/aman-state-v1.json"),
  "utf8",
)) as AMANStateEvent;

function state() {
  const value = structuredClone(golden.data);
  value.flights[0].flight_id = "flight-1";
  value.flights.push({...structuredClone(value.flights[0]), flight_id: "flight-2", callsign: "SAS456", order: 4});
  return value;
}

function renderControls(overrides: Partial<AMANControlsViewProps> = {}) {
  const onCommand = vi.fn<(intent: AMANCommandIntent) => void>();
  const props: AMANControlsViewProps = {
    state: state(),
    connectionState: "connected",
    readOnly: false,
    hasFMPAuthority: true,
    pendingCommands: {},
    commandRejections: {},
    onCommand,
    ...overrides,
  };
  render(<AMANControlsView {...props} />);
  return {onCommand, props};
}

describe("AMAN FMP controls", () => {
  it("maps every flight control to only its typed command fields", () => {
    const {onCommand} = renderControls();

    fireEvent.change(screen.getByLabelText("Move target"), {target: {value: "flight-2"}});
    fireEvent.click(screen.getByRole("button", {name: "Move before"}));
    fireEvent.click(screen.getByRole("button", {name: "Move after"}));
    fireEvent.click(screen.getByRole("button", {name: "Apply manual freeze"}));
    fireEvent.click(screen.getByRole("button", {name: "Accept calculated TETA"}));
    fireEvent.click(screen.getByRole("button", {name: "Keep initial/FPL ETA"}));
    fireEvent.click(screen.getByRole("button", {name: "Reset ETA override"}));

    expect(onCommand.mock.calls.map(([intent]) => intent)).toEqual([
      {type: "aman.move_flight", flight_id: "flight-1", runway_group_id: "ARRIVAL-22", before_flight_id: "flight-2"},
      {type: "aman.move_flight", flight_id: "flight-1", runway_group_id: "ARRIVAL-22", after_flight_id: "flight-2"},
      {type: "aman.lock_flight", flight_id: "flight-1"},
      {type: "aman.accept_teta", flight_id: "flight-1"},
      {type: "aman.keep_fpl_eta", flight_id: "flight-1"},
      {type: "aman.reset_teta_override", flight_id: "flight-1"},
    ]);
  });

  it("maps rate, manual ETA, and go-around inputs to their typed timestamps", () => {
    const {onCommand} = renderControls();
    fireEvent.change(screen.getByLabelText("Arrivals per hour"), {target: {value: "24"}});
    fireEvent.change(screen.getByLabelText("Rate effective at"), {target: {value: "2026-07-22T12:05"}});
    fireEvent.click(screen.getByRole("button", {name: "Set arrival rate"}));
    fireEvent.change(screen.getByLabelText("Manual ETA"), {target: {value: "2026-07-22T12:10"}});
    fireEvent.click(screen.getByRole("button", {name: "Set manual ETA"}));
    fireEvent.change(screen.getByLabelText("Go-around detected at"), {target: {value: "2026-07-22T12:15"}});
    fireEvent.click(screen.getByRole("button", {name: "Report go-around"}));

    expect(onCommand).toHaveBeenNthCalledWith(1, {
      type: "aman.set_rate", runway_group_id: "ARRIVAL-22", arrivals_per_hour: 24,
      effective_at: new Date("2026-07-22T12:05").toISOString(),
    });
    expect(onCommand).toHaveBeenNthCalledWith(2, {
      type: "aman.set_manual_eta", flight_id: "flight-1", manual_eta: new Date("2026-07-22T12:10").toISOString(),
    });
    expect(onCommand).toHaveBeenNthCalledWith(3, {
      type: "aman.report_go_around", flight_id: "flight-1", detected_at: new Date("2026-07-22T12:15").toISOString(),
    });
  });

  it.each([
    ["disconnected", {connectionState: "disconnected" as const}, "Disconnected"],
    ["observer", {readOnly: true}, "Observer session"],
    ["unauthorized", {hasFMPAuthority: false}, "FMP authority is required"],
    ["read-only mode", {state: {...state(), authoritative: false, effective_mode: "read_only" as const}}, "not authoritative"],
    ["degraded readiness", {state: {...state(), technical_health: {...state().technical_health, ready: false}}}, "technically degraded"],
  ])("disables mutation when %s", (_name, overrides, warning) => {
    renderControls(overrides);
    expect(screen.getByRole("button", {name: "Accept calculated TETA"})).toBeDisabled();
    expect(screen.getByRole("status")).toHaveTextContent(warning);
  });

  it("keeps pending, rejection, conflict, freeze, drift, direct, and degraded facts visually explicit", () => {
    const degraded = state();
    degraded.flights[0].freeze_reason = "superstable";
    degraded.flights[0].raw_teta = "2026-07-22T12:19:00.000Z";
    degraded.flights[0].operational_teta = "2026-07-22T12:18:00.000Z";
    degraded.flights[0].geometry_version = null;
    degraded.flights[0].holding_fix = null;
    degraded.technical_health.navigation = {...degraded.technical_health.navigation, status: "degraded", reason: "geometry_stale"};
    degraded.technical_health.weather = {...degraded.technical_health.weather, status: "unavailable", reason: "metar_missing"};

    renderControls({
      state: degraded,
      pendingCommands: {pending: {command_id: "pending", type: "aman.accept_teta", expected_revision: 7, flight_id: "flight-1"}},
      commandRejections: {conflict: {command_id: "conflict", code: "revision_conflict", message: "revision changed", current_revision: 8, retryable: true}},
    });

    expect(screen.getByText(/Pending: aman.accept_teta \(pending\)/)).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("revision_conflict");
    expect(screen.getByText(/Freeze:/)).toHaveTextContent("superstable");
    expect(screen.getByText(/Raw TETA \(informational\)/)).toHaveTextContent("12:19");
    expect(screen.getByText(/Operational TETA/)).toHaveTextContent("12:18");
    expect(screen.getByText(/Operational slot/)).toHaveTextContent("fixed by superstable freeze");
    expect(screen.getByText("Accepted direct SOK")).toBeInTheDocument();
    expect(screen.getByText(/Geometry:/)).toHaveTextContent("Unavailable");
    expect(screen.getByText(/Holding fix:/)).toHaveTextContent("Unavailable");
    expect(screen.getByText(/Geometry\/navigation: degraded/)).toHaveTextContent("geometry_stale");
    expect(screen.getByText(/Weather: unavailable/)).toHaveTextContent("metar_missing");
    expect(screen.getByRole("button", {name: "Release manual freeze"})).toBeDisabled();
  });

  it("distinguishes manual freeze and enables only deliberate release", () => {
    const manual = state();
    manual.flights[0].freeze_reason = "manual";
    renderControls({state: manual});

    expect(screen.getByText(/Freeze:/)).toHaveTextContent("manual");
    expect(screen.getByRole("button", {name: "Apply manual freeze"})).toBeDisabled();
    expect(screen.getByRole("button", {name: "Release manual freeze"})).toBeEnabled();
  });
});
