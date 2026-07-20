import { useState, useEffect } from "react";
import { computeCTOTColors, type CTOTColors } from "@/lib/cdmColors";

type ComputeCTOTColors = (ctot: string, nowMs: number) => CTOTColors;

export function useCTOTColor(ctot: string, computeColors: ComputeCTOTColors = computeCTOTColors): CTOTColors {
  const [nowMs, setNowMs] = useState(Date.now);

  useEffect(() => {
    const interval = setInterval(() => setNowMs(Date.now()), 1000);
    return () => clearInterval(interval);
  }, []);

  return computeColors(ctot, nowMs);
}
