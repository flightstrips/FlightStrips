import { Bay } from "@/api/models";
import { normalizeCdmTime } from "@/lib/cdmTime";

const MINUTE_MS = 60_000;

// ── Shared colour tokens ────────────────────────────────────────────────────
export const CDM_GREEN  = "#00FF26";
export const CDM_YELLOW = "#F3EA1F";
export const CDM_RED    = "#dc2626";
export const CDM_ORANGE = "#DD6A12";
export const CTOT_YELLOW = "#F3EA1F";
export const CTOT_BLUE   = "#00008B";
export const TWY_DEP_CTOT_ORANGE = "#DD6A12";
export const TWY_DEP_CTOT_BLUE   = "#131376";
export const TWY_DEP_CTOT_YELLOW = "#FFF500";
export const TWY_DEP_CTOT_RED    = "#FF0000";

// ── HHMM parser ────────────────────────────────────────────────────────────

/**
 * Parse a normalized HHMM string into epoch ms relative to refMs, rolling over
 * midnight if needed. Callers are expected to pass valid HHMM values.
 */
function parseHHMM(hhmm: string, refMs: number): number {
  const normalizedTime = normalizeCdmTime(hhmm);
  const h = parseInt(normalizedTime.slice(0, 2), 10);
  const m = parseInt(normalizedTime.slice(2, 4), 10);
  const d = new Date(refMs);
  d.setUTCHours(h, m, 0, 0);
  const offsetMs = d.getTime() - refMs;
  if (offsetMs <= -12 * 60 * 60 * 1000) d.setUTCDate(d.getUTCDate() + 1);
  if (offsetMs > 12 * 60 * 60 * 1000) d.setUTCDate(d.getUTCDate() - 1);
  return d.getTime();
}

export function isTsatWithinStartRequestWindow(tsat: string, nowMs: number): boolean {
  const normalizedTsat = normalizeCdmTime(tsat);
  if (!normalizedTsat) return false;

  return Math.abs(parseHHMM(normalizedTsat, nowMs) - nowMs) < 6 * MINUTE_MS;
}

// ── TOBT / TSAT ────────────────────────────────────────────────────────────

export interface CDMColors {
  tobtBg: string;
  tsatBg: string;
}

export function hasManualTobtSource(tobtSetBy?: string): boolean {
	return Boolean(tobtSetBy?.trim());
}

export function isTobtRelevant(bay?: Bay | string): boolean {
  return bay === Bay.NotCleared || bay === Bay.Cleared || bay === Bay.Push;
}

/**
 * Compute TOBT/TSAT cell background colours.
 * Returns empty string (no colour) when a cell should be transparent.
 *
 * Planned, taxiing, and inbound strips remain neutral.
 * Cleared TOBT is red only while it differs from TSAT by more than five minutes.
 * TSAT is colored according to its start-request window.
 */
export function computeCDMColors(
  tsat: string,
  tobt: string,
  nowMs: number,
  bay?: Bay,
  phase?: string,
): CDMColors {
  if (!isTobtRelevant(bay) || bay === Bay.NotCleared) return { tobtBg: "", tsatBg: "" };
  const normalizedTsat = normalizeCdmTime(tsat);
  const normalizedTobt = normalizeCdmTime(tobt);

  if (bay !== Bay.Cleared) return { tobtBg: "", tsatBg: "" };

  const tobtOutsideLimits = !!normalizedTsat && !!normalizedTobt &&
    Math.abs(parseHHMM(normalizedTsat, nowMs) - parseHHMM(normalizedTobt, nowMs)) > 5 * MINUTE_MS;
  const tobtBg = tobtOutsideLimits ? CDM_RED : "";

  if (phase === "I" || !normalizedTsat) return { tobtBg, tsatBg: "" };

  const diffMs = nowMs - parseHHMM(normalizedTsat, nowMs);
  if (diffMs > 5 * MINUTE_MS) return { tobtBg, tsatBg: "" };
  if (diffMs > 4 * MINUTE_MS) return { tobtBg, tsatBg: CDM_YELLOW };
  if (diffMs > -5 * MINUTE_MS) return { tobtBg, tsatBg: CDM_GREEN };
  return { tobtBg, tsatBg: "" };
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
  const normalizedCtot = normalizeCdmTime(ctot);

  if (!normalizedCtot) return { ctotBg: "", ctotColor: "black", showCtot: false };

  const ctotMs = parseHHMM(normalizedCtot, nowMs);
  const diffMs = nowMs - ctotMs;

  if (diffMs < -5 * MINUTE_MS)  return { ctotBg: CTOT_YELLOW, ctotColor: "black", showCtot: true };
  if (diffMs <= 10 * MINUTE_MS) return { ctotBg: CTOT_BLUE,   ctotColor: "white", showCtot: true };
  return { ctotBg: "", ctotColor: "black", showCtot: false };
}

/**
 * Compute the CTOT colours used by the TE/TW departure strips.
 *
 * The operational CTOT window opens five minutes before CTOT. It turns yellow
 * at CTOT + 9 minutes and red at CTOT + 11 minutes.
 */
export function computeTwyDepCTOTColors(ctot: string, nowMs: number): CTOTColors {
  const normalizedCtot = normalizeCdmTime(ctot);

  if (!normalizedCtot) return { ctotBg: "", ctotColor: "black", showCtot: false };

  const ctotMs = parseHHMM(normalizedCtot, nowMs);
  const diffMs = nowMs - ctotMs;

  if (diffMs < -5 * MINUTE_MS) return { ctotBg: TWY_DEP_CTOT_ORANGE, ctotColor: "black", showCtot: true };
  if (diffMs < 9 * MINUTE_MS)  return { ctotBg: TWY_DEP_CTOT_BLUE,   ctotColor: "white", showCtot: true };
  if (diffMs < 11 * MINUTE_MS) return { ctotBg: TWY_DEP_CTOT_YELLOW, ctotColor: "black", showCtot: true };
  return { ctotBg: TWY_DEP_CTOT_RED, ctotColor: "black", showCtot: true };
}
