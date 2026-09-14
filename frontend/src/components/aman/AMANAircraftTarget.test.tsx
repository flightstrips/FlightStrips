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

  it("suppresses non-authoritative or stale guidance explicitly", () => {
    render(<AMANAircraftTarget flight={flight({data_status: "stale"})} guidance={guidance} />);
    expect(screen.getByText("Unavailable")).toBeInTheDocument();
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
    expect(target).toHaveTextContent(dataStatus === "fresh" ? "L02" : "Unavailable");
    expect(screen.getByText("SAS123")).toHaveClass("text-[#96d796]");
  });

  it("is keyboard-focusable, semantically selected, and invokes its selection callback", () => {
    const onSelect = vi.fn();
    render(<AMANAircraftTarget flight={flight()} guidance={guidance} onSelect={onSelect} selected />);
    const target = screen.getByRole("button");

    target.focus();
    expect(target).toHaveFocus();
    expect(target).toHaveAttribute("aria-pressed", "true");
    expect(target).toHaveClass("ring-inset", "ring-[#f3d02e]");
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
