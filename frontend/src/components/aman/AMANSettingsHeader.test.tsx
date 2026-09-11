import {fireEvent, render, screen} from "@testing-library/react";
import {describe, expect, it, vi} from "vitest";

import type {AMANState} from "@/api/aman";
import {AMANSettingsHeader} from "./AMANSettingsHeader";

const state = {
  airport: "EKCH", generated_at: "2026-09-12T10:00:00.000Z", authoritative: false, effective_mode: "read_only",
  flights: [{sequence_disposition: "desequenced"}],
  header: {
    active_runway_groups: [{id: "ARRIVAL-22", active_rate_per_hour: 30, rate_effective_at: null}],
    readiness: {status: "degraded", ready: false, blocked_reasons: ["predictor_stale"]},
    traffic_summary: {status: "degraded", tma_above_1500_feet_count: 7, maestro_horizon_count: 11},
    wind: null,
  },
} as unknown as AMANState;

describe("AMANSettingsHeader", () => {
  it("renders authoritative projections with explicit non-color degraded and read-only cues", () => {
    render(<AMANSettingsHeader connectionState="disconnected" onOpenTargetPreferences={vi.fn()} onRunwayGroupViewChange={vi.fn()} onViewChange={vi.fn()} presentationStatus="degraded" selectedRunwayGroupID="ARRIVAL-22" state={state} view="holds" />);

    expect(screen.getByRole("button", {name: "ARRIVAL-22"})).toBeInTheDocument();
    expect(screen.getByText("ARRIVAL-22: 30/h")).toBeInTheDocument();
    expect(screen.getByText("TMA 7 · Horizon 11")).toBeInTheDocument();
    expect(screen.getByText("Unavailable")).toBeInTheDocument();
    expect(screen.getByText(/DISCONNECTED.*STALE.*DEGRADED.*READ ONLY/)).toBeInTheDocument();
    expect(screen.getByText("DSEQ · 1")).toBeInTheDocument();
    expect(screen.getByRole("button", {name: "ALL"})).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", {name: "ALL"})).toHaveTextContent("✓ ALL");
  });

  it("keeps view and preference interactions local to callbacks", () => {
    const onViewChange = vi.fn();
    const onOpenTargetPreferences = vi.fn();
    render(<AMANSettingsHeader connectionState="connected" onOpenTargetPreferences={onOpenTargetPreferences} onRunwayGroupViewChange={vi.fn()} onViewChange={onViewChange} presentationStatus="ready" selectedRunwayGroupID="ARRIVAL-22" state={state} view="holds" />);

    fireEvent.click(screen.getByRole("button", {name: "RWY"}));
    fireEvent.click(screen.getByRole("button", {name: "Open target information preferences"}));
    expect(onViewChange).toHaveBeenCalledWith("runway");
    expect(onOpenTargetPreferences).toHaveBeenCalledOnce();
  });
});
