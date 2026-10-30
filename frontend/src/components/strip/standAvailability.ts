import { Bay, type FrontendStandAssignmentEntry, type FrontendStandBlockEntry, type FrontendStrip } from "@/api/models";

const OCCUPYING_BAYS = new Set<string>([
  Bay.NotCleared,
  Bay.Cleared,
  Bay.Push,
  Bay.Stand,
]);

export interface StandBadge {
  label: string;
  blocked: boolean;
}

export function deriveStandAvailability(
  strips: Pick<FrontendStrip, "callsign" | "stand" | "bay">[],
  assignments: FrontendStandAssignmentEntry[],
  blocks: FrontendStandBlockEntry[],
  callsign: string,
): Record<string, StandBadge[]> {
  const badges: Record<string, StandBadge[]> = {};
  const addBadge = (stand: string, label: string, blocked: boolean) => {
    if (!stand) return;
    const entries = badges[stand] ?? (badges[stand] = []);
    const existing = entries.find(entry => entry.label === label);
    if (existing) existing.blocked ||= blocked;
    else entries.push({ label, blocked });
  };

  for (const block of blocks) {
    if (block.callsign === callsign) continue;
    for (const stand of [block.stand, ...(block.blocks ?? [])]) {
      addBadge(stand, block.callsign || "BLOCKED", true);
    }
  }

  for (const assignment of assignments) {
    if (assignment.callsign === callsign) continue;
    if (assignment.stage === "CONFIRMED" || assignment.stage === "ASSIGNED" || assignment.stage === "DEPARTURE_BLOCK") {
      addBadge(assignment.stand, assignment.callsign, assignment.stage !== "ASSIGNED");
    }
    for (const neighbor of assignment.blocks ?? []) {
      addBadge(neighbor, assignment.callsign, true);
    }
  }

  for (const strip of strips) {
    if (strip.callsign !== callsign && OCCUPYING_BAYS.has(strip.bay)) {
      addBadge(strip.stand, strip.callsign, true);
    }
  }

  return badges;
}
