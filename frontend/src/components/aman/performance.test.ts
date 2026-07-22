import {describe, expect, it} from "vitest";

import {percentile95} from "@/lib/aman-performance";

describe("AMAN paint metrics", () => {
  it("reports p95 deterministically for repeated full replacement samples", () => {
    expect(percentile95([])).toBeNull();
    expect(percentile95(Array.from({length: 20}, (_, index) => index + 1))).toBe(19);
  });
});
