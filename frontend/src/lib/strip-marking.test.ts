import { describe, expect, it, vi } from "vitest";
import { effectiveStripMarked, toggleEffectiveStripMark } from "./strip-marking";

describe("effective strip marking", () => {
  it("uses the local override for rendering, including false over a shared mark", () => {
    expect(effectiveStripMarked(false, true)).toBe(true);
    expect(effectiveStripMarked(true, false)).toBe(false);
    expect(effectiveStripMarked(true, undefined)).toBe(true);
  });

  it("toggles an existing local override without publishing a shared mark", () => {
    const setLocalMarked = vi.fn();
    const toggleMarked = vi.fn();

    toggleEffectiveStripMark("SAS123", false, true, setLocalMarked, toggleMarked);

    expect(setLocalMarked).toHaveBeenCalledWith("SAS123", false);
    expect(toggleMarked).not.toHaveBeenCalled();
  });

  it("toggles a false local override back on without publishing the server mark", () => {
    const setLocalMarked = vi.fn();
    const toggleMarked = vi.fn();

    toggleEffectiveStripMark("SAS123", true, false, setLocalMarked, toggleMarked);

    expect(setLocalMarked).toHaveBeenCalledWith("SAS123", true);
    expect(toggleMarked).not.toHaveBeenCalled();
  });

  it("uses the shared mark action when there is no local override", () => {
    const setLocalMarked = vi.fn();
    const toggleMarked = vi.fn();

    toggleEffectiveStripMark("SAS123", true, undefined, setLocalMarked, toggleMarked);

    expect(toggleMarked).toHaveBeenCalledWith("SAS123", false);
    expect(setLocalMarked).not.toHaveBeenCalled();
  });
});
