import type {AMANRunwayGap} from "@/api/aman";
import {AMAN_TIMELINE_RULER_HALF_WIDTH} from "./AMANTimelineAxis";
import type {AMANTimelineRange} from "./presentation";

export function RunwayGapOverlay({gaps, range, runway, disabled = false, onRemove}: {gaps: AMANRunwayGap[]; range: AMANTimelineRange; runway: string; disabled?: boolean; onRemove?: (gap: AMANRunwayGap, runway: string) => void}) {
  return gaps.map((gap) => {
    const startMs = Date.parse(gap.start), endMs = Date.parse(gap.end), span = range.endMs - range.startMs;
    if (!Number.isFinite(startMs) || !Number.isFinite(endMs) || span <= 0 || endMs <= range.startMs || startMs >= range.endMs) return null;
    const visibleStart = Math.max(startMs, range.startMs);
    const visibleEnd = Math.min(endMs, range.endMs);
    const top = (range.endMs - visibleEnd) / span * 100;
    const height = (visibleEnd - visibleStart) / span * 100;
    return <button
      aria-label={`GAP ${runway}: ${gap.label}, ${gap.start} to ${gap.end}, end exclusive`}
      className="absolute left-1/2 z-10 -translate-x-1/2 appearance-none border-0 bg-transparent p-0 focus-visible:outline focus-visible:outline-2 focus-visible:outline-white disabled:pointer-events-none"
      data-gap-id={gap.id}
      disabled={disabled || onRemove === undefined}
      key={gap.id}
      onClick={() => onRemove?.(gap, runway)}
      // Scale Figma's 10px bars to the 46px ruler, leaving a 3px gutter on each side.
      style={{height: `${height}%`, top: `${top}%`, width: AMAN_TIMELINE_RULER_HALF_WIDTH * 2 + 18}}
      title={`GAP · ${runway} · ${gap.label}`}
      type="button"
    >
      <span aria-hidden="true" className="absolute inset-y-0 left-0 w-1.5 bg-[#9c0000]" />
      <span aria-hidden="true" className="absolute inset-y-0 right-0 w-1.5 bg-[#9c0000]" />
    </button>;
  });
}
