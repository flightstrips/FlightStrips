import {describe, expect, it} from "vitest";
import {normalizeCdmTime} from "./cdmTime";

describe("UTC CDM clocks", () => {
  it("formats explicit instants in UTC across midnight", () => {
    expect(normalizeCdmTime("2026-10-03T00:05:00Z")).toBe("0005");
    expect(normalizeCdmTime("2026-10-03T01:05:00+02:00")).toBe("2305");
  });
  it("preserves compact times, empty values and status labels", () => {
    expect(normalizeCdmTime("5")).toBe("0005");
    expect(normalizeCdmTime(undefined)).toBe("");
    expect(normalizeCdmTime("REA")).toBe("REA");
  });
});
