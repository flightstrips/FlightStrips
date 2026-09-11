import type {AMANState, AMANWarning} from "@/api/aman";

export interface AMANCurrentWarnings {
  items: AMANWarning[];
  snapshot: "available" | "omitted";
}

const severityOrder: Record<AMANWarning["severity"], number> = {
  error: 0,
  warning: 1,
};

/** Builds the read-only current view from one complete AMAN replacement. */
export function currentWarningsFromAMANState(state: AMANState | null): AMANCurrentWarnings {
  if (state?.warnings === undefined) return {items: [], snapshot: "omitted"};

  const unique = new Map<string, AMANWarning>();
  for (const warning of state.warnings) {
    if (!unique.has(warning.id)) unique.set(warning.id, warning);
  }

  return {
    items: [...unique.values()].sort((left, right) =>
      severityOrder[left.severity] - severityOrder[right.severity]
      || left.id.localeCompare(right.id)),
    snapshot: "available",
  };
}
