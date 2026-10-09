import { isFlight, type AnyStrip } from "@/api/models";
import { normalizeCdmTime } from "@/lib/cdmTime";

function tsatMinutes(tsat: string, now: Date): number | null {
  const normalized = normalizeCdmTime(tsat);
  if (!/^\d{4}$/.test(normalized)) return null;
  const hours = Number(normalized.slice(0, 2));
  const minutes = Number(normalized.slice(2));
  if (hours > 23 || minutes > 59) return null;

  const nowMinutes = now.getUTCHours() * 60 + now.getUTCMinutes();
  const scheduled = hours * 60 + minutes;
  const delta = ((scheduled - nowMinutes + 2160) % 1440) - 720;
  return nowMinutes + delta;
}

/** The earliest TSAT sits nearest the command bar. A live TSAT update reorders the bay. */
export function orderSeqPlnStartup(strips: AnyStrip[], now = new Date()): AnyStrip[] {
  return [...strips].sort((a, b) => {
    const aTime = isFlight(a) ? tsatMinutes(a.tsat, now) : null;
    const bTime = isFlight(b) ? tsatMinutes(b.tsat, now) : null;
    if (aTime === null && bTime !== null) return -1;
    if (bTime === null && aTime !== null) return 1;
    if (aTime !== null && bTime !== null && aTime !== bTime) return bTime - aTime;
    return b.sequence - a.sequence;
  });
}
