import type {AMANRunwayGap} from "@/api/aman";
import type {AMANTimelineRange} from "./presentation";

export function RunwayGapOverlay({gaps, range, runway}: {gaps: AMANRunwayGap[]; range: AMANTimelineRange; runway: string}) {
  return gaps.map((gap) => {
    const startMs = Date.parse(gap.start), endMs = Date.parse(gap.end), span = range.endMs - range.startMs;
    if (endMs <= range.startMs || startMs >= range.endMs) return null;
    const top = (Math.max(startMs, range.startMs) - range.startMs) / span * 100;
    const bottom = (Math.min(endMs, range.endMs) - range.startMs) / span * 100;
    return <div
      aria-label={`GAP ${runway}: ${gap.label}, ${gap.start} to ${gap.end}, end exclusive`}
      className="pointer-events-none absolute inset-x-2 z-10 overflow-hidden border-y-2 border-dashed border-amber-200 bg-amber-950/70 px-2 text-center text-[10px] font-bold tracking-wide text-amber-100"
      data-gap-id={gap.id}
      key={gap.id}
      role="note"
      style={{height: `${Math.max(0.4, bottom - top)}%`, top: `${top}%`}}
    >GAP · {runway} · {gap.label}</div>;
  });
}
