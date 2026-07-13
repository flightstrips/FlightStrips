import { describe, expect, it } from "vitest";

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

    expect(state.blocked).toEqual({ A1: true, A2: true });
    expect(state.reasons.A1).toBe("Marshaller closed");
    expect(state.reasons.A2).toBe("Blocked by manual block A1: Marshaller closed");
  });

  it("combines assignment and manual-block adjacency reasons", () => {
    const state = deriveEstStandBlocking(
      [{ callsign: "SAS123", stand: "A3", direction: "ARRIVAL", stage: "ASSIGNED", source: "AUTOMATIC", blocks: ["A2"] }],
      [{ stand: "A1", block_type: "MANUAL", blocks: ["A2"] }],
    );

    expect(state.reasons.A2).toBe("Blocked by manual block A1; Blocked by A3 (SAS123)");
  });
});
