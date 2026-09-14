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
