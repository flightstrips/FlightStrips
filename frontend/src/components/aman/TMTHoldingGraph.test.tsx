import {render, screen, within} from "@testing-library/react";
import {describe, expect, it} from "vitest";

import type {AMANHoldingEntry} from "@/api/aman";
import {TMTHoldingGraph} from "./TMTHoldingGraph";

const now = new Date("2026-09-11T10:00:00.000Z");

function entry(callsign: string, eat: string | null, clearedAltitude: number | null): AMANHoldingEntry {
  return {
    flight_id: callsign.toLowerCase(),
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
    expect(screen.getByLabelText("SAS101, OLPIB, FL195, EAT 10:30, SCHEDULED")).toHaveStyle({top: "clamp(14px, 50%, calc(100% - 14px))"});
    expect(screen.getByText("1030")).toHaveClass("text-[#f0e129]");
  });

  it("marks the exact four-minute boundary and overdue entries without relying on color", () => {
    render(<TMTHoldingGraph entries={[
      entry("SOON4", "2026-09-11T10:04:00.000Z", 12000),
      entry("LATE1", "2026-09-11T09:59:59.000Z", 13000),
    ]} now={now} />);

    expect(screen.getByLabelText("SOON4, OLPIB, FL120, EAT 10:04, DUE ≤4 MIN")).toHaveTextContent("≤4");
    expect(screen.getByLabelText("LATE1, OLPIB, FL130, EAT OVERDUE, PAST EAT")).toHaveTextContent("!OVERDUE");
    expect(screen.getByText("! PAST EAT")).toBeVisible();
  });

  it("pins out-of-range values and keeps missing EAT or CFL visible", () => {
    render(<TMTHoldingGraph entries={[
      entry("EDGES", "2026-09-11T11:00:01.000Z", 30100),
      entry("NOEAT", null, 11000),
      entry("NOCFL", "2026-09-11T10:20:00.000Z", null),
    ]} now={now} />);

    expect(screen.getByLabelText("EDGES, OLPIB, >300, EAT >60, SCHEDULED")).toBeVisible();
    const missing = screen.getByLabelText("Holding aircraft with missing values");
    expect(within(missing).getByText("NOEAT")).toBeVisible();
    expect(within(missing).getByText("EAT —")).toBeVisible();
    expect(within(missing).getByText("NOCFL")).toBeVisible();
    expect(within(missing).getByText("CFL —")).toBeVisible();
  });
});
