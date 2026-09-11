/* eslint-disable react-refresh/only-export-components -- reusable axis model and hook intentionally share this module */
import {useEffect, useMemo, useState} from "react";

import type {AMANDataStatus} from "@/api/aman";
import {cn} from "@/lib/utils";
import type {AMANTimelineRange} from "./presentation";

export const AMAN_HORIZON_PREFERENCE_KEY = "flightstrips.aman.timeline-horizon.v1";
export const AMAN_DEFAULT_HORIZON_MINUTES = 30;
export const AMAN_CLOCK_STEP_MS = 6_000;
const MINIMUM_HORIZON_MINUTES = 30;
const MAXIMUM_HORIZON_MINUTES = 90;
const ONE_MINUTE_MS = 60_000;

export function validateAMANHorizon(value: unknown): number {
  const parsed = typeof value === "number" ? value : Number.NaN;
  if (!Number.isFinite(parsed)) return AMAN_DEFAULT_HORIZON_MINUTES;
  return Math.min(MAXIMUM_HORIZON_MINUTES, Math.max(MINIMUM_HORIZON_MINUTES, Math.round(parsed)));
}

export function readAMANHorizon(storage: Pick<Storage, "getItem"> = localStorage): number {
  try {
    const saved = JSON.parse(storage.getItem(AMAN_HORIZON_PREFERENCE_KEY) ?? "null") as {minutes?: unknown} | null;
    return validateAMANHorizon(saved?.minutes);
  } catch {
    return AMAN_DEFAULT_HORIZON_MINUTES;
  }
}

export function writeAMANHorizon(minutes: number, storage: Pick<Storage, "setItem"> = localStorage): void {
  storage.setItem(AMAN_HORIZON_PREFERENCE_KEY, JSON.stringify({version: 1, minutes: validateAMANHorizon(minutes)}));
}

export function buildAMANAxisRange(clockMs: number, horizonMinutes: number): AMANTimelineRange {
  return {startMs: clockMs, endMs: clockMs + validateAMANHorizon(horizonMinutes) * 60_000};
}

/** Later UTC times run upward, matching the operational MAESTRO ruler. */
export function AMANAxisTopPercent(timestamp: string | number | null, range: AMANTimelineRange): number | null {
  const value = typeof timestamp === "number" ? timestamp : timestamp === null ? Number.NaN : Date.parse(timestamp);
  if (!Number.isFinite(value) || range.endMs <= range.startMs) return null;
  return 100 - Math.max(0, Math.min(1, (value - range.startMs) / (range.endMs - range.startMs))) * 100;
}

export function formatAMANAxisLabel(timestampMs: number, referenceMs: number): string {
  const timestamp = new Date(timestampMs);
  const reference = new Date(referenceMs);
  const label = `${String(timestamp.getUTCHours()).padStart(2, "0")}:${String(timestamp.getUTCMinutes()).padStart(2, "0")}`;
  const day = Date.UTC(timestamp.getUTCFullYear(), timestamp.getUTCMonth(), timestamp.getUTCDate());
  const referenceDay = Date.UTC(reference.getUTCFullYear(), reference.getUTCMonth(), reference.getUTCDate());
  const offset = Math.round((day - referenceDay) / 86_400_000);
  return offset === 0 ? label : `${label} ${offset > 0 ? "+" : ""}${offset}d`;
}

export function useAMANTimelineAxis(authoritativeTime: string) {
  const authoritativeMs = Date.parse(authoritativeTime);
  const [horizonMinutes] = useState(readAMANHorizon);
  const [clock, setClock] = useState({anchorMs: authoritativeMs, valueMs: authoritativeMs});

  useEffect(() => {
    const receivedAt = Date.now();
    const timer = window.setInterval(() => {
      const elapsedSteps = Math.floor((Date.now() - receivedAt) / AMAN_CLOCK_STEP_MS);
      setClock({anchorMs: authoritativeMs, valueMs: authoritativeMs + elapsedSteps * AMAN_CLOCK_STEP_MS});
    }, AMAN_CLOCK_STEP_MS);
    return () => window.clearInterval(timer);
  }, [authoritativeMs]);

  const clockMs = clock.anchorMs === authoritativeMs ? clock.valueMs : authoritativeMs;
  return useMemo(
    () => ({clockMs, horizonMinutes, range: buildAMANAxisRange(authoritativeMs, horizonMinutes)}),
    [authoritativeMs, clockMs, horizonMinutes],
  );
}

export function AMANTimelineAxis({range, clockMs, status}: {range: AMANTimelineRange; clockMs: number; status: AMANDataStatus}) {
  const firstTick = Math.ceil(range.startMs / ONE_MINUTE_MS) * ONE_MINUTE_MS;
  const ticks = Array.from(
    {length: Math.max(0, Math.floor((range.endMs - firstTick) / ONE_MINUTE_MS) + 1)},
    (_, index) => firstTick + index * ONE_MINUTE_MS,
  );
  const finalRegionBottom = AMANAxisTopPercent(range.endMs - 10 * 60_000, range) ?? 0;

  return (
    <div
      aria-label={`UTC timeline, ${formatAMANAxisLabel(range.startMs, range.startMs)} to ${formatAMANAxisLabel(range.endMs, range.startMs)}, ${status}`}
      className="absolute inset-y-0 left-1/2 w-[58px] -translate-x-1/2 overflow-visible border border-[#d8d8d8]"
      data-status={status}
      role="img"
    >
      <div className="absolute inset-x-0 top-0 border-b border-amber-300/80 bg-amber-300/15" data-testid="final-ten-minute-region" style={{height: `${finalRegionBottom}%`}}>
        <span className="absolute right-full top-1 whitespace-nowrap pr-1 text-[9px] font-semibold text-amber-200">FINAL 10</span>
      </div>
      <div className="absolute inset-x-0 bottom-0 bg-[#3a3a3a]" style={{top: `${AMANAxisTopPercent(clockMs, range) ?? 100}%`}} />
      <div className="absolute inset-x-0 z-10 -translate-y-1/2 border-t border-dashed border-white" data-testid="aman-visual-clock" style={{top: `${AMANAxisTopPercent(clockMs, range) ?? 100}%`}} />
      {ticks.map((timeMs) => {
        const major = new Date(timeMs).getUTCMinutes() % 5 === 0;
        return <div className="absolute inset-x-0 z-[1] -translate-y-1/2" data-major={major} data-testid="aman-axis-tick" key={timeMs} style={{top: `${AMANAxisTopPercent(timeMs, range)}%`}}>
          <span className={cn("absolute left-0 border-t border-[#d8d8d8]", major ? "w-3" : "w-1.5")} />
          <span className={cn("absolute right-0 border-t border-[#d8d8d8]", major ? "w-3" : "w-1.5")} />
          {major && <span className="absolute left-1/2 -translate-x-1/2 -translate-y-1/2 bg-[#505052] px-0.5 font-mono text-[11px] font-semibold text-white">{formatAMANAxisLabel(timeMs, range.startMs)}</span>}
        </div>;
      })}
      {status !== "fresh" && <span className={cn("absolute bottom-1 left-1/2 z-20 -translate-x-1/2 text-[8px] font-bold uppercase", status === "disconnected" ? "text-red-200" : "text-amber-200")}>{status}</span>}
    </div>
  );
}
