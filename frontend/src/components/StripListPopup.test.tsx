import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { FrontendStrip } from "@/api/models";
import { StripListPopup } from "./StripListPopup";

const state = vi.hoisted(() => ({
  strips: [] as FrontendStrip[],
  selectStrip: vi.fn(),
}));

vi.mock("@/store/store-hooks", () => ({
  useWebSocketStore: (selector: (value: typeof state) => unknown) => selector(state),
}));

vi.mock("@/components/strip/Strip.tsx", () => ({
  Strip: ({ strip, clearanceFromPlannedBay }: { strip: FrontendStrip; clearanceFromPlannedBay?: boolean }) => (
    <span data-testid="strip-row" data-clearance-from-planned-bay={clearanceFromPlannedBay}>
      {strip.callsign}
    </span>
  ),
}));

const flight = { callsign: "TEST123" } as FrontendStrip;
const sortModes = [{ key: "callsign", label: "CALLSIGN", compareFn: () => 0 }];

describe("StripListPopup planned departure interaction", () => {
  beforeEach(() => {
    state.strips = [flight];
    state.selectStrip.mockClear();
    vi.stubGlobal(
      "ResizeObserver",
      class {
        observe() {}
        disconnect() {}
      },
    );
  });

  it("enables stand-cell interaction for planned departures", () => {
    render(
      <StripListPopup
        strips={state.strips}
        sortModes={sortModes}
        onRowClick={vi.fn()}
        onDismiss={vi.fn()}
        myPosition="118.105"
        allowClearanceFromPlannedDepartures
      />,
    );

    const strip = screen.getByTestId("strip-row");
    expect(strip).toHaveAttribute("data-clearance-from-planned-bay", "true");
    expect(strip.parentElement).toHaveStyle({ pointerEvents: "auto" });
  });

  it("keeps other popup rows non-interactive", () => {
    render(
      <StripListPopup
        strips={state.strips}
        sortModes={sortModes}
        onRowClick={vi.fn()}
        onDismiss={vi.fn()}
        myPosition="118.105"
      />,
    );

    expect(screen.getByTestId("strip-row").parentElement).toHaveStyle({ pointerEvents: "none" });
  });
});
