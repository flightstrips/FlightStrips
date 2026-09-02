import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useNextFrequencyDisplay } from "./shared";

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
