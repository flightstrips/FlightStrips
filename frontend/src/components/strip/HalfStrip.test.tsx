import { act, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { HalfStrip } from "./HalfStrip";
import { getHalfStripFrameColor } from "./halfStripFrame";
import { COLOR_DEP_STRIP_BG, SELECTION_COLOR } from "./shared";

vi.mock("@/store/store-hooks", () => ({
  useControllers: () => [],
  useSelectedCallsign: () => null,
  useSelectStrip: () => vi.fn(),
  useTagRequestArmed: () => false,
  useMarkArmed: () => false,
  useStripTransfers: () => ({ SAS123: { from: "A", to: "B", isTagRequest: true } }),
  useRunwaySetup: () => ({ runway_status: {} }),
  useWebSocketStore: (selector: (store: object) => unknown) => selector({}),
}));
vi.mock("./DepartureAwareFlightPlanDialog", () => ({ DepartureAwareFlightPlanDialog: () => null }));
vi.mock("@/components/FlightPlanDialog", () => ({ default: () => null }));

describe("compact special-strip frames", () => {
  it("uses the issue-defined crossing and land/start colors instead of cyan", () => {
    expect(getHalfStripFrameColor("CROSSING", false)).toBe("#FCC800");
    expect(getHalfStripFrameColor("LAND-START", false)).toBe("#DD6A12");
  });

  it("frames compact messages with their own strip color", () => {
    expect(getHalfStripFrameColor("MESSAGES", false)).toBe("#285A5C");
  });

  it("keeps ordinary compact strips tied to flight direction", () => {
    expect(getHalfStripFrameColor("APN-PUSH", false)).toBe("var(--color-strip-frame)");
    expect(getHalfStripFrameColor("APN-ARR", true)).toBe("var(--color-cell-border-arr)");
  });

  it("flashes the request background instead of keeping the compact strip pink", () => {
    vi.useFakeTimers();
    try {
      const { unmount } = render(<HalfStrip callsign="SAS123" />);
      const callsign = screen.getByText("SAS123");
      const strip = callsign.parentElement!;
      expect(strip.style.backgroundImage).toBe(`linear-gradient(${SELECTION_COLOR}, ${SELECTION_COLOR})`);
      expect(callsign.style.backgroundColor).toBe(COLOR_DEP_STRIP_BG);
      expect(callsign.style.color).toBe("black");
      act(() => vi.advanceTimersByTime(1000));
      expect(strip.style.backgroundImage).toBe(`linear-gradient(${COLOR_DEP_STRIP_BG}, ${COLOR_DEP_STRIP_BG})`);
      expect(callsign.style.backgroundColor).toBe(COLOR_DEP_STRIP_BG);
      act(() => vi.advanceTimersByTime(1000));
      expect(strip.style.backgroundImage).toBe(`linear-gradient(${SELECTION_COLOR}, ${SELECTION_COLOR})`);
      expect(callsign.style.backgroundColor).toBe(COLOR_DEP_STRIP_BG);
      unmount();
    } finally {
      vi.useRealTimers();
    }
  });
});
