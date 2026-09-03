import { describe, expect, it } from "vitest";
import { getHalfStripFrameColor } from "./halfStripFrame";

describe("compact special-strip frames", () => {
  it("uses the issue-defined crossing and land/start colors instead of cyan", () => {
    expect(getHalfStripFrameColor("CROSSING", false)).toBe("#FCC800");
    expect(getHalfStripFrameColor("LAND-START", false)).toBe("#DD6A12");
  });

  it("frames compact messages with their own strip color", () => {
    expect(getHalfStripFrameColor("MESSAGES", false)).toBe("#285A5C");
  });

  it("keeps ordinary compact strips tied to flight direction", () => {
    expect(getHalfStripFrameColor("APN-PUSH", false)).toBe("var(--color-strip-frame)");
    expect(getHalfStripFrameColor("APN-ARR", true)).toBe("var(--color-cell-border-arr)");
  });
});
