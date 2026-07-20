import { describe, expect, it } from "vitest";

import { Bay } from "@/api/models";
import {
  CDM_GREEN,
  CDM_YELLOW,
  TWY_DEP_CTOT_BLUE,
  TWY_DEP_CTOT_ORANGE,
  TWY_DEP_CTOT_RED,
  TWY_DEP_CTOT_YELLOW,
  computeCDMColors,
  computeTwyDepCTOTColors,
  hasManualTobtSource,
  isTsatWithinStartRequestWindow,
} from "./cdmColors";

describe("hasManualTobtSource", () => {
  it("returns true for pilot-filed TOBT requests", () => {
    expect(hasManualTobtSource("PILOT", "")).toBe(true);
  });

  it("returns true when a controller has set the TOBT", () => {
    expect(hasManualTobtSource("", "EKCH_DEL")).toBe(true);
  });

  it("returns false for auto-synced TOBT values", () => {
    expect(hasManualTobtSource("", "")).toBe(false);
  });
});

describe("isTsatWithinStartRequestWindow", () => {
  it("accepts TSAT values less than six minutes from now", () => {
    const now = Date.UTC(2026, 6, 14, 10, 0, 0);

    expect(isTsatWithinStartRequestWindow("1005", now)).toBe(true);
    expect(isTsatWithinStartRequestWindow("0955", now)).toBe(true);
  });

  it("rejects TSAT values outside the window", () => {
    const now = Date.UTC(2026, 6, 14, 10, 0, 0);

    expect(isTsatWithinStartRequestWindow("1006", now)).toBe(false);
    expect(isTsatWithinStartRequestWindow("0954", now)).toBe(false);
    expect(isTsatWithinStartRequestWindow("", now)).toBe(false);
  });

  it("handles the TSAT window across midnight", () => {
    expect(isTsatWithinStartRequestWindow("0002", Date.UTC(2026, 6, 14, 23, 58, 0))).toBe(true);
    expect(isTsatWithinStartRequestWindow("2358", Date.UTC(2026, 6, 15, 0, 2, 0))).toBe(true);
  });
});

describe("computeCDMColors", () => {
  it("uses the design green token", () => {
    expect(CDM_GREEN).toBe("#00FF26");
  });

  it("makes both TOBT and TSAT green inside the startup window", () => {
    const now = Date.UTC(2026, 6, 14, 10, 0, 0);

    expect(computeCDMColors("1003", "0955", now, Bay.Cleared)).toEqual({
      tobtBg: CDM_GREEN,
      tsatBg: CDM_GREEN,
    });
  });

  it("keeps TOBT green and makes TSAT yellow in the last minute", () => {
    const now = Date.UTC(2026, 6, 14, 10, 5, 0);

    expect(computeCDMColors("1000", "0955", now, Bay.Cleared)).toEqual({
      tobtBg: CDM_GREEN,
      tsatBg: CDM_YELLOW,
    });
  });
});

describe("computeTwyDepCTOTColors", () => {
  const ctot = "1000";

  it("hides an empty CTOT", () => {
    expect(computeTwyDepCTOTColors("", Date.UTC(2026, 6, 14, 10, 0, 0))).toEqual({
      ctotBg: "",
      ctotColor: "black",
      showCtot: false,
    });
  });

  it.each([
    ["before the CTOT window", Date.UTC(2026, 6, 14, 9, 54, 59), TWY_DEP_CTOT_ORANGE, "black"],
    ["at CTOT - 5 minutes", Date.UTC(2026, 6, 14, 9, 55, 0), TWY_DEP_CTOT_BLUE, "white"],
    ["at CTOT + 9 minutes", Date.UTC(2026, 6, 14, 10, 9, 0), TWY_DEP_CTOT_YELLOW, "black"],
    ["at CTOT + 11 minutes", Date.UTC(2026, 6, 14, 10, 11, 0), TWY_DEP_CTOT_RED, "black"],
  ])("uses the expected colour %s", (_description, now, ctotBg, ctotColor) => {
    expect(computeTwyDepCTOTColors(ctot, now)).toEqual({
      ctotBg,
      ctotColor,
      showCtot: true,
    });
  });

  it("handles the CTOT window across midnight", () => {
    expect(computeTwyDepCTOTColors("0002", Date.UTC(2026, 6, 14, 23, 56, 0))).toEqual({
      ctotBg: TWY_DEP_CTOT_ORANGE,
      ctotColor: "black",
      showCtot: true,
    });
  });
});
