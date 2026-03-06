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

export function TacticalCrossingStrip({ strip, width }: Props) {
  const myPosition = useMyPosition();
  const deleteTacticalStrip = useWebSocketStore(s => s.deleteTacticalStrip);

  const isProducer = strip.produced_by === myPosition;
  const siBackground = isProducer ? "#FFFFFF" : "#800080";
  const label = strip.aircraft ? `${strip.label} (${strip.aircraft})` : strip.label;

  return (
    <div
      className="flex select-none"
      style={{
        height: HEIGHT,
        width: width ?? "fit-content",
        backgroundColor: "#fcc800",
        ...getFlatStripBorderStyle({ borderBottom: "1px solid #b39200" }),
      }}
    >
      {/* SI box */}
      <div
        className="flex-shrink-0 border-r-2"
        style={{ width: W_SI, height: "100%", backgroundColor: siBackground, borderRightColor: "#b39200" }}
      />

      {/* Label */}
      <div
        className="flex-1 flex items-center justify-center pl-2 overflow-hidden font-bold text-xs"
        style={{ fontFamily: FONT, color: "#000000" }}
      >
        <span className="truncate">{label}</span>
      </div>

      {/* Delete button (X) */}
      <div
        className="flex-shrink-0 flex items-center justify-center border-l-2 cursor-pointer hover:bg-yellow-400"
        style={{ width: W_BTN, height: "100%", borderLeftColor: "#b39200", color: "#000000" }}
        onClick={(e) => { e.stopPropagation(); deleteTacticalStrip(strip.id); }}
      >
        <span style={{ fontFamily: FONT, fontSize: 13 }}>✕</span>
      </div>
    </div>
  );
}
