import { describe, expect, it } from "vitest";
import { airborneHandoverLabel } from "./airborneHandover";

describe("airborneHandoverLabel", () => {
  it.each([
    ["119.805", "W"],
    ["118.455", "O"],
    ["127.300", "DK"],
    ["124.980", undefined],
    ["120.255", undefined],
    ["120.205", undefined],
    ["", undefined],
    ["EKCH_TWR", undefined],
  ])("maps %s to %s", (frequency, expected) => {
    expect(airborneHandoverLabel(frequency)).toBe(expected);
  });
});
