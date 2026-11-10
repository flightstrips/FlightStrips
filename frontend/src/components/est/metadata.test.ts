import { describe, expect, it } from "vitest";
import { EST_CELL_HEIGHT, EST_STANDS } from "./metadata";

describe("EST stand layout", () => {
  it("places C10 directly above C26", () => {
    const c10 = EST_STANDS.find(({ label }) => label === "C10");
    const c26 = EST_STANDS.find(({ label }) => label === "C26");

    if (!c10 || !c26) throw new Error("Expected C10 and C26 in the EST layout");

    expect(c10.left).toBe(c26.left);
    expect(c10.top + EST_CELL_HEIGHT).toBeLessThan(c26.top);
  });
});
