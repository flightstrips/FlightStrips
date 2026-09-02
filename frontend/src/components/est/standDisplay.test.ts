import { describe, expect, it } from "vitest";

import { Bay, type FrontendStandAssignmentEntry, type FrontendStrip } from "@/api/models";
import { deriveEstStandDisplay } from "./standDisplay";

function strip(callsign: string, stand: string, bay: Bay): FrontendStrip {
  return { callsign, stand, bay } as FrontendStrip;
}

function assignment(
  callsign: string,
  stand: string,
  overrides: Partial<FrontendStandAssignmentEntry> = {},
): FrontendStandAssignmentEntry {
  return {
    callsign,
    stand,
    direction: "ARRIVAL",
    stage: "ASSIGNED",
    source: "AUTOMATIC",
    ...overrides,
  };
}

describe("deriveEstStandDisplay", () => {
  it.each([Bay.NotCleared, Bay.Cleared, Bay.Push, Bay.Stand])(
    "shows the aircraft occupying the stand in %s instead of an inbound assignment",
    (bay) => {
      const inbound = strip("SAS100", "A18", Bay.Final);
      const occupant = strip("NAX200", "A18", bay);

      const display = deriveEstStandDisplay([inbound, occupant], [assignment(inbound.callsign, "A18")], true);

      expect(display.stripsByStand.get("A18")?.callsign).toBe("NAX200");
    },
  );

  it("does not show inbound stand assignments before the aircraft occupies the stand", () => {
    const assignedInbound = strip("SAS100", "A18", Bay.Final);
    const display = deriveEstStandDisplay([assignedInbound], [assignment(assignedInbound.callsign, "A18")], true);

    expect(display.stripsByStand.has("A18")).toBe(false);
  });

  it("shows an arrival once it reaches the stand", () => {
    const arrival = strip("NAX200", "A18", Bay.Stand);
    const display = deriveEstStandDisplay([arrival], [assignment(arrival.callsign, "A18")], true);

    expect(display.stripsByStand.get("A18")).toBe(arrival);
  });

  it("does not display a departure reservation alongside a different physical stand", () => {
    const departure = strip("SAS300", "A2", Bay.NotCleared);
    const reservation = assignment(departure.callsign, "A1", {
      direction: "DEPARTURE",
      stage: "RESERVED",
    });

    const display = deriveEstStandDisplay([departure], [reservation], true);

    expect(display.stripsByStand.has("A1")).toBe(false);
    expect(display.stripsByStand.get("A2")).toBe(departure);
  });

  it.each(["VOC4000", "BTI3EL"])("renders %s once at its observed stand during a mismatch", (callsign) => {
    const occupant = strip(callsign, "A19", Bay.Stand);
    const staleAssignment = assignment(callsign, "A18");

    const display = deriveEstStandDisplay([occupant], [staleAssignment], true);

    expect(display.stripsByStand.get("A19")).toBe(occupant);
    expect(display.stripsByStand.has("A18")).toBe(false);
    expect([...display.stripsByStand.values()].filter((value) => value.callsign === callsign)).toHaveLength(1);
  });

  it("preserves ordinary strip-based stand display when SAT is disabled", () => {
    const occupant = strip("NAX200", "A18", Bay.Cleared);
    const inbound = strip("SAS100", "A18", Bay.Final);

    const display = deriveEstStandDisplay([occupant, inbound], [], false);

    expect(display.stripsByStand.get("A18")).toBe(occupant);
  });
});
