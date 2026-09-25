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
      planned_count: index === 0 ? 1 : index === 2 ? 4 : 0,
      airborne_count: index === 0 ? 2 : index === 2 ? 8 : 0,
      count: index === 0 ? 3 : index === 2 ? 12 : 0,
      load_factor: index === 0 ? 12 : index === 2 ? 48 : 0,
      selected_rate: index === 2 ? {runway_group_id: "22L", arrivals_per_hour: 40, effective_at: new Date(start).toISOString()} : null,
      bucket_high: index === 2, window_high: index === 2,
      alert: index === 1 ? "yellow" : index === 2 ? "red" : "none",
      flights: index === 0 ? [{callsign: "SAS101", airborne: true, landing_at: "2026-07-22T20:32:00.000Z", timing_source: "aman", data_status: "fresh"}] : [],
    })),
  };
}

describe("TMTTrafficPrediction", () => {
  it("renders one continuous, accessible bucket chart with stacked and alert segments", () => {
    render(<TMTTrafficPrediction prediction={prediction()} />);

    const buckets = screen.getAllByRole("listitem");
    expect(buckets).toHaveLength(12);
    expect(screen.getByLabelText("20:30 to 20:45: 3 arrivals, load factor 12")).toBeInTheDocument();
    expect(screen.getByText(/Arrival rate unavailable/)).toBeInTheDocument();
    expect(screen.getByTestId("traffic-bar-20:30")).toBeInTheDocument();
    expect(screen.getByTestId("traffic-bar-21:00").querySelector('[data-alert="red"]')).toBeInTheDocument();
    expect(screen.queryByText("RATE —")).not.toBeInTheDocument();
    expect(screen.queryByText("Load = aircraft × 4")).not.toBeInTheDocument();

    fireEvent.focus(buckets[0]);
    expect(screen.getByText(/SAS101 20:32/, {selector: ".sr-only"})).toBeInTheDocument();
  });
});
