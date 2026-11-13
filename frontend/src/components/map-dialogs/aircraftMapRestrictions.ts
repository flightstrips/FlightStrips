export type AircraftMap = "pushback" | "apron-taxi" | "twr-taxi";

export interface AircraftMapRestrictions {
  blockedLabels: ReadonlySet<string>;
  allowedLabels?: ReadonlySet<string>;
}

const CAT_D_TYPES = new Set([
  "B752", "B753", "B762", "B763", "A30B", "A306", "A3ST", "A400",
  "A310", "DC10", "L101", "T204", "IL62", "IL76", "IL86", "AN22",
]);
const A359_TYPES = new Set(["A359", "B78X"]);
const A35K_TYPES = new Set(["A35K", "B773", "B77W"]);
const A345_TYPES = new Set(["A345", "A346", "B778", "B779"]);

const CAT_D_PUSHBACK = new Set([
  "K1", "Z1", "Z2", "Z3", "J1", "J2", "J3", "J4", "A50", "M1",
  "R1", "S1", "U1", "W3", "F89", "T3", "T5", "W1", "R3", "S3",
]);
const ADDITIONAL_PUSHBACK = new Set(["Z4", "Z5", "Y1", "Y2", "Y0", "L2", "L3"]);
const WTC_J_PUSHBACK = new Set(["B", "V3", "V4", "T4"]);

const CAT_D_APRON_TAXI = new Set(["K1", "K/J"]);
const ADDITIONAL_APRON_TAXI = new Set(["Y/A", "Y/M", "Y/L", "L/Y", "K3"]);
const A35K_APRON_TAXI = new Set([
  "F", "Z/D", "Y/Q", "Q/Y", "D", "Y/V", "B/Y", "B", "R/V", "R/W",
  "S/W", "S/V", "W/S", "V/S", "DE-ICE V", "V2", "V/T", "T/S", "V1",
]);
const WTC_J_APRON_TAXI = new Set(["Z/D", "D", "B", "R/V", "S/W", "S/V", "V2", "V1", "V/T", "T/S"]);

const CAT_D_TWR_TAXI = new Set(["A4", "LINE 1", "LINE 3"]);
const ADDITIONAL_TWR_H = new Set(["A3"]);
const ADDITIONAL_TWR_A359 = new Set(["A5", "B2"]);
const ADDITIONAL_TWR_A35K = new Set(["C/30"]);
const ADDITIONAL_TWR_A345 = new Set(["B3"]);
const WTC_J_TWR_TAXI = new Set([
  "B/Z", "B/30", "B/C", "C/D", "D/30", "30/D", "D/A", "A/D", "A/A1",
  "LINE 2", "B1", "B3",
]);

type AircraftProfile = "default" | "cat-d" | "a359" | "a35k" | "a345" | "h" | "j";

function getAircraftProfile(aircraftType: string | undefined, aircraftCategory: string | undefined): AircraftProfile {
  const type = (aircraftType ?? "").split("/")[0].trim().toUpperCase();
  if (CAT_D_TYPES.has(type)) return "cat-d";
  if (A359_TYPES.has(type)) return "a359";
  if (A35K_TYPES.has(type)) return "a35k";
  if (A345_TYPES.has(type)) return "a345";

  const category = (aircraftCategory ?? "").trim().toUpperCase();
  if (category === "J") return "j";
  if (category === "H") return "h";
  return "default";
}

const unrestricted: AircraftMapRestrictions = { blockedLabels: new Set() };

export function getAircraftMapRestrictions(
  aircraftType: string | undefined,
  aircraftCategory: string | undefined,
  map: AircraftMap,
): AircraftMapRestrictions {
  const profile = getAircraftProfile(aircraftType, aircraftCategory);
  if (profile === "default") return unrestricted;

  if (profile === "j") {
    const allowedLabels = map === "pushback"
      ? WTC_J_PUSHBACK
      : map === "apron-taxi"
        ? WTC_J_APRON_TAXI
        : WTC_J_TWR_TAXI;
    return { blockedLabels: new Set(), allowedLabels };
  }

  if (map === "pushback") {
    if (profile === "cat-d") return { blockedLabels: CAT_D_PUSHBACK };
    return { blockedLabels: new Set([...CAT_D_PUSHBACK, ...ADDITIONAL_PUSHBACK]) };
  }

  if (map === "apron-taxi") {
    if (profile === "cat-d") return { blockedLabels: CAT_D_APRON_TAXI };
    if (profile === "a35k" || profile === "a345") return { blockedLabels: new Set(), allowedLabels: A35K_APRON_TAXI };
    return { blockedLabels: new Set([...CAT_D_APRON_TAXI, ...ADDITIONAL_APRON_TAXI]) };
  }

  const blockedLabels = new Set([...CAT_D_APRON_TAXI, ...CAT_D_TWR_TAXI]);
  if (profile !== "cat-d") {
    for (const label of ADDITIONAL_APRON_TAXI) blockedLabels.add(label);
    for (const label of ADDITIONAL_TWR_H) blockedLabels.add(label);
  }
  if (profile === "a359") for (const label of ADDITIONAL_TWR_A359) blockedLabels.add(label);
  if (profile === "a35k") for (const label of ADDITIONAL_TWR_A35K) blockedLabels.add(label);
  if (profile === "a345") for (const label of ADDITIONAL_TWR_A345) blockedLabels.add(label);
  return { blockedLabels };
}

export function isRestrictedMapPoint(label: string, restrictions: AircraftMapRestrictions): boolean {
  return restrictions.blockedLabels.has(label)
    || (restrictions.allowedLabels !== undefined && !restrictions.allowedLabels.has(label));
}

export function isRestrictedTwrPoint(
  label: string,
  pointType: "hp" | "cl" | undefined,
  aircraftType: string | undefined,
  aircraftCategory: string | undefined,
): boolean {
  const profile = getAircraftProfile(aircraftType, aircraftCategory);
  if (pointType === "hp" && label === "B1") return false;
  if (pointType === "hp" && label === "B3") return profile === "a345";

  const twrRestrictions = getAircraftMapRestrictions(aircraftType, aircraftCategory, "twr-taxi");
  if (isRestrictedMapPoint(label, twrRestrictions)) return true;

  if (pointType !== "hp" || profile === "j") return false;
  if (label === "F1" || label === "F2") return false;
  if (profile === "a345" && (label === "A1" || label === "E1")) return false;
  const apronRestrictions = getAircraftMapRestrictions(aircraftType, aircraftCategory, "apron-taxi");
  return isRestrictedMapPoint(label, apronRestrictions);
}
