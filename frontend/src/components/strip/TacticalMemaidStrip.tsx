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

export function TacticalMemaidStrip({ strip, width }: Props) {
  const myPosition = useMyPosition();
  const confirmTacticalStrip = useWebSocketStore(s => s.confirmTacticalStrip);
  const deleteTacticalStrip = useWebSocketStore(s => s.deleteTacticalStrip);

  const isProducer = strip.produced_by === myPosition;
  const siBackground = isProducer ? "#FFFFFF" : "#800080";
  const label = strip.aircraft ? `${strip.label} (${strip.aircraft})` : strip.label;

  const canConfirm = !isProducer && !strip.confirmed;

  return (
    <div
      className="flex select-none"
      style={{
        height: HEIGHT,
        width: width ?? "fit-content",
        backgroundColor: "#004FD6",
        ...getFlatStripBorderStyle({ borderBottom: "1px solid white" }),
      }}
    >
      {/* SI box */}
      <div
        className="flex-shrink-0 border-r-2"
        style={{ width: W_SI, height: "100%", backgroundColor: siBackground, borderRightColor: "#d9d9d9" }}
      />

      {/* Label */}
      <div
        className="flex-1 flex items-center pl-2 overflow-hidden text-white font-bold text-xs"
        style={{ fontFamily: FONT }}
      >
        <span className="truncate">{label}</span>
      </div>

      {/* Confirm button (hourglass / tickmark) */}
      <div
        className="flex-shrink-0 flex items-center justify-center border-l-2 text-white"
        style={{
          width: W_BTN,
          height: "100%",
          borderLeftColor: "#d9d9d9",
          cursor: canConfirm ? "pointer" : "default",
          opacity: isProducer && !strip.confirmed ? 0.35 : 1,
        }}
        onClick={canConfirm ? (e) => { e.stopPropagation(); confirmTacticalStrip(strip.id); } : undefined}
      >
        <span style={{ fontFamily: FONT, fontSize: 13 }}>
          {strip.confirmed ? "✓" : "⌛"}
        </span>
      </div>

      {/* Delete button (X) */}
      <div
        className="flex-shrink-0 flex items-center justify-center border-l-2 text-white cursor-pointer hover:bg-blue-400"
        style={{ width: W_BTN, height: "100%", borderLeftColor: "#d9d9d9" }}
        onClick={(e) => { e.stopPropagation(); deleteTacticalStrip(strip.id); }}
      >
        <span style={{ fontFamily: FONT, fontSize: 13 }}>✕</span>
      </div>
    </div>
  );
}
