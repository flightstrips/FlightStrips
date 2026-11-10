import type { FrontendStandAssignmentEntry, FrontendStandBlockEntry } from "@/api/models";

export interface EstStandBlockingState {
  blocked: Record<string, true>;
  reasons: Record<string, string>;
}

export const DEFAULT_BLOCKED_EST_STANDS = ["C10", "C26", "B3", "B5", "B16"] as const;
export const DEFAULT_BLOCKED_EST_STAND_SET: ReadonlySet<string> = new Set(DEFAULT_BLOCKED_EST_STANDS);
const DEFAULT_BLOCK_REASON = "Not available for arrival stand assignment";

const addReason = (reasons: Record<string, string>, stand: string, reason: string) => {
  reasons[stand] = reasons[stand] ? `${reasons[stand]}; ${reason}` : reason;
};

export function deriveEstStandBlocking(
  assignments: FrontendStandAssignmentEntry[],
  blocks: FrontendStandBlockEntry[],
): EstStandBlockingState {
  const blocked: Record<string, true> = {};
  const reasons: Record<string, string> = {};

  for (const stand of DEFAULT_BLOCKED_EST_STANDS) {
    blocked[stand] = true;
    addReason(reasons, stand, DEFAULT_BLOCK_REASON);
  }

  for (const block of blocks) {
    blocked[block.stand] = true;
    const cause = block.callsign ? ` by ${block.callsign}` : "";
    const directReason = block.reason ?? `${block.block_type.replace(/_/g, " ")}${cause}`;
    addReason(reasons, block.stand, directReason);

    for (const neighbor of block.blocks ?? []) {
      blocked[neighbor] = true;
      const adjacencyReason = `Blocked by manual block ${block.stand}${block.reason ? `: ${block.reason}` : ""}`;
      addReason(reasons, neighbor, adjacencyReason);
    }
  }

  for (const assignment of assignments) {
    for (const neighbor of assignment.blocks ?? []) {
      blocked[neighbor] = true;
      addReason(reasons, neighbor, `Blocked by ${assignment.stand} (${assignment.callsign})`);
    }
  }

  return { blocked, reasons };
}
