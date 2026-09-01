import { Bay, type FrontendStandAssignmentEntry, type FrontendStrip } from "@/api/models";

export interface EstStandDisplay {
  stripsByStand: Map<string, FrontendStrip>;
}

const OCCUPYING_BAYS = new Set<string>([
  Bay.NotCleared,
  Bay.Cleared,
  Bay.Push,
  Bay.Stand,
]);

function isOccupyingStand(strip: FrontendStrip) {
  return !!strip.stand && OCCUPYING_BAYS.has(strip.bay);
}

export function deriveEstStandDisplay(
  strips: FrontendStrip[],
  _assignments: FrontendStandAssignmentEntry[],
  satEnabled: boolean,
): EstStandDisplay {
  const stripsByStand = new Map<string, FrontendStrip>();

  if (!satEnabled) {
    for (const strip of strips) {
      if (strip.stand && strip.bay !== Bay.Hidden) {
        stripsByStand.set(strip.stand, strip);
      }
    }

    for (const strip of strips) {
      if (isOccupyingStand(strip)) {
        stripsByStand.set(strip.stand, strip);
      }
    }

    return { stripsByStand };
  }

  // SAT assignments affect allocation and blocking, but are not operational
  // EST display content. Only aircraft physically occupying a stand appear.
  for (const strip of strips) {
    if (isOccupyingStand(strip)) {
      stripsByStand.set(strip.stand, strip);
    }
  }

  return { stripsByStand };
}
