import { act, fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { Bay } from "@/api/models";
import { ApnPushStrip } from "./ApnPushStrip";
import { COLOR_CLOSED_RWY, SELECTION_COLOR } from "./shared";

const state = vi.hoisted(() => ({
  displayedLayout: "AAAD",
  strips: [],
  localMarked: {} as Record<string, boolean>,
  transferStrip: vi.fn(),
  selectedCallsign: null as string | null,
  transfers: {} as Record<string, { from: string; to: string; isTagRequest: boolean }>,
  runwayStatus: {} as Record<string, string>,
}));

vi.mock("@/store/store-hooks", () => ({
  useControllers: () => [],
  useSelectedCallsign: () => state.selectedCallsign,
  useSelectStrip: () => vi.fn(),
  useTagRequestArmed: () => false,
  useMarkArmed: () => false,
  useStripTransfers: () => state.transfers,
  useRunwaySetup: () => ({ runway_status: state.runwayStatus }),
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

describe("REQ callsign coloring", () => {
  it("keeps both callsign rows on the normal background while the strip flashes", () => {
    vi.useFakeTimers();
    try {
      state.transfers = { SAS123: { from: "121.730", to: "118.580", isTagRequest: true } };
      state.runwayStatus = { "04L-22R": "CLOSED" };
      state.selectedCallsign = null;
      const { unmount } = render(<ApnPushStrip callsign="SAS123" runway="04L" />);
      const callsignBox = screen.getByText("SAS123").parentElement!.parentElement!;
      const strip = callsignBox.parentElement!;
      expect(strip.style.backgroundColor).toBe(SELECTION_COLOR);
      expect(callsignBox.style.backgroundColor).toBe(COLOR_CLOSED_RWY);
      expect(callsignBox.style.color).toBe("white");
      act(() => vi.advanceTimersByTime(1000));
      expect(strip.style.backgroundColor).toBe(COLOR_CLOSED_RWY);
      expect(callsignBox.style.backgroundColor).toBe(COLOR_CLOSED_RWY);
      act(() => vi.advanceTimersByTime(1000));
      expect(strip.style.backgroundColor).toBe(SELECTION_COLOR);
      expect(callsignBox.style.backgroundColor).toBe(COLOR_CLOSED_RWY);
      expect(callsignBox.style.color).toBe("white");
      unmount();
    } finally {
      state.transfers = {};
      state.runwayStatus = {};
      state.selectedCallsign = null;
      vi.useRealTimers();
    }
  });

  it("retains the independent callsign selection highlight", () => {
    state.transfers = { SAS123: { from: "121.730", to: "118.580", isTagRequest: true } };
    state.selectedCallsign = "SAS123";
    try {
      state.displayedLayout = "AA";
      render(<ApnPushStrip callsign="SAS123" selectable pdcStatus="CLEARED" bay={Bay.Cleared} owner="121.730" myPosition="121.730" nextControllers={["118.580"]} nextDisplay={{ label: "GW", frequency: "118.580" }} />);
      expect(screen.getByText("SAS123").parentElement!.style.backgroundColor).toBe(SELECTION_COLOR);
      expect(screen.getByText("SAS123").parentElement!.style.color).toBe("black");
      const column = screen.getByText("SAS123").parentElement!.parentElement!;
      expect(column.style.color).toBe("white");
      expect(screen.getByText(":118.580").parentElement!.style.color).toBe("");
      expect(column.style.backgroundColor).not.toBe(SELECTION_COLOR);
    } finally {
      state.transfers = {};
      state.selectedCallsign = null;
    }
  });
});
