import { describe, expect, it } from "vitest";
import { Bay } from "@/api/models";
import { getStripBg } from "./types";

describe("direction-based strip paper color", () => {
  it("keeps arrivals yellow in departure-format bays", () => {
    expect(getStripBg("NONE", true, Bay.Depart)).toBe("var(--color-strip-arr-bg)");
  });

  it("keeps departures blue in arrival-format bays", () => {
    expect(getStripBg("NONE", false, Bay.Final)).toBe("var(--color-strip-dep-bg)");
  });

  it("preserves the existing PDC override in clearance bays", () => {
    expect(getStripBg("CLEARED", false, Bay.Cleared)).toBe("var(--color-pdc-cleared)");
  });
});
