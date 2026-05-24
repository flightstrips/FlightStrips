import { describe, expect, it } from "vitest";

import { hasManualTobtSource } from "./cdmColors";

describe("hasManualTobtSource", () => {
  it("returns true for pilot-filed TOBT requests", () => {
    expect(hasManualTobtSource("PILOT", "")).toBe(true);
  });

  it("returns true when a controller has set the TOBT", () => {
    expect(hasManualTobtSource("", "EKCH_DEL")).toBe(true);
  });

  it("returns false for auto-synced TOBT values", () => {
    expect(hasManualTobtSource("", "")).toBe(false);
  });
});
