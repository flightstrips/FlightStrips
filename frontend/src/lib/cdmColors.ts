import { Bay } from "@/api/models";

const MINUTE_MS = 60_000;

// ── Shared colour tokens ────────────────────────────────────────────────────
export const CDM_GREEN  = "#16a34a";
export const CDM_YELLOW = "#F3EA1F";
export const CDM_RED    = "#dc2626";
export const CDM_ORANGE = "#DD6A12";
export const CTOT_YELLOW = "#F3EA1F";
export const CTOT_BLUE   = "#00008B";

// ── HHMM parser ────────────────────────────────────────────────────────────

/** Parse a HHMM string into epoch ms relative to refMs, rolling over midnight if needed. */
function parseHHMM(hhmm: string, refMs: number): number {
  const h = parseInt(hhmm.slice(0, 2), 10);
  const m = parseInt(hhmm.slice(2, 4), 10);
  const d = new Date(refMs);
  d.setUTCHours(h, m, 0, 0);
  if (refMs - d.getTime() > 12 * 60 * 60 * 1000) d.setUTCDate(d.getUTCDate() + 1);
  return d.getTime();
}

// ── TOBT / TSAT ────────────────────────────────────────────────────────────

export interface CDMColors {
  tobtBg: string;
  tsatBg: string;
}

/**
 * Compute TOBT/TSAT cell background colours.
 * Returns empty string (no colour) when a cell should be transparent.
 *
 * Rules (first match wins):
 *   - Not in CLEARED/PUSH bay, or no tsat → ""  /  ""
 *   - now > TSAT + 5 min                  → red / ""
 *   - now > TSAT + 4 min                  → green / yellow
 *   - now >= TSAT - 5 min                 → green / green
 *   - |TOBT - TSAT| > 5 min              → orange / ""
 *   - else                                → "" / ""
 */
export function computeCDMColors(
  tsat: string,
  tobt: string,
  nowMs: number,
  _bay?: Bay,
): CDMColors {
  if (!tsat) return { tobtBg: "", tsatBg: "" };

  const tsatMs = parseHHMM(tsat, nowMs);
  const tobtMs = tobt ? parseHHMM(tobt, nowMs) : null;
  const diffMs = nowMs - tsatMs;

  if (diffMs > 5 * MINUTE_MS)  return { tobtBg: CDM_RED,    tsatBg: ""          };
  if (diffMs > 4 * MINUTE_MS)  return { tobtBg: CDM_GREEN,  tsatBg: CDM_YELLOW  };
  if (diffMs > -5 * MINUTE_MS) return { tobtBg: CDM_GREEN,  tsatBg: CDM_GREEN   };
  if (tobtMs !== null && Math.abs(tsatMs - tobtMs) > 5 * MINUTE_MS)
    return { tobtBg: CDM_ORANGE, tsatBg: "" };
  return { tobtBg: "", tsatBg: "" };
}

// ── CTOT ───────────────────────────────────────────────────────────────────

export interface CTOTColors {
  ctotBg: string;
  ctotColor: string;
  showCtot: boolean;
}

/**
 * Compute CTOT cell background colour.
 *
 * Rules (first match wins):
 *   - empty ctot                        → transparent, hidden
 *   - now < CTOT - 5 min               → yellow, black text
 *   - now <= CTOT + 10 min             → dark blue, white text
 *   - now > CTOT + 10 min              → transparent, hidden
 */
export function computeCTOTColors(ctot: string, nowMs: number): CTOTColors {
  if (!ctot) return { ctotBg: "", ctotColor: "black", showCtot: false };

  const ctotMs = parseHHMM(ctot, nowMs);
  const diffMs = nowMs - ctotMs;

  if (diffMs < -5 * MINUTE_MS)  return { ctotBg: CTOT_YELLOW, ctotColor: "black", showCtot: true };
  if (diffMs <= 10 * MINUTE_MS) return { ctotBg: CTOT_BLUE,   ctotColor: "white", showCtot: true };
  return { ctotBg: "", ctotColor: "black", showCtot: false };
}
