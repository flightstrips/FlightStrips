import { describe, expect, it } from "vitest";
import { Bay } from "@/api/models";
import { canConfirmRwyArrRunway, getRwyArrRunwayColor } from "./finalArrRunwayColor";

describe("getRwyArrRunwayColor", () => {
  it("shows a cleared strip red until it is confirmed", () => {
    expect(getRwyArrRunwayColor({ runwayCleared: true, runwayConfirmed: false }))
      .toBe("var(--color-runway-closed)");
  });

  it("shows a confirmed clearance green", () => {
    expect(getRwyArrRunwayColor({ runwayCleared: true, runwayConfirmed: true }))
      .toBe("var(--color-runway-confirmed)");
  });

  it("removes the fill when runway clearance is cleared", () => {
    expect(getRwyArrRunwayColor({ runwayCleared: false, runwayConfirmed: false }))
      .toBeUndefined();
  });
});

describe("canConfirmRwyArrRunway", () => {
  const strips = [
    { callsign: "FIRST", bay: Bay.RwyArr, runway_cleared: true, runway_confirmed: true },
    { callsign: "SECOND", bay: Bay.RwyArr, runway_cleared: true, runway_confirmed: false },
    { callsign: "DEPARTURE", bay: Bay.Depart, runway_cleared: true, runway_confirmed: true },
  ];

  it("blocks confirmation while another arrival strip is green", () => {
    expect(canConfirmRwyArrRunway("SECOND", strips)).toBe(false);
  });

  it("allows the current green strip to be toggled off", () => {
    expect(canConfirmRwyArrRunway("FIRST", strips)).toBe(true);
  });

  it("allows confirmation when no other RWY ARR strip is green", () => {
    expect(canConfirmRwyArrRunway("SECOND", strips.slice(1))).toBe(true);
  });
});
