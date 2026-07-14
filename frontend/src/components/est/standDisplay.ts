import { Bay, type FrontendStandAssignmentEntry, type FrontendStrip } from "@/api/models";

export interface EstStandDisplay {
  assignmentsByStand: Map<string, FrontendStandAssignmentEntry>;
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
  assignments: FrontendStandAssignmentEntry[],
  satEnabled: boolean,
): EstStandDisplay {
  const assignmentsByStand = new Map<string, FrontendStandAssignmentEntry>();
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

    return { assignmentsByStand, stripsByStand };
  }

  const stripsByCallsign = new Map(strips.map((strip) => [strip.callsign, strip]));
  const assignmentsByCallsign = new Map(assignments.map((assignment) => [assignment.callsign, assignment]));

  for (const assignment of assignments) {
    assignmentsByStand.set(assignment.stand, assignment);

    const strip = stripsByCallsign.get(assignment.callsign);
    if (strip && strip.bay !== Bay.Hidden && strip.bay !== Bay.DepHidden) {
      stripsByStand.set(assignment.stand, strip);
    }
  }

  // A live strip parked at a stand is the operational occupant. It must take
  // display precedence over an arrival that only has a reservation there.
  for (const strip of strips) {
    if (isOccupyingStand(strip)) {
      stripsByStand.set(strip.stand, strip);
    }
  }

  for (const [stand, strip] of stripsByStand) {
    const displayedAssignment = assignmentsByStand.get(stand);
    if (displayedAssignment?.callsign === strip.callsign) {
      continue;
    }

    const occupantAssignment = assignmentsByCallsign.get(strip.callsign);
    if (occupantAssignment?.stand === stand) {
      assignmentsByStand.set(stand, occupantAssignment);
    } else {
      assignmentsByStand.delete(stand);
    }
  }

  return { assignmentsByStand, stripsByStand };
}
