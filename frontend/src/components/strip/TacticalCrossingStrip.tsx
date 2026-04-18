import type { TacticalStrip } from "@/api/models";
import { useMyPosition, useWebSocketStore } from "@/store/store-hooks";
import { getFlatStripBorderStyle, FONT } from "./shared";

const HEIGHT = "2.36vh";
const W_SI = "1.77vw";
const W_BTN = "1.25vw";

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
  const confirmTacticalStrip = useWebSocketStore(s => s.confirmTacticalStrip);
  const deleteTacticalStrip = useWebSocketStore(s => s.deleteTacticalStrip);

  const isProducer = strip.produced_by === myPosition;
  const siBackground = isProducer ? COLOR_PRODUCER : COLOR_OTHER;
  const label = strip.aircraft ? `${strip.label} (${strip.aircraft})` : strip.label;
  const canConfirm = !isProducer && !strip.confirmed;

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
        className="flex-1 flex items-center justify-center pl-[0.42vw] overflow-hidden font-bold"
        style={{ fontFamily: FONT, color: "black", fontSize: "0.63vw" }}
      >
        <span className="truncate">{label}</span>
      </div>

      {/* Confirm button (hourglass / tickmark) */}
      <div
        className="flex-shrink-0 flex items-center justify-center border-l-2"
        style={{
          width: W_BTN,
          height: "100%",
          borderLeftColor: CELL_BORDER_CLR,
          color: "black",
          cursor: canConfirm ? "pointer" : "default",
          opacity: isProducer && !strip.confirmed ? 0.35 : 1,
        }}
        onClick={canConfirm ? (e) => { e.stopPropagation(); confirmTacticalStrip(strip.id); } : undefined}
      >
        <span style={{ fontFamily: FONT, fontSize: "0.68vw" }}>
          {strip.confirmed ? "✓" : "⌛"}
        </span>
      </div>

      {/* Delete button (X) */}
      <div
        className="flex-shrink-0 flex items-center justify-center border-l-2 cursor-pointer hover:bg-yellow-400"
        style={{ width: W_BTN, height: "100%", borderLeftColor: CELL_BORDER_CLR, color: "black" }}
        onClick={(e) => { e.stopPropagation(); deleteTacticalStrip(strip.id); }}
      >
        <span style={{ fontFamily: FONT, fontSize: "0.68vw" }}>✕</span>
      </div>
    </div>
  );
}
