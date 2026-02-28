import { CLXBtn } from "@/components/clxbtn";
import { getStripBg } from "./types";
import type { StripProps } from "./types";
import {
  useStripSelection,
  getFramedStripStyle,
  getCellBorderColor,
  SELECTION_COLOR,
} from "./shared";

const RUBIK = "'Arial', sans-serif";
const FULL_H  = "4.44vh";
const HALF_H  = "2.22vh";

/**
 * DelStrip - shown before departure clearance is issued (status="CLR").
 *
 * Width: 80% of bay. Left 50% | Right 50%
 *   Left:  Callsign (2/3) | Dest+Stand (1/3, stacked no line)
 *   Right: EOBT (left half) | TOBT/TSAT (right half, stacked with line between)
 */
export function DelStrip({
  callsign,
  pdcStatus,
  destination,
  stand,
  eobt,
  tobt,
  tsat,
  arrival,
  selectable,
}: StripProps) {
  const { isSelected, handleClick } = useStripSelection(callsign, selectable);
  const cellBorderColor = getCellBorderColor(false);

  return (
    <div
      className={`select-none${selectable ? " cursor-pointer" : ""}`}
      style={{
        height: FULL_H,
        width: "80%",
        ...getFramedStripStyle(false),
        borderBottom: "1px solid white",
      }}
      onClick={handleClick}
    >
      <div
        className="flex text-black"
        style={{ height: "100%", overflow: "hidden", backgroundColor: getStripBg(pdcStatus, arrival) }}
      >
        {/* ── Left 50% ── */}

        {/* Callsign — 2/3 of left half */}
        <button
          className="flex items-center justify-start overflow-hidden active:bg-[#F237AA] border-r-2"
          style={{ flex: "2 0 0%", height: "100%", minWidth: 0, fontFamily: RUBIK, fontWeight: "bold", fontSize: 24, textAlign: "left", paddingLeft: "4px", borderRightColor: cellBorderColor, backgroundColor: isSelected ? SELECTION_COLOR : undefined }}
        >
          <span className="truncate w-full">{callsign}</span>
        </button>

        {/* Dest / Stand — 1/3 of left half, top=dest bottom=stand, no line */}
        <div
          className="flex flex-col overflow-hidden border-r-2"
          style={{ flex: "1 0 0%", height: "100%", minWidth: 0, borderRightColor: cellBorderColor }}
        >
          <CLXBtn callsign={callsign}>
            <div className="flex items-center justify-center overflow-hidden" style={{ height: HALF_H, fontFamily: RUBIK, fontWeight: 700, fontSize: 14 }}>
              {destination}
            </div>
            <div className="flex items-center justify-center overflow-hidden" style={{ height: HALF_H, fontFamily: RUBIK, fontWeight: 700, fontSize: 14 }}>
              {stand}
            </div>
          </CLXBtn>
        </div>

        {/* ── Right 50% ── */}

        {/* EOBT (left) | TOBT/TSAT (right) — horizontal split */}
        <div
          className="flex flex-row overflow-hidden border-r-2"
          style={{ flex: "3 0 0%", height: "100%", minWidth: 0, borderRightColor: cellBorderColor }}
        >
          {/* EOBT — left half */}
          <div className="flex flex-col justify-start overflow-hidden border-r-2" style={{ flex: "1 0 0%", height: "100%", minWidth: 0, borderRightColor: cellBorderColor }}>
            <div className="flex items-center justify-between px-1 overflow-hidden" style={{ height: HALF_H, fontFamily: RUBIK, fontWeight: 400, fontSize: 14 }}>
              <span className="text-black shrink-0">EOBT</span>
              <span>{eobt}</span>
            </div>
          </div>

          {/* TOBT / TSAT — right half, stacked with line between */}
          <div className="flex flex-col" style={{ flex: "1 0 0%", height: "100%" }}>
            <div className="flex items-center justify-between px-1 border-b-2 overflow-hidden" style={{ height: HALF_H, fontFamily: RUBIK, fontWeight: 400, fontSize: 14, borderBottomColor: cellBorderColor }}>
              <span className="text-black shrink-0">TOBT</span>
              <span>{tobt}</span>
            </div>
            <div className="flex items-center justify-between px-1 overflow-hidden" style={{ height: HALF_H, fontFamily: RUBIK, fontWeight: 400, fontSize: 14 }}>
              <span className="text-black shrink-0">TSAT</span>
              <span>{tsat}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
