type TimelineAssignment = {
  direction: string;
  eta?: string;
  expires_at?: string;
  planned_release_at?: string;
};

export type AssignmentTimelineTiming = {
  end?: Date;
  plannedRelease?: Date;
};

const TIMELINE_FUTURE_MS = 2 * 60 * 60 * 1000;
const DEPARTURE_CONTINUATION_MS = 30 * 60 * 1000;

function timelineDate(value?: string): Date | undefined {
  if (!value) return undefined;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? undefined : date;
}

export function assignmentTimelineTiming(assignment: TimelineAssignment, now = new Date()): AssignmentTimelineTiming {
  const expiry = timelineDate(assignment.expires_at);
  if (assignment.direction === "DEPARTURE") {
    const plannedRelease = timelineDate(assignment.planned_release_at);
    const continuationEnd = new Date(now.getTime() + DEPARTURE_CONTINUATION_MS);
    const visibleHorizonEnd = new Date(now.getTime() + TIMELINE_FUTURE_MS);
    return {
      // A live departure may remain operationally assigned after its planned
      // release. Keep a near-term continuation visible without allowing a
      // stale or next-day planning value to fill the complete timeline.
      end: expiry ?? (plannedRelease && plannedRelease > now && plannedRelease <= visibleHorizonEnd
        ? plannedRelease
        : continuationEnd),
      plannedRelease,
    };
  }
  if (expiry) return { end: expiry };
  const eta = timelineDate(assignment.eta);
  return { end: eta ? new Date(eta.getTime() + 30 * 60 * 1000) : undefined };
}

export function timelineRangeEnd(now: Date): Date {
  return new Date(now.getTime() + TIMELINE_FUTURE_MS);
}
