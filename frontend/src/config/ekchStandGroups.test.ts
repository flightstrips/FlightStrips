import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { GEGW_APRON_STAND_GROUPS, isGegwApronStand, shouldShowInGegwApronBay } from "./ekchStandGroups";

interface StandAssignmentConfig {
  stand_groups: Record<string, string[]>;
}

describe("GE/GW apron stand groups", () => {
  it("matches the operational groups in the backend stand configuration", () => {
    const configPath = resolve(process.cwd(), "../backend/config/ekch/airline_assignment.json");
    const config = JSON.parse(readFileSync(configPath, "utf8")) as StandAssignmentConfig;

    for (const [group, stands] of Object.entries(GEGW_APRON_STAND_GROUPS)) {
      expect([...stands].sort()).toEqual([...config.stand_groups[group]].sort());
    }
  });

  it("recognizes every configured stand and rejects unrelated stands", () => {
    const configuredStands = Object.values(GEGW_APRON_STAND_GROUPS).flat();

    expect(configuredStands.every(isGegwApronStand)).toBe(true);
    expect(isGegwApronStand("A4")).toBe(false);
    expect(isGegwApronStand("AS")).toBe(false);
    expect(isGegwApronStand(undefined)).toBe(false);
  });

  it("limits GE/GW to its configured groups while another apron controller is online", () => {
    expect(shouldShowInGegwApronBay("G110", true, true)).toBe(true);
    expect(shouldShowInGegwApronBay("A4", true, true)).toBe(false);
    expect(shouldShowInGegwApronBay(undefined, true, true)).toBe(false);
    expect(shouldShowInGegwApronBay(undefined, false, true)).toBe(true);
  });

  it("shows all apron aircraft when GE/GW owns the uncovered area", () => {
    expect(shouldShowInGegwApronBay("G110", true, false)).toBe(true);
    expect(shouldShowInGegwApronBay("A4", true, false)).toBe(true);
    expect(shouldShowInGegwApronBay(undefined, true, false)).toBe(true);
  });
});
