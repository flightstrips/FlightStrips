import type { EcfmpRestriction } from "@/api/models";

export function getMandatoryRouteRestriction(restrictions?: EcfmpRestriction[]): EcfmpRestriction | undefined {
  if (!restrictions) return undefined;
  return restrictions.find((r) => r.type === "mandatory_route");
}

export function getGroundStopRestriction(restrictions?: EcfmpRestriction[]): EcfmpRestriction | undefined {
  if (!restrictions) return undefined;
  return restrictions.find((r) => r.type === "ground_stop");
}

export function getProhibitRestriction(restrictions?: EcfmpRestriction[]): EcfmpRestriction | undefined {
  if (!restrictions) return undefined;
  return restrictions.find((r) => r.type === "prohibit");
}

export function isFlightLevelViolated(
  restrictions: EcfmpRestriction[] | undefined,
  requestedAltitude: number | undefined
): boolean {
  if (!restrictions || requestedAltitude === undefined) return false;
  const prohibit = getProhibitRestriction(restrictions);
  if (!prohibit) return false;

  const fl = Math.floor(requestedAltitude / 100);

  if (prohibit.max_level !== undefined && fl > prohibit.max_level) return true;
  if (prohibit.min_level !== undefined && fl < prohibit.min_level) return true;
  if (prohibit.exact_levels && prohibit.exact_levels.length > 0 && !prohibit.exact_levels.includes(fl)) return true;

  return false;
}

export function getEcfmpNitosRemarks(restrictions: EcfmpRestriction[] | undefined): string[] {
  if (!restrictions || restrictions.length === 0) return [];

  const remarks: string[] = [];

  for (const r of restrictions) {
    switch (r.type) {
      case "mandatory_route":
        remarks.push("NEW ROUTE MANDATED BY ECFMP LOADED");
        break;
      case "ground_stop":
        remarks.push("GROUND STOP ISSUED BY ECFMP. STRICT ADHERANCE TO CTOT REQUIRED!!");
        break;
      case "prohibit":
        if (r.max_level !== undefined) {
          remarks.push(`Max level FL${r.max_level} by ECFMP! Adjust FL accordingly`);
        }
        if (r.min_level !== undefined) {
          remarks.push(`Minlevel FL${r.min_level} by ECFMP! Adjust FL accordingly`);
        }
        if (r.exact_levels && r.exact_levels.length > 0) {
          const levels = r.exact_levels.map((l) => `FL${l}`).join(", ");
          remarks.push(`Only ${levels} accepted by ECFMP! Adjust FL accordingly`);
        }
        break;
    }
  }

  return remarks;
}