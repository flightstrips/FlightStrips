import { describe, expect, it } from "vitest";
import { assignmentTimelineTiming, latestTimelineDate, timelineRangeEnd } from "./stand-status-timeline";

describe("stand status timeline", () => {
  it("keeps an active departure open while exposing its planned release", () => {
    const plannedRelease = "2026-08-10T17:40:00Z";
    const timing = assignmentTimelineTiming({
      direction: "DEPARTURE",
      planned_release_at: plannedRelease,
    });

    expect(timing.end).toBeUndefined();
    expect(timing.plannedRelease?.toISOString()).toBe(plannedRelease.replace("Z", ".000Z"));
  });

  it("prefers a persisted hard expiry over the planned release", () => {
    const timing = assignmentTimelineTiming({
      direction: "DEPARTURE",
      expires_at: "2026-08-10T17:45:00Z",
      planned_release_at: "2026-08-10T17:40:00Z",
    });

    expect(timing.end?.toISOString()).toBe("2026-08-10T17:45:00.000Z");
    expect(timing.plannedRelease?.toISOString()).toBe("2026-08-10T17:40:00.000Z");
  });

  it("retains the arrival occupancy window after its ETA", () => {
    const timing = assignmentTimelineTiming({
      direction: "ARRIVAL",
      eta: "2026-08-10T17:40:00Z",
    });

    expect(timing.end?.toISOString()).toBe("2026-08-10T18:10:00.000Z");
    expect(timing.plannedRelease).toBeUndefined();
  });

  it("extends the timeline range to a long-horizon planned release", () => {
    const now = new Date("2026-08-10T17:00:00Z");
    const plannedEnd = new Date("2026-08-10T23:00:00Z");
    const latest = latestTimelineDate([{ plannedEnd }], now);

    expect(latest).toEqual(plannedEnd);
    expect(timelineRangeEnd(latest, now).toISOString()).toBe("2026-08-10T23:30:00.000Z");
  });
});
