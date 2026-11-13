import {fireEvent, render, screen} from "@testing-library/react";
import {describe, expect, it, vi} from "vitest";

import type {AMANFlight} from "@/api/aman";
import {AMANSwapDialog} from "./AMANSwapDialog";

const flight = (callsign: string, sequence: number, overrides: Partial<AMANFlight> = {}): AMANFlight => ({
  callsign, star_family: "TESPI", runway_group_id: "ARRIVAL-22", lifecycle_state: "stable",
  freeze_reason: "none", sequence_disposition: "active",
  slot: {sequence, time: "2026-07-22T12:10:00Z", runway_group_id: "ARRIVAL-22", revision: 1},
  ...overrides,
} as AMANFlight);

describe("AMAN sequence swap", () => {
  it("offers sequenced aircraft without manual or TMA freezes on the same STAR and runway and sends one atomic command", () => {
    const selected = flight("SAS123", 1);
    const onCommand = vi.fn();
    render(<AMANSwapDialog flight={selected} flights={[
      selected, flight("SAS456", 3), flight("SAS234", 2),
      flight("OTHER-STAR", 4, {star_family: "TIDVU"}),
      flight("OTHER-RUNWAY", 4, {runway_group_id: "ARRIVAL-04"}),
      flight("LOCKED", 4, {freeze_reason: "manual"}),
      flight("TMA", 4, {freeze_reason: "tma"}),
      flight("REMOVED", 4, {lifecycle_state: "removed"}),
      flight("EXPEDITED", 4, {expedited: true}),
      flight("DESEQUENCED", 4, {sequence_disposition: "desequenced"}),
      flight("NO-SLOT", 4, {slot: null}),
    ]} disabled={false} busy={false} rejection={null} onClose={vi.fn()} onCommand={onCommand} />);
    expect(screen.getAllByRole("option").map(option => option.textContent)).toEqual([
      "Select aircraft", "SAS234 · #2 · 12:10 UTC", "SAS456 · #3 · 12:10 UTC",
    ]);
    expect(screen.getByRole("button", {name: "Swap aircraft"})).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Swap with"), {target: {value: "SAS456"}});
    fireEvent.click(screen.getByRole("button", {name: "Swap aircraft"}));
    expect(onCommand).toHaveBeenCalledExactlyOnceWith({type: "aman.swap_flights", callsign: "SAS123", other_callsign: "SAS456"});
    expect(selected.slot?.sequence).toBe(1);
  });

  it.each([
    {selectedFreeze: "superstable", otherFreeze: "none"},
    {selectedFreeze: "none", otherFreeze: "superstable"},
    {selectedFreeze: "superstable", otherFreeze: "superstable"},
  ] as const)("allows an explicit swap of automatic superstable slots (%j)", ({selectedFreeze, otherFreeze}) => {
    const selected = flight("CCA808", 1, {freeze_reason: selectedFreeze});
    const other = flight("SAS1778", 2, {freeze_reason: otherFreeze});
    const onCommand = vi.fn();
    const props = {flight: selected, flights: [selected, other], disabled: false, busy: false, rejection: null, onClose: vi.fn(), onCommand};
    const {rerender} = render(<AMANSwapDialog {...props} />);
    fireEvent.change(screen.getByLabelText("Swap with"), {target: {value: "SAS1778"}});
    expect(screen.getByRole("button", {name: "Swap aircraft"})).toBeEnabled();
    // A routine transition to Superstable must not invalidate the chosen pair.
    rerender(<AMANSwapDialog {...props} flights={[selected, {...other, freeze_reason: "superstable"}]} />);
    fireEvent.click(screen.getByRole("button", {name: "Swap aircraft"}));
    expect(onCommand).toHaveBeenCalledExactlyOnceWith({type: "aman.swap_flights", callsign: "CCA808", other_callsign: "SAS1778"});
    rerender(<AMANSwapDialog {...props} flights={[selected, {...other, freeze_reason: "manual"}]} />);
    expect(screen.getByRole("button", {name: "Swap aircraft"})).toBeDisabled();
  });

  it("revalidates the pair when authoritative updates exclude or freeze an aircraft", () => {
    const selected = flight("SAS123", 1);
    const onCommand = vi.fn();
    const props = {flight: selected, flights: [selected, flight("SAS456", 2)], disabled: false, busy: false, rejection: null, onClose: vi.fn(), onCommand};
    const {rerender} = render(<AMANSwapDialog {...props} />);
    fireEvent.change(screen.getByLabelText("Swap with"), {target: {value: "SAS456"}});
    rerender(<AMANSwapDialog {...props} flights={[selected]} />);
    expect(screen.getByRole("button", {name: "Swap aircraft"})).toBeDisabled();
    expect(screen.getByRole("status")).toHaveTextContent("Two sequenced");
    expect(onCommand).not.toHaveBeenCalled();
    rerender(<AMANSwapDialog {...props} flight={{...selected, freeze_reason: "tma"}} />);
    expect(screen.getByRole("button", {name: "Swap aircraft"})).toBeDisabled();
  });

  it.each([{disabled: true, busy: false}, {disabled: false, busy: true}])("blocks unavailable and pending swaps (%j)", (gate) => {
    const selected = flight("SAS123", 1);
    const onCommand = vi.fn();
    render(<AMANSwapDialog flight={selected} flights={[selected, flight("SAS456", 2)]} {...gate} rejection="Protected slot prevents this swap" onClose={vi.fn()} onCommand={onCommand} />);
    expect(screen.getByRole("combobox")).toBeDisabled();
    expect(screen.getByRole("button", {name: "Swap aircraft"})).toBeDisabled();
    expect(screen.getByRole("alert")).toHaveTextContent("Protected slot prevents this swap");
    expect(onCommand).not.toHaveBeenCalled();
  });
});
