/**
 * Parses PDC clearance text produced by buildPDCClearance (Hoppie/CPDLC payload).
 * Strips optional /data2/seq//flags/ wrapper before extracting @-delimited fields.
 */

export type ParsedPdcClearance = {
  /** Short header line (time/date/PDC id) when present */
  headerSummary?: string;
  callsign?: string;
  clearedTo?: string;
  runway?: string;
  heading?: string;
  climbTo?: string;
  vectors?: string;
  sid?: string;
  squawk?: string;
  atis?: string;
  nextFrequency?: string;
  departureFrequency?: string;
  remarks?: string;
};

const HOPPIE_PREFIX = /^\/data2\/\d+\/\/[A-Z]\//;

export function extractPdcPayload(raw: string): string {
  const t = raw.trim();
  if (HOPPIE_PREFIX.test(t)) {
    return t.replace(HOPPIE_PREFIX, "").trim();
  }
  return t;
}

function pick(payload: string, re: RegExp): string | undefined {
  const m = payload.match(re);
  if (!m?.[1]) return undefined;
  const v = m[1].trim();
  return v.length ? v : undefined;
}

/**
 * Best-effort parse; returns partial fields if the template differs.
 */
export function parsePdcClearance(raw: string): ParsedPdcClearance {
  const payload = extractPdcPayload(raw);
  if (!payload) return {};

  let headerSummary: string | undefined;
  const firstAt = payload.indexOf("@");
  if (firstAt > 12) {
    const h = payload.slice(0, firstAt).replace(/\s+/g, " ").trim();
    if (h.length > 10) headerSummary = h.slice(0, 220);
  }

  const callsign =
    pick(payload, /@([A-Z0-9]{2,10})@\s*CLRD TO:/i) ??
    pick(payload, /PDC\s+\d+\s+@([A-Z0-9]{2,10})@/i);

  return {
    headerSummary,
    callsign,
    clearedTo: pick(payload, /CLRD TO:\s*@([^@]+)@/i),
    runway: pick(payload, /RWY:\s*@([^@]+)@/i),
    heading: pick(payload, /HDG:\s*@([^@]+)@/i),
    climbTo: pick(payload, /CLIMB TO:\s*@([^@]+)@/i),
    vectors: pick(payload, /VECTORS:\s*@([^@]+)@/i),
    sid: pick(payload, /SID:\s*@([^@]+)@/i),
    squawk: pick(payload, /SQK:\s*@([^@]+)@/i),
    atis: pick(payload, /ATIS\s*@([^@]+)@/i),
    nextFrequency: pick(payload, /NEXT FRQ:\s*@([^@]+)@/i),
    departureFrequency: pick(
      payload,
      /Departure frequency:\s*@([^@]+)@/i
    ),
    remarks: pick(
      payload,
      /Check charts for confirmation\.\s*@([^@]*)@/i
    ),
  };
}

export function hasParsedRows(p: ParsedPdcClearance): boolean {
  return Object.values(p).some(
    (v) => v !== undefined && String(v).length > 0
  );
}
