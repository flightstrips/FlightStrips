import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Bay, type FrontendStrip } from "@/api/models";
import { FindDialog } from "./FindDialog";

const state = vi.hoisted(() => ({
  strips: [] as FrontendStrip[],
  moveToControlzone: vi.fn(),
}));

vi.mock("@/store/store-hooks", () => ({
  useStrips: () => state.strips,
  useMyPosition: () => "118.105",
  useWebSocketStore: (selector: (value: typeof state) => unknown) => selector(state),
}));

vi.mock("@/components/StripListPopup", () => ({
  StripListPopup: ({ strips, onRowClick, onDismiss }: {
    strips: FrontendStrip[];
    onRowClick: (strip: FrontendStrip) => void;
    onDismiss: () => void;
  }) => (
    <div>
      {strips.map((strip) => <button key={strip.callsign} onClick={() => onRowClick(strip)}>{strip.callsign}</button>)}
      <button onClick={onDismiss}>DISMISS</button>
    </div>
  ),
}));

const flight = (callsign: string, origin: string, destination: string, flight_rules: string, bay: Bay = Bay.NotCleared) =>
  ({ callsign, origin, destination, flight_rules, bay, has_fp: true } as FrontendStrip);

describe("CONTROLZONE FIND", () => {
  beforeEach(() => {
    state.strips = [
      flight("VFRDEP", "EKCH", "EKRK", "V"),
      flight("VFRARR", "EKRK", "EKCH", "V"),
      flight("VFRTRANSIT", "EKRK", "EKOD", "V"),
      flight("IFRDEP", "EKCH", "EKRK", "I"),
      flight("UNKNOWN", "EKCH", "EKRK", ""),
      { ...flight("NOFPL", "EKCH", "EKRK", "V"), has_fp: false },
      flight("INZONE", "EKRK", "EKOD", "V", Bay.Controlzone),
    ];
    state.moveToControlzone.mockClear();
  });

  it("lists filed VFR arrivals, departures and transits only", () => {
    render(<FindDialog open onOpenChange={vi.fn()} />);
    expect(screen.getAllByRole("button").map((button) => button.textContent)).toEqual([
      "VFRDEP", "VFRARR", "VFRTRANSIT", "INZONE", "DISMISS",
    ]);
  });

  it("moves the selected flight into CONTROLZONE and closes", () => {
    const onOpenChange = vi.fn();
    render(<FindDialog open onOpenChange={onOpenChange} />);
    fireEvent.click(screen.getByText("VFRTRANSIT"));
    expect(state.moveToControlzone).toHaveBeenCalledWith("VFRTRANSIT");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("does not move a flight already in CONTROLZONE", () => {
    render(<FindDialog open onOpenChange={vi.fn()} />);
    fireEvent.click(screen.getByText("INZONE"));
    expect(state.moveToControlzone).not.toHaveBeenCalled();
  });

  it("dismisses without moving a flight", () => {
    const onOpenChange = vi.fn();
    render(<FindDialog open onOpenChange={onOpenChange} />);
    fireEvent.click(screen.getByText("DISMISS"));
    expect(state.moveToControlzone).not.toHaveBeenCalled();
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
