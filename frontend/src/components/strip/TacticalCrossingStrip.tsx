import type { TacticalStrip } from "@/api/models";
import { useMyPosition, useWebSocketStore } from "@/store/store-hooks";
import { getFlatStripBorderStyle, FONT } from "./shared";

const HEIGHT = "2.36vh";
const W_SI = 34;
const W_BTN = 24;

const STRIP_BG        = "#fcc800"; // amber/gold crossing strip background
const CELL_BORDER_CLR = "#b39200"; // darker gold for cell borders / bottom border
const COLOR_PRODUCER  = "white";   // SI box when strip produced by current position
const COLOR_OTHER     = "#800080"; // SI box when produced by another position

interface Props {
  strip: TacticalStrip;
  width?: string | number;
}

export function TacticalCrossingStrip({ strip, width }: Props) {
  const myPosition = useMyPosition();
  const deleteTacticalStrip = useWebSocketStore(s => s.deleteTacticalStrip);

  const isProducer = strip.produced_by === myPosition;
  const siBackground = isProducer ? COLOR_PRODUCER : COLOR_OTHER;
  const label = strip.aircraft ? `${strip.label} (${strip.aircraft})` : strip.label;

  return (
    <div
      className="flex select-none"
      style={{
        height: HEIGHT,
        width: width ?? "100%",
        backgroundColor: STRIP_BG,
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
        className="flex-1 flex items-center justify-center pl-2 overflow-hidden font-bold text-xs"
        style={{ fontFamily: FONT, color: "black" }}
      >
        <span className="truncate">{label}</span>
      </div>

      {/* Delete button (X) */}
      <div
        className="flex-shrink-0 flex items-center justify-center border-l-2 cursor-pointer hover:bg-yellow-400"
        style={{ width: W_BTN, height: "100%", borderLeftColor: CELL_BORDER_CLR, color: "black" }}
        onClick={(e) => { e.stopPropagation(); deleteTacticalStrip(strip.id); }}
      >
        <span style={{ fontFamily: FONT, fontSize: 13 }}>✕</span>
      </div>
    </div>
  );
}
