import { Bay, type FrontendStandAssignmentEntry, type FrontendStrip } from "@/api/models";

export interface EstStandDisplay {
  stripsByStand: Map<string, FrontendStrip>;
  plannedArrivalsByStand: Map<string, string>;
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

const INBOUND_BAYS = new Set<string>([Bay.Final, Bay.RwyArr, Bay.TwyArr, Bay.ArrHidden]);

export function deriveEstStandDisplay(
  strips: FrontendStrip[],
  assignments: FrontendStandAssignmentEntry[],
  satEnabled: boolean,
): EstStandDisplay {
  const stripsByStand = new Map<string, FrontendStrip>();
  const plannedArrivalsByStand = new Map<string, string>();

  if (!satEnabled) {
    for (const strip of strips) {
      if (strip.stand && strip.bay !== Bay.Hidden && strip.bay !== Bay.HiddenDep && !INBOUND_BAYS.has(strip.bay)) {
        stripsByStand.set(strip.stand, strip);
      }
    }
  }

  for (const strip of strips) {
    if (isOccupyingStand(strip)) {
      stripsByStand.set(strip.stand, strip);
    }
  }

  const occupyingCallsigns = new Set([...stripsByStand.values()].map((strip) => strip.callsign));
  if (satEnabled) {
    for (const assignment of assignments) {
      if (assignment.direction === "ARRIVAL" && ["ESTIMATED", "ASSIGNED", "CONFIRMED"].includes(assignment.stage) &&
          assignment.stand && !stripsByStand.has(assignment.stand) &&
          !occupyingCallsigns.has(assignment.callsign) && !plannedArrivalsByStand.has(assignment.stand)) {
        plannedArrivalsByStand.set(assignment.stand, assignment.callsign);
      }
    }
  }
  const assignedCallsigns = new Set(assignments.map((assignment) => assignment.callsign));
  for (const strip of strips) {
    if (strip.stand && INBOUND_BAYS.has(strip.bay) && !stripsByStand.has(strip.stand) &&
        !plannedArrivalsByStand.has(strip.stand) && (!satEnabled || !assignedCallsigns.has(strip.callsign))) {
      plannedArrivalsByStand.set(strip.stand, strip.callsign);
    }
  }

  return { stripsByStand, plannedArrivalsByStand };
}
