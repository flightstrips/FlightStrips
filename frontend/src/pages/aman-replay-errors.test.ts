import {describe, expect, it} from "vitest";
import {operationalErrorReference, type ReplayErrorEvent} from "./aman-replay-errors";

const events: ReplayErrorEvent[] = [
  {type: "threshold_crossing", at: "2026-07-28T11:41:11Z", sequence: 1},
  {type: "go_around_detected", at: "2026-07-28T11:41:38.6462921Z"},
  {type: "threshold_crossing", at: "2026-07-28T11:52:49Z", sequence: 2},
  {type: "landing_proxy", at: "2026-07-28T11:53:23Z"},
];

describe("operationalErrorReference", () => {
  it("uses the first threshold crossing before go-around detection", () => {
    expect(operationalErrorReference(events, "2026-07-28T11:41:37Z", "2026-07-28T11:53:23Z", 600)).toEqual({
      at: "2026-07-28T11:41:11Z",
      kind: "first_touchdown",
    });
  });

  it("uses detection plus the exported fallback delay after a go-around", () => {
    expect(operationalErrorReference(events, "2026-07-28T11:41:49Z", "2026-07-28T11:53:23Z", 600)).toEqual({
      at: "2026-07-28T11:51:38.646Z",
      kind: "go_around_target",
      goAroundDetectedAt: "2026-07-28T11:41:38.6462921Z",
    });
  });

  it("falls back to the landing proxy when no threshold crossing was detected", () => {
    expect(operationalErrorReference([], "2026-07-28T11:30:00Z", "2026-07-28T11:53:23Z", 600)).toEqual({
      at: "2026-07-28T11:53:23Z",
      kind: "landing_proxy",
    });
  });
});
