import type { FrontendStandAssignmentEntry, FrontendStandBlockEntry } from "@/api/models";

export interface EstStandBlockingState {
  blocked: Record<string, true>;
  reasons: Record<string, string>;
}

const addReason = (reasons: Record<string, string>, stand: string, reason: string) => {
  reasons[stand] = reasons[stand] ? `${reasons[stand]}; ${reason}` : reason;
};

export function deriveEstStandBlocking(
  assignments: FrontendStandAssignmentEntry[],
  blocks: FrontendStandBlockEntry[],
): EstStandBlockingState {
  const blocked: Record<string, true> = {};
  const reasons: Record<string, string> = {};

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
