import {isExcludedAMANFlight, type AMANFlight, type AMANTrafficBucket} from "@/api/aman";

export const STAR_OVERLOAD_THRESHOLD = 5;
export const STAR_OVERLOAD_MIN_LEAD_MS = 15 * 60_000;

export interface StarOverloadFlight {
  callsign: string;
  feederFix: string | null;
  feederEta: string;
}

export interface StarOverload {
  key: string;
  family: string;
  bucketStart: string;
  bucketEnd: string;
  flights: StarOverloadFlight[];
}

export function starOverloadKey(family: string, bucketStart: string): string {
  return `${family}|${bucketStart}`;
}

/**
 * More than five aircraft crossing the feeder fixes of one STAR family within the same
 * 15-minute bucket. Aircraft less than 15 minutes from the feeder fix no longer count.
 * Only the most loaded STAR is returned for each bucket.
 */
export function computeStarOverloads(flights: readonly AMANFlight[], buckets: readonly AMANTrafficBucket[], now: number, erased: ReadonlySet<string> = new Set()): Map<string, StarOverload> {
  const groups = new Map<string, StarOverload>();
  for (const flight of flights) {
    const family = flight.star_family;
    if (!family || !flight.feeder_fix_eta || flight.feeder_fix_passed || isExcludedAMANFlight(flight)) continue;
    const eta = Date.parse(flight.feeder_fix_eta);
    if (!Number.isFinite(eta) || eta - now < STAR_OVERLOAD_MIN_LEAD_MS) continue;
    const bucket = buckets.find((candidate) => eta >= Date.parse(candidate.start) && eta < Date.parse(candidate.end));
    if (!bucket) continue;
    const key = starOverloadKey(family, bucket.start);
    const group = groups.get(key) ?? {key, family, bucketStart: bucket.start, bucketEnd: bucket.end, flights: []};
    group.flights.push({callsign: flight.callsign, feederFix: flight.feeder_fix ?? null, feederEta: flight.feeder_fix_eta});
    groups.set(key, group);
  }

  const worstPerBucket = new Map<string, StarOverload>();
  for (const group of groups.values()) {
    if (group.flights.length <= STAR_OVERLOAD_THRESHOLD || erased.has(group.key)) continue;
    const current = worstPerBucket.get(group.bucketStart);
    if (!current || group.flights.length > current.flights.length) worstPerBucket.set(group.bucketStart, group);
  }
  for (const group of worstPerBucket.values()) group.flights.sort((a, b) => a.feederEta.localeCompare(b.feederEta));
  return worstPerBucket;
}
