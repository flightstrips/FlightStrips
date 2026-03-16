import { useState, useEffect } from "react";

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

export function useCTOTColor(ctot: string): { ctotBg: string; ctotColor: string; showCtot: boolean } {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    const interval = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(interval);
  }, []);

  if (!ctot) {
    return { ctotBg: "transparent", ctotColor: "black", showCtot: false };
  }

  const ctotTime = parseHHMM(ctot, now);
  const diffMs = now.getTime() - ctotTime.getTime(); // positive = past CTOT

  if (diffMs < -5 * 60 * 1000) {
    return { ctotBg: "yellow", ctotColor: "black", showCtot: true };
  }
  if (diffMs <= 10 * 60 * 1000) {
    return { ctotBg: "#00008B", ctotColor: "white", showCtot: true };
  }
  return { ctotBg: "transparent", ctotColor: "black", showCtot: false };
}
