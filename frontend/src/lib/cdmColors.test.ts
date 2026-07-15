import { describe, expect, it } from "vitest";

import { Bay } from "@/api/models";
import { CDM_GREEN, CDM_YELLOW, computeCDMColors, hasManualTobtSource, isTsatWithinStartRequestWindow } from "./cdmColors";

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
