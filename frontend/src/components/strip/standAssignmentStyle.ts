import type { FrontendStandAssignmentEntry } from "@/api/models";

export const SAT_CHANGED_YELLOW = "#FFF500";
export const SAT_BLOCKED_ORANGE = "#DD6A12";
export const SAT_MANUAL_BLUE = "#0D4259";

export function getStandAssignmentStyle(assignment?: FrontendStandAssignmentEntry) {
  // Pending manual assignments take visual priority over conflicts.
  // Automatic assignments still retain their acknowledgement state.
  const changed = assignment?.pending_acknowledgement === true;
  const blocked = Boolean(assignment?.conflict_reason || assignment?.blocked_by?.length);
  const manual = assignment?.manual === true || assignment?.source.toUpperCase().includes("MANUAL");
  return {
    backgroundColor: changed && manual ? SAT_CHANGED_YELLOW : blocked ? SAT_BLOCKED_ORANGE : undefined,
    color: manual ? SAT_MANUAL_BLUE : undefined,
    changed,
    blocked,
  };
}
