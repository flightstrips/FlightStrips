/**
 * EKCH stand groups handled by GE/GW while apron is staffed separately.
 *
 * Keep these values aligned with the corresponding stand_groups entries in
 * backend/config/ekch/airline_assignment.json. The accompanying test guards
 * against configuration drift.
 */
export const GEGW_APRON_STAND_GROUPS = {
  APRONWEST: ["W1", "RI", "RII", "RIII"],
  APRONSOUTH: [
    "262", "273-1", "273", "145", "144", "243", "253", "117", "276",
    "259", "105", "106", "303", "302", "104", "142", "141",
  ],
  CARGO: [
    "G110", "G111", "G112", "G113", "G114", "G117", "G118", "G119",
    "G120", "G121", "G122", "G123", "G124", "G125", "G126", "G127",
    "G128", "G129", "G130", "G131", "G132", "G133", "G134", "G135",
    "G136", "G137",
  ],
  GOLFEAST: ["G15", "G16", "G17", "G18", "G19"],
} as const;

const GEGW_APRON_STANDS = new Set<string>(Object.values(GEGW_APRON_STAND_GROUPS).flat());

export function isGegwApronStand(stand: string | null | undefined): boolean {
  return stand != null && GEGW_APRON_STANDS.has(stand);
}

export function shouldShowInGegwApronBay(
  stand: string | null | undefined,
  flightStrip: boolean,
  apronOnline: boolean,
): boolean {
  return !apronOnline || !flightStrip || isGegwApronStand(stand);
}
