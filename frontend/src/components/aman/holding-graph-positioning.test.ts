import {describe, expect, it} from "vitest";

import type {AMANHoldingEntry} from "@/api/aman";
import {
  holdingGraphUrgency,
  layoutHoldingGraph,
  layoutHoldingLabels,
  positionHoldingAltitude,
  positionHoldingTime,
} from "./holding-graph-positioning";

const now = new Date("2026-09-11T23:58:00.000Z");

function entry(callsign: string, eat: string | null, altitude: number | null): AMANHoldingEntry {
  return {
    callsign: callsign.toUpperCase(),
    holding: "OLPIB",
    eat,
    cleared_altitude: altitude,
    source_status: "fresh",
    observed_at: now.toISOString(),
  };
}

describe("holding graph time positioning", () => {
  it.each([
    ["2026-09-11T23:57:59.999Z", 0, "OVERDUE", "OVERDUE"],
    ["2026-09-11T23:58:00.000Z", 0, "23:58", null],
    ["2026-09-12T00:28:00.000Z", 50, "00:28", null],
    ["2026-09-12T00:58:00.000Z", 100, "00:58", null],
    ["2026-09-12T00:58:00.001Z", 100, ">60", ">60"],
  ])("positions %s on the fixed now-to-60-minute axis", (eat, percent, label, edgeLabel) => {
    expect(positionHoldingTime(eat, now)).toEqual({percent, label, edgeLabel});
  });

  it("keeps backend-resolved next-day timestamps on the correct side of midnight", () => {
    expect(positionHoldingTime("2026-09-12T00:02:00.000Z", now)).toEqual({
      percent: 4 / 60 * 100,
      label: "00:02",
      edgeLabel: null,
    });
    expect(holdingGraphUrgency("2026-09-12T00:02:00.000Z", now)).toBe("soon");
  });

  it.each([
    ["2026-09-12T00:01:59.999Z", "soon"],
    ["2026-09-12T00:02:00.000Z", "soon"],
    ["2026-09-12T00:02:00.001Z", "later"],
    ["2026-09-11T23:57:59.999Z", "later"],
    [null, "missing"],
    ["invalid", "missing"],
  ])("classifies %s as %s at the exact four-minute boundary", (eat, urgency) => {
    expect(holdingGraphUrgency(eat, now)).toBe(urgency);
  });

  it("retains missing and invalid timestamps without inventing a position", () => {
    expect(positionHoldingTime(null, now)).toEqual({percent: null, label: "—", edgeLabel: null});
    expect(positionHoldingTime("invalid", now)).toEqual({percent: null, label: "—", edgeLabel: null});
  });
});

