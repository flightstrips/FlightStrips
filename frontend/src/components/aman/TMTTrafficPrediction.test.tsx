import {fireEvent, render, screen} from "@testing-library/react";
import {describe, expect, it} from "vitest";

import type {AMANTrafficPrediction} from "@/api/aman";
import {TMTTrafficPrediction} from "./TMTTrafficPrediction";

function prediction(): AMANTrafficPrediction {
  const start = Date.parse("2026-07-22T20:30:00.000Z");
  return {
    generated_at: "2026-07-22T20:44:00.000Z", range_start: new Date(start).toISOString(),
    range_end: new Date(start + 3 * 60 * 60_000).toISOString(), bucket_minutes: 15, source_status: "fresh", status: "degraded",
    degraded_reasons: ["missing_selected_rate"],
    buckets: Array.from({length: 12}, (_, index) => ({
      start: new Date(start + index * 15 * 60_000).toISOString(), end: new Date(start + (index + 1) * 15 * 60_000).toISOString(),
      planned_count: index === 0 ? 1 : 0, airborne_count: index === 0 ? 2 : 0, count: index === 0 ? 3 : 0,
      load_factor: index === 0 ? 12 : 0, selected_rate: null, bucket_high: false, window_high: false,
      alert: index === 1 ? "yellow" : index === 2 ? "red" : "none",
      flights: index === 0 ? [{flight_id: "1", callsign: "SAS101", airborne: true, landing_at: "2026-07-22T20:32:00.000Z", timing_source: "aman", data_status: "fresh"}] : [],
    })),
  };
}

describe("TMTTrafficPrediction", () => {
  it("renders backend-authored buckets, status, rates, colours, and local details", () => {
    const model = prediction();
    render(<TMTTrafficPrediction prediction={model} />);

    expect(screen.getAllByRole("listitem")).toHaveLength(12);
    expect(screen.getByLabelText("20:30 to 20:45: 3 arrivals, load factor 12")).toBeInTheDocument();
    expect(screen.getByText(/Arrival rate unavailable/)).toBeInTheDocument();
    fireEvent.focus(screen.getAllByRole("listitem")[0]);
    expect(screen.getByText(/SAS101 20:32/)).toBeInTheDocument();
    expect(screen.getAllByText("RATE —")).toHaveLength(12);
    expect(screen.getAllByRole("listitem")[1]).toHaveClass("bg-[#f0e129]");
    expect(screen.getAllByRole("listitem")[2]).toHaveClass("bg-[#9c0000]");
    expect(screen.getByText("20:30–23:30 UTC")).toBeInTheDocument();
  });
});
