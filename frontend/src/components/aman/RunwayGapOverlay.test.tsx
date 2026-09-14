import {fireEvent, render, screen} from "@testing-library/react";
import {describe, expect, it, vi} from "vitest";

import {RunwayGapOverlay} from "./RunwayGapOverlay";

describe("runway GAP overlay", () => {
  it("maps a future interval upward and presents the design's paired red bars", () => {
    const onRemove = vi.fn();
    render(<div className="relative h-[900px]">
      <RunwayGapOverlay
        gaps={[{id: "gap-1", start: "2026-07-22T10:05:00.000Z", end: "2026-07-22T10:10:00.000Z", label: "spacing", created_at: "2026-07-22T10:00:00.000Z", created_by: "fmp"}]}
        onRemove={onRemove}
        range={{startMs: Date.parse("2026-07-22T10:00:00.000Z"), endMs: Date.parse("2026-07-22T10:30:00.000Z")}}
        runway="ARRIVAL-22"
      />
    </div>);

    const gap = screen.getByRole("button", {name: /GAP ARRIVAL-22/});
    expect(Number.parseFloat(gap.style.top)).toBeCloseTo(66.6667, 3);
    expect(Number.parseFloat(gap.style.height)).toBeCloseTo(16.6667, 3);
    expect(gap.querySelectorAll("span")).toHaveLength(2);
    expect(Array.from(gap.children).every((bar) => bar.classList.contains("bg-[#b90000]"))).toBe(true);
    fireEvent.click(gap);
    expect(onRemove).toHaveBeenCalledWith(expect.objectContaining({id: "gap-1"}), "ARRIVAL-22");
  });
});
