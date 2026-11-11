import { describe, expect, it } from "vitest";
import { getAircraftMapRestrictions, isRestrictedMapPoint, isRestrictedTwrPoint } from "./aircraftMapRestrictions";

describe("aircraft map restrictions", () => {
  it("uses the CAT-D release-point restrictions for listed aircraft", () => {
    const restrictions = getAircraftMapRestrictions("B752/H", "H", "pushback");

    expect(isRestrictedMapPoint("K1", restrictions)).toBe(true);
    expect(isRestrictedMapPoint("Z4", restrictions)).toBe(false);
    expect(isRestrictedMapPoint("J4", restrictions)).toBe(true);
  });

  it("adds the H and named-group release-point restrictions", () => {
    const hRestrictions = getAircraftMapRestrictions("B738", "H", "pushback");
    const namedGroupRestrictions = getAircraftMapRestrictions("B77W", "H", "pushback");

    for (const restrictions of [hRestrictions, namedGroupRestrictions]) {
      expect(isRestrictedMapPoint("K1", restrictions)).toBe(true);
      expect(isRestrictedMapPoint("Z4", restrictions)).toBe(true);
      expect(isRestrictedMapPoint("T4", restrictions)).toBe(false);
    }
  });

  it("keeps Light and Medium at the default and limits WTC/J per map", () => {
    const medium = getAircraftMapRestrictions("B738", "M", "apron-taxi");
    const jPushback = getAircraftMapRestrictions("B738", "J", "pushback");
    const jApronTaxi = getAircraftMapRestrictions("B738", "J", "apron-taxi");
    const jTwrTaxi = getAircraftMapRestrictions("B738", "J", "twr-taxi");

    expect(isRestrictedMapPoint("K1", medium)).toBe(false);
    expect(isRestrictedMapPoint("K1", jPushback)).toBe(true);
    expect(isRestrictedMapPoint("T4", jPushback)).toBe(false);
    expect(isRestrictedMapPoint("F", jApronTaxi)).toBe(true);
    expect(isRestrictedMapPoint("Z/D", jApronTaxi)).toBe(false);
    expect(isRestrictedMapPoint("B/Z", jTwrTaxi)).toBe(false);
    expect(isRestrictedMapPoint("V2", jTwrTaxi)).toBe(true);
  });

  it("uses the specific taxi restrictions for named aircraft groups", () => {
    const a359 = getAircraftMapRestrictions("A359", "H", "twr-taxi");
    const a35k = getAircraftMapRestrictions("A35K", "H", "twr-taxi");
    const a345 = getAircraftMapRestrictions("A345", "H", "twr-taxi");
    const a35kApron = getAircraftMapRestrictions("B773", "H", "apron-taxi");
    const a345Apron = getAircraftMapRestrictions("B779", "H", "apron-taxi");

    expect(isRestrictedMapPoint("A3", a359)).toBe(true);
    expect(isRestrictedMapPoint("A5", a359)).toBe(true);
    expect(isRestrictedMapPoint("B2", a359)).toBe(true);
    expect(isRestrictedMapPoint("C/30", a35k)).toBe(true);
    expect(isRestrictedMapPoint("B3", a345)).toBe(true);
    expect(isRestrictedMapPoint("F", a35kApron)).toBe(false);
    expect(isRestrictedMapPoint("K1", a35kApron)).toBe(true);
    expect(isRestrictedMapPoint("F", a345Apron)).toBe(false);
  });

  it("adds APRON restrictions to TWR hold points while retaining TWR-specific rules", () => {
    expect(isRestrictedTwrPoint("K/J", "hp", "B752", "H")).toBe(true);
    expect(isRestrictedTwrPoint("A4", "hp", "B752", "H")).toBe(true);
    expect(isRestrictedTwrPoint("A5", "hp", "A359", "H")).toBe(true);
    expect(isRestrictedTwrPoint("C", "hp", "A35K", "H")).toBe(true);
    expect(isRestrictedTwrPoint("F", "hp", "A35K", "H")).toBe(false);
    expect(isRestrictedTwrPoint("C", "cl", "A35K", "H")).toBe(false);
    expect(isRestrictedTwrPoint("V2", "hp", "B738", "J")).toBe(true);
    expect(isRestrictedTwrPoint("B/Z", "hp", "B738", "J")).toBe(false);
    expect(isRestrictedTwrPoint("F1", "hp", "B738", "J")).toBe(true);
    expect(isRestrictedTwrPoint("F2", "hp", "B738", "J")).toBe(true);
    expect(isRestrictedTwrPoint("F1", "hp", "B738", "H")).toBe(false);
    expect(isRestrictedTwrPoint("F2", "hp", "B738", "H")).toBe(false);
    expect(isRestrictedTwrPoint("F1", "hp", "A35K", "H")).toBe(false);
    expect(isRestrictedTwrPoint("F2", "hp", "A35K", "H")).toBe(false);
  });

  it("allows A1 and E1 holding points for A345-family aircraft only", () => {
    for (const aircraftType of ["A345", "A346", "B778", "B779"]) {
      expect(isRestrictedTwrPoint("A1", "hp", aircraftType, "H")).toBe(false);
      expect(isRestrictedTwrPoint("E1", "hp", aircraftType, "H")).toBe(false);
    }

    expect(isRestrictedTwrPoint("A1", "hp", "A35K", "H")).toBe(true);
    expect(isRestrictedTwrPoint("E1", "hp", "A35K", "H")).toBe(true);
  });

  it("allows B1 for all aircraft and B3 except for A345-family aircraft", () => {
    const aircraftTypes = ["B738", "B752", "A359", "A35K", "A345", "A346", "B778", "B779"];
    for (const aircraftType of aircraftTypes) {
      expect(isRestrictedTwrPoint("B1", "hp", aircraftType, "H")).toBe(false);
    }

    expect(isRestrictedTwrPoint("B1", "hp", "B738", "J")).toBe(false);
    for (const aircraftType of ["B738", "B752", "A359", "A35K"]) {
      expect(isRestrictedTwrPoint("B3", "hp", aircraftType, "H")).toBe(false);
    }
    expect(isRestrictedTwrPoint("B3", "hp", "B738", "J")).toBe(false);
    for (const aircraftType of ["A345", "A346", "B778", "B779"]) {
      expect(isRestrictedTwrPoint("B3", "hp", aircraftType, "H")).toBe(true);
    }
  });
});
