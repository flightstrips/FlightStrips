import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { MouseEvent as ReactMouseEvent } from "react";
import { canForceAssumeStrip, getStripFrameColor, getValidationBlockedCursor, useNextFrequencyDisplay, useStripCallsignInteraction } from "./shared";

const storeState = vi.hoisted(() => ({
  strips: [] as Array<Record<string, unknown>>,
  openStripContextMenu: vi.fn(),
  requestTag: vi.fn(),
  toggleMarked: vi.fn(),
  setLocalMarked: vi.fn(),
  setMarkArmed: vi.fn(),
  markArmed: false,
  localMarked: {} as Record<string, boolean>,
  selectStrip: vi.fn(),
  stripTransfers: {} as Record<string, { from: string; to: string; isTagRequest: boolean }>,
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
  useSelectStrip: () => storeState.selectStrip,
  useTagRequestArmed: () => false,
  useMarkArmed: () => storeState.markArmed,
  useStripTransfers: () => storeState.stripTransfers,
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

describe("getValidationBlockedCursor", () => {
  it("shows no cursor for actions blocked by validation", () => {
    expect(getValidationBlockedCursor(true)).toBeUndefined();
  });

  it("keeps the hand cursor for available actions", () => {
    expect(getValidationBlockedCursor(false)).toBe("pointer");
    expect(getValidationBlockedCursor(true, "pointer", true)).toBe("pointer");
  });
});

describe("canForceAssumeStrip", () => {
  it("allows force-assuming an unowned strip", () => {
    expect(canForceAssumeStrip({ owner: undefined, myPosition: "EKCH_A_TWR", isClrDel: false })).toBe(true);
  });

  it("still prevents clearance delivery and the current owner from force-assuming", () => {
    expect(canForceAssumeStrip({ owner: "EKCH_A_TWR", myPosition: "EKCH_A_TWR", isClrDel: false })).toBe(false);
    expect(canForceAssumeStrip({ owner: "EKCH_A_TWR", myPosition: "EKCH_D_TWR", isClrDel: true })).toBe(false);
  });
});

describe("useStripCallsignInteraction", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    storeState.markArmed = false;
    storeState.localMarked = {};
    storeState.stripTransfers = {};
    storeState.strips = [];
  });

  it("marks an unowned, non-selectable strip locally when MRK is armed", () => {
    storeState.markArmed = true;
    storeState.localMarked = {};
    storeState.strips = [{ callsign: "SAS123", marked: false }];
    const { result } = renderHook(() => useStripCallsignInteraction({
      callsign: "SAS123",
      selectable: false,
      owner: "121.905",
      myPosition: "118.105",
    }));

    act(() => result.current.handleClick({
      stopPropagation: vi.fn(),
    } as unknown as ReactMouseEvent<HTMLElement>));

    expect(storeState.setLocalMarked).toHaveBeenCalledWith("SAS123", true);
    expect(storeState.setMarkArmed).toHaveBeenCalledWith(false);
    expect(storeState.toggleMarked).not.toHaveBeenCalled();
  });

  it("opens the context menu instead of selecting a strip during an active handover", () => {
    storeState.stripTransfers = {
      SAS123: { from: "118.105", to: "121.905", isTagRequest: false },
    };
    const { result } = renderHook(() => useStripCallsignInteraction({
      callsign: "SAS123",
      selectable: true,
      bay: "CLEARED",
      owner: "118.105",
      myPosition: "118.105",
    }));
    const stopPropagation = vi.fn();

    act(() => result.current.handleClick({
      stopPropagation,
      clientX: 120,
      clientY: 80,
    } as unknown as ReactMouseEvent<HTMLElement>));

    expect(storeState.openStripContextMenu).toHaveBeenCalledWith("SAS123", { x: 120, y: 80 });
    expect(storeState.selectStrip).not.toHaveBeenCalled();
  });

  it.each([
    { serverMarked: false, localMarked: true, nextMarked: false },
    { serverMarked: true, localMarked: false, nextMarked: true },
  ])("toggles the local override during a handover with MRK armed: $localMarked", ({ serverMarked, localMarked, nextMarked }) => {
    storeState.markArmed = true;
    storeState.strips = [{ callsign: "SAS123", marked: serverMarked }];
    storeState.localMarked = { SAS123: localMarked };
    storeState.stripTransfers = {
      SAS123: { from: "118.105", to: "121.905", isTagRequest: false },
    };
    const { result } = renderHook(() => useStripCallsignInteraction({
      callsign: "SAS123",
      selectable: true,
      bay: "CLEARED",
      owner: "118.105",
      myPosition: "118.105",
    }));

    act(() => result.current.handleClick({
      stopPropagation: vi.fn(),
      clientX: 120,
      clientY: 80,
    } as unknown as ReactMouseEvent<HTMLElement>));

    expect(storeState.setLocalMarked).toHaveBeenCalledWith("SAS123", nextMarked);
    expect(storeState.setMarkArmed).toHaveBeenCalledWith(false);
    expect(storeState.openStripContextMenu).not.toHaveBeenCalled();
    expect(storeState.selectStrip).not.toHaveBeenCalled();
    expect(storeState.toggleMarked).not.toHaveBeenCalled();
  });

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
