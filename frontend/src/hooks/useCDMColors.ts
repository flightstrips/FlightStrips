import { useState, useEffect } from "react";
import { Bay } from "@/api/models";

/** Parse a HHMM string into a Date on the same UTC day as `ref`, rolling over midnight if needed. */
function parseHHMM(hhmm: string, ref: Date): Date {
  const h = parseInt(hhmm.slice(0, 2), 10);
  const m = parseInt(hhmm.slice(2, 4), 10);
  const d = new Date(ref);
  d.setUTCHours(h, m, 0, 0);
  // If the result is more than 12 hours in the past, it's next day
  if (ref.getTime() - d.getTime() > 12 * 60 * 60 * 1000) d.setUTCDate(d.getUTCDate() + 1);
  return d;
}

interface CDMColorInput {
  bay: Bay;
  tsat: string;
  tobt: string;
}

export function useCDMColors(strip: CDMColorInput): { tobtBg: string; tsatBg: string } {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    const interval = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(interval);
  }, []);

  const inCdmBay = strip.bay === Bay.Cleared || strip.bay === Bay.Push;
  if (!inCdmBay || !strip.tsat) {
    return { tobtBg: "transparent", tsatBg: "transparent" };
  }

  const tsat = parseHHMM(strip.tsat, now);
  const tobt = strip.tobt ? parseHHMM(strip.tobt, now) : null;
  const diffMs = now.getTime() - tsat.getTime(); // positive = past TSAT

  if (diffMs > 5 * 60 * 1000) {
    return { tobtBg: "red", tsatBg: "transparent" };
  }
  if (diffMs > 4 * 60 * 1000) {
    return { tobtBg: "green", tsatBg: "yellow" };
  }
  if (diffMs > -5 * 60 * 1000) {
    return { tobtBg: "green", tsatBg: "green" };
  }
  // More than 5 min before TSAT
  if (tobt && Math.abs(tobt.getTime() - tsat.getTime()) > 5 * 60 * 1000) {
    return { tobtBg: "orange", tsatBg: "transparent" };
  }
  return { tobtBg: "transparent", tsatBg: "transparent" };
}
