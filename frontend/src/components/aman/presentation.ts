import type {AMANFlight, AMANRunwayClosure, AMANRunwayGap, AMANState} from "@/api/aman";

export const AMAN_TIMELINE_MINUTES = 10;
export const AMAN_MARKER_GAP_PERCENT = 8;

export interface AMANFlightLane {
  id: string;
  label: string;
  flights: AMANFlight[];
  gaps?: AMANRunwayGap[];
  closures?: AMANRunwayClosure[];
}

export function buildRWYTimelineLanes(state: AMANState): {lanes: AMANFlightLane[]; unavailable: boolean; truncated: boolean} {
  const configured = new Set(state.runway_groups.map((group) => group.id));
  const published = state.active_runway_groups;
  const activeIDs = published === undefined
    ? state.runway_groups.filter((group) => group.selected === true).map((group) => group.id)
    : published;
  const valid = Array.isArray(activeIDs) && activeIDs.length > 0
    && new Set(activeIDs).size === activeIDs.length
    && activeIDs.every((id) => typeof id === "string" && configured.has(id));
  if (!valid) return {lanes: [], unavailable: true, truncated: false};

  const lanes = state.runway_groups
    .filter((group) => activeIDs.includes(group.id))
    .map((group, configuredIndex) => ({
      id: group.id,
      label: group.id,
      configuredIndex,
      flights: orderAMANFlights(state.flights.filter((flight) => flight.runway_group_id === group.id)),
      gaps: group.gaps ?? [],
      closures: group.closures ?? [],
    }))
    .sort((left, right) => right.flights.length - left.flights.length || left.configuredIndex - right.configuredIndex);
  return {
    lanes: lanes.slice(0, 4).map(({id, label, flights, gaps, closures}) => ({id, label, flights, gaps, closures})),
    unavailable: false,
    truncated: lanes.length > 4,
  };
}

export interface AMANTimelineRange {
  startMs: number;
  endMs: number;
}

export interface AMANTimelineMarker {
  flight: AMANFlight;
  timestamp: string;
  leftPercent: number;
  track: number;
}

export interface AMANGainLossPresentationContext {
  authoritative: boolean;
  connected: boolean;
  fresh: boolean;
}

function sequenceValue(flight: AMANFlight): number | null {
  return flight.order ?? flight.slot?.sequence ?? null;
}

/**
 * AMAN order is backend-owned. This only turns the serialized order/sequence
 * into a stable display order and preserves wire order for ties or missing data.
 */
export function orderAMANFlights(flights: AMANFlight[]): AMANFlight[] {
  return flights
    .map((flight, index) => ({flight, index, sequence: sequenceValue(flight)}))
    .sort((left, right) => {
      if (left.sequence === null && right.sequence === null) return left.index - right.index;
      if (left.sequence === null) return 1;
      if (right.sequence === null) return -1;
      return left.sequence - right.sequence || left.index - right.index;
    })
    .map(({flight}) => flight);
}

export function buildAMANLanes(state: AMANState): AMANFlightLane[] {
  const declared = state.runway_groups.map((group) => group.id);
  const discovered = state.flights
    .map((flight) => flight.runway_group_id)
    .filter((groupID): groupID is string => groupID !== null && !declared.includes(groupID));
  const groupIDs = [...declared, ...new Set(discovered)];

  const lanes = groupIDs.map((groupID) => ({
    id: groupID,
    label: groupID,
    flights: orderAMANFlights(state.flights.filter((flight) => flight.runway_group_id === groupID)),
    gaps: state.runway_groups.find((group) => group.id === groupID)?.gaps ?? [],
    closures: state.runway_groups.find((group) => group.id === groupID)?.closures ?? [],
  }));
  const unassigned = orderAMANFlights(state.flights.filter((flight) => flight.runway_group_id === null));
  if (unassigned.length > 0) {
    lanes.push({id: "unassigned", label: "Unassigned runway group", flights: unassigned, gaps: [], closures: []});
  }
  return lanes;
}

