import { describe, expect, it } from "vitest";

import { Bay, type FrontendStandAssignmentEntry, type FrontendStrip } from "@/api/models";
import { deriveEstStandDisplay } from "./standDisplay";

function strip(callsign: string, stand: string, bay: Bay): FrontendStrip {
  return { callsign, stand, bay } as FrontendStrip;
}

function assignment(callsign: string, stand: string): FrontendStandAssignmentEntry {
  return {
    callsign,
    stand,
    direction: "ARRIVAL",
    stage: "ASSIGNED",
    source: "AUTOMATIC",
  };
}

describe("deriveEstStandDisplay", () => {
  it.each([Bay.NotCleared, Bay.Cleared, Bay.Push, Bay.Stand])(
    "shows the aircraft occupying the stand in %s instead of an inbound assignment",
    (bay) => {
      const inbound = strip("SAS100", "A18", Bay.Final);
      const occupant = strip("NAX200", "A18", bay);

      const display = deriveEstStandDisplay(
        [inbound, occupant],
        [assignment(inbound.callsign, "A18")],
        true,
      );

      expect(display.stripsByStand.get("A18")?.callsign).toBe("NAX200");
      expect(display.assignmentsByStand.has("A18")).toBe(false);
    },
  );

  it("does not let another inbound strip override the assigned arrival", () => {
    const assignedInbound = strip("SAS100", "A18", Bay.Final);
    const otherInbound = strip("NAX200", "A18", Bay.Final);
    const assigned = assignment(assignedInbound.callsign, "A18");

    const display = deriveEstStandDisplay([assignedInbound, otherInbound], [assigned], true);

    expect(display.stripsByStand.get("A18")?.callsign).toBe("SAS100");
    expect(display.assignmentsByStand.get("A18")).toBe(assigned);
  });

  it("retains the occupant's assignment metadata when it matches the displayed stand", () => {
    const occupant = strip("NAX200", "A18", Bay.Cleared);
    const occupantAssignment = assignment(occupant.callsign, "A18");

    const display = deriveEstStandDisplay([occupant], [occupantAssignment], true);

    expect(display.stripsByStand.get("A18")).toBe(occupant);
    expect(display.assignmentsByStand.get("A18")).toBe(occupantAssignment);
  });

  it("preserves ordinary strip-based stand display when SAT is disabled", () => {
    const occupant = strip("NAX200", "A18", Bay.Cleared);
    const inbound = strip("SAS100", "A18", Bay.Final);

    const display = deriveEstStandDisplay([occupant, inbound], [], false);

    expect(display.stripsByStand.get("A18")).toBe(occupant);
    expect(display.assignmentsByStand.size).toBe(0);
  });
});
