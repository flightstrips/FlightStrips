import {fireEvent, render, screen, within} from "@testing-library/react";
import {describe, expect, it} from "vitest";

import type {AMANHoldingEntry} from "@/api/aman";
import {TMTHoldingGraph} from "./TMTHoldingGraph";
import {layoutHoldingGraph} from "./holding-graph-positioning";

const now = new Date("2026-09-11T10:00:00.000Z");

function entry(callsign: string, eat: string | null, clearedAltitude: number | null): AMANHoldingEntry {
  return {
    callsign,
    holding: "OLPIB",
    eat,
    cleared_altitude: clearedAltitude,
    source_status: "fresh",
    observed_at: now.toISOString(),
  };
}

describe("TMT holding graph", () => {
  it("renders fixed time and altitude axes with positioned authoritative entries", () => {
    render(<TMTHoldingGraph entries={[entry("SAS101", "2026-09-11T10:30:00.000Z", 19500)]} now={now} />);

    const graph = screen.getByTestId("holding-graph");
    expect(within(graph).getByText("10:00")).toBeInTheDocument();
    expect(within(graph).getByText("11:00")).toBeInTheDocument();
    expect(within(graph).getByText("300")).toBeInTheDocument();
    expect(within(graph).getByText("90")).toBeInTheDocument();
    expect(layoutHoldingGraph([entry("SAS101", "2026-09-11T10:30:00.000Z", 19500)], now)[0].altitude.percent).toBe(50);
    expect(screen.getByText("1030")).toHaveClass("text-[#f0e129]");
  });

  it("uses the graph-only Figma composition in compact dashboard cards", () => {
    render(<TMTHoldingGraph compact entries={[]} holding="OLPIB" now={now} />);

    expect(screen.queryByText("TMT · HOLDING INFORMATION")).not.toBeInTheDocument();
    expect(screen.queryByText("S SCHEDULED")).not.toBeInTheDocument();
    expect(screen.getByText("OLPIB")).toBeInTheDocument();
    expect(screen.getAllByText("00")).toHaveLength(2);
    expect(screen.getByText("NONE")).toBeInTheDocument();
  });

  it("marks the exact four-minute boundary and overdue entries without relying on color", () => {
    render(<TMTHoldingGraph entries={[
      entry("SOON4", "2026-09-11T10:04:00.000Z", 12000),
      entry("LATE1", "2026-09-11T09:59:59.000Z", 13000),
    ]} now={now} />);

    expect(screen.getByLabelText("SOON4, OLPIB, FL120, EAT 10:04, DUE ≤4 MIN, LIVE DATA")).toHaveTextContent("≤4");
    expect(screen.getByLabelText("LATE1, OLPIB, FL130, EAT OVERDUE, PAST EAT, LIVE DATA")).toHaveTextContent("!OVERDUE");
    expect(screen.getByText("! PAST EAT")).toBeVisible();
  });

  it("pins out-of-range values and keeps missing EAT or CFL visible", () => {
    render(<TMTHoldingGraph entries={[
      entry("EDGES", "2026-09-11T11:00:01.000Z", 30100),
      entry("NOEAT", null, 11000),
      entry("NOCFL", "2026-09-11T10:20:00.000Z", null),
    ]} now={now} />);

    expect(screen.getByLabelText("EDGES, OLPIB, >300, EAT >60, SCHEDULED, LIVE DATA")).toBeVisible();
    const graph = screen.getByTestId("holding-graph");
    expect(within(graph).getByLabelText(/NOEAT, OLPIB, FL110, EAT unavailable/)).toBeVisible();
    const missing = screen.getByLabelText("Holding aircraft with missing values");
    expect(within(missing).queryByText("NOEAT")).not.toBeInTheDocument();
    expect(within(missing).getByText("NOCFL")).toBeVisible();
    expect(within(missing).getByText("CFL —")).toBeVisible();
  });

  it.each([false, true])("shows aircraft without EAT at their altitude without a time line (compact=%s)", (compact) => {
    render(<TMTHoldingGraph compact={compact} entries={[
      entry("NOEAT", null, 12000),
      entry("TIMED", "2026-09-11T10:20:00.000Z", 12000),
    ]} now={now} />);

    const graph = screen.getByTestId("holding-graph");
    const noEat = within(graph).getByLabelText(/NOEAT, OLPIB, FL120, EAT unavailable/);
    const timed = within(graph).getByLabelText(/TIMED, OLPIB, FL120/);
    expect(noEat).toBeVisible();
    expect(noEat.style.top).not.toBe(timed.style.top);
    expect(noEat.style.left).toBe(timed.style.left);
    expect(noEat).toHaveTextContent("—");
    expect(graph.querySelectorAll("svg line")).toHaveLength(1);
    expect(screen.queryByLabelText("Holding aircraft with missing values")).not.toBeInTheDocument();
  });

  it("announces stale and disconnected source data with visible non-color cues", () => {
    const stale = {...entry("STALE1", "2026-09-11T10:20:00.000Z", 12000), source_status: "stale" as const};
    const disconnected = {...entry("LOST1", null, 13000), source_status: "disconnected" as const};
    render(<TMTHoldingGraph entries={[stale, disconnected]} now={now} />);

    expect(screen.queryByText(/DEGRADED DATA/)).not.toBeInTheDocument();
    expect(screen.getByLabelText(/STALE1.*STALE DATA/)).toHaveTextContent("△");
    expect(screen.getByLabelText(/LOST1.*EAT unavailable.*SOURCE DISCONNECTED/)).toHaveTextContent("×");
    expect(screen.getByText("△ STALE")).toBeVisible();
    expect(screen.getByText("× DISCONNECTED")).toBeVisible();
  });

  it("keeps colliding and incomplete entries focusable in authoritative input order", () => {
    render(<TMTHoldingGraph entries={[
      entry("FIRST", "2026-09-11T10:20:00.000Z", 12000),
      entry("MISSING", null, 12000),
      entry("THIRD", "2026-09-11T10:20:00.000Z", 12000),
    ]} now={now} />);

    const first = screen.getByLabelText(/FIRST/);
    const missing = screen.getByLabelText(/MISSING/);
    const third = screen.getByLabelText(/THIRD/);
    first.focus();
    fireEvent.keyDown(first, {key: "ArrowDown"});
    expect(missing).toHaveFocus();
    fireEvent.keyDown(missing, {key: "ArrowDown"});
    expect(third).toHaveFocus();
    fireEvent.keyDown(third, {key: "Home"});
    expect(first).toHaveFocus();
    fireEvent.keyDown(first, {key: "End"});
    expect(third).toHaveFocus();
    expect(first).toHaveAttribute("tabindex", "0");
    expect(third).toHaveAttribute("tabindex", "0");
  });
});
