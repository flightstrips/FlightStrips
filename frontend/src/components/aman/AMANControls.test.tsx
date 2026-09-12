import {readFileSync} from "node:fs";
import {resolve} from "node:path";
import {cleanup, fireEvent, render, screen} from "@testing-library/react";
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

  it("shows authoritative disposition and confirms permanent removal accessibly", () => {
    const desequenced = state();
    desequenced.flights[0].sequence_disposition = "desequenced";
    const {onCommand} = renderControls({state: desequenced});

    expect(screen.getByLabelText("SAS123 sequence disposition")).toHaveTextContent("Server-confirmed disposition: desequenced");
    fireEvent.click(screen.getByRole("button", {name: "Resume at earliest legal opportunity"}));
    fireEvent.click(screen.getByRole("button", {name: "Remove flight…"}));
    expect(screen.getByRole("dialog", {name: "Confirm removal · SAS123"})).toHaveTextContent("cannot be resumed");
    fireEvent.click(screen.getByRole("button", {name: "Confirm remove flight"}));

    expect(onCommand).toHaveBeenNthCalledWith(1, {type: "aman.resume_flight", flight_id: "flight-1"});
    expect(onCommand).toHaveBeenNthCalledWith(2, {type: "aman.remove_flight", flight_id: "flight-1"});
  });

  it("shows disposition pending, stale revision, authorization, and no-capacity states", () => {
    const desequenced = state();
    desequenced.flights[0].sequence_disposition = "desequenced";
    renderControls({
      state: desequenced,
      pendingCommands: {resume: {command_id: "resume", type: "aman.resume_flight", expected_revision: 7, flight_id: "flight-1"}},
      commandRejections: {
        stale: {command_id: "stale", command_type: "aman.resume_flight", code: "revision_conflict", message: "revision changed", current_revision: 8, retryable: true},
        capacity: {command_id: "capacity", command_type: "aman.resume_flight", code: "invalid_transition", message: "resume could not produce a complete legal sequence", current_revision: 8, retryable: false},
      },
    });

    expect(screen.getByText("Waiting for server-confirmed disposition")).toBeInTheDocument();
    expect(screen.getAllByRole("alert").some((alert) => alert.textContent?.includes("revision_conflict"))).toBe(true);
    expect(screen.getAllByRole("alert").some((alert) => alert.textContent?.includes("complete legal sequence"))).toBe(true);
    expect(screen.getByRole("button", {name: "Resume at earliest legal opportunity"})).toBeDisabled();

    cleanup();
    renderControls({state: desequenced, hasFMPAuthority: false});
    expect(screen.getByRole("status")).toHaveTextContent("FMP authority is required");
    expect(screen.getByRole("button", {name: "Resume at earliest legal opportunity"})).toBeDisabled();
  });

  it("maps rate, manual ETA, and go-around inputs to their typed timestamps", () => {
    const multiRunwayState = state();
    multiRunwayState.runway_groups.push({id: "ARRIVAL-04", selected: false, selection_schedule: []});
    const {onCommand} = renderControls({state: multiRunwayState});
    fireEvent.change(screen.getByLabelText("Rate runway group"), {target: {value: "ARRIVAL-04"}});
    fireEvent.change(screen.getByLabelText("Arrivals per hour"), {target: {value: "24"}});
    fireEvent.change(screen.getByLabelText("Rate effective at"), {target: {value: "2026-07-22T12:05"}});
    fireEvent.click(screen.getByRole("button", {name: "Set arrival rate"}));
    fireEvent.change(screen.getByLabelText("Manual ETA"), {target: {value: "2026-07-22T12:10"}});
    fireEvent.click(screen.getByRole("button", {name: "Set manual ETA"}));
    fireEvent.change(screen.getByLabelText("Go-around detected at"), {target: {value: "2026-07-22T12:15"}});
    fireEvent.click(screen.getByRole("button", {name: "Report go-around"}));

    expect(onCommand).toHaveBeenNthCalledWith(1, {
      type: "aman.set_rate", runway_group_id: "ARRIVAL-04", arrivals_per_hour: 24,
      effective_at: new Date("2026-07-22T12:05").toISOString(),
    });
    expect(onCommand).toHaveBeenNthCalledWith(2, {
      type: "aman.set_manual_eta", flight_id: "flight-1", manual_eta: new Date("2026-07-22T12:10").toISOString(),
    });
    expect(onCommand).toHaveBeenNthCalledWith(3, {
      type: "aman.report_go_around", flight_id: "flight-1", detected_at: new Date("2026-07-22T12:15").toISOString(),
    });
  });

  it("sets and resets feeder ETA from a dialog with server-confirmed provenance", () => {
    const manual = state();
    manual.flights[0].feeder_fix_eta = "2026-07-22T12:09:00.000Z";
    manual.flights[0].feeder_fix_eta_source = "manual";
    const {onCommand} = renderControls({state: manual});

    fireEvent.click(screen.getByRole("button", {name: "Edit feeder-fix ETA"}));
    expect(screen.getByRole("dialog")).toHaveTextContent("Server-confirmed: 12:09 · provenance manual");
    fireEvent.change(screen.getByLabelText("Manual feeder-fix ETA"), {target: {value: "2026-07-22T12:10"}});
    fireEvent.click(screen.getByRole("button", {name: "Set feeder ETA"}));
    fireEvent.click(screen.getByRole("button", {name: "Reset to predicted ETA"}));

    expect(onCommand).toHaveBeenNthCalledWith(1, {type: "aman.set_manual_feeder_eta", flight_id: "flight-1", feeder_eta: new Date("2026-07-22T12:10").toISOString()});
    expect(onCommand).toHaveBeenNthCalledWith(2, {type: "aman.reset_manual_feeder_eta", flight_id: "flight-1"});
  });

  it("requests an alternate active runway and exposes pending and rejected results", () => {
    const multiRunwayState = state();
    multiRunwayState.runway_groups.push({id: "ARRIVAL-04", selected: false, selection_schedule: []});
    multiRunwayState.active_runway_groups = ["ARRIVAL-22", "ARRIVAL-04"];
    renderControls({
      state: multiRunwayState,
      pendingCommands: {runway: {command_id: "runway", type: "aman.change_runway", expected_revision: 7, flight_id: "flight-1", runway_group_id: "ARRIVAL-04"}},
      commandRejections: {rejected: {command_id: "rejected", command_type: "aman.change_runway", code: "invalid_transition", message: "protected conflict", current_revision: 7, retryable: false}},
    });

    fireEvent.click(screen.getByRole("button", {name: "Change runway"}));
    expect(screen.getByRole("dialog")).toHaveTextContent("Server-confirmed runway: ARRIVAL-22");
    expect(screen.getByRole("status")).toHaveTextContent("Waiting for server confirmation");
    expect(screen.getAllByRole("alert").some((alert) => alert.textContent?.includes("protected conflict"))).toBe(true);
    expect(screen.getByRole("button", {name: "Request runway change"})).toBeDisabled();

    cleanup();
    const {onCommand} = renderControls({state: multiRunwayState});
    fireEvent.click(screen.getByRole("button", {name: "Change runway"}));
    fireEvent.click(screen.getByRole("button", {name: "Request runway change"}));
    expect(onCommand).toHaveBeenCalledWith({type: "aman.change_runway", flight_id: "flight-1", runway_group_id: "ARRIVAL-04"});
  });

  it("shows feeder ETA pending and rejection states", () => {
    renderControls({
      pendingCommands: {feeder: {command_id: "feeder", type: "aman.set_manual_feeder_eta", expected_revision: 7, flight_id: "flight-1"}},
      commandRejections: {rejected: {command_id: "rejected", command_type: "aman.set_manual_feeder_eta", code: "invalid_argument", message: "past ETA", current_revision: 7, retryable: false}},
    });
    fireEvent.click(screen.getByRole("button", {name: "Edit feeder-fix ETA"}));

    expect(screen.getByRole("status")).toHaveTextContent("Waiting for server confirmation");
    expect(screen.getAllByRole("alert").some((alert) => alert.textContent?.includes("past ETA"))).toBe(true);
    expect(screen.getByRole("button", {name: "Set feeder ETA"})).toBeDisabled();
  });

  it("represents every active runway without treating traffic on another active runway as a conflict", () => {
    const multiRunwayState = state();
    multiRunwayState.runway_groups.push({id: "ARRIVAL-04", selected: false, selection_schedule: []});
    multiRunwayState.active_runway_groups = ["ARRIVAL-22", "ARRIVAL-04"];
    multiRunwayState.flights[1].runway_group_id = "ARRIVAL-04";
    multiRunwayState.flights[1].lifecycle_state = "stable";

    renderControls({state: multiRunwayState});

    expect(screen.getByText(/Active:/)).toHaveTextContent("ARRIVAL-22, ARRIVAL-04");
    expect(screen.queryByText(/Protected traffic retained/)).not.toBeInTheDocument();
  });

  it("keeps runway-group rate controls available without active flights", () => {
    const empty = state();
    empty.flights = [];
    renderControls({state: empty});

    expect(screen.getByLabelText("Rate runway group")).toBeInTheDocument();
    expect(screen.getByRole("button", {name: "Set arrival rate"})).toBeInTheDocument();
    expect(screen.getByText("No AMAN flights available.")).toBeInTheDocument();
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
    expect(screen.getByText(/STAR family:/)).toHaveTextContent("TESPI");
    expect(screen.getByText(/Feeder fix:/)).toHaveTextContent("TNO");
    expect(screen.getByText(/Holding fix:/)).toHaveTextContent("Unavailable");
    expect(screen.getByText(/Geometry\/navigation: degraded/)).toHaveTextContent("geometry_stale");
    expect(screen.getByText(/Weather: unavailable/)).toHaveTextContent("metar_missing");
    expect(screen.getByRole("button", {name: "Release manual freeze"})).toBeDisabled();
  });

  it("presents terminal identities independently without legacy-field fallback", () => {
    const identities = state();
    identities.flights[0].star = "LEGACY-STAR";
    identities.flights[0].feeder = "LEGACY-FEEDER";
    identities.flights[0].star_family = null;
    identities.flights[0].feeder_fix = "TNO";
    identities.flights[0].holding_fix = "ROSBI";

    renderControls({state: identities});

    expect(screen.getByText(/STAR family:/)).toHaveTextContent("Unavailable");
    expect(screen.getByText(/Feeder fix:/)).toHaveTextContent("TNO");
    expect(screen.getByText(/Holding fix:/)).toHaveTextContent("ROSBI");
    expect(screen.queryByText(/LEGACY-STAR|LEGACY-FEEDER/)).not.toBeInTheDocument();
  });

  it("shows pending state for the remaining detailed rate control", () => {
    renderControls({
      pendingCommands: {
        rate: {command_id: "rate", type: "aman.set_rate", expected_revision: 7, runway_group_id: "ARRIVAL-22"},
      },
    });

    expect(screen.getByText("Arrival rate change pending")).toBeInTheDocument();
    expect(screen.getByRole("button", {name: "Set arrival rate"})).toBeDisabled();
  });

  it("reports protected traffic retained outside the selected runway group", () => {
    const protectedState = state();
    protectedState.flights[0].runway_group_id = "ARRIVAL-04";

    renderControls({state: protectedState});

    expect(screen.getByRole("alert")).toHaveTextContent("Protected traffic retained");
    expect(screen.getByRole("alert")).toHaveTextContent("SAS123");
  });

  it("shows a blocked scheduled selection reported by the backend", () => {
    const conflicted = state();
    conflicted.runway_groups[0].selection_conflict = "protected slot conflict";

    renderControls({state: conflicted});

    expect(screen.getByRole("alert")).toHaveTextContent("Scheduled selection of ARRIVAL-22 is blocked");
    expect(screen.getByRole("alert")).toHaveTextContent("protected slot conflict");
  });

  it("distinguishes manual freeze and enables only deliberate release", () => {
    const manual = state();
    manual.flights[0].freeze_reason = "manual";
    renderControls({state: manual});

    expect(screen.getByText(/Freeze:/)).toHaveTextContent("manual");
    expect(screen.getByRole("button", {name: "Apply manual freeze"})).toBeDisabled();
    expect(screen.getByRole("button", {name: "Release manual freeze"})).toBeEnabled();
  });

  it("shows detected go-arounds to authorized controllers and sends episode-bound decisions", () => {
    const detected = state();
    detected.flights[0].go_around_confirmation = {
      episode_id: "flight-1/go-around/1", reason: "track_away", detected_at: "2026-07-22T12:00:00.000Z",
      evidence_times: ["2026-07-22T11:59:58.000Z", "2026-07-22T11:59:59.000Z"], status: "pending",
      decided_at: null, decided_by: null, resulting_revision: null,
    };
    const {onCommand} = renderControls({state: detected});

    expect(screen.getByRole("alert", {name: "SAS123 go-around confirmation request"})).toHaveTextContent("track away");
    fireEvent.click(screen.getByRole("button", {name: "Confirm go-around"}));
    fireEvent.click(screen.getByRole("button", {name: "Reject detection"}));
    expect(onCommand).toHaveBeenNthCalledWith(1, {type: "aman.confirm_go_around", flight_id: "flight-1", episode_id: "flight-1/go-around/1"});
    expect(onCommand).toHaveBeenNthCalledWith(2, {type: "aman.reject_go_around", flight_id: "flight-1", episode_id: "flight-1/go-around/1"});
  });

  it("does not present detector confirmation controls without FMP authority", () => {
    const detected = state();
    detected.flights[0].go_around_confirmation = {
      episode_id: "flight-1/go-around/1", reason: "climb", detected_at: "2026-07-22T12:00:00.000Z",
      evidence_times: ["2026-07-22T11:59:59.000Z"], status: "pending", decided_at: null, decided_by: null, resulting_revision: null,
    };
    renderControls({state: detected, hasFMPAuthority: false});

    expect(screen.queryByRole("button", {name: "Confirm go-around"})).not.toBeInTheDocument();
  });

  it("requires an explicit GAP mode and sends both input forms without normalizing duration", () => {
    const {onCommand} = renderControls();
    fireEvent.click(screen.getByRole("button", {name: "Manage GAPs…"}));
    expect(screen.getByLabelText("GAP runway group")).toHaveFocus();
    expect(screen.getByLabelText("GAP input mode")).toHaveValue("");
    expect(screen.getByRole("button", {name: "Create GAP"})).toBeDisabled();
    fireEvent.change(screen.getByLabelText("GAP input mode"), {target: {value: "absolute"}});
    fireEvent.change(screen.getByLabelText("GAP start UTC"), {target: {value: "2026-07-22T12:00"}});
    fireEvent.change(screen.getByLabelText("GAP end UTC exclusive"), {target: {value: "2026-07-22T12:06"}});
    fireEvent.change(screen.getByLabelText("GAP operational reason"), {target: {value: "approach stop"}});
    fireEvent.click(screen.getByRole("button", {name: "Create GAP"}));
    expect(onCommand).toHaveBeenLastCalledWith({type: "aman.create_gap", runway_group_id: "ARRIVAL-22", start: "2026-07-22T12:00:00.000Z", end: "2026-07-22T12:06:00.000Z", label: "approach stop"});

    fireEvent.change(screen.getByLabelText("GAP input mode"), {target: {value: "slots"}});
    fireEvent.change(screen.getByLabelText("GAP slot count"), {target: {value: "2"}});
    fireEvent.click(screen.getByRole("button", {name: "Create GAP"}));
    expect(onCommand).toHaveBeenLastCalledWith({type: "aman.create_gap", runway_group_id: "ARRIVAL-22", start: "2026-07-22T12:00:00.000Z", slot_count: 2, label: "approach stop"});
  });

  it("shows the server-confirmed merged union, removal, pending, and atomic rejection", () => {
    const current = state();
    current.runway_groups[0].gaps = [{id: "gap-merged", start: "2026-07-22T12:00:00.000Z", end: "2026-07-22T12:09:00.000Z", label: "merged stop", created_at: "2026-07-22T11:59:00.000Z", created_by: "fmp-1"}];
    const {onCommand} = renderControls({state: current, pendingCommands: {gap: {command_id: "gap", type: "aman.create_gap", expected_revision: 7, runway_group_id: "ARRIVAL-22"}}, commandRejections: {gap: {command_id: "rejected", command_type: "aman.create_gap", code: "revision_conflict", message: "protected flights cannot be displaced atomically", current_revision: 9, retryable: true}}});
    expect(screen.getByText(/Server-confirmed union:/).parentElement).toHaveTextContent("12:00–12:09 UTC [start,end)");
    expect(screen.getAllByRole("status").some((item) => item.textContent?.includes("server-confirmed revision"))).toBe(true);
    expect(screen.getByRole("button", {name: /Remove GAP merged stop/})).toBeDisabled();
    fireEvent.click(screen.getByRole("button", {name: "Manage GAPs…"}));
    expect(screen.getAllByRole("alert").some((item) => item.textContent?.includes("revision_conflict; server revision 9") && item.textContent.includes("No displacement was applied"))).toBe(true);

    cleanup();
    const ready = renderControls({state: current});
    fireEvent.click(screen.getByRole("button", {name: /Remove GAP merged stop/}));
    expect(ready.onCommand).toHaveBeenCalledWith({type: "aman.remove_gap", runway_group_id: "ARRIVAL-22", gap_id: "gap-merged"});
    expect(onCommand).not.toHaveBeenCalled();
  });

  it("keeps GAP controls compatible with an older V1 state", () => {
    const legacy = state();
    delete legacy.runway_groups[0].gaps;
    renderControls({state: legacy});
    expect(screen.getByText("GAP state unavailable from this older server.")).toBeInTheDocument();
  });
});
