import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { Bay } from "@/api/models";
import { ApnPushStrip } from "./ApnPushStrip";

const state = vi.hoisted(() => ({
  displayedLayout: "AAAD",
  strips: [],
  localMarked: {} as Record<string, boolean>,
  transferStrip: vi.fn(),
}));

vi.mock("@/store/store-hooks", () => ({
  useControllers: () => [],
  useSelectedCallsign: () => null,
  useSelectStrip: () => vi.fn(),
  useTagRequestArmed: () => false,
  useMarkArmed: () => false,
  useStripTransfers: () => ({}),
  useRunwaySetup: () => ({ runway_status: {} }),
  useIsTwr: () => false,
  useWebSocketStore: (selector: (store: typeof state) => unknown) => selector(state),
}));

vi.mock("@/components/map-dialogs/PushbackMapDialog", () => ({ PushbackMapDialog: () => null }));
vi.mock("@/components/map-dialogs/ApronTaxiMapDialog", () => ({ ApronTaxiMapDialog: () => null }));
vi.mock("@/components/map-dialogs/TaxiMapDialog", () => ({ TaxiMapDialog: () => null }));
vi.mock("./RunwayDialog", () => ({ RunwayDialog: () => null }));
vi.mock("./DepartureAwareFlightPlanDialog", () => ({ DepartureAwareFlightPlanDialog: () => null }));

describe("apron startup and pushback frequency display", () => {
  it.each([
    ["AAAD", Bay.Cleared, false],
    ["AAAD", Bay.Push, false],
    ["AD", Bay.Cleared, false],
    ["AD", Bay.Push, false],
    ["AA", Bay.Cleared, true],
    ["AA", Bay.Push, true],
    ["GEGW", Bay.Push, true],
    ["AAAD", Bay.DeIce, true],
    ["AD", Bay.DeIce, true],
  ] as const)("shows next frequency in %s / %s: %s", (layout, bay, visible) => {
    state.displayedLayout = layout;
    state.transferStrip.mockClear();

    render(
      <ApnPushStrip
        callsign="SAS123"
        bay={bay}
        owner="121.730"
        myPosition="121.730"
        nextControllers={["118.580"]}
        nextDisplay={{ label: "GW", frequency: "118.580" }}
      />,
    );

    expect(screen.queryByText(":118.580") !== null).toBe(visible);
    const si = screen.getByText("GW");
    fireEvent.click(si);
    expect(state.transferStrip).toHaveBeenCalledExactlyOnceWith("SAS123", "118.580");
  });
});
