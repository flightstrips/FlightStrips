import { describe, expect, it } from "vitest";
import type { FrontendStandAssignmentEntry } from "@/api/models";
import { getStandAssignmentStyle, SAT_BLOCKED_ORANGE, SAT_CHANGED_YELLOW, SAT_MANUAL_BLUE } from "./standAssignmentStyle";

function assignment(overrides: Partial<FrontendStandAssignmentEntry> = {}): FrontendStandAssignmentEntry {
  return { callsign: "SAS123", stand: "A23", direction: "ARRIVAL", stage: "CONFIRMED", source: "AUTOMATIC", ...overrides };
}

describe("getStandAssignmentStyle", () => {
  it("keeps free automatic assignments transparent", () => {
    expect(getStandAssignmentStyle(assignment()).backgroundColor).toBeUndefined();
  });

  it("renders blocked automatic assignments orange", () => {
    expect(getStandAssignmentStyle(assignment({ blocked_by: ["A22"] })).backgroundColor).toBe(SAT_BLOCKED_ORANGE);
  });

  it("gives changed yellow precedence while preserving manual blue text", () => {
    const style = getStandAssignmentStyle(assignment({ source: "MANUAL_OVERRIDE", manual: true, conflict_reason: "occupied", pending_acknowledgement: true }));
    expect(style.backgroundColor).toBe(SAT_CHANGED_YELLOW);
    expect(style.color).toBe(SAT_MANUAL_BLUE);
  });
});
