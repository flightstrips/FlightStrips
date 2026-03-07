import { useState, useEffect } from "react";

export function useCTOTColor(ctot: string): { ctotBg: string; ctotColor: string; showCtot: boolean } {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    const interval = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(interval);
  }, []);

  if (!ctot) {
    return { ctotBg: "transparent", ctotColor: "black", showCtot: false };
  }

  const ctotTime = new Date(ctot);
  const diffMs = now.getTime() - ctotTime.getTime(); // positive = past CTOT

  if (diffMs < -5 * 60 * 1000) {
    return { ctotBg: "yellow", ctotColor: "black", showCtot: true };
  }
  if (diffMs <= 10 * 60 * 1000) {
    return { ctotBg: "#00008B", ctotColor: "white", showCtot: true };
  }
  return { ctotBg: "transparent", ctotColor: "black", showCtot: false };
}
