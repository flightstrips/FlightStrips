import {fireEvent, render, screen} from "@testing-library/react";
import {describe, expect, it, vi} from "vitest";

import type {AMANFlight} from "@/api/aman";
import {AMANQuickGapDialog} from "./AMANQuickGapDialog";

const flight = {
  callsign: "SAS123",
  runway_group_id: "ARRIVAL-22",
  slot: {time: "2026-07-22T12:10:00.000Z"},
} as AMANFlight;

describe("quick GAP dialog", () => {
  it("starts after the selected slot without reserving the aircraft's own opportunity", () => {
    const onCommand = vi.fn();
    render(<AMANQuickGapDialog busy={false} disabled={false} flight={flight} groups={[{id: "ARRIVAL-22"}]} onCommand={onCommand} onOpenChange={vi.fn()} open />);
    fireEvent.click(screen.getByRole("button", {name: "OK"}));
    const intent = onCommand.mock.calls[0][0];
    expect(Date.parse(intent.start)).toBeGreaterThan(Date.parse(flight.slot!.time));
    expect(Date.parse(intent.end) - Date.parse(intent.start)).toBe(5 * 60_000);
  });

  it("preserves the operator's input when a live AMAN replacement arrives", () => {
    const onCommand = vi.fn();
    const props = {busy: false, disabled: false, flight, groups: [{id: "ARRIVAL-22"}, {id: "ARRIVAL-04"}], onCommand, onOpenChange: vi.fn(), open: true};
    const {rerender} = render(<AMANQuickGapDialog {...props} />);
    fireEvent.click(screen.getByRole("button", {name: "before"}));
    fireEvent.change(screen.getByLabelText("GAP minutes"), {target: {value: "7"}});
    fireEvent.click(screen.getByRole("button", {name: "04"}));
    rerender(<AMANQuickGapDialog {...props} flight={{...flight, slot: {...flight.slot!, time: "2026-07-22T12:12:00.000Z"}}} groups={props.groups.map(group => ({...group}))} />);
    expect(screen.getByLabelText("GAP minutes")).toHaveValue(7);
    expect(screen.getByRole("button", {name: "before"})).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", {name: "04"})).toHaveAttribute("aria-pressed", "true");
    fireEvent.click(screen.getByRole("button", {name: "OK"}));
    expect(onCommand).toHaveBeenCalledWith(expect.objectContaining({runway_group_id: "ARRIVAL-04", start: "2026-07-22T12:05:00.000Z", end: "2026-07-22T12:12:00.000Z"}));
  });

  it("creates a design-style minute interval before or after the selected tag", () => {
    const onCommand = vi.fn();
    render(<AMANQuickGapDialog
      busy={false}
      disabled={false}
      flight={flight}
      groups={[{id: "ARRIVAL-22"}, {id: "ARRIVAL-04"}]}
      onCommand={onCommand}
      onOpenChange={vi.fn()}
      open
    />);

    expect(screen.getByRole("button", {name: "after"})).toHaveAttribute("aria-pressed", "true");
    fireEvent.click(screen.getByRole("button", {name: "before"}));
    fireEvent.change(screen.getByLabelText("GAP minutes"), {target: {value: "7"}});
    fireEvent.click(screen.getByRole("button", {name: "04"}));
    fireEvent.click(screen.getByRole("button", {name: "OK"}));

    expect(onCommand).toHaveBeenCalledWith({
      type: "aman.create_gap",
      runway_group_id: "ARRIVAL-04",
      start: "2026-07-22T12:03:00.000Z",
      end: "2026-07-22T12:10:00.000Z",
      label: "7 min before SAS123",
    });
  });
});
