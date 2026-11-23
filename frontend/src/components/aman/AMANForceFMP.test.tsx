import {fireEvent, render, screen} from "@testing-library/react";
import {describe, expect, it, vi} from "vitest";

import type {AMANFMPAuthority, AMANState} from "@/api/aman";
import {AMANSettingsHeader} from "./AMANSettingsHeader";

const state = {
  airport: "EKCH", generated_at: "2026-11-23T10:00:00.000Z", authoritative: true, effective_mode: "authoritative",
  flights: [],
  technical_health: {status: "ready", ready: true, blocked_reasons: []},
  header: {
    active_runway_groups: [{id: "ARRIVAL-22", active_rate_per_hour: 30, rate_effective_at: null}],
    readiness: {status: "ready", ready: true, blocked_reasons: []},
    traffic_summary: {status: "ready", tma_above_1500_feet_count: 0, maestro_horizon_count: 0},
    wind: null,
  },
} as unknown as AMANState;

const status = (overrides: Partial<AMANFMPAuthority> = {}): AMANFMPAuthority => ({
  has_fmp_authority: false, forced: false, forced_holder: null, fmp_online: false, can_force: true, ...overrides,
});

function renderHeader(forcedFMP: AMANFMPAuthority | null, onForceFMP = vi.fn()) {
  render(<AMANSettingsHeader connectionState="connected" forcedFMP={forcedFMP} hasFMPAuthority={forcedFMP?.has_fmp_authority ?? false} onForceFMP={onForceFMP} onRunwayGroupViewChange={vi.fn()} onViewChange={vi.fn()} presentationStatus="ready" readOnly={false} selectedRunwayGroupID="ARRIVAL-22" state={state} view="holds" />);
  return onForceFMP;
}

const forceButton = () => screen.getByRole("button", {name: "FORCE FMP"});

describe("FORCE FMP", () => {
  it("is gray and asks for confirmation before forcing FMP", () => {
    const onForceFMP = renderHeader(status());

    expect(forceButton()).toHaveAttribute("aria-pressed", "false");
    expect(forceButton()).toHaveClass("bg-[#d6d6d6]");
    fireEvent.click(forceButton());
    expect(screen.getByText(/Are you sure you want to force FMP\?/)).toBeInTheDocument();
    expect(onForceFMP).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", {name: "Confirm FORCE FMP"}));

    expect(onForceFMP).toHaveBeenCalledWith(true);
  });

  it("does nothing when the confirmation is declined", () => {
    const onForceFMP = renderHeader(status());

    fireEvent.click(forceButton());
    fireEvent.click(screen.getByRole("button", {name: "Cancel"}));

    expect(onForceFMP).not.toHaveBeenCalled();
  });

  it("is green for the holder and releases after confirmation", () => {
    const onForceFMP = renderHeader(status({has_fmp_authority: true, forced: true, forced_holder: "EKCH_W_APP", can_force: false}));

    expect(forceButton()).toHaveAttribute("aria-pressed", "true");
    expect(forceButton()).toHaveClass("bg-[#2f9a46]");
    expect(screen.queryByText("FMP: EKCH_W_APP")).not.toBeInTheDocument();
    fireEvent.click(forceButton());
    expect(screen.getByText(/Are you sure you want to release FORCE FMP\?/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", {name: "Release FORCE FMP"}));

    expect(onForceFMP).toHaveBeenCalledWith(false);
  });

  it("is green and locked for a real FMP", () => {
    renderHeader(status({has_fmp_authority: true, fmp_online: true, can_force: false}));

    expect(forceButton()).toHaveClass("bg-[#2f9a46]");
    expect(forceButton()).toBeDisabled();
  });

  it("is gray and disabled while FMP is online", () => {
    renderHeader(status({fmp_online: true, can_force: false}));

    expect(forceButton()).toBeDisabled();
    expect(forceButton()).toHaveAttribute("title", expect.stringContaining("FMP is online"));
  });

  it("shows who holds FORCE FMP and warns before taking it over", () => {
    const onForceFMP = renderHeader(status({forced_holder: "EKDK_V_CTR"}));

    expect(screen.getByText("FMP: EKDK_V_CTR")).toBeInTheDocument();
    fireEvent.click(forceButton());
    expect(screen.getByText("This takes FMP authority from EKDK_V_CTR.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", {name: "Confirm FORCE FMP"}));

    expect(onForceFMP).toHaveBeenCalledWith(true);
  });

  it("closes an open release confirmation when someone else takes FMP", () => {
    const onForceFMP = vi.fn();
    const header = (forcedFMP: AMANFMPAuthority) => <AMANSettingsHeader connectionState="connected" forcedFMP={forcedFMP} hasFMPAuthority={forcedFMP.has_fmp_authority} onForceFMP={onForceFMP} onRunwayGroupViewChange={vi.fn()} onViewChange={vi.fn()} presentationStatus="ready" readOnly={false} selectedRunwayGroupID="ARRIVAL-22" state={state} view="holds" />;
    const {rerender} = render(header(status({has_fmp_authority: true, forced: true, forced_holder: "EKCH_W_APP", can_force: false})));
    fireEvent.click(forceButton());
    expect(screen.getByRole("button", {name: "Release FORCE FMP"})).toBeInTheDocument();

    rerender(header(status({forced_holder: "EKDK_V_CTR"})));

    expect(screen.queryByRole("button", {name: "Release FORCE FMP"})).not.toBeInTheDocument();
    expect(screen.queryByRole("button", {name: "Confirm FORCE FMP"})).not.toBeInTheDocument();
    expect(onForceFMP).not.toHaveBeenCalled();
  });

  it("is hidden until the server reports FMP authority", () => {
    renderHeader(null);

    expect(screen.queryByRole("button", {name: "FORCE FMP"})).not.toBeInTheDocument();
  });
});