/**
 * A runway's timeline is split by the assigned holding fix. Holding assignment
 * is backend-owned; flights without one remain visible in their own lane rather
 * than being guessed from a route fact.
 */
export function buildAMANHoldingLanes(flights: AMANFlight[]): AMANFlightLane[] {
  const byHolding = new Map<string, AMANFlight[]>();

  for (const flight of flights) {
    const holdingID = flight.holding_fix ?? "unassigned";
    const lane = byHolding.get(holdingID);
    if (lane) lane.push(flight);
    else byHolding.set(holdingID, [flight]);
  }

  return [...byHolding.entries()].map(([id, laneFlights]) => ({
    id,
    label: id === "unassigned" ? "No holding assigned" : id,
    flights: orderAMANFlights(laneFlights),
  }));
}

export function operationalMarkerTimestamp(flight: AMANFlight): string | null {
  return flight.slot?.time ?? flight.operational_teta;
}

export function buildTimelineRange(flights: AMANFlight[]): AMANTimelineRange | null {
  const timestamps = flights
    .map(operationalMarkerTimestamp)
    .filter((value): value is string => value !== null)
    .map((value) => Date.parse(value))
    .filter(Number.isFinite);
  if (timestamps.length === 0) return null;

  const minimumSpan = AMAN_TIMELINE_MINUTES * 60_000;
  const minimum = Math.min(...timestamps);
  const maximum = Math.max(...timestamps);
  const span = Math.max(maximum - minimum, minimumSpan);
  const padding = Math.max(span * 0.05, 60_000);
  return {startMs: minimum - padding, endMs: minimum + span + padding};
}

/** Collision tracks are display-only; marker time and order remain untouched. */
export function layoutTimelineMarkers(
  flights: AMANFlight[],
  range: AMANTimelineRange,
  minimumGapPercent = AMAN_MARKER_GAP_PERCENT,
): AMANTimelineMarker[] {
  const span = range.endMs - range.startMs;
  if (span <= 0) return [];

  const candidates = orderAMANFlights(flights).flatMap((flight) => {
    const timestamp = operationalMarkerTimestamp(flight);
    if (timestamp === null) return [];
    const timeMs = Date.parse(timestamp);
    if (!Number.isFinite(timeMs)) return [];
    if (timeMs < range.startMs || timeMs > range.endMs) return [];
    return [{flight, timestamp, leftPercent: ((timeMs - range.startMs) / span) * 100}];
  });

  const trackEnds: number[] = [];
  const tracks = new Map<string, number>();
  candidates
    .map((candidate, index) => ({candidate, index}))
    .sort((left, right) => left.candidate.leftPercent - right.candidate.leftPercent || left.index - right.index)
    .forEach(({candidate}) => {
      let track = trackEnds.findIndex((end) => candidate.leftPercent - end >= minimumGapPercent);
      if (track === -1) track = trackEnds.length;
      trackEnds[track] = candidate.leftPercent;
      tracks.set(candidate.flight.flight_id, track);
    });
  return candidates.map((candidate) => ({...candidate, track: tracks.get(candidate.flight.flight_id) ?? 0}));
}

export function formatAMANTime(value: string | null): string {
  if (value === null) return "Unavailable";
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf()) ? "Invalid" : parsed.toISOString().slice(11, 16);
}

export function formatGainLoss(seconds: number | null, context: AMANGainLossPresentationContext): string {
  if (seconds === null || !context.authoritative || !context.connected || !context.fresh) return "Unavailable";

  const absolute = Math.abs(seconds);
  if (absolute < 30) return "=00";

  const prefix = seconds > 0 ? "G" : "L";
  const minutes = Math.floor((absolute + 30) / 60);
  return minutes > 99 ? `${prefix}99+` : `${prefix}${String(minutes).padStart(2, "0")}`;
}
