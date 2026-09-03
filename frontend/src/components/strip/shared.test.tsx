import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { MouseEvent as ReactMouseEvent } from "react";
import { getStripFrameColor, useNextFrequencyDisplay, useStripCallsignInteraction } from "./shared";

const storeState = vi.hoisted(() => ({
  strips: [] as Array<Record<string, unknown>>,
  openStripContextMenu: vi.fn(),
  requestTag: vi.fn(),
  toggleMarked: vi.fn(),
}));

vi.mock("@/store/store-hooks", () => ({
  useControllers: () => [
    {
      callsign: "EKCH_A_TWR",
      position: "118.105",
      identifier: "TE",
      section: "TWR",
      owned_sectors: ["TE"],
    },
    {
      callsign: "EKCH_B_GND",
      position: "121.905",
      identifier: "SQ",
      section: "GND",
      owned_sectors: ["AA", "SQ"],
    },
  ],
  useSelectedCallsign: () => null,
  useSelectStrip: () => vi.fn(),
  useTagRequestArmed: () => false,
  useMarkArmed: () => false,
  useStripTransfers: () => ({}),
  useWebSocketStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));

describe("useNextFrequencyDisplay", () => {
  it("prefers the backend-resolved cross-coupled frequency", () => {
    const { result } = renderHook(() =>
      useNextFrequencyDisplay(
        { label: "AA", frequency: "121.630" },
        ["118.105", "121.905"],
        "118.105",
      ),
    );

    expect(result.current).toBe(":121.630");
  });

  it("falls back to the next controller's primary frequency", () => {
    const { result } = renderHook(() =>
      useNextFrequencyDisplay(undefined, ["118.105", "121.905"], "118.105"),
    );

    expect(result.current).toBe(":121.905");
  });
});

describe("getStripFrameColor", () => {
  it("keeps departure and arrival frames tied to the flight direction", () => {
    expect(getStripFrameColor(false)).toBe("var(--color-strip-frame)");
    expect(getStripFrameColor(true)).toBe("var(--color-cell-border-arr)");
  });
});

describe("useStripCallsignInteraction", () => {
  it("opens a non-blocking validation advisory without blocking other guarded actions", () => {
    storeState.strips = [{
      callsign: "SAS123",
      marked: false,
      validation_status: {
        issue_type: "STAND ASSIGNMENT",
        message: "Reserved by an inbound aircraft.",
        owning_position: "118.105",
        active: true,
        activation_key: "key",
      },
    }];

    const { result } = renderHook(() => useStripCallsignInteraction({
      callsign: "SAS123",
      owner: "118.105",
      myPosition: "118.105",
    }));
    const stopPropagation = vi.fn();

    act(() => result.current.handleClick({ stopPropagation } as unknown as ReactMouseEvent<HTMLElement>));

    expect(stopPropagation).toHaveBeenCalledOnce();
    expect(result.current.validationDialogOpen).toBe(true);
    expect(result.current.isValidationActive).toBe(false);

    const action = vi.fn();
    act(() => result.current.guardValidationAction({ stopPropagation: vi.fn() } as unknown as ReactMouseEvent<HTMLElement>, action));
    expect(action).toHaveBeenCalledOnce();
  });
});
