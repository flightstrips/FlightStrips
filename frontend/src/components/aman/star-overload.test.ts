import {describe, expect, it} from "vitest";

import type {AMANFlight, AMANTrafficBucket} from "@/api/aman";
import {computeStarOverloads, starOverloadKey} from "./star-overload";

const start = Date.parse("2026-07-22T20:30:00.000Z");
const bucket = (index: number): AMANTrafficBucket => ({
  start: new Date(start + index * 15 * 60_000).toISOString(), end: new Date(start + (index + 1) * 15 * 60_000).toISOString(),
  planned_count: 0, airborne_count: 0, count: 0, load_factor: 0, selected_rate: null, bucket_high: false, window_high: false, alert: "none", flights: [],
});
const buckets = [bucket(0), bucket(1), bucket(2)];
const flight = (callsign: string, family: string, minute: number, extra: Partial<AMANFlight> = {}) => ({
  callsign, star_family: family, feeder_fix: "FIX", feeder_fix_eta: new Date(start + minute * 60_000).toISOString(), feeder_fix_passed: false, sequence_disposition: "active", ...extra,
}) as AMANFlight;
const now = start - 20 * 60_000;

describe("computeStarOverloads", () => {
  const six = ["A", "B", "C", "D", "E", "F"].map((name, index) => flight(name, "ERNOV", 16 + index));

  it("flags more than five aircraft to one STAR in a bucket", () => {
    const result = computeStarOverloads(six, buckets, now);
    expect(result.get(buckets[1].start)?.flights).toHaveLength(6);
  });
  it("does not flag five aircraft or aircraft spread over STARs", () => {
    expect(computeStarOverloads(six.slice(0, 5), buckets, now).size).toBe(0);
    expect(computeStarOverloads(six.map((item, index) => ({...item, star_family: index % 2 ? "TESPI" : "ERNOV"})), buckets, now).size).toBe(0);
  });
  it("ignores aircraft less than 15 minutes from the feeder fix", () => {
    expect(computeStarOverloads(six, buckets, start + 10 * 60_000).size).toBe(0);
  });
  it("stays erased by STAR and bucket", () => {
    expect(computeStarOverloads(six, buckets, now, new Set([starOverloadKey("ERNOV", buckets[1].start)])).size).toBe(0);
  });
});
