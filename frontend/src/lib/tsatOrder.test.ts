import { describe, expect, it } from "vitest";
import { Bay, stripDndId, type FrontendStrip, type TacticalStrip } from "@/api/models";
import { orderStripsByTsat } from "./tsatOrder";

function cleared(callsign: string, tsat: string, sequence: number): FrontendStrip {
  return { callsign, tsat, sequence, bay: Bay.Cleared } as FrontendStrip;
}

const now = new Date("2026-10-09T12:00:00Z");

describe("TSAT bay order", () => {
  it.each([
    ["ascending", ["EARLY", "LATE"]],
    ["descending", ["LATE", "EARLY"]],
  ] as const)("orders %s independently of sequence and reorders updated TSATs", (direction, expected) => {
    const strips = [cleared("LATE", "1215", 1), cleared("EARLY", "1205", 2)];
    expect(orderStripsByTsat(strips, direction, now).map(stripDndId)).toEqual(expected);
    const updated = [strips[0], { ...strips[1], tsat: "1220" }];
    expect(orderStripsByTsat(updated, direction, now).map(stripDndId)).toEqual([...expected].reverse());
    expect(strips.map(stripDndId)).toEqual(["LATE", "EARLY"]);
  });

  it.each(["ascending", "descending"] as const)("orders across midnight in %s order", direction => {
    const strips = [cleared("TOMORROW", "0005", 1), cleared("TONIGHT", "2358", 2)];
    const expected = direction === "ascending" ? ["TONIGHT", "TOMORROW"] : ["TOMORROW", "TONIGHT"];
    for (const date of ["2026-10-09T23:55:00Z", "2026-10-10T00:02:00Z"]) {
      expect(orderStripsByTsat(strips, direction, new Date(date)).map(stripDndId)).toEqual(expected);
    }
  });

  it("normalizes compact TSATs and uses sequence for tied times", () => {
    const strips = [cleared("SECOND", " 915 ", 2), cleared("FIRST", "0915", 1)];
    expect(orderStripsByTsat(strips, "ascending", now).map(stripDndId)).toEqual(["FIRST", "SECOND"]);
    expect(orderStripsByTsat(strips, "descending", now).map(stripDndId)).toEqual(["SECOND", "FIRST"]);
  });

  it("groups missing/invalid TSATs and tactical strips away from the earliest flight", () => {
    const strips = [
      cleared("EMPTY", "", 1),
      cleared("INVALID-HOUR", "2400", 2),
      cleared("INVALID-MINUTE", "1260", 3),
      cleared("INVALID-TEXT", "unknown", 4),
      { id: 7, sequence: 5, bay: Bay.Cleared } as TacticalStrip,
      cleared("TIMED", "1205", 6),
    ];
    const ascending = ["TIMED", "EMPTY", "INVALID-HOUR", "INVALID-MINUTE", "INVALID-TEXT", "tactical-7"];
    expect(orderStripsByTsat(strips, "ascending", now).map(stripDndId)).toEqual(ascending);
    expect(orderStripsByTsat(strips, "descending", now).map(stripDndId)).toEqual([...ascending].reverse());
  });
});
