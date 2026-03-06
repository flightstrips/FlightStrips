import { useState, useEffect } from "react";
import type { TacticalStrip } from "@/api/models";
import { useMyPosition, useWebSocketStore } from "@/store/store-hooks";
import { getFlatStripBorderStyle } from "./shared";

const FONT = "'Arial', sans-serif";
const HEIGHT = 24;
const W_SI = 34;
const W_BTN = 24;

interface Props {
  strip: TacticalStrip;
  width?: string | number;
}

export function TacticalRwyStrip({ strip, width }: Props) {
  const myPosition = useMyPosition();
  const startTacticalTimer = useWebSocketStore(s => s.startTacticalTimer);
  const deleteTacticalStrip = useWebSocketStore(s => s.deleteTacticalStrip);

  const isProducer = strip.produced_by === myPosition;
  const siBackground = isProducer ? "#FFFFFF" : "#800080";

  const label = strip.aircraft
    ? `${strip.type} ${strip.label} (${strip.aircraft})`
    : `${strip.type} ${strip.label}`;

  const [elapsed, setElapsed] = useState(0);
  useEffect(() => {
    if (!strip.timer_start) return;
    const start = new Date(strip.timer_start).getTime();
    const interval = setInterval(() => {
      setElapsed(Math.floor((Date.now() - start) / 1000));
    }, 1000);
    return () => clearInterval(interval);
  }, [strip.timer_start]);

  const timerText = strip.timer_start
    ? `${String(Math.floor(elapsed / 60)).padStart(2, "0")}:${String(elapsed % 60).padStart(2, "0")}`
    : null;

  return (
    <div
      className="flex select-none"
      style={{
        height: HEIGHT,
        width: width ?? "fit-content",
        backgroundColor: "#DD6A12",
        ...getFlatStripBorderStyle({ borderBottom: "1px solid #a04a00" }),
      }}
    >
      {/* SI box */}
      <div
        className="flex-shrink-0 border-r-2"
        style={{ width: W_SI, height: "100%", backgroundColor: siBackground, borderRightColor: "#a04a00" }}
      />

      {/* Label */}
      <div
        className="flex-1 flex items-center pl-2 overflow-hidden text-white font-bold text-xs"
        style={{ fontFamily: FONT }}
      >
        <span className="truncate">{label}</span>
      </div>

      {/* Timer / hourglass button */}
      <div
        className="flex-shrink-0 flex items-center justify-center border-l-2 text-white"
        style={{
          width: strip.timer_start ? 48 : W_BTN,
          height: "100%",
          borderLeftColor: "#a04a00",
          cursor: strip.timer_start ? "default" : "pointer",
        }}
        onClick={strip.timer_start ? undefined : (e) => { e.stopPropagation(); startTacticalTimer(strip.id); }}
      >
        <span style={{ fontFamily: FONT, fontSize: strip.timer_start ? 11 : 13, letterSpacing: 0 }}>
          {timerText ?? "⌛"}
        </span>
      </div>

      {/* Delete button (X) */}
      <div
        className="flex-shrink-0 flex items-center justify-center border-l-2 text-white cursor-pointer hover:bg-orange-600"
        style={{ width: W_BTN, height: "100%", borderLeftColor: "#a04a00" }}
        onClick={(e) => { e.stopPropagation(); deleteTacticalStrip(strip.id); }}
      >
        <span style={{ fontFamily: FONT, fontSize: 13 }}>✕</span>
      </div>
    </div>
  );
}