describe("holding label spacing", () => {
  it("orders a shared level by actual release time without crossing connectors", () => {
    const positions = layoutHoldingGraph([
      entry("A-EARLY", "2026-09-12T00:08:00.000Z", 12000),
      entry("Z-LATE", "2026-09-12T00:18:00.000Z", 12000),
      entry("M-MISSING", null, 12000),
      entry("B-BEYOND", "2026-09-12T01:18:00.000Z", 12000),
    ], now);
    const {labelY} = layoutHoldingLabels(positions, 441, 20);
    expect(labelY[3]).toBeLessThan(labelY[1]);
    expect(labelY[1]).toBeLessThan(labelY[0]);
    expect(labelY[0]).toBeLessThan(labelY[2]);
    expect(layoutHoldingLabels([...positions].reverse(), 441, 20).labelY.reverse()).toEqual(labelY);
  });
  it.each([16, 28])("separates equal, nearby and edge altitudes with %ipx rows", (rowHeight) => {
    const positions = layoutHoldingGraph([30000, 30000, 29900, 12000, 12000, 9000, 8000].map((altitude, index) =>
      entry(`SAS${index}`, now.toISOString(), altitude)), now);
    const {height, labelY, edgePadding, altitudeY} = layoutHoldingLabels(positions, 216, rowHeight);
    positions.forEach(position => {
      const group = positions.flatMap((value, index) => value.altitude.percent === position.altitude.percent ? [labelY[index]] : []);
      expect(group.reduce((sum, y) => sum + y, 0) / group.length).toBeCloseTo(altitudeY(position.altitude.percent!));
    });
    const sorted = [...new Set(labelY)].sort((a, b) => a - b);
    sorted.slice(1).forEach((center, index) => expect(center - sorted[index]).toBeGreaterThanOrEqual(rowHeight + 0.99));
    expect(sorted[0] + edgePadding).toBeGreaterThanOrEqual(rowHeight / 2);
    expect(sorted.at(-1)! + rowHeight / 2).toBeLessThanOrEqual(height + edgePadding);
    expect(positions[0].altitude.percent).toBe(0);
    expect(positions.at(-1)?.altitude.percent).toBe(100);
  });

  it("expands crowded plots and assigns stable positions after input reordering", () => {
    const positions = layoutHoldingGraph(Array.from({length: 40}, (_, index) =>
      entry(`SAS${index}`, now.toISOString(), 12000)), now);
    const first = layoutHoldingLabels(positions, 100, 16);
    const reversed = layoutHoldingLabels([...positions].reverse(), 100, 16);
    expect(first.height).toBeGreaterThan(100);
    expect(first.labelY).toEqual([...reversed.labelY].reverse());
    expect(new Set(first.labelY).size).toBe(40);
  });

  it("expands only the occupied level and preserves spacing elsewhere", () => {
    const single = layoutHoldingGraph([entry("A", now.toISOString(), 12000)], now);
    const double = layoutHoldingGraph([entry("A", now.toISOString(), 12000), entry("B", null, 12000)], now);
    const baseline = layoutHoldingLabels(single, 441, 20);
    const stacked = layoutHoldingLabels(double, 441, 20);
    const y = (layout: typeof baseline, level: number) => layout.altitudeY((300 - level) / 210 * 100);
    expect(y(stacked, 290) - y(stacked, 300)).toBeCloseTo(y(baseline, 290) - y(baseline, 300));
    expect(y(stacked, 100) - y(stacked, 110)).toBeCloseTo(y(baseline, 100) - y(baseline, 110));
    expect(y(stacked, 110) - y(stacked, 130)).toBeCloseTo(y(baseline, 110) - y(baseline, 130) + 21);
    expect(stacked.labelY[1] - stacked.labelY[0]).toBe(21);
  });
});

describe("holding graph altitude positioning", () => {
  it.each([
    [8999, 100, "<090", "<090"],
    [9000, 100, "FL090", null],
    [19500, 50, "FL195", null],
    [30000, 0, "FL300", null],
    [30001, 0, ">300", ">300"],
  ])("positions %i feet on the fixed FL090-to-FL300 axis", (altitude, percent, label, edgeLabel) => {
    expect(positionHoldingAltitude(altitude)).toEqual({percent, label, edgeLabel});
  });

  it("retains missing altitude without inventing a level", () => {
    expect(positionHoldingAltitude(null)).toEqual({percent: null, label: "—", edgeLabel: null});
  });
});

describe("holding graph collision tracks", () => {
  it("allocates independent display tracks without changing times or levels", () => {
    const entries = [
      entry("b", "2026-09-12T00:08:00.000Z", 12000),
      entry("a", "2026-09-12T00:08:00.000Z", 12000),
      entry("c", "2026-09-12T00:18:00.000Z", 13000),
      entry("missing", null, null),
    ];
    const positions = layoutHoldingGraph(entries, now);

    expect(positions.map(({timeTrack, altitudeTrack}) => [timeTrack, altitudeTrack])).toEqual([
      [1, 2], [0, 1], [0, 0], [null, null],
    ]);
    expect(positions.map(({entry: value}) => value)).toEqual(entries);
    expect(positions.map(({time}) => time.percent)).toEqual([10 / 60 * 100, 10 / 60 * 100, 20 / 60 * 100, null]);
    expect(positions.map(({altitude}) => altitude.percent)).toEqual([1800 / 21, 1800 / 21, 1700 / 21, null]);
  });

  it("stacks flights pinned to the same graph edges", () => {
    const positions = layoutHoldingGraph([
      entry("overdue-b", "2026-09-11T22:00:00.000Z", 8000),
      entry("overdue-a", "2026-09-11T23:00:00.000Z", 7000),
      entry("late-b", "2026-09-12T02:00:00.000Z", 32000),
      entry("late-a", "2026-09-12T01:00:00.000Z", 31000),
    ], now);

    expect(positions.map(({timeTrack, altitudeTrack}) => [timeTrack, altitudeTrack])).toEqual([
      [1, 1], [0, 0], [1, 1], [0, 0],
    ]);
  });
});
