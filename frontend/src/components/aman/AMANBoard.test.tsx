import {readFileSync} from "node:fs";
import {resolve} from "node:path";
import {fireEvent, render, screen, within} from "@testing-library/react";
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
    const strip = screen.getByRole("button", {name: "SAS123 AMAN strip"});

    expect(screen.getByText(/Revision 7/)).toBeInTheDocument();
    expect(within(strip).getByText("Unavailable / Unavailable")).toBeInTheDocument();
    expect(within(strip).getByText("Unavailable / ARRIVAL-22")).toBeInTheDocument();
    expect(within(strip).getByText("Accepted direct SOK")).toBeInTheDocument();
    expect(within(strip).getByText("+1:00")).toBeInTheDocument();
    expect(within(strip).getByText("87.5 NM")).toBeInTheDocument();
    expect(within(strip).getByText(/SOK-HF \/ 10:10/)).toBeInTheDocument();
  });

  it("keeps frozen operational markers fixed while showing raw drift separately", () => {
    const frozen = state();
    frozen.flights[0].freeze_reason = "superstable";
    frozen.flights[0].raw_teta = "2026-07-22T10:40:00.000Z";
    frozen.flights[0].slot!.time = "2026-07-22T10:18:00.000Z";
    renderBoard(frozen);

    expect(screen.getByTestId("operational-marker-flight-123")).toHaveAttribute("data-marker-time", "2026-07-22T10:18:00.000Z");
    expect(screen.getByTestId("raw-marker-flight-123")).toHaveAttribute("title", "Raw TETA 10:40 (informational only)");
    expect(screen.getByText("Freeze superstable")).toBeInTheDocument();
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
    expect(screen.getByRole("alert")).toHaveTextContent("predictor stale");
    expect(screen.getByText("go around")).toBeInTheDocument();
    expect(screen.getByText("stale")).toBeInTheDocument();
    expect(screen.getByText("Freeze manual")).toBeInTheDocument();
    expect(screen.getByText("No accepted direct")).toBeInTheDocument();
    expect(screen.getByText(/Discrepancy pending/)).toBeInTheDocument();
    expect(screen.getByText(/#1 ARRIVAL-22 at 10:16/)).toBeInTheDocument();
    expect(screen.getAllByText("Unavailable").length).toBeGreaterThan(2);
  });

  it("supports timeline and strip hit testing from the responsive scrolling layout", () => {
    const onSelectFlight = renderBoard(state());
    const marker = screen.getByRole("button", {name: "Select SAS123 timeline marker"});
    fireEvent.click(marker);
    fireEvent.click(screen.getByRole("button", {name: "SAS123 AMAN strip"}));

    expect(onSelectFlight).toHaveBeenNthCalledWith(1, "flight-123");
    expect(onSelectFlight).toHaveBeenNthCalledWith(2, "flight-123");
    expect(screen.getByTestId("aman-timeline-grid")).toHaveClass("min-w-[880px]");
  });
});
