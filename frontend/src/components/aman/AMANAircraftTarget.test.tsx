import {fireEvent, render, screen} from "@testing-library/react";
import {describe, expect, it, vi} from "vitest";

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
  it("shows only callsign, current delay, and a non-color lifecycle cue by default", () => {
    render(<AMANAircraftTarget flight={flight()} guidance={guidance} />);

    const target = screen.getByRole("button", {name: /Select SAS123; Unstable; current delay L02/});
    expect(target).toHaveTextContent("SAS123L02U");
    expect(target).not.toHaveTextContent("TNO");
    expect(screen.getByTitle("Unstable")).toHaveTextContent("U");
  });

  it.each([
    [-30, "L01", "bg-[#f0e129]"],
    [-240, "L04", "bg-[#9c0000]"],
    [0, "=00", "bg-[#96d796]"],
    [60, "G01", "bg-[#96d796]"],
  ] as const)("presents %i seconds as %s with the documented emphasis", (seconds, label, tone) => {
    render(<AMANAircraftTarget flight={flight({gain_loss_seconds: seconds})} guidance={guidance} />);
    expect(screen.getByText(label)).toHaveClass(tone);
  });

  it("suppresses non-authoritative or stale guidance explicitly", () => {
    render(<AMANAircraftTarget flight={flight({data_status: "stale"})} guidance={guidance} />);
    expect(screen.getByText("Unavailable")).toBeInTheDocument();
  });

  it("uses the authoritative freeze reason for Superstable and manual protection cues", () => {
    const {rerender} = render(<AMANAircraftTarget flight={flight({freeze_reason: "superstable", lifecycle_state: "stable"})} guidance={guidance} />);
    expect(screen.getByTitle("Superstable")).toHaveTextContent("SS");

    rerender(<AMANAircraftTarget flight={flight({freeze_reason: "manual", lifecycle_state: "stable"})} guidance={guidance} />);
    expect(screen.getByRole("button")).toHaveAccessibleName(/Stable, manual freeze/);
    expect(screen.getByTitle("Stable, manual freeze")).toHaveTextContent("S·M");
  });

  it("is keyboard-focusable, semantically selected, and invokes its selection callback", () => {
    const onSelect = vi.fn();
    render(<AMANAircraftTarget flight={flight()} guidance={guidance} onSelect={onSelect} selected />);
    const target = screen.getByRole("button");

    target.focus();
    expect(target).toHaveFocus();
    expect(target).toHaveAttribute("aria-pressed", "true");
    fireEvent.click(target);
    expect(onSelect).toHaveBeenCalledOnce();
  });

  it("keeps future optional fields caller-owned and ordered around the default core", () => {
    const leadingFields: AMANAircraftTargetField[] = [{id: "feeder-fix-eta", label: "Feeder-fix ETA", value: "10:12"}];
    const trailingFields: AMANAircraftTargetField[] = [{id: "runway", label: "Runway", value: "22L"}];
    render(<AMANAircraftTarget flight={flight()} guidance={guidance} leadingFields={leadingFields} trailingFields={trailingFields} />);

    expect(screen.getByRole("button")).toHaveTextContent("10:12SAS123L0222LU");
  });
});
