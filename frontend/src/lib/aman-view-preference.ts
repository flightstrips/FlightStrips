import type {AMANState} from "@/api/aman";

export const AMAN_ALL_VIEW = "ALL" as const;
export const AMAN_VIEW_PREFERENCE_KEY = "flightstrips.aman.selected-view.v1";
export type AMANView = typeof AMAN_ALL_VIEW | string;

const ACC_POSITION_MAPPINGS = [
  {suffix: "_B_CTR", timeline: 2, side: "left"},
  {suffix: "_D_CTR", timeline: 1, side: "right"},
  {suffix: "_E_CTR", timeline: 1, side: "left"},
  {suffix: "_K_CTR", timeline: 3, side: "left"},
  {suffix: "ESMS_APP", timeline: 2, side: "right"},
] as const;

export function availableAMANViews(state: AMANState | null): Set<string> {
  const result = new Set<string>();
  for (const mapping of state?.timeline_configuration?.mappings ?? []) {
    if (mapping.left !== null) result.add(mapping.left);
    if (mapping.right !== null) result.add(mapping.right);
  }
  return result;
}

export function readAMANViewPreference(storage: Pick<Storage, "getItem"> = localStorage): AMANView | null {
  try {
    const stored = JSON.parse(storage.getItem(AMAN_VIEW_PREFERENCE_KEY) ?? "null") as Record<string, unknown> | null;
    return stored?.version === 1 && typeof stored.view === "string" && stored.view.trim() === stored.view && stored.view !== ""
      ? stored.view
      : null;
  } catch {
    return null;
  }
}

export function resolveAMANView(state: AMANState | null, preferred: AMANView | null, controllerViews: readonly string[] = []): AMANView {
  const available = availableAMANViews(state);
  if (preferred === AMAN_ALL_VIEW || (preferred !== null && available.has(preferred))) return preferred;
  const matches = [...new Set(controllerViews.filter((view) => available.has(view)))];
  return matches.length === 1 ? matches[0] : AMAN_ALL_VIEW;
}

/** Resolve position-owned sides through the versioned backend family mappings. */
export function controllerAMANViews(state: AMANState | null, positions: readonly string[]): string[] {
  const normalized = positions.map((position) => position.trim().toUpperCase());
  return ACC_POSITION_MAPPINGS.flatMap(({suffix, timeline, side}) => {
    if (!normalized.some((position) => position.endsWith(suffix))) return [];
    const mapping = state?.timeline_configuration?.mappings.find(({id}) => id === timeline);
    const family = mapping?.[side] ?? null;
    return family === null ? [] : [family];
  });
}

export function writeAMANViewPreference(view: AMANView, storage: Pick<Storage, "setItem"> = localStorage): void {
  try {
    storage.setItem(AMAN_VIEW_PREFERENCE_KEY, JSON.stringify({version: 1, view}));
  } catch {
    // Browser privacy/quota restrictions must not break local view selection.
  }
}
