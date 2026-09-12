import type {AMANDataStatus, AMANRunwayClosure} from "@/api/aman";
import type {AMANTimelineRange} from "./presentation";

export function RunwayClosureOverlay({closures, range, runway, status}: {closures: AMANRunwayClosure[]; range: AMANTimelineRange; runway: string; status: AMANDataStatus}) {
  return closures.map((closure) => {
    const start = Date.parse(closure.start), end = closure.end === null ? range.endMs : Date.parse(closure.end);
    if (!Number.isFinite(start) || !Number.isFinite(end) || end <= range.startMs || start >= range.endMs) return null;
    const top = (Math.max(start, range.startMs) - range.startMs) / (range.endMs - range.startMs) * 100;
    const bottom = (Math.min(end, range.endMs) - range.startMs) / (range.endMs - range.startMs) * 100;
    const termination = closure.end === null ? "indefinite until removed" : `until ${closure.end}, end exclusive`;
    return <div
      aria-label={`RUNWAY CLOSED ${runway}: ${closure.reason}, from ${closure.start} ${termination}${status === "fresh" ? "" : `; ${status} state`}`}
      className="pointer-events-none absolute inset-x-1 z-10 overflow-hidden border-y-4 border-double border-red-200 bg-[repeating-linear-gradient(135deg,rgba(127,29,29,.88)_0,rgba(127,29,29,.88)_8px,rgba(239,68,68,.72)_8px,rgba(239,68,68,.72)_16px)] px-2 text-center text-[10px] font-black tracking-widest text-white"
      data-closure-id={closure.id} key={closure.id} role="note"
      style={{height: `${Math.max(0.8, bottom - top)}%`, top: `${top}%`}}
    >⛔ RWY CLOSED · {runway} · {closure.end === null ? "INDEFINITE" : closure.reason}{status === "fresh" ? "" : ` · ${status.toUpperCase()}`}</div>;
  });
}
