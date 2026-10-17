import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { Bay } from "@/api/models";
import { ClxClearedStrip } from "./ClxClearedStrip";
import { COLOR_CLOSED_RWY, COLOR_UNCONCERNED_BG, SELECTION_COLOR } from "./shared";

const state = vi.hoisted(() => ({
  strips: [],
  runwayStatus: {} as Record<string, string>,
  transfers: {} as Record<string, { from: string; to: string; isTagRequest: boolean }>,
}));

vi.mock("@/components/clxbtn", () => ({
  CLXBtn: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@/store/store-hooks", () => ({
  useControllers: () => [],
  useSelectedCallsign: () => null,
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
});
