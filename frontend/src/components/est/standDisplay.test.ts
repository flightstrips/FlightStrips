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

  it("shows an inbound assignment separately from physical occupancy", () => {
    const assignedInbound = strip("SAS100", "A18", Bay.Final);
    const display = deriveEstStandDisplay([assignedInbound], [assignment(assignedInbound.callsign, "A18")], true);

    expect(display.stripsByStand.has("A18")).toBe(false);
    expect(display.plannedArrivalsByStand.get("A18")).toBe("SAS100");
  });

  it("shows an arrival once it reaches the stand", () => {
    const arrival = strip("NAX200", "A18", Bay.Stand);
    const display = deriveEstStandDisplay([arrival], [assignment(arrival.callsign, "A18")], true);

    expect(display.stripsByStand.get("A18")).toBe(arrival);
    expect(display.plannedArrivalsByStand.has("A18")).toBe(false);
  });

  it("does not show completed departures as stand occupants", () => {
    const completed = strip("SAS123", "A18", Bay.HiddenDep);

    expect(deriveEstStandDisplay([completed], [], false).stripsByStand.has("A18")).toBe(false);
    expect(deriveEstStandDisplay([completed], [], true).stripsByStand.has("A18")).toBe(false);
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
    expect(display.plannedArrivalsByStand.has("A18")).toBe(false);
    expect([...display.stripsByStand.values()].filter((value) => value.callsign === callsign)).toHaveLength(1);
  });

  it("preserves ordinary strip-based stand display when SAT is disabled", () => {
    const occupant = strip("NAX200", "A18", Bay.Cleared);
    const inbound = strip("SAS100", "A18", Bay.Final);

    const display = deriveEstStandDisplay([occupant, inbound], [], false);

    expect(display.stripsByStand.get("A18")).toBe(occupant);
    expect(display.plannedArrivalsByStand.has("A18")).toBe(false);
  });

  it("shows a strip-based inbound assignment when SAT is disabled", () => {
    const display = deriveEstStandDisplay([strip("SAS100", "A18", Bay.TwyArr)], [], false);
    expect(display.stripsByStand.has("A18")).toBe(false);
    expect(display.plannedArrivalsByStand.get("A18")).toBe("SAS100");
  });

  it("updates and removes planned assignments without retaining stale stands", () => {
    expect(deriveEstStandDisplay([], [assignment("SAS100", "A19")], true).plannedArrivalsByStand.get("A19")).toBe("SAS100");
    expect(deriveEstStandDisplay([], [], true).plannedArrivalsByStand.size).toBe(0);
  });

  it("uses the current assignment instead of an outdated strip stand", () => {
    const display = deriveEstStandDisplay([strip("SAS100", "A18", Bay.Final)], [assignment("SAS100", "A19")], true);
    expect(display.plannedArrivalsByStand.has("A18")).toBe(false);
    expect(display.plannedArrivalsByStand.get("A19")).toBe("SAS100");
  });

  it("uses a strip's inbound stand when there is no SAT assignment", () => {
    const display = deriveEstStandDisplay([strip("SAS100", "A18", Bay.Final)], [], true);
    expect(display.plannedArrivalsByStand.get("A18")).toBe("SAS100");
  });

  it("does not display inactive arrival assignments", () => {
    const display = deriveEstStandDisplay([], [assignment("SAS100", "A18", { stage: "RELEASED" })], true);
    expect(display.plannedArrivalsByStand.size).toBe(0);
  });
});
