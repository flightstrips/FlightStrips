import { useState, useEffect } from "react";
import { type Bay } from "@/api/models";
import { computeCDMColors, type CDMColors } from "@/lib/cdmColors";

interface CDMColorInput {
  bay: Bay;
  tsat: string;
  tobt: string;
}

export function useCDMColors(strip: CDMColorInput): CDMColors {
  const [nowMs, setNowMs] = useState(Date.now);

  useEffect(() => {
    const interval = setInterval(() => setNowMs(Date.now()), 1000);
    return () => clearInterval(interval);
  }, []);

  return computeCDMColors(strip.tsat, strip.tobt, nowMs, strip.bay);
}
