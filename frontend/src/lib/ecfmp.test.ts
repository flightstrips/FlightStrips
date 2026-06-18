import { describe, expect, it } from "vitest";

import { getCtotSlotDisplay } from "./ecfmp";

describe("getCtotSlotDisplay", () => {
  it("shows NSR when no CTOT is available", () => {
    expect(getCtotSlotDisplay({ ctot: "" })).toEqual({
      restrictionLabel: "NSR",
      ctot: "",
      hasCtot: false,
    });
  });

  it("uses the CDM slot label when CTOT is available", () => {
    expect(getCtotSlotDisplay({
      ctot: "945",
      most_penalizing_airspace: "DK-E",
    })).toEqual({
      restrictionLabel: "DK-E",
      ctot: "0945",
      hasCtot: true,
    });
  });

  it("leaves the label blank when CDM does not provide one", () => {
    expect(getCtotSlotDisplay({
      ctot: "1040",
      most_penalizing_airspace: "",
    })).toEqual({
      restrictionLabel: "",
      ctot: "1040",
      hasCtot: true,
    });
  });
});
