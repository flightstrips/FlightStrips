import type { TacticalStrip } from "@/api/models";
import { useMyPosition, useWebSocketStore } from "@/store/store-hooks";
import { getFlatStripBorderStyle, FONT, COLOR_BTN_BLUE } from "./shared";

const HEIGHT = "2.36dvh";
const W_SI = "1.77vw";
const W_BTN = "1.25vw";

const CELL_BORDER_CLR = "#d9d9d9"; // light grey cell borders on memaid strip
const COLOR_PRODUCER  = "white";   // SI box when strip produced by current position
const COLOR_OTHER     = "#800080"; // SI box when produced by another position

interface Props {
  strip: TacticalStrip;
  width?: string | number;
}

export function TacticalMemaidStrip({ strip, width }: Props) {
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
        backgroundColor: COLOR_BTN_BLUE,
        ...getFlatStripBorderStyle({ borderBottom: "1px solid white" }),
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

      {/* Confirm button (hourglass / tickmark) */}
      <div
        className="flex-shrink-0 flex items-center justify-center border-l-2 text-white"
        style={{
          width: W_BTN,
          height: "100%",
          borderLeftColor: CELL_BORDER_CLR,
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
        className="flex-shrink-0 flex items-center justify-center border-l-2 text-white cursor-pointer hover:bg-primary/80"
        style={{ width: W_BTN, height: "100%", borderLeftColor: CELL_BORDER_CLR }}
        onClick={(e) => { e.stopPropagation(); deleteTacticalStrip(strip.id); }}
      >
        <span style={{ fontFamily: FONT, fontSize: "0.68vw" }}>✕</span>
      </div>
    </div>
  );
}
