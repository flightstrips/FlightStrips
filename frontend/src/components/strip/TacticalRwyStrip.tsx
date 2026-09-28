import { useEffect, useState } from "react";
import type { TacticalStrip } from "@/api/models";
import { useMyPosition, useWebSocketStore } from "@/store/store-hooks";
import { FONT } from "./shared";
import { TacticalActionCell, TacticalStripShell } from "./TacticalStripShell";

const STRIP_BG = "#dd6a12";
const CELL_BORDER_CLR = "#a04a00"; // dark burnt-orange cell borders on rwy strip

interface Props {
  strip: TacticalStrip;
  width?: string | number;
}

export function TacticalRwyStrip({ strip, width }: Props) {
  const myPosition = useMyPosition();
  const startTacticalTimer = useWebSocketStore((state) => state.startTacticalTimer);
  const [now, setNow] = useState(Date.now);

  useEffect(() => {
    if (!strip.timer_start) return;
    setNow(Date.now());
    const interval = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(interval);
  }, [strip.timer_start]);

  const elapsed = strip.timer_start
    ? Math.floor(Math.max(0, now - Date.parse(strip.timer_start)) / 1000)
    : 0;
  const timerText = strip.timer_start
    ? `${String(Math.floor(elapsed / 60)).padStart(2, "0")}:${String(elapsed % 60).padStart(2, "0")}`
    : null;
  const label = strip.aircraft
    ? `${strip.type}${strip.label ? ` ${strip.label}` : ""} (${strip.aircraft})`
    : `${strip.type}${strip.label ? ` ${strip.label}` : ""}`;

  return (
    <TacticalStripShell
      strip={strip}
      width={width}
      backgroundColor={STRIP_BG}
      borderColor={CELL_BORDER_CLR}
      textColor="white"
      action={(
        <TacticalActionCell
          borderColor={CELL_BORDER_CLR}
          color="white"
          width={timerText ? "2.5vw" : undefined}
          clickable={strip.owner === myPosition && !timerText}
          ariaLabel={strip.owner === myPosition && !timerText ? "Start tactical timer" : undefined}
          onClick={strip.owner === myPosition && !timerText ? () => startTacticalTimer(strip.id) : undefined}
        >
          <span style={{ fontFamily: FONT, fontSize: timerText ? "0.57vw" : "0.68vw" }}>
            {timerText ?? "⌛"}
          </span>
        </TacticalActionCell>
      )}
      deleteHoverClass="hover:bg-orange-600"
    >
      {label}
    </TacticalStripShell>
  );
}
