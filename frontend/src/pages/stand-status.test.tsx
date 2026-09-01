import { describe, expect, it } from "vitest";
import { assignmentTimelineTiming, timelineRangeEnd } from "./stand-status-timeline";

describe("stand status timeline", () => {
  it("ends an active departure at its future planned release", () => {
    const plannedRelease = "2026-08-10T17:40:00Z";
    const timing = assignmentTimelineTiming({
      direction: "DEPARTURE",
      planned_release_at: plannedRelease,
    }, new Date("2026-08-10T17:00:00Z"));

    expect(timing.end?.toISOString()).toBe(plannedRelease.replace("Z", ".000Z"));
    expect(timing.plannedRelease?.toISOString()).toBe(plannedRelease.replace("Z", ".000Z"));
  });

  it("shows an overdue or unscheduled active departure continuing beyond now", () => {
    const now = new Date("2026-08-10T18:00:00Z");
    const continuationEnd = new Date("2026-08-10T18:30:00Z");

    expect(assignmentTimelineTiming({
      direction: "DEPARTURE",
      planned_release_at: "2026-08-10T17:40:00Z",
    }, now).end).toEqual(continuationEnd);
    expect(assignmentTimelineTiming({ direction: "DEPARTURE" }, now).end).toEqual(continuationEnd);
  });

  it("caps a long-horizon departure without pretending it ends now", () => {
    const now = new Date("2026-08-10T17:00:00Z");
    const timing = assignmentTimelineTiming({
      direction: "DEPARTURE",
      planned_release_at: "2026-08-10T23:00:00Z",
    }, now);

    expect(timing.end?.toISOString()).toBe("2026-08-10T17:30:00.000Z");
    expect(timing.plannedRelease?.toISOString()).toBe("2026-08-10T23:00:00.000Z");
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

  it("keeps a long-horizon planned release from stretching the timeline", () => {
    const now = new Date("2026-08-10T17:00:00Z");

    expect(timelineRangeEnd(now).toISOString()).toBe("2026-08-10T19:00:00.000Z");
  });
});
