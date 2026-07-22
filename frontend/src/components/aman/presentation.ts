import type {AMANFlight, AMANState} from "@/api/aman";

export const AMAN_TIMELINE_MINUTES = 10;
export const AMAN_MARKER_GAP_PERCENT = 8;

export interface AMANFlightLane {
  id: string;
  label: string;
  flights: AMANFlight[];
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
  }));
  const unassigned = orderAMANFlights(state.flights.filter((flight) => flight.runway_group_id === null));
  if (unassigned.length > 0) {
    lanes.push({id: "unassigned", label: "Unassigned runway group", flights: unassigned});
  }
  return lanes;
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
    return [{flight, timestamp, leftPercent: Math.max(0, Math.min(100, ((timeMs - range.startMs) / span) * 100))}];
  });

  const trackEnds: number[] = [];
  return candidates
    .sort((left, right) => left.leftPercent - right.leftPercent)
    .map((candidate) => {
      let track = trackEnds.findIndex((end) => candidate.leftPercent - end >= minimumGapPercent);
      if (track === -1) track = trackEnds.length;
      trackEnds[track] = candidate.leftPercent;
      return {...candidate, track};
    });
}

export function formatAMANTime(value: string | null): string {
  if (value === null) return "Unavailable";
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf()) ? "Invalid" : parsed.toISOString().slice(11, 16);
}

export function formatGainLoss(seconds: number | null): string {
  if (seconds === null) return "Unavailable";
  if (seconds === 0) return "0:00";
  const sign = seconds > 0 ? "+" : "−";
  const absolute = Math.abs(seconds);
  return `${sign}${Math.floor(absolute / 60)}:${String(absolute % 60).padStart(2, "0")}`;
}
