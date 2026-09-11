import {useState} from "react";

import type {AMANAircraftTargetField, AMANAircraftTargetOptionalField} from "./AMANAircraftTarget";

export type AMANAircraftTargetSide = "feeder" | "runway";

export interface AMANAircraftTargetPreferences {
  version: 1;
  feeder: AMANAircraftTargetOptionalField[];
  runway: AMANAircraftTargetOptionalField[];
}

export const AMAN_TARGET_PREFERENCES_KEY = "flightstrips.aman.target-fields.v1";
export const AMAN_TARGET_FIELD_LABELS: Record<AMANAircraftTargetOptionalField, string> = {
  "feeder-fix-eta": "Feeder-fix ETA",
  "total-delay": "Total delay",
  runway: "Runway",
  wtc: "WTC",
  "aircraft-type": "Aircraft type",
  "feeder-fix": "Feeder fix",
};

export function defaultAMANAircraftTargetPreferences(): AMANAircraftTargetPreferences {
  return {version: 1, feeder: [], runway: []};
}

function normalizeOrder(value: unknown): AMANAircraftTargetOptionalField[] {
  if (!Array.isArray(value)) return [];
  return value.filter((id, index): id is AMANAircraftTargetOptionalField =>
    typeof id === "string" && id in AMAN_TARGET_FIELD_LABELS && value.indexOf(id) === index);
}

export function readAMANAircraftTargetPreferences(storage: Pick<Storage, "getItem"> = localStorage): AMANAircraftTargetPreferences {
  try {
    const value = JSON.parse(storage.getItem(AMAN_TARGET_PREFERENCES_KEY) ?? "null") as Record<string, unknown> | null;
    if (value?.version !== 1) return defaultAMANAircraftTargetPreferences();
    return {version: 1, feeder: normalizeOrder(value.feeder), runway: normalizeOrder(value.runway)};
  } catch {
    return defaultAMANAircraftTargetPreferences();
  }
}

export function useAMANAircraftTargetPreferences(storage: Storage = localStorage) {
  const [preferences, setPreferences] = useState(() => readAMANAircraftTargetPreferences(storage));

  function update(next: AMANAircraftTargetPreferences) {
    setPreferences(next);
    try {
      storage.setItem(AMAN_TARGET_PREFERENCES_KEY, JSON.stringify(next));
    } catch {
      // Browser privacy/quota restrictions must not break the local controls.
    }
  }

  return [preferences, update] as const;
}

export function fieldsForAMANAircraftTargetSide(
  fields: readonly AMANAircraftTargetField[],
  preferences: AMANAircraftTargetPreferences,
  side: AMANAircraftTargetSide,
): AMANAircraftTargetField[] {
  const byID = new Map(fields.map((field) => [field.id, field]));
  return preferences[side].flatMap((id) => byID.get(id) ?? []);
}
