import type {AMANHoldingEntry} from "@/api/aman";

export const HOLDING_GRAPH_WINDOW_MINUTES = 60;
export const HOLDING_GRAPH_MIN_FLIGHT_LEVEL = 90;
export const HOLDING_GRAPH_MAX_FLIGHT_LEVEL = 300;
export const HOLDING_GRAPH_SOON_MINUTES = 4;
export const HOLDING_GRAPH_COLLISION_GAP_PERCENT = 6;

export type HoldingGraphUrgency = "soon" | "later" | "missing";
export type HoldingTimeEdgeLabel = "OVERDUE" | ">60";
export type HoldingAltitudeEdgeLabel = "<090" | ">300";

export interface HoldingAxisPosition<EdgeLabel extends string> {
  percent: number | null;
  label: string;
  edgeLabel: EdgeLabel | null;
}

export interface HoldingGraphPosition {
  entry: AMANHoldingEntry;
  time: HoldingAxisPosition<HoldingTimeEdgeLabel>;
  altitude: HoldingAxisPosition<HoldingAltitudeEdgeLabel>;
  urgency: HoldingGraphUrgency;
  timeTrack: number | null;
  altitudeTrack: number | null;
}

/** Stack each level vertically, keeping the stack centered on its altitude. */
export function layoutHoldingLabels(positions: HoldingGraphPosition[], availableHeight: number, rowHeight: number) {
  const gap = rowHeight + 1;
  const sorted = positions.map((position, index) => ({position, index})).sort((left, right) =>
    left.position.altitude.percent! - right.position.altitude.percent! ||
    compareHoldingTimes(left.position, right.position) ||
    compareKeys(left.position.entry.callsign, right.position.entry.callsign));
  const groups: Array<{percent: number; indices: number[]}> = [];
  sorted.forEach(({position, index}) => {
    const previous = groups.at(-1);
    if (previous?.percent === position.altitude.percent) previous.indices.push(index);
    else groups.push({percent: position.altitude.percent!, indices: [index]});
  });
  const baseHeight = Math.max(availableHeight, gap * (HOLDING_GRAPH_MAX_FLIGHT_LEVEL - HOLDING_GRAPH_MIN_FLIGHT_LEVEL) / 10);
  const byPercent = new Map(groups.map(group => [group.percent, group]));
  const percents = [...new Set([
    ...Array.from({length: 22}, (_, index) => index * 10 / (HOLDING_GRAPH_MAX_FLIGHT_LEVEL - HOLDING_GRAPH_MIN_FLIGHT_LEVEL) * 100),
    ...groups.map(group => group.percent),
  ])].sort((left, right) => left - right);
  const anchors = new Map<number, number>();
  let height = 0;
  percents.forEach((percent, index) => {
    if (index > 0) {
      const previous = percents[index - 1];
      const left = byPercent.get(previous);
      const right = byPercent.get(percent);
      const leftBand = left ? (left.indices.length - 1) * gap : 0;
      const rightBand = right ? (right.indices.length - 1) * gap : 0;
      height += Math.max((percent - previous) / 100 * baseHeight, left && right ? gap : 0) + (leftBand + rightBand) / 2;
    }
    anchors.set(percent, height);
  });
  function altitudeY(percent: number): number {
    const exact = anchors.get(percent);
    if (exact !== undefined) return exact;
    const upperIndex = percents.findIndex(value => value > percent);
    if (upperIndex <= 0) return percent <= 0 ? 0 : height;
    const lower = percents[upperIndex - 1];
    const upper = percents[upperIndex];
    return anchors.get(lower)! + (percent - lower) / (upper - lower) * (anchors.get(upper)! - anchors.get(lower)!);
  }
  let edgePadding = rowHeight / 2 + 2;
  groups.filter(group => group.percent === 0 || group.percent === 100).forEach(group => {
    edgePadding = Math.max(edgePadding, ((group.indices.length - 1) * gap + rowHeight) / 2 + 2);
  });
  const labelY: number[] = [];
  groups.forEach(group => group.indices.forEach((index, slot) => {
    labelY[index] = altitudeY(group.percent) + (slot - (group.indices.length - 1) / 2) * gap;
  }));
  return {height, labelY, edgePadding, altitudeY};
}

function compareHoldingTimes(left: HoldingGraphPosition, right: HoldingGraphPosition): number {
  const leftTime = left.entry.eat === null ? NaN : Date.parse(left.entry.eat);
  const rightTime = right.entry.eat === null ? NaN : Date.parse(right.entry.eat);
  if (!Number.isFinite(leftTime)) return Number.isFinite(rightTime) ? 1 : 0;
  if (!Number.isFinite(rightTime)) return -1;
  // The time axis runs from later at the top to earlier at the bottom. Keep
  // labels at a shared level in that order so their connectors cannot cross.
  return rightTime - leftTime;
}

