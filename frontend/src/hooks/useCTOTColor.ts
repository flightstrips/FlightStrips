import { useState, useEffect } from "react";
import { computeCTOTColors, type CTOTColors } from "@/lib/cdmColors";

export function useCTOTColor(ctot: string): CTOTColors {
  const [nowMs, setNowMs] = useState(Date.now);

  useEffect(() => {
    const interval = setInterval(() => setNowMs(Date.now()), 1000);
    return () => clearInterval(interval);
  }, []);

  return computeCTOTColors(ctot, nowMs);
}
