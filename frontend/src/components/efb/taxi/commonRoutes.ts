// Standard taxi routes offered in the EFB, keyed by runway in use and holding point.
// Selecting one fills the clearance box; the pilot still has to read back what ATC actually said.
export interface CommonRoute {
  /** Runway in use for departure, as written on the strip (e.g. "22R"). */
  runway: string;
  /** Holding point the route ends at (e.g. "A4"). */
  holdingPoint: string;
  /** Clearance in the EFB syntax, e.g. "KZAx30A4.22R". */
  clearance: string;
  /** Only offer the route from these release points; omit to offer it from any. */
  releasePoints?: string[];
  /** Optional short note shown under the route. */
  note?: string;
}

// To be filled in per runway and holding point.
export const EKCH_COMMON_ROUTES: CommonRoute[] = [];

export function commonRoutesFor(runway: string, releasePoint: string, routes: CommonRoute[] = EKCH_COMMON_ROUTES): CommonRoute[] {
  const rwy = runway.trim().toUpperCase(), rp = releasePoint.trim().toUpperCase();
  return routes.filter((route) => route.runway.toUpperCase() === rwy && (!route.releasePoints || !rp || route.releasePoints.includes(rp)));
}