interface TrackCandidate {
  index: number;
  percent: number;
  key: string;
}

function compareKeys(left: string, right: string): number {
  return left < right ? -1 : left > right ? 1 : 0;
}

function formatUTCTime(timestampMs: number): string {
  return new Date(timestampMs).toISOString().slice(11, 16);
}

export function positionHoldingTime(eat: string | null, now: Date): HoldingAxisPosition<HoldingTimeEdgeLabel> {
  if (eat === null) return {percent: null, label: "—", edgeLabel: null};

  const eatMs = Date.parse(eat);
  const nowMs = now.valueOf();
  if (!Number.isFinite(eatMs) || !Number.isFinite(nowMs)) return {percent: null, label: "—", edgeLabel: null};

  const minutes = (eatMs - nowMs) / 60_000;
  if (minutes < 0) return {percent: 0, label: "OVERDUE", edgeLabel: "OVERDUE"};
  if (minutes > HOLDING_GRAPH_WINDOW_MINUTES) return {percent: 100, label: ">60", edgeLabel: ">60"};
  return {
    percent: (minutes / HOLDING_GRAPH_WINDOW_MINUTES) * 100,
    label: formatUTCTime(eatMs),
    edgeLabel: null,
  };
}

export function positionHoldingAltitude(altitudeFeet: number | null): HoldingAxisPosition<HoldingAltitudeEdgeLabel> {
  if (altitudeFeet === null || !Number.isFinite(altitudeFeet)) return {percent: null, label: "—", edgeLabel: null};

  const flightLevel = altitudeFeet / 100;
  if (flightLevel < HOLDING_GRAPH_MIN_FLIGHT_LEVEL) return {percent: 100, label: "<090", edgeLabel: "<090"};
  if (flightLevel > HOLDING_GRAPH_MAX_FLIGHT_LEVEL) return {percent: 0, label: ">300", edgeLabel: ">300"};
  return {
    percent: ((HOLDING_GRAPH_MAX_FLIGHT_LEVEL - flightLevel) /
      (HOLDING_GRAPH_MAX_FLIGHT_LEVEL - HOLDING_GRAPH_MIN_FLIGHT_LEVEL)) * 100,
    label: `FL${String(Math.round(flightLevel)).padStart(3, "0")}`,
    edgeLabel: null,
  };
}

export function holdingGraphUrgency(eat: string | null, now: Date): HoldingGraphUrgency {
  if (eat === null) return "missing";
  const remainingMs = Date.parse(eat) - now.valueOf();
  if (!Number.isFinite(remainingMs)) return "missing";
  return remainingMs >= 0 && remainingMs <= HOLDING_GRAPH_SOON_MINUTES * 60_000 ? "soon" : "later";
}

/**
 * Tracks are a display-only offset. Sorting by graph position and stable entry
 * identity makes allocation independent of replacement-array arrival order.
 */
function allocateTracks(candidates: TrackCandidate[], count: number, minimumGapPercent: number): Array<number | null> {
  const result: Array<number | null> = Array.from({length: count}, () => null);
  const trackEnds: number[] = [];

  for (const candidate of candidates.sort((left, right) =>
    left.percent - right.percent || compareKeys(left.key, right.key) || left.index - right.index)) {
    let track = trackEnds.findIndex((end) => candidate.percent - end >= minimumGapPercent);
    if (track === -1) track = trackEnds.length;
    trackEnds[track] = candidate.percent;
    result[candidate.index] = track;
  }
  return result;
}

export function layoutHoldingGraph(
  entries: AMANHoldingEntry[],
  now: Date,
  minimumGapPercent = HOLDING_GRAPH_COLLISION_GAP_PERCENT,
): HoldingGraphPosition[] {
  const positioned = entries.map((entry) => ({
    entry,
    time: positionHoldingTime(entry.eat, now),
    altitude: positionHoldingAltitude(entry.cleared_altitude),
    urgency: holdingGraphUrgency(entry.eat, now),
  }));
  const candidates = (axis: "time" | "altitude"): TrackCandidate[] => positioned.flatMap((position, index) => {
    const percent = position[axis].percent;
    return percent === null ? [] : [{index, percent, key: `${position.entry.callsign}\0${position.entry.callsign}`}];
  });
  const timeTracks = allocateTracks(candidates("time"), positioned.length, minimumGapPercent);
  const altitudeTracks = allocateTracks(candidates("altitude"), positioned.length, minimumGapPercent);

  return positioned.map((position, index) => ({
    ...position,
    timeTrack: timeTracks[index],
    altitudeTrack: altitudeTracks[index],
  }));
}
