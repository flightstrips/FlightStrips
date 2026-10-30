import { describe, expect, it } from "vitest";

import { Bay } from "@/api/models";
import {
  CDM_GREEN,
  CDM_YELLOW,
  CDM_ORANGE,
  CDM_RED,
  CTOT_YELLOW,
  TWY_DEP_CTOT_BLUE,
  TWY_DEP_CTOT_ORANGE,
  TWY_DEP_CTOT_RED,
  TWY_DEP_CTOT_YELLOW,
  computeCDMColors,
  computeCTOTColors,
  computeTwyDepCTOTColors,
  hasManualTobtSource,
  isTobtUnrealistic,
  isTsatWithinStartRequestWindow,
} from "./cdmColors";

describe("hasManualTobtSource", () => {
	it("returns true when a controller has set the TOBT", () => {
		expect(hasManualTobtSource("EKCH_DEL")).toBe(true);
	});

  it("returns false for auto-synced TOBT values", () => {
		expect(hasManualTobtSource("")).toBe(false);
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

describe("isTobtUnrealistic", () => {
  const now = Date.UTC(2026, 6, 14, 10, 0, 30);

  it.each([
    ["past", "0959", true],
    ["current minute", "1000", false],
    ["45 minutes ahead", "1045", false],
    ["46 minutes ahead", "1046", true],
    ["invalid time", "2460", false],
    ["empty", "", false],
  ])("returns the expected result for a TOBT %s", (_description, tobt, unrealistic) => {
    expect(isTobtUnrealistic(tobt, now)).toBe(unrealistic);
  });

  it("handles the 45-minute threshold across midnight", () => {
    const midnight = Date.UTC(2026, 6, 14, 23, 30);

    expect(isTobtUnrealistic("0015", midnight)).toBe(false);
    expect(isTobtUnrealistic("0016", midnight)).toBe(true);
  });
});

describe("computeCDMColors", () => {
  it.each([
    ["startup window", "1003", "0955", ""],
    ["last minute", "0955", "0950", ""],
    ["expired TSAT", "0950", "0950", ""],
    ["TOBT discrepancy", "1020", "1000", ""],
    ["invalid phase", "1003", "0955", "I"],
    ["invalid phase without TSAT", "", "0955", "I"],
  ])("does not color planned departures during %s", (_description, tsat, tobt, phase) => {
    expect(computeCDMColors(tsat, tobt, Date.UTC(2026, 6, 14, 10, 0), Bay.NotCleared, phase)).toEqual({
      tobtBg: "", tsatBg: "",
    });
  });

  it("does not show TOBT colors for planned departures", () => {
    expect(computeCDMColors("0950", "0950", Date.UTC(2026, 6, 14, 10, 0), Bay.NotCleared, "I")).toEqual({
      tobtBg: "",
      tsatBg: "",
    });
  });

  it("shows orange TOBT discrepancy only before the green and yellow TSAT windows", () => {
    const now = Date.UTC(2026, 6, 14, 10, 0);

    expect(computeCDMColors("1010", "1020", now, Bay.Cleared)).toEqual({
      tobtBg: CDM_ORANGE,
      tsatBg: "",
    });
    expect(computeCDMColors("1005", "1011", now, Bay.Cleared)).toEqual({
      tobtBg: "",
      tsatBg: "",
    });
    expect(computeCDMColors("1010", "1020", now, Bay.Cleared, "I")).toEqual({
      tobtBg: CDM_ORANGE,
      tsatBg: "",
    });
    expect(computeCDMColors("1000", "1020", now, Bay.Cleared)).toEqual({
      tobtBg: "",
      tsatBg: CDM_GREEN,
    });
    expect(computeCDMColors("0955", "1020", now, Bay.Cleared)).toEqual({
      tobtBg: "",
      tsatBg: CDM_YELLOW,
    });
    expect(computeCDMColors("0954", "1020", now, Bay.Cleared)).toEqual({
      tobtBg: CDM_RED,
      tsatBg: "",
    });
    expect(computeCDMColors("0953", "1020", now, Bay.Cleared)).toEqual({
      tobtBg: CDM_RED,
      tsatBg: "",
    });
  });

  it("leaves CTOT coloring active while planned TOBT and TSAT are neutral", () => {
    const now = Date.UTC(2026, 6, 14, 10, 0);
    expect(computeCDMColors("1003", "0955", now, Bay.NotCleared)).toEqual({ tobtBg: "", tsatBg: "" });
    expect(computeCTOTColors("1020", now)).toEqual({ ctotBg: CTOT_YELLOW, ctotColor: "black", showCtot: true });
  });

  it("uses the design green token", () => {
    expect(CDM_GREEN).toBe("#00FF26");
  });

  it("keeps TOBT unfilled and colors TSAT green inside the startup window", () => {
    const now = Date.UTC(2026, 6, 14, 10, 0, 0);

    expect(computeCDMColors("1003", "1000", now, Bay.Cleared)).toEqual({
      tobtBg: "",
      tsatBg: CDM_GREEN,
    });
  });

  it("keeps TOBT unfilled and makes TSAT yellow in the last minute", () => {
    const now = Date.UTC(2026, 6, 14, 10, 5, 0);

    expect(computeCDMColors("1000", "0955", now, Bay.Cleared)).toEqual({
      tobtBg: "",
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
