import { useState, useEffect } from "react";
import { Bay } from "@/api/models";

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

  const tsat = new Date(strip.tsat);
  const tobt = strip.tobt ? new Date(strip.tobt) : null;
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
