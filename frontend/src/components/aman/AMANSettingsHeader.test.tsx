import {fireEvent, render, screen} from "@testing-library/react";
import {describe, expect, it, vi} from "vitest";

import type {AMANPendingCommand, AMANState} from "@/api/aman";
import {AMANSettingsHeader} from "./AMANSettingsHeader";

const state = {
  airport: "EKCH", generated_at: "2026-09-12T10:00:00.000Z", authoritative: false, effective_mode: "read_only",
  flights: [{sequence_disposition: "desequenced"}],
  technical_health: {vatsim: {status: "degraded"}},
  header: {
    active_runway_groups: [{id: "ARRIVAL-22", active_rate_per_hour: 30, rate_effective_at: null}],
    readiness: {status: "degraded", ready: false, blocked_reasons: ["predictor_stale"]},
    traffic_summary: {status: "degraded", tma_above_1500_feet_count: 7, maestro_horizon_count: 11},
    wind: null,
  },
} as unknown as AMANState;

describe("AMANSettingsHeader", () => {
  it("does not mark the header stale or degraded for weather fallback and an individual retained estimate", () => {
    const current = structuredClone(state);
    current.header!.readiness = {status: "ready", ready: true, blocked_reasons: []};
    current.technical_health = {...current.technical_health, ready: true, status: "ready",
      vatsim: {status: "ready", reason: null, updated_at: null, age_seconds: null},
      weather: {status: "unavailable", reason: "weather_refresh_failed", updated_at: null, age_seconds: null}};
    render(<AMANSettingsHeader commandRejections={{}} connectionState="connected" hasFMPAuthority={false} onCommand={vi.fn()} onRunwayGroupViewChange={vi.fn()} onViewChange={vi.fn()} pendingCommands={{}} presentationStatus="ready" readOnly={true} selectedRunwayGroupID="ARRIVAL-22" state={current} view="holds" />);
    expect(screen.queryByText(/△ STALE|△ DEGRADED/)).not.toBeInTheDocument();
    expect(screen.getByText("TMA").parentElement).toHaveTextContent("TMA:7");
    expect(screen.getByText("TOT").parentElement).toHaveTextContent("TOT:11");
  });

  it("renders authoritative projections with explicit non-color degraded and read-only cues", () => {
    render(<AMANSettingsHeader commandRejections={{}} connectionState="disconnected" hasFMPAuthority={false} onCommand={vi.fn()} onRunwayGroupViewChange={vi.fn()} onViewChange={vi.fn()} pendingCommands={{}} presentationStatus="degraded" readOnly={true} selectedRunwayGroupID="ARRIVAL-22" state={state} view="holds" />);

    expect(screen.getByRole("button", {name: "22"})).toBeInTheDocument();
    expect(screen.getByRole("button", {name: "22L: 4022R: 4012: 4030: 40"})).toBeInTheDocument();
    expect(screen.getByText("TMA").parentElement).toHaveTextContent("TMA:7");
    expect(screen.getByText("TOT").parentElement).toHaveTextContent("TOT:11");
    expect(screen.getByText("Unavailable")).toBeInTheDocument();
    expect(screen.getByText(/DISCONNECTED.*STALE.*DEGRADED.*READ ONLY/)).toBeInTheDocument();
    expect(screen.getByText("DSEQ · 1")).toBeInTheDocument();
    expect(screen.getByRole("button", {name: "ALL"})).toHaveAttribute("aria-pressed", "true");
    const selectedView = screen.getByRole("button", {name: "ALL"});
    expect(selectedView).toHaveTextContent("✓ALL");
    expect(selectedView.children).toHaveLength(2);
  });

  it("shows only runway identifiers in the RIU box", () => {
    const current = structuredClone(state);
    current.header!.active_runway_groups = [
      {id: "ARRIVAL-22L", active_rate_per_hour: 30, rate_effective_at: null},
      {id: "ARRIVAL-30", active_rate_per_hour: 24, rate_effective_at: null},
    ];
    render(<AMANSettingsHeader connectionState="connected" onRunwayGroupViewChange={vi.fn()} onViewChange={vi.fn()} presentationStatus="ready" selectedRunwayGroupID="ARRIVAL-22L" state={current} view="holds" />);

    expect(screen.getByRole("button", {name: "22L, 30"})).toBeInTheDocument();
  });

  it("shows compact runway rates separated by gray vertical dividers", () => {
    const current = structuredClone(state);
    current.header!.active_runway_groups = [
      {id: "ARRIVAL-22L", active_rate_per_hour: 30, rate_effective_at: null},
      {id: "ARRIVAL-30", active_rate_per_hour: 24, rate_effective_at: null},
    ];
    render(<AMANSettingsHeader connectionState="connected" onRunwayGroupViewChange={vi.fn()} onViewChange={vi.fn()} presentationStatus="ready" selectedRunwayGroupID="ARRIVAL-22L" state={current} view="holds" />);

    const rateButton = screen.getByRole("button", {name: "22L: 3022R: 4012: 4030: 24"});
    expect(rateButton.querySelector("span.border-l")).toHaveClass("border-[#dcdcdc]");
  });

  it("shows 04L and 04R rates when a 04 runway is active", () => {
    const current = structuredClone(state);
    current.header!.active_runway_groups = [{id: "ARRIVAL-04R", active_rate_per_hour: 28, rate_effective_at: null}];
    render(<AMANSettingsHeader connectionState="connected" onRunwayGroupViewChange={vi.fn()} onViewChange={vi.fn()} presentationStatus="ready" selectedRunwayGroupID="ARRIVAL-04R" state={current} view="holds" />);

    expect(screen.getByRole("button", {name: "04L: 4004R: 2812: 4030: 40"})).toBeInTheDocument();
  });

  it("keeps view interactions local and opens the MAESTRO selector", () => {
    const onViewChange = vi.fn();
    render(<AMANSettingsHeader commandRejections={{}} connectionState="connected" hasFMPAuthority={false} onCommand={vi.fn()} onRunwayGroupViewChange={vi.fn()} onViewChange={onViewChange} pendingCommands={{}} presentationStatus="ready" readOnly={true} selectedRunwayGroupID="ARRIVAL-22" state={state} view="holds" />);

    fireEvent.click(screen.getByRole("button", {name: "RWY"}));
    fireEvent.click(screen.getByRole("button", {name: "MAESTRO"}));
    expect(onViewChange).toHaveBeenCalledWith("runway");
    expect(screen.getByRole("menu", {name: "MAESTRO"})).toBeInTheDocument();
  });

  it("toggles STAR selections like tick marks without changing the active timeline view", () => {
    const current = structuredClone(state);
    current.timeline_configuration = {version: "mapping-v1", mappings: [{id: 1, left: "TESPI", right: "TUDLO"}]};
    const onStarFamiliesChange = vi.fn();
    const onViewChange = vi.fn();
    const {rerender} = render(<AMANSettingsHeader connectionState="connected" onRunwayGroupViewChange={vi.fn()} onStarFamiliesChange={onStarFamiliesChange} onViewChange={onViewChange} presentationStatus="ready" selectedStarFamilies={[]} selectedRunwayGroupID="ARRIVAL-22" state={current} view="runway" />);

    fireEvent.click(screen.getByRole("button", {name: "MAESTRO"}));
    expect(screen.getByRole("menuitemcheckbox", {name: "MAESTRO", checked: true})).toBeInTheDocument();
    fireEvent.click(screen.getByRole("menuitemcheckbox", {name: "TUDLO"}));
    expect(onStarFamiliesChange).toHaveBeenLastCalledWith(["TUDLO"]);

    rerender(<AMANSettingsHeader connectionState="connected" onRunwayGroupViewChange={vi.fn()} onStarFamiliesChange={onStarFamiliesChange} onViewChange={onViewChange} presentationStatus="ready" selectedStarFamilies={["TUDLO"]} selectedRunwayGroupID="ARRIVAL-22" state={current} view="runway" />);
    fireEvent.click(screen.getByRole("menuitemcheckbox", {name: "TESPI"}));
    expect(onStarFamiliesChange).toHaveBeenLastCalledWith(["TUDLO", "TESPI"]);
    fireEvent.click(screen.getByRole("menuitemcheckbox", {name: "TUDLO"}));
    expect(onStarFamiliesChange).toHaveBeenLastCalledWith([]);
    fireEvent.click(screen.getByRole("menuitemcheckbox", {name: "MAESTRO"}));
    expect(onStarFamiliesChange).toHaveBeenLastCalledWith([]);
    expect(onViewChange).not.toHaveBeenCalled();
  });

  it("sends independent active-set and rate commands without optimistic replacement", () => {
    const writable = structuredClone(state);
    writable.authoritative = true;
    writable.effective_mode = "authoritative";
    writable.technical_health = {status: "ready", ready: true, blocked_reasons: [], ...({navigation: {status: "ready", reason: null, updated_at: null, age_seconds: null}, weather: {status: "ready", reason: null, updated_at: null, age_seconds: null}, vatsim: {status: "ready", reason: null, updated_at: null, age_seconds: null}, repository: {status: "ready", reason: null, updated_at: null, age_seconds: null}, predictor: {status: "ready", reason: null, updated_at: null, age_seconds: null}, replay_validation: {status: "ready", reason: null, updated_at: null, age_seconds: null}})};
    writable.runway_groups = [{id: "ARRIVAL-22", selected: true, selection_schedule: []}, {id: "ARRIVAL-04", selected: false, selection_schedule: []}];
    writable.header!.active_runway_groups = [{id: "ARRIVAL-22", active_rate_per_hour: 30, rate_effective_at: null}];
    const onCommand = vi.fn();
    const props = {commandRejections: {}, connectionState: "connected" as const, hasFMPAuthority: true, onCommand, onRunwayGroupViewChange: vi.fn(), onViewChange: vi.fn(), pendingCommands: {}, presentationStatus: "ready" as const, readOnly: false, selectedRunwayGroupID: "ARRIVAL-22", state: writable, view: "holds" as const};
    const {rerender} = render(<AMANSettingsHeader {...props} />);

    fireEvent.click(screen.getByRole("button", {name: "22"}));
    fireEvent.click(screen.getByRole("checkbox", {name: /ARRIVAL-04/}));
    fireEvent.click(screen.getByRole("button", {name: "Replace active set"}));
    expect(onCommand).toHaveBeenCalledWith({type: "aman.set_active_runway_groups", runway_group_ids: ["ARRIVAL-22", "ARRIVAL-04"]});
    expect(screen.getByText(/Server-confirmed:/)).toHaveTextContent("ARRIVAL-22");

    fireEvent.click(screen.getByRole("button", {name: "Cancel"}));
    fireEvent.click(screen.getByRole("button", {name: "22L: 4022R: 4012: 4030: 40"}));
    fireEvent.change(screen.getByLabelText("Rate 22R"), {target: {value: "24"}});
    fireEvent.change(screen.getByLabelText("Apply rates at"), {target: {value: "10"}});
    fireEvent.click(screen.getByRole("button", {name: "Set arrival rate"}));
    expect(onCommand).toHaveBeenLastCalledWith({type: "aman.set_rate", runway_group_id: "ARRIVAL-22R", arrivals_per_hour: 24, effective_at: expect.any(String)});
    expect(new Date((onCommand.mock.lastCall![0] as {effective_at: string}).effective_at).getTime()).toBeGreaterThan(new Date(writable.generated_at).getTime());
    rerender(<AMANSettingsHeader {...props} pendingCommands={{rate: {command_id: "rate", type: "aman.set_rate", expected_revision: 7, runway_group_id: "ARRIVAL-22R", arrivals_per_hour: 24, effective_at: writable.generated_at}}} />);
    fireEvent.click(screen.getByRole("button", {name: /^22L: 40.*PENDING$/, hidden: true}));
    expect(screen.getByRole("status")).toHaveTextContent("Pending rate: ARRIVAL-22R · 24/h");
  });

  it("sends runway rate changes one at a time as each revision is confirmed", () => {
    const writable = structuredClone(state);
    writable.authoritative = true;
    writable.effective_mode = "authoritative";
    writable.revision = 7;
    writable.technical_health = {status: "ready", ready: true, blocked_reasons: [], ...({navigation: {status: "ready", reason: null, updated_at: null, age_seconds: null}, weather: {status: "ready", reason: null, updated_at: null, age_seconds: null}, vatsim: {status: "ready", reason: null, updated_at: null, age_seconds: null}, repository: {status: "ready", reason: null, updated_at: null, age_seconds: null}, predictor: {status: "ready", reason: null, updated_at: null, age_seconds: null}, replay_validation: {status: "ready", reason: null, updated_at: null, age_seconds: null}})};
    writable.header!.active_runway_groups = [
      {id: "ARRIVAL-22L", active_rate_per_hour: 30, rate_effective_at: null},
      {id: "ARRIVAL-22R", active_rate_per_hour: 30, rate_effective_at: null},
    ];
    const onCommand = vi.fn().mockReturnValueOnce("rate-1").mockReturnValueOnce("rate-2");
    const props = {commandRejections: {}, connectionState: "connected" as const, hasFMPAuthority: true, onCommand, onRunwayGroupViewChange: vi.fn(), onViewChange: vi.fn(), pendingCommands: {}, presentationStatus: "ready" as const, readOnly: false, selectedRunwayGroupID: "ARRIVAL-22L", state: writable, view: "holds" as const};
    const {rerender} = render(<AMANSettingsHeader {...props} />);

    fireEvent.click(screen.getByRole("button", {name: "22L: 3022R: 3012: 4030: 40"}));
    fireEvent.change(screen.getByLabelText("Rate 22L"), {target: {value: "24"}});
    fireEvent.change(screen.getByLabelText("Rate 22R"), {target: {value: "25"}});
    fireEvent.click(screen.getByRole("button", {name: "Set arrival rate"}));

    expect(onCommand).toHaveBeenCalledTimes(1);
    expect(onCommand).toHaveBeenNthCalledWith(1, {type: "aman.set_rate", runway_group_id: "ARRIVAL-22L", arrivals_per_hour: 24, effective_at: writable.generated_at});
    const firstPending = {rate: {command_id: "rate-1", type: "aman.set_rate", expected_revision: 7, runway_group_id: "ARRIVAL-22L", arrivals_per_hour: 24, effective_at: writable.generated_at}} satisfies Record<string, AMANPendingCommand>;
    rerender(<AMANSettingsHeader {...props} pendingCommands={firstPending} />);
    expect(onCommand).toHaveBeenCalledTimes(1);

    const confirmed = structuredClone(writable);
    confirmed.revision = 8;
    rerender(<AMANSettingsHeader {...props} pendingCommands={{}} state={confirmed} />);

    expect(onCommand).toHaveBeenCalledTimes(2);
    expect(onCommand).toHaveBeenNthCalledWith(2, {type: "aman.set_rate", runway_group_id: "ARRIVAL-22R", arrivals_per_hour: 25, effective_at: writable.generated_at});
  });

  it("blocks empty, rejected, disconnected, and unauthorized runway mutations", () => {
    const writable = structuredClone(state);
    writable.authoritative = true;
    writable.effective_mode = "authoritative";
    writable.technical_health = {status: "ready", ready: true, blocked_reasons: [], ...({navigation: {status: "ready", reason: null, updated_at: null, age_seconds: null}, weather: {status: "ready", reason: null, updated_at: null, age_seconds: null}, vatsim: {status: "ready", reason: null, updated_at: null, age_seconds: null}, repository: {status: "ready", reason: null, updated_at: null, age_seconds: null}, predictor: {status: "ready", reason: null, updated_at: null, age_seconds: null}, replay_validation: {status: "ready", reason: null, updated_at: null, age_seconds: null}})};
    writable.runway_groups = [{id: "ARRIVAL-22", selected: true, selection_schedule: []}];
    const common = {connectionState: "connected" as const, hasFMPAuthority: true, onCommand: vi.fn(), onRunwayGroupViewChange: vi.fn(), onViewChange: vi.fn(), presentationStatus: "ready" as const, readOnly: false, selectedRunwayGroupID: "ARRIVAL-22", state: writable, view: "holds" as const};
    const {rerender} = render(<AMANSettingsHeader {...common} commandRejections={{stale: {command_id: "stale", command_type: "aman.set_active_runway_groups", code: "revision_conflict", message: "revision changed", current_revision: 8, retryable: true}}} pendingCommands={{}} />);

    fireEvent.click(screen.getByRole("button", {name: "22"}));
    expect(screen.getByRole("alert")).toHaveTextContent("revision_conflict");
    fireEvent.click(screen.getByRole("checkbox", {name: /ARRIVAL-22/}));
    expect(screen.getByText("Select at least one runway.")).toHaveAttribute("role", "alert");
    expect(screen.getByRole("button", {name: "Replace active set"})).toBeDisabled();

    rerender(<AMANSettingsHeader {...common} commandRejections={{}} connectionState="disconnected" hasFMPAuthority={false} pendingCommands={{}} readOnly={true} />);
    expect(screen.getByRole("status")).toHaveTextContent("Read-only");
    expect(screen.getByRole("button", {name: "Replace active set"})).toBeDisabled();
  });
});
