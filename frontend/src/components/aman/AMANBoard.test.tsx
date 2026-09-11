import {readFileSync} from "node:fs";
import {resolve} from "node:path";
import {fireEvent, render, screen} from "@testing-library/react";
import {beforeEach, describe, expect, it, vi} from "vitest";

import type {AMANState, AMANStateEvent} from "@/api/aman";
import {AMANBoardView, type AMANBoardViewProps} from "./AMANBoard";

const golden = JSON.parse(readFileSync(
  resolve(process.cwd(), "../backend/pkg/events/frontend/testdata/aman-state-v1.json"),
  "utf8",
)) as AMANStateEvent;

function state(): AMANState {
  return structuredClone(golden.data);
}

function renderBoard(value: AMANState | null, overrides: Partial<AMANBoardViewProps> = {}) {
  const onSelectFlight = vi.fn();
  render(
    <AMANBoardView
      connectionState="connected"
      error={null}
      onSelectFlight={onSelectFlight}
      presentationStatus={value ? "ready" : "empty"}
      selectedFlightID={null}
      state={value}
      {...overrides}
    />,
  );
  return onSelectFlight;
}

describe("complete AMAN timeline and strips", () => {
  beforeEach(() => localStorage.clear());

  it("renders null and invalid replacement state explicitly without stale partial data", () => {
    const {rerender} = render(
      <AMANBoardView connectionState="disconnected" error={null} onSelectFlight={() => undefined} presentationStatus="empty" selectedFlightID={null} state={null} />,
    );
    expect(screen.getByText("AMAN timeline unavailable")).toBeInTheDocument();
    expect(screen.getByText(/Waiting for a complete AMAN state replacement/)).toBeInTheDocument();

    rerender(<AMANBoardView connectionState="connected" error="invalid_aman_state" onSelectFlight={() => undefined} presentationStatus="degraded" selectedFlightID={null} state={null} />);
    expect(screen.getByText(/State rejected: invalid_aman_state/)).toBeInTheDocument();
    expect(screen.queryByText("SAS123")).not.toBeInTheDocument();
  });

  it("renders the normal golden state with backend values and explicit unavailable contract fields", () => {
    renderBoard(state());
    const marker = screen.getByRole("button", {name: /Select SAS123; Stable; current delay G01/});

    expect(screen.getByText("EKCH")).toBeInTheDocument();
    expect(screen.getByText("ARRIVAL-22 : 1")).toBeInTheDocument();
    expect(marker).toHaveTextContent("SAS123");
    expect(marker).toHaveTextContent("G01");
    expect(marker).not.toHaveTextContent("Prediction");
    expect(screen.getByTestId("operational-marker-flight-123")).toHaveAttribute("data-marker-time", "2026-07-22T10:18:00.000Z");
  });

  it("keeps frozen operational markers fixed without adding a raw-TETA timeline marker", () => {
    const frozen = state();
    frozen.flights[0].freeze_reason = "superstable";
    frozen.flights[0].raw_teta = "2026-07-22T10:40:00.000Z";
    frozen.flights[0].slot!.time = "2026-07-22T10:18:00.000Z";
    renderBoard(frozen);

    expect(screen.getByTestId("operational-marker-flight-123")).toHaveAttribute("data-marker-time", "2026-07-22T10:18:00.000Z");
    expect(screen.queryByTestId("raw-marker-flight-123")).not.toBeInTheDocument();
    expect(screen.getByTitle("Superstable")).toHaveTextContent("SS");
  });

  it("golden-renders degraded, stale, go-around, manual freeze, queue, and discrepancy facts", () => {
    const degraded = state();
    const flight = degraded.flights[0];
    degraded.technical_health.status = "degraded";
    degraded.technical_health.ready = false;
    degraded.technical_health.blocked_reasons = ["predictor stale"];
    flight.data_status = "stale";
    flight.lifecycle_state = "go_around";
    flight.freeze_reason = "manual";
    flight.confidence = null;
    flight.provenance = null;
    flight.route_fact = null;
    flight.queue_offers = [{
      flight_id: flight.flight_id,
      runway_group_id: "ARRIVAL-22",
      candidate_slot: {...flight.slot!, time: "2026-07-22T10:16:00.000Z"},
      queue_position: 1,
      expires_at: "2026-07-22T10:05:00.000Z",
      airport_revision: 7,
      reason: "earlier_available",
    }];
    flight.eta_review = {
      status: "pending",
      created_at: "2026-07-22T10:00:00.000Z",
      deadline_at: "2026-07-22T10:05:00.000Z",
      resolved_at: null,
      actor: null,
      note: null,
      initial_baseline_teta: "2026-07-22T10:22:00.000Z",
      calculated_operational_teta: "2026-07-22T10:19:00.000Z",
      selected_teta: "2026-07-22T10:22:00.000Z",
      manual_teta: null,
    };

    renderBoard(degraded, {presentationStatus: "degraded", connectionState: "disconnected"});
    expect(screen.getAllByText("degraded").length).toBeGreaterThan(0);
    expect(screen.getByText("predictor stale")).toBeInTheDocument();
    expect(screen.getByRole("button", {name: /Select SAS123; go around; current delay Unavailable/})).toBeInTheDocument();
    expect(screen.getByTestId("operational-marker-flight-123")).toHaveTextContent("Unavailable");
  });

  it("does not show guidance from a non-authoritative AMAN state", () => {
    const readOnly = state();
    readOnly.authoritative = false;
    readOnly.effective_mode = "read_only";

    renderBoard(readOnly);

    expect(screen.getByTestId("operational-marker-flight-123")).toHaveTextContent("Unavailable");
    expect(screen.getByTestId("operational-marker-flight-123")).not.toHaveTextContent("G01");
  });

  it("supports compact timeline marker hit testing from the designed scrolling layout", () => {
    const onSelectFlight = vi.fn();
    const onOpenFlightDetails = vi.fn();
    renderBoard(state(), {onSelectFlight, onOpenFlightDetails});
    const marker = screen.getByRole("button", {name: /Select SAS123/});
    fireEvent.click(marker);

    expect(onSelectFlight).toHaveBeenNthCalledWith(1, "flight-123");
    expect(onOpenFlightDetails).toHaveBeenNthCalledWith(1, "flight-123");
    expect(screen.getByTestId("aman-timeline-grid")).toHaveClass("min-w-max");
    expect(screen.getByTestId("holding-timeline-lane-ROSBI")).toBeInTheDocument();
  });

  it("activates a focused compact target through the keyboard click contract", () => {
    const onSelectFlight = vi.fn();
    const onOpenFlightDetails = vi.fn();
    renderBoard(state(), {onSelectFlight, onOpenFlightDetails});
    const target = screen.getByRole("button", {name: /Select SAS123/});

    target.focus();
    fireEvent.click(target, {detail: 0});

    expect(target).toHaveFocus();
    expect(onSelectFlight).toHaveBeenCalledWith("flight-123");
    expect(onOpenFlightDetails).toHaveBeenCalledWith("flight-123");
  });

  it("applies local feeder and runway fields and labels their preferences dialog", () => {
    localStorage.setItem("flightstrips.aman.target-fields.v1", JSON.stringify({version: 1, feeder: ["feeder-fix-eta"], runway: ["runway"]}));
    const current = state();
    current.flights[0].feeder_fix_eta = "2026-07-22T10:12:00.000Z";
    renderBoard(current);

    expect(screen.getByRole("button", {name: /Select SAS123/})).toHaveTextContent("10:12SAS123G01ARRIVAL-22S");
    fireEvent.click(screen.getByRole("button", {name: "Open target information preferences"}));
    expect(screen.getByRole("dialog", {name: "Target information"})).toBeInTheDocument();
  });

  it("opens the selected flight's on-demand route detail without changing the board state", () => {
    const onOpenFlightDetails = vi.fn();
    renderBoard(state(), {selectedFlightID: "flight-123", onOpenFlightDetails});

    fireEvent.click(screen.getByRole("button", {name: "DETAIL"}));

    expect(onOpenFlightDetails).toHaveBeenCalledOnce();
  });

  it("stacks near-simultaneous flight strips in their assigned ruler column", () => {
    const overlapping = state();
    const original = overlapping.flights[0];
    overlapping.flights.push({
      ...structuredClone(original),
      flight_id: "flight-124",
      callsign: "SAS124",
      order: 4,
      operational_teta: "2026-07-22T10:18:30.000Z",
      raw_teta: "2026-07-22T10:18:30.000Z",
      slot: {...original.slot!, sequence: 4, time: "2026-07-22T10:18:30.000Z"},
    });

    renderBoard(overlapping);
    expect(screen.getByTestId("operational-marker-flight-123").parentElement).toHaveClass("-translate-x-full");
    expect(screen.getByTestId("operational-marker-flight-124").parentElement).toHaveClass("-translate-x-full");
  });

  it("shows one runway group at a time and splits its flights by holding", () => {
    const multiRunwayState = state();
    multiRunwayState.runway_groups.push({id: "ARRIVAL-04"});
    multiRunwayState.flights.push({
      ...structuredClone(multiRunwayState.flights[0]),
      flight_id: "flight-04",
      callsign: "SKY404",
      runway_group_id: "ARRIVAL-04",
      holding_fix: "TIDVU",
      order: 2,
    });

    renderBoard(multiRunwayState);
    expect(screen.getByTestId("holding-timeline-lane-ROSBI")).toBeInTheDocument();
    expect(screen.queryByTestId("holding-timeline-lane-TIDVU")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", {name: /ARRIVAL-04/}));
    expect(screen.getByTestId("holding-timeline-lane-TIDVU")).toBeInTheDocument();
    expect(screen.queryByTestId("holding-timeline-lane-ROSBI")).not.toBeInTheDocument();
  });

  it("switches to active runway timelines with the local horizon scale", () => {
    renderBoard(state());

    expect(screen.getByText("10:00–10:30 UTC · 30 min")).toBeInTheDocument();
    expect(screen.getByTestId("aman-timeline-grid")).toHaveStyle({height: "720px"});
    fireEvent.click(screen.getByRole("button", {name: "RWY"}));
    expect(screen.getByTestId("rwy-lane-ARRIVAL-22")).toBeInTheDocument();
    expect(screen.queryByTestId("holding-timeline-lane-ROSBI")).not.toBeInTheDocument();
    expect(screen.getByTestId("aman-timeline-grid")).toHaveClass("min-w-full");
  });

  it("renders the backend-configured FMP paired timelines without inferring an unused family", () => {
    const configured = state();
    configured.timeline_configuration = {version: "mapping-v1", mappings: [
      {id: 1, left: "TESPI", right: "TUDLO"},
      {id: 2, left: "MONAK", right: "TIDVU"},
      {id: 3, left: "ERNOV", right: null},
    ]};

    renderBoard(configured);

    expect(screen.getAllByTestId(/^fmp-timeline-/)).toHaveLength(3);
    expect(screen.getByRole("region", {name: "Timeline 1: TESPI left, TUDLO right"})).toBeInTheDocument();
    expect(screen.getByLabelText("Unused right side")).toBeInTheDocument();
    expect(screen.getByLabelText(/SAS123 at/)).toHaveAttribute("data-family", "TESPI");
  });

  it("shows only the explicit STAR family on runway markers", () => {
    const identities = state();
    identities.flights[0].star = "LEGACY-STAR";
    identities.flights[0].star_family = "TESPI";
    renderBoard(identities);

    expect(screen.getByTestId("operational-marker-flight-123")).not.toHaveTextContent("TESPI");
    fireEvent.click(screen.getByRole("button", {name: "RWY"}));
    expect(screen.getByTestId("operational-marker-flight-123")).toHaveTextContent("TESPI");
    expect(screen.getByTestId("operational-marker-flight-123")).not.toHaveTextContent("LEGACY-STAR");
  });

});
