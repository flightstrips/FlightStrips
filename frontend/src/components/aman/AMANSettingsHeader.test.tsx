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
    render(<AMANSettingsHeader commandRejections={{}} connectionState="disconnected" hasFMPAuthority={false} onCommand={vi.fn()} onOpenTargetPreferences={vi.fn()} onRunwayGroupViewChange={vi.fn()} onViewChange={vi.fn()} pendingCommands={{}} presentationStatus="degraded" readOnly={true} selectedRunwayGroupID="ARRIVAL-22" state={state} view="holds" />);

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
    render(<AMANSettingsHeader commandRejections={{}} connectionState="connected" hasFMPAuthority={false} onCommand={vi.fn()} onOpenTargetPreferences={onOpenTargetPreferences} onRunwayGroupViewChange={vi.fn()} onViewChange={onViewChange} pendingCommands={{}} presentationStatus="ready" readOnly={true} selectedRunwayGroupID="ARRIVAL-22" state={state} view="holds" />);

    fireEvent.click(screen.getByRole("button", {name: "RWY"}));
    fireEvent.click(screen.getByRole("button", {name: "Open target information preferences"}));
    expect(onViewChange).toHaveBeenCalledWith("runway");
    expect(onOpenTargetPreferences).toHaveBeenCalledOnce();
  });

  it("sends independent active-set and rate commands without optimistic replacement", () => {
    const writable = structuredClone(state);
    writable.authoritative = true;
    writable.effective_mode = "authoritative";
    writable.technical_health = {status: "ready", ready: true, blocked_reasons: [], ...({navigation: {status: "ready", reason: null, updated_at: null, age_seconds: null}, weather: {status: "ready", reason: null, updated_at: null, age_seconds: null}, vatsim: {status: "ready", reason: null, updated_at: null, age_seconds: null}, repository: {status: "ready", reason: null, updated_at: null, age_seconds: null}, predictor: {status: "ready", reason: null, updated_at: null, age_seconds: null}, replay_validation: {status: "ready", reason: null, updated_at: null, age_seconds: null}})};
    writable.runway_groups = [{id: "ARRIVAL-22", selected: true, selection_schedule: []}, {id: "ARRIVAL-04", selected: false, selection_schedule: []}];
    writable.header!.active_runway_groups = [{id: "ARRIVAL-22", active_rate_per_hour: 30, rate_effective_at: null}];
    const onCommand = vi.fn();
    const props = {commandRejections: {}, connectionState: "connected" as const, hasFMPAuthority: true, onCommand, onOpenTargetPreferences: vi.fn(), onRunwayGroupViewChange: vi.fn(), onViewChange: vi.fn(), pendingCommands: {}, presentationStatus: "ready" as const, readOnly: false, selectedRunwayGroupID: "ARRIVAL-22", state: writable, view: "holds" as const};
    const {rerender} = render(<AMANSettingsHeader {...props} />);

    fireEvent.click(screen.getByRole("button", {name: "ARRIVAL-22"}));
    fireEvent.click(screen.getByRole("checkbox", {name: /ARRIVAL-04/}));
    fireEvent.click(screen.getByRole("button", {name: "Replace active set"}));
    expect(onCommand).toHaveBeenCalledWith({type: "aman.set_active_runway_groups", runway_group_ids: ["ARRIVAL-22", "ARRIVAL-04"]});
    expect(screen.getByText(/Server-confirmed:/)).toHaveTextContent("ARRIVAL-22");

    fireEvent.click(screen.getByRole("button", {name: "Cancel"}));
    fireEvent.click(screen.getByRole("button", {name: /ARRIVAL-22: 30\/h/}));
    fireEvent.change(screen.getByLabelText("Rate runway group"), {target: {value: "ARRIVAL-04"}});
    fireEvent.change(screen.getByLabelText("Arrivals per hour"), {target: {value: "24"}});
    fireEvent.click(screen.getByRole("button", {name: "Set arrival rate"}));
    expect(onCommand).toHaveBeenLastCalledWith({type: "aman.set_rate", runway_group_id: "ARRIVAL-04", arrivals_per_hour: 24, effective_at: writable.generated_at});
    rerender(<AMANSettingsHeader {...props} pendingCommands={{rate: {command_id: "rate", type: "aman.set_rate", expected_revision: 7, runway_group_id: "ARRIVAL-04", arrivals_per_hour: 24, effective_at: writable.generated_at}}} />);
    expect(screen.getByRole("status")).toHaveTextContent("Pending rate: ARRIVAL-04 · 24/h");
  });

  it("blocks empty, rejected, disconnected, and unauthorized runway mutations", () => {
    const writable = structuredClone(state);
    writable.authoritative = true;
    writable.effective_mode = "authoritative";
    writable.technical_health = {status: "ready", ready: true, blocked_reasons: [], ...({navigation: {status: "ready", reason: null, updated_at: null, age_seconds: null}, weather: {status: "ready", reason: null, updated_at: null, age_seconds: null}, vatsim: {status: "ready", reason: null, updated_at: null, age_seconds: null}, repository: {status: "ready", reason: null, updated_at: null, age_seconds: null}, predictor: {status: "ready", reason: null, updated_at: null, age_seconds: null}, replay_validation: {status: "ready", reason: null, updated_at: null, age_seconds: null}})};
    writable.runway_groups = [{id: "ARRIVAL-22", selected: true, selection_schedule: []}];
    const common = {connectionState: "connected" as const, hasFMPAuthority: true, onCommand: vi.fn(), onOpenTargetPreferences: vi.fn(), onRunwayGroupViewChange: vi.fn(), onViewChange: vi.fn(), presentationStatus: "ready" as const, readOnly: false, selectedRunwayGroupID: "ARRIVAL-22", state: writable, view: "holds" as const};
    const {rerender} = render(<AMANSettingsHeader {...common} commandRejections={{stale: {command_id: "stale", command_type: "aman.set_active_runway_groups", code: "revision_conflict", message: "revision changed", current_revision: 8, retryable: true}}} pendingCommands={{}} />);

    fireEvent.click(screen.getByRole("button", {name: "ARRIVAL-22"}));
    expect(screen.getByRole("alert")).toHaveTextContent("revision_conflict");
    fireEvent.click(screen.getByRole("checkbox", {name: /ARRIVAL-22/}));
    expect(screen.getByText("Select at least one runway.")).toHaveAttribute("role", "alert");
    expect(screen.getByRole("button", {name: "Replace active set"})).toBeDisabled();

    rerender(<AMANSettingsHeader {...common} commandRejections={{}} connectionState="disconnected" hasFMPAuthority={false} pendingCommands={{}} readOnly={true} />);
    expect(screen.getByRole("status")).toHaveTextContent("Read-only");
    expect(screen.getByRole("button", {name: "Replace active set"})).toBeDisabled();
  });
});
