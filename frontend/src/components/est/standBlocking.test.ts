import { describe, expect, it } from "vitest";

import type { FrontendStandArrivalRestrictionEntry } from "@/api/models";
import { deriveEstStandBlocking } from "./standBlocking";

describe("deriveEstStandBlocking", () => {
  it("marks manual-block neighbors with an explanation", () => {
    const state = deriveEstStandBlocking([], [
      {
        id: 7,
        stand: "A1",
        block_type: "MANUAL",
        blocks: ["A2"],
        reason: "Marshaller closed",
        version: 1,
      },
    ]);

    expect(state.blocked).toMatchObject({ A1: true, A2: true });
    expect(state.reasons.A1).toBe("Marshaller closed");
    expect(state.reasons.A2).toBe("Blocked by manual block A1: Marshaller closed");
  });

  it("displays backend-provided arrival restrictions and reasons", () => {
    const restrictions: FrontendStandArrivalRestrictionEntry[] = ["C10", "C26", "B3", "B5", "B16"].map(stand => ({
      stand,
      reason: "Not available for arrival stand assignment",
    }));
    const state = deriveEstStandBlocking([], [], restrictions);

    expect(state.blocked).toMatchObject({ C10: true, C26: true, B3: true, B5: true, B16: true });
    for (const stand of ["C10", "C26", "B3", "B5", "B16"]) {
      expect(state.reasons[stand]).toBe("Not available for arrival stand assignment");
    }
  });

  it("combines assignment and manual-block adjacency reasons", () => {
    const state = deriveEstStandBlocking(
      [{ callsign: "SAS123", stand: "A3", direction: "ARRIVAL", stage: "ASSIGNED", source: "AUTOMATIC", blocks: ["A2"] }],
      [{ stand: "A1", block_type: "MANUAL", blocks: ["A2"] }],
    );

    expect(state.reasons.A2).toBe("Blocked by manual block A1; Blocked by A3 (SAS123)");
  });
});
