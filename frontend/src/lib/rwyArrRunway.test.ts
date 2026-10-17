import { describe, expect, it } from "vitest";
import { Bay } from "@/api/models";
import { hasOtherRwyArrStrip } from "./rwyArrRunway";

describe("hasOtherRwyArrStrip", () => {
  it("does not treat the moving strip as an existing RWY ARR occupant", () => {
    expect(hasOtherRwyArrStrip(
      "FIRST",
      [{ callsign: "FIRST", bay: Bay.RwyArr }],
      [],
    )).toBe(false);
  });

  it("counts another flight strip or tactical strip in RWY ARR", () => {
    expect(hasOtherRwyArrStrip(
      "SECOND",
      [{ callsign: "FIRST", bay: Bay.RwyArr }],
      [],
    )).toBe(true);
    expect(hasOtherRwyArrStrip(
      "FIRST",
      [{ callsign: "FIRST", bay: Bay.RwyArr }],
      [{ bay: Bay.RwyArr }],
    )).toBe(true);
  });

  it("ignores strips in other bays", () => {
    expect(hasOtherRwyArrStrip(
      "SECOND",
      [{ callsign: "FIRST", bay: Bay.Final }],
      [{ bay: Bay.Stand }],
    )).toBe(false);
  });
});
