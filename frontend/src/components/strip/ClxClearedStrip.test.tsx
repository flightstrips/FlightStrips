import { act, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { Bay, type ValidationStatus } from "@/api/models";
import { ClxClearedStrip } from "./ClxClearedStrip";
import { COLOR_CLOSED_RWY, COLOR_UNCONCERNED_BG, SELECTION_COLOR } from "./shared";

const state = vi.hoisted(() => ({
  strips: [] as { callsign: string; validation_status?: ValidationStatus }[],
  localMarked: {} as Record<string, boolean>,
  runwayStatus: {} as Record<string, string>,
  selectedCallsign: null as string | null,
  transfers: {} as Record<string, { from: string; to: string; isTagRequest: boolean }>,
}));

vi.mock("@/components/clxbtn", () => ({
  CLXBtn: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));
vi.mock("./ValidationStatusDialog", () => ({ ValidationStatusDialog: () => null }));

vi.mock("@/store/store-hooks", () => ({
  useControllers: () => [],
  useSelectedCallsign: () => state.selectedCallsign,
  useSelectStrip: () => vi.fn(),
  useTagRequestArmed: () => false,
  useMarkArmed: () => false,
  useStripTransfers: () => state.transfers,
  useRunwaySetup: () => ({ runway_status: state.runwayStatus }),
  useWebSocketStore: (selector: (store: typeof state) => unknown) => selector(state),
}));

describe("CLR DEL cleared-strip background", () => {
  it.each([
    { ignoreOwnershipBackground: true, closed: false, tagRequest: false, expected: "var(--color-strip-dep-bg)" },
    { ignoreOwnershipBackground: false, closed: false, tagRequest: false, expected: COLOR_UNCONCERNED_BG },
    { ignoreOwnershipBackground: true, closed: true, tagRequest: false, expected: COLOR_CLOSED_RWY },
    { ignoreOwnershipBackground: true, closed: true, tagRequest: true, expected: SELECTION_COLOR },
  ])("uses $expected with ownership override=$ignoreOwnershipBackground, closed=$closed, tag=$tagRequest", ({
    ignoreOwnershipBackground, closed, tagRequest, expected,
  }) => {
    state.runwayStatus = closed ? { "04L-22R": "CLOSED" } : {};
    state.transfers = tagRequest
      ? { SAS123: { from: "121.905", to: "119.905", isTagRequest: true } }
      : {};

    render(
      <ClxClearedStrip
        callsign="SAS123"
        bay={Bay.Cleared}
        pdcStatus="CLEARED"
        runway="04L"
        owner="121.905"
        myPosition="119.905"
        nextControllers={["121.730"]}
        previousControllers={[]}
        ignoreOwnershipBackground={ignoreOwnershipBackground}
      />,
    );

    expect(screen.getByRole("button", { name: "SAS123" }).parentElement?.style.backgroundColor).toBe(expected);
  });

  it("flashes an active tag request over the regular runway color", () => {
    vi.useFakeTimers();
    try {
      state.runwayStatus = { "04L-22R": "CLOSED" };
      state.transfers = { SAS123: { from: "121.905", to: "119.905", isTagRequest: true } };
      const { unmount } = render(
        <ClxClearedStrip callsign="SAS123" bay={Bay.Cleared} runway="04L" />,
      );
      const callsign = screen.getByRole("button", { name: "SAS123" });
      const strip = callsign.parentElement!;
      expect(strip.style.backgroundColor).toBe(SELECTION_COLOR);
      expect(callsign.style.backgroundColor).toBe(COLOR_CLOSED_RWY);
      expect(callsign.style.color).toBe("white");
      act(() => vi.advanceTimersByTime(1000));
      expect(strip.style.backgroundColor).toBe(COLOR_CLOSED_RWY);
      expect(callsign.style.backgroundColor).toBe(COLOR_CLOSED_RWY);
      expect(callsign.style.color).toBe("white");
      expect(strip.className).toContain("text-white");
      act(() => vi.advanceTimersByTime(1000));
      expect(strip.style.backgroundColor).toBe(SELECTION_COLOR);
      expect(callsign.style.backgroundColor).toBe(COLOR_CLOSED_RWY);
      unmount();
    } finally {
      vi.useRealTimers();
    }
  });

  it("uses black text for a selected tag request on a closed runway", () => {
    state.runwayStatus = { "04L-22R": "CLOSED" };
    state.transfers = { SAS123: { from: "121.905", to: "119.905", isTagRequest: true } };
    state.selectedCallsign = "SAS123";
    try {
      render(<ClxClearedStrip callsign="SAS123" bay={Bay.Cleared} runway="04L" selectable />);
      const callsign = screen.getByRole("button", { name: "SAS123" });
      expect(callsign.style.backgroundColor).toBe(SELECTION_COLOR);
      expect(callsign.style.color).toBe("black");
    } finally {
      state.runwayStatus = {};
      state.transfers = {};
      state.selectedCallsign = null;
    }
  });

  it("preserves the PDC callsign highlight over the REQ flash", () => {
    state.runwayStatus = {};
    state.transfers = { SAS123: { from: "121.905", to: "119.905", isTagRequest: true } };
    render(<ClxClearedStrip callsign="SAS123" bay={Bay.Cleared} pdcStatus="CLEARED" />);
    const callsign = screen.getByRole("button", { name: "SAS123" });
    expect(callsign.style.backgroundColor).toBe("var(--color-pdc-cleared)");
    expect(callsign.style.color).toBe("white");
  });

  it("keeps validation's non-grey phase on the callsign color rather than REQ pink", () => {
    state.runwayStatus = {};
    state.transfers = { SAS123: { from: "121.905", to: "119.905", isTagRequest: true } };
    state.strips = [{ callsign: "SAS123", validation_status: {
      issue_type: "STAND ASSIGNMENT", message: "Advisory", owning_position: "121.905", active: true, activation_key: "key",
    } }];
    try {
      render(<ClxClearedStrip callsign="SAS123" bay={Bay.Cleared} owner="121.905" myPosition="121.905" />);
      const callsign = screen.getByRole("button", { name: "SAS123" });
      expect(callsign.style.animation).toContain("validation-blink");
      expect(callsign.style.getPropertyValue("--validation-blink-background")).toBe("var(--color-strip-dep-bg)");
    } finally {
      state.strips = [];
    }
  });
});
