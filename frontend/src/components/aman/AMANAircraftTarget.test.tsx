import {act, fireEvent, render, screen} from "@testing-library/react";
import {afterEach, beforeEach, describe, expect, it, vi} from "vitest";

import type {AMANFlight} from "@/api/aman";
import {AMANAircraftTarget, type AMANAircraftTargetField} from "./AMANAircraftTarget";

function flight(overrides: Partial<AMANFlight> = {}): AMANFlight {
  return {
    callsign: "SAS123",
    data_status: "fresh",
    freeze_reason: "none",
    gain_loss_seconds: -120,
    lifecycle_state: "unstable",
    ...overrides,
  } as AMANFlight;
}

const guidance = {authoritative: true, connected: true};

describe("compact MAESTRO aircraft target", () => {
  beforeEach(() => {
    vi.stubGlobal("ResizeObserver", class {
      observe() {}
      unobserve() {}
      disconnect() {}
    });
  });
  afterEach(() => vi.unstubAllGlobals());

  it("marks a basic estimate beside the full callsign and exposes its reason on keyboard focus", async () => {
    const onSelect = vi.fn();
    render(<AMANAircraftTarget compact delayFirst flight={flight({callsign: "BAW81CB", basic_estimate: true, prediction_reason: "Provider unavailable"})} guidance={guidance} onSelect={onSelect} />);
    const target = screen.getByRole("button", {name: /Select BAW81CB.*Basic estimate: Provider unavailable/});
    expect(target).toHaveTextContent("L02BAW81CB≈");
    expect(screen.getByText("≈")).toHaveAttribute("aria-hidden", "true");
    expect(screen.queryByText("Basic estimate")).not.toBeInTheDocument();
    await act(async () => target.focus());
    expect(await screen.findByRole("tooltip")).toHaveTextContent("Basic estimate: Provider unavailable");
    fireEvent.click(target);
    expect(onSelect).toHaveBeenCalledOnce();
  });

  it("clears the estimate marker and explanation without losing keyboard focus when the provider recovers", async () => {
    const {rerender} = render(<AMANAircraftTarget flight={flight({basic_estimate: true})} guidance={guidance} />);
    const target = screen.getByRole("button");
    await act(async () => target.focus());
    expect(target).toHaveAccessibleName(/Basic estimate/);
    expect(screen.getByText("≈")).toBeInTheDocument();
    rerender(<AMANAircraftTarget flight={flight({basic_estimate: false, prediction_reason: "Other prediction information"})} guidance={guidance} />);
    expect(screen.queryByText("≈")).not.toBeInTheDocument();
    expect(screen.getByRole("button")).not.toHaveAccessibleName(/Basic estimate|Other prediction information/);
    expect(target).toHaveFocus();
  });

  it("keeps basic estimate quality separate from stale gain/lose guidance", () => {
    render(<AMANAircraftTarget flight={flight({data_status: "stale", basic_estimate: true, prediction_reason: "Provider unavailable"})} guidance={guidance} />);
    expect(screen.getByText("≈")).toBeInTheDocument();
    expect(screen.getByText("L02")).toHaveAttribute("title", "Last known gain/lose; awaiting current prediction");
    expect(screen.getByText("L02")).toHaveClass("text-[#a0a0a0]");
  });

  it("keeps estimated EAT and holding duration out of timeline aircraft labels", () => {
    render(<AMANAircraftTarget flight={flight({
      gain_loss_seconds: -1800,
      approach_release_time: "2026-10-04T14:20:00Z",
      expected_holding_seconds: 1800,
    })} guidance={guidance} />);
    expect(screen.queryByText(/EAT/)).not.toBeInTheDocument();
    expect(screen.queryByText(/HOLD/)).not.toBeInTheDocument();
    expect(screen.getByText("L30")).toBeInTheDocument();
  });

  it("presents desequenced disposition through colour and an accessible label", () => {
    render(<AMANAircraftTarget flight={flight({sequence_disposition: "desequenced"})} guidance={guidance} />);

    expect(screen.getByRole("button", {name: /Desequenced/})).toHaveTextContent("SAS123L02");
    expect(screen.getByText("SAS123")).toHaveClass("text-violet-300");
  });

  it("shows only callsign and current delay while conveying lifecycle by colour", () => {
    render(<AMANAircraftTarget flight={flight()} guidance={guidance} />);

    const target = screen.getByRole("button", {name: /Select SAS123; Unstable; current delay L02/});
    expect(target).toHaveTextContent("SAS123L02");
    expect(target).not.toHaveTextContent("TNO");
    expect(screen.getByText("SAS123")).toHaveClass("text-[#6e996e]");
  });

  it.each([
    [-30, "L01", "text-[#f0e129]"],
    [-240, "L04", "text-[#e65b5b]"],
    [0, "=00", "text-[#96d796]"],
    [60, "G01", "text-[#96d796]"],
  ] as const)("presents %i seconds as %s with the documented emphasis", (seconds, label, tone) => {
    render(<AMANAircraftTarget flight={flight({gain_loss_seconds: seconds})} guidance={guidance} />);
    expect(screen.getByText(label)).toHaveClass(tone);
  });

  it("greys stale guidance with an explanation", () => {
    render(<AMANAircraftTarget flight={flight({data_status: "stale"})} guidance={guidance} />);
    expect(screen.getByText("L02")).toHaveClass("text-[#a0a0a0]");
    expect(screen.getByText("L02")).toHaveAttribute("title", "Last known gain/lose; awaiting current prediction");
  });

  it.each(["stale", "disconnected"] as const)("greys retained losses with %s data", (dataStatus) => {
    render(<AMANAircraftTarget flight={flight({data_status: dataStatus, gain_loss_seconds: -1800})} guidance={guidance} />);
    expect(screen.getByText("L30")).toHaveClass("text-[#a0a0a0]");
    expect(screen.getByText("L30")).toHaveAttribute("title", "Last known gain/lose; awaiting current prediction");
  });

  it.each(["none", "tma", "superstable", "manual"] as const)("does not specially mark current guidance with %s retention", (freezeReason) => {
    render(<AMANAircraftTarget flight={flight({freeze_reason: freezeReason})} guidance={guidance} />);
    const delay = screen.getByText("L02");
    expect(delay).toHaveClass("text-[#f0e129]");
    expect(delay).not.toHaveAttribute("title");
    expect(delay).not.toHaveAttribute("data-retained-guidance");
  });

  it("greys guidance when the connection is lost even if its last status was fresh", () => {
    render(<AMANAircraftTarget flight={flight()} guidance={{...guidance, connected: false}} />);
    expect(screen.getByText("L02")).toHaveClass("text-[#a0a0a0]");
    expect(screen.getByText("L02")).toHaveAttribute("title", "Last known gain/lose; awaiting current prediction");
  });

  it("uses the authoritative freeze reason for visible protection cues", () => {
    const {rerender} = render(<AMANAircraftTarget flight={flight({freeze_reason: "superstable", lifecycle_state: "stable"})} guidance={guidance} />);
    expect(screen.getByRole("button", {name: /Superstable/})).toBeInTheDocument();
    expect(screen.getByText("SAS123")).toHaveClass("text-[#dcdcdc]");

    rerender(<AMANAircraftTarget flight={flight({freeze_reason: "manual", lifecycle_state: "stable"})} guidance={guidance} />);
    expect(screen.getByRole("button")).toHaveAccessibleName(/Stable, manual freeze/);
    expect(screen.getByText("SAS123")).toHaveClass("text-[#96d796]");
  });

  it.each(["fresh", "stale", "disconnected"] as const)("announces TMA protection with %s surveillance data", (dataStatus) => {
    render(<AMANAircraftTarget flight={flight({data_status: dataStatus, freeze_reason: "tma", lifecycle_state: "stable"})} guidance={guidance} />);
    const target = screen.getByRole("button", {name: /TMA entry protection/});
    expect(target).toHaveTextContent("L02");
    expect(screen.getByText("SAS123")).toHaveClass("text-[#96d796]");
  });

  it("is keyboard-focusable, semantically selected, and invokes its selection callback", () => {
    const onSelect = vi.fn();
    render(<AMANAircraftTarget flight={flight()} guidance={guidance} onSelect={onSelect} selected />);
    const target = screen.getByRole("button");

    target.focus();
    expect(target).toHaveFocus();
    expect(target).toHaveAttribute("aria-pressed", "true");
    expect(target).toHaveClass("border-2", "border-white", "bg-[#5174B8]");
    expect(target.className).not.toContain("hover:bg-[#a3d5e8]");
    fireEvent.click(target);
    expect(onSelect).toHaveBeenCalledOnce();
  });

  it("keeps future optional fields caller-owned and ordered around the default core", () => {
    const leadingFields: AMANAircraftTargetField[] = [{id: "feeder-fix-eta", label: "Feeder-fix ETA", value: "10:12"}];
    const trailingFields: AMANAircraftTargetField[] = [{id: "runway", label: "Runway", value: "22L"}];
    render(<AMANAircraftTarget flight={flight()} guidance={guidance} leadingFields={leadingFields} trailingFields={trailingFields} />);

    expect(screen.getByRole("button")).toHaveTextContent("10:12SAS123L0222L");
  });

  it("vertically centers every added field with the callsign and delay", () => {
    const leadingFields: AMANAircraftTargetField[] = [{id: "wtc", label: "WTC", value: "—"}];
    const trailingFields: AMANAircraftTargetField[] = [
      {id: "feeder-fix", label: "Feeder fix", value: "KOR"},
      {id: "aircraft-type", label: "Aircraft type", value: "—"},
    ];
    const {container} = render(<AMANAircraftTarget flight={flight({callsign: "BAW81CB", gain_loss_seconds: 0})} guidance={guidance} leadingFields={leadingFields} trailingFields={trailingFields} />);

    expect(screen.getByRole("button")).toHaveTextContent("—BAW81CB=00KOR—");
    for (const field of container.querySelectorAll("[data-field]")) {
      expect(field).toHaveClass("flex", "items-center", "truncate");
    }
  });
});
