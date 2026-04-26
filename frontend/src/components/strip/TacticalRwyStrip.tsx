import { useState, useEffect } from "react";
import type { TacticalStrip } from "@/api/models";
import { useMyPosition, useWebSocketStore } from "@/store/store-hooks";
import { getFlatStripBorderStyle, FONT, COLOR_BTN_ORANGE } from "./shared";

const HEIGHT = "2.36dvh";
const W_SI = "1.77vw";
const W_BTN = "1.25vw";

const CELL_BORDER_CLR = "#a04a00"; // dark burnt-orange cell borders on rwy strip
const COLOR_PRODUCER  = "white";   // SI box when strip produced by current position
const COLOR_OTHER     = "#800080"; // SI box when produced by another position

interface Props {
  strip: TacticalStrip;
  width?: string | number;
}

export function TacticalRwyStrip({ strip, width }: Props) {
  const myPosition = useMyPosition();
  const startTacticalTimer = useWebSocketStore(s => s.startTacticalTimer);
  const deleteTacticalStrip = useWebSocketStore(s => s.deleteTacticalStrip);

  const isProducer = strip.produced_by === myPosition;
  const siBackground = isProducer ? COLOR_PRODUCER : COLOR_OTHER;

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
        width: width ?? "100%",
        backgroundColor: COLOR_BTN_ORANGE,
        ...getFlatStripBorderStyle({ borderBottom: `1px solid ${CELL_BORDER_CLR}` }),
      }}
    >
      {/* SI box */}
      <div
        className="flex-shrink-0 border-r-2"
        style={{ width: W_SI, height: "100%", backgroundColor: siBackground, borderRightColor: CELL_BORDER_CLR }}
      />

      {/* Label */}
      <div
        className="flex-1 flex items-center pl-[0.42vw] overflow-hidden text-white font-bold"
        style={{ fontFamily: FONT, fontSize: "0.63vw" }}
      >
        <span className="truncate">{label}</span>
      </div>

      {/* Timer / hourglass button */}
      <div
        className="flex-shrink-0 flex items-center justify-center border-l-2 text-white"
        style={{
          width: strip.timer_start ? "2.5vw" : W_BTN,
          height: "100%",
          borderLeftColor: CELL_BORDER_CLR,
          cursor: strip.timer_start ? "default" : "pointer",
        }}
        onClick={strip.timer_start ? undefined : (e) => { e.stopPropagation(); startTacticalTimer(strip.id); }}
      >
        <span style={{ fontFamily: FONT, fontSize: strip.timer_start ? "0.57vw" : "0.68vw", letterSpacing: 0 }}>
          {timerText ?? "⌛"}
        </span>
      </div>

      {/* Delete button (X) */}
      <div
        className="flex-shrink-0 flex items-center justify-center border-l-2 text-white cursor-pointer hover:bg-orange-600"
        style={{ width: W_BTN, height: "100%", borderLeftColor: CELL_BORDER_CLR }}
        onClick={(e) => { e.stopPropagation(); deleteTacticalStrip(strip.id); }}
      >
        <span style={{ fontFamily: FONT, fontSize: "0.68vw" }}>✕</span>
      </div>
    </div>
  );
}
