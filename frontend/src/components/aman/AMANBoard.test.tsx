import {readFileSync} from "node:fs";
import {resolve} from "node:path";
import {fireEvent, render, screen} from "@testing-library/react";
import {describe, expect, it, vi} from "vitest";

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
    const marker = screen.getByRole("button", {name: "Select SAS123 timeline marker"});

    expect(screen.getByText("EKCH")).toBeInTheDocument();
    expect(screen.getByText("ARRIVAL-22 : 1")).toBeInTheDocument();
    expect(marker).toHaveTextContent("SAS123");
    expect(marker).toHaveTextContent("10:18");
    expect(marker).toHaveTextContent("+1:00");
    expect(marker).not.toHaveTextContent("Prediction");
    expect(marker).toHaveAttribute("title", expect.stringContaining("fresh"));
  });

  it("keeps frozen operational markers fixed without adding a raw-TETA timeline marker", () => {
    const frozen = state();
    frozen.flights[0].freeze_reason = "superstable";
    frozen.flights[0].raw_teta = "2026-07-22T10:40:00.000Z";
    frozen.flights[0].slot!.time = "2026-07-22T10:18:00.000Z";
    renderBoard(frozen);

    expect(screen.getByTestId("operational-marker-flight-123")).toHaveAttribute("data-marker-time", "2026-07-22T10:18:00.000Z");
    expect(screen.queryByTestId("raw-marker-flight-123")).not.toBeInTheDocument();
    expect(screen.getByTestId("operational-marker-flight-123")).toHaveClass("border-cyan-200");
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
    expect(screen.getByTestId("operational-marker-flight-123")).toHaveAttribute("title", expect.stringContaining("go_around"));
    expect(screen.getByTestId("operational-marker-flight-123")).toHaveAttribute("title", expect.stringContaining("stale"));
    expect(screen.getByTestId("operational-marker-flight-123")).toHaveClass("border-fuchsia-200");
  });

  it("supports compact timeline marker hit testing from the designed scrolling layout", () => {
    const onSelectFlight = renderBoard(state());
    const marker = screen.getByRole("button", {name: "Select SAS123 timeline marker"});
    fireEvent.click(marker);

    expect(onSelectFlight).toHaveBeenNthCalledWith(1, "flight-123");
    expect(screen.getByTestId("aman-timeline-grid")).toHaveClass("min-w-max");
    expect(screen.getByTestId("holding-timeline-lane-SOK-HF")).toBeInTheDocument();
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
    expect(screen.getByTestId("holding-timeline-lane-SOK-HF")).toBeInTheDocument();
    expect(screen.queryByTestId("holding-timeline-lane-TIDVU")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", {name: /ARRIVAL-04/}));
    expect(screen.getByTestId("holding-timeline-lane-TIDVU")).toBeInTheDocument();
    expect(screen.queryByTestId("holding-timeline-lane-SOK-HF")).not.toBeInTheDocument();
  });

  it("switches to one unsplit runway timeline with a scrollable hour-aligned scale", () => {
    renderBoard(state());

    expect(screen.getByText("10:00–11:00 · scroll timeline")).toBeInTheDocument();
    expect(screen.getByTestId("aman-timeline-grid")).toHaveStyle({height: "1080px"});
    fireEvent.click(screen.getByRole("button", {name: "RWY"}));
    expect(screen.getByTestId("holding-timeline-lane-ARRIVAL-22")).toBeInTheDocument();
    expect(screen.queryByTestId("holding-timeline-lane-SOK-HF")).not.toBeInTheDocument();
    expect(screen.getByTestId("aman-timeline-grid")).toHaveClass("min-w-full");
  });

  it("shows the AMAN event STAR on runway markers only", () => {
    renderBoard(state());

    expect(screen.getByTestId("operational-marker-flight-123")).not.toHaveTextContent("SOK");
    fireEvent.click(screen.getByRole("button", {name: "RWY"}));
    expect(screen.getByTestId("operational-marker-flight-123")).toHaveTextContent("SOK");
  });

});
