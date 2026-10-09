import type { FrontendStrip } from "@/api/models";

export type StripSortMode = {
  key: string;
  label: string;
  compareFn: (a: FrontendStrip, b: FrontendStrip) => number;
};

// Alphabetical compare that always places empty values last; ties fall back to callsign.
const byText = (get: (s: FrontendStrip) => string | undefined) => (a: FrontendStrip, b: FrontendStrip) => {
  const x = (get(a) ?? "").trim();
  const y = (get(b) ?? "").trim();
  if (!x !== !y) return x ? -1 : 1;
  return x.localeCompare(y, undefined, { sensitivity: "base", numeric: true }) || a.callsign.localeCompare(b.callsign);
};

export const arrivalSortModes: StripSortMode[] = [
  { key: "ETA", label: "ETA", compareFn: byText((s) => s.eldt) },
  { key: "CALLSIGN", label: "CALLSIGN", compareFn: byText((s) => s.callsign) },
  { key: "ADEP", label: "ADEP", compareFn: byText((s) => s.origin) },
];

export const startupSortModes: StripSortMode[] = [
  { key: "EOBT", label: "EOBT", compareFn: byText((s) => s.eobt) },
  { key: "CALLSIGN", label: "CALLSIGN", compareFn: byText((s) => s.callsign) },
  { key: "ADES", label: "ADES", compareFn: byText((s) => s.destination) },
];

export const plannedDepartureSortModes: StripSortMode[] = [
  { key: "CALLSIGN", label: "CALLSIGN", compareFn: byText((s) => s.callsign) },
  { key: "EOBT", label: "EOBT", compareFn: byText((s) => s.eobt) },
  { key: "ADES", label: "ADES", compareFn: byText((s) => s.destination) },
];
