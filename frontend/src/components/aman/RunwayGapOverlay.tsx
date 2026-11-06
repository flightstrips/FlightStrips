import type {AMANRunwayGap} from "@/api/aman";
import {AMAN_TIMELINE_RULER_HALF_WIDTH} from "./AMANTimelineAxis";
import type {AMANTimelineRange} from "./presentation";

export function RunwayGapOverlay({gaps, range, runway, side = "both", disabled = false, onRemove}: {gaps: AMANRunwayGap[]; range: AMANTimelineRange; runway: string; side?: "left" | "right" | "both"; disabled?: boolean; onRemove?: (gap: AMANRunwayGap, runway: string) => void}) {
  return gaps.map((gap) => {
    const startMs = Date.parse(gap.start), endMs = Date.parse(gap.end), span = range.endMs - range.startMs;
    if (!Number.isFinite(startMs) || !Number.isFinite(endMs) || endMs <= startMs || span <= 0 || endMs <= range.startMs || startMs >= range.endMs) return null;
    const visibleStart = Math.max(startMs, range.startMs);
    const visibleEnd = Math.min(endMs, range.endMs);
    const top = (range.endMs - visibleEnd) / span * 100;
    const height = (visibleEnd - visibleStart) / span * 100;
    return <button
      aria-label={`GAP ${runway}: ${gap.label}, ${gap.start} to ${gap.end}, end exclusive`}
      className="absolute z-30 appearance-none border-0 bg-transparent p-0 before:absolute before:inset-x-0 before:top-1/2 before:h-full before:min-h-6 before:-translate-y-1/2 focus-visible:outline focus-visible:outline-2 focus-visible:outline-white disabled:pointer-events-none"
      data-gap-id={gap.id}
      data-gap-side={side}
      disabled={disabled || onRemove === undefined}
      key={gap.id}
      onClick={() => onRemove?.(gap, runway)}
      // Leave a 3px gutter beside the ruler. Paired runway lanes own only their
      // side, so overlapping GAPs cannot conceal or intercept each other.
      style={{height: `${height}%`, top: `${top}%`, width: side === "both" ? AMAN_TIMELINE_RULER_HALF_WIDTH * 2 + 18 : 12, left: side === "right" ? `calc(50% + ${AMAN_TIMELINE_RULER_HALF_WIDTH - 3}px)` : `calc(50% - ${AMAN_TIMELINE_RULER_HALF_WIDTH + 9}px)`}}
      title={`GAP · ${runway} · ${gap.label}${disabled || !onRemove ? "" : " · Click to remove"}`}
      type="button"
    >
      {side !== "right" && <span aria-hidden="true" className="absolute inset-y-0 left-0 w-1.5 bg-[#9c0000]" />}
      {side !== "left" && <span aria-hidden="true" className="absolute inset-y-0 right-0 w-1.5 bg-[#9c0000]" />}
    </button>;
  });
}
