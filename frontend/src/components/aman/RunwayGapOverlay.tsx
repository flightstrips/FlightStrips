import type {AMANRunwayGap} from "@/api/aman";
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
      className="absolute left-1/2 z-10 w-[66px] -translate-x-1/2 appearance-none border-0 bg-transparent p-0 disabled:pointer-events-none"
      data-gap-id={gap.id}
      disabled={disabled || onRemove === undefined}
      key={gap.id}
      onClick={() => onRemove?.(gap, runway)}
      style={{height: `${Math.max(0.4, height)}%`, top: `${top}%`}}
      title={`GAP · ${runway} · ${gap.label}`}
      type="button"
    >
      <span aria-hidden="true" className="absolute inset-y-0 left-0 w-2 bg-[#b90000]" />
      <span aria-hidden="true" className="absolute inset-y-0 right-0 w-2 bg-[#b90000]" />
    </button>;
  });
}
