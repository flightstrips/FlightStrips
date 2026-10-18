import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Bay, type FrontendStrip } from "@/api/models";
import { PlannedDialog } from "./PlannedDialog";

const state = vi.hoisted(() => ({
  strips: [] as FrontendStrip[],
  move: vi.fn(),
}));

vi.mock("@/store/store-hooks", () => ({
  useAirport: () => "EKCH",
  useMyPosition: () => "118.105",
  useStrips: () => state.strips,
  useWebSocketStore: (selector: (value: typeof state) => unknown) => selector(state),
}));

vi.mock("@/components/StripListPopup.tsx", () => ({
  StripListPopup: ({
    strips,
    allowClearanceFromPlannedDepartures,
    onRowClick,
  }: {
    strips: FrontendStrip[];
    allowClearanceFromPlannedDepartures?: boolean;
    onRowClick: (strip: FrontendStrip) => void;
  }) => (
    <div data-testid="planned-popup" data-allow-clearance={allowClearanceFromPlannedDepartures}>
      {strips.map((strip) => (
        <button key={strip.callsign} onClick={() => onRowClick(strip)}>{strip.callsign}</button>
      ))}
    </div>
  ),
}));

const flight = (callsign: string, origin: string, bay: Bay): FrontendStrip =>
  ({ callsign, origin, destination: "EKRK", bay } as FrontendStrip);

describe("PlannedDialog", () => {
  beforeEach(() => {
    state.strips = [
      flight("LOCALDEP", "EKCH", Bay.Stand),
      flight("OTHERDEP", "EKRK", Bay.Stand),
    ];
    state.move.mockClear();
  });

  it("enables stand-field clearance for all listed planned departures", () => {
    render(<PlannedDialog open onOpenChange={vi.fn()} />);

    expect(screen.getByTestId("planned-popup")).toHaveAttribute("data-allow-clearance", "true");
    expect(screen.getByRole("button", { name: "LOCALDEP" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "OTHERDEP" })).not.toBeInTheDocument();
  });

  it("keeps the existing row-click move to CLR/DEL", () => {
    const onOpenChange = vi.fn();
    render(<PlannedDialog open onOpenChange={onOpenChange} />);

    fireEvent.click(screen.getByRole("button", { name: "LOCALDEP" }));

    expect(state.move).toHaveBeenCalledWith("LOCALDEP", Bay.NotCleared);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
