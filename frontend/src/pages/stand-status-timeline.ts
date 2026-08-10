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

type TimelineRangeItem = {
  end?: Date;
  plannedEnd?: Date;
};

const TIMELINE_MIN_FUTURE_MS = 4 * 60 * 60 * 1000;
const TIMELINE_END_PADDING_MS = 30 * 60 * 1000;

function timelineDate(value?: string): Date | undefined {
  if (!value) return undefined;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? undefined : date;
}

export function assignmentTimelineTiming(assignment: TimelineAssignment): AssignmentTimelineTiming {
  const expiry = timelineDate(assignment.expires_at);
  if (assignment.direction === "DEPARTURE") {
    return {
      end: expiry,
      plannedRelease: timelineDate(assignment.planned_release_at),
    };
  }
  if (expiry) return { end: expiry };
  const eta = timelineDate(assignment.eta);
  return { end: eta ? new Date(eta.getTime() + 30 * 60 * 1000) : undefined };
}

export function latestTimelineDate(items: TimelineRangeItem[], now: Date): Date {
  return items.reduce((latest, item) => {
    const candidate = item.plannedEnd && (!item.end || item.plannedEnd > item.end)
      ? item.plannedEnd
      : item.end;
    return candidate && candidate > latest ? candidate : latest;
  }, now);
}

export function timelineRangeEnd(latest: Date, now: Date): Date {
  return new Date(Math.max(
    latest.getTime() + TIMELINE_END_PADDING_MS,
    now.getTime() + TIMELINE_MIN_FUTURE_MS,
  ));
}
