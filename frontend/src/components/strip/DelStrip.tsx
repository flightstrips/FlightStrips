import { CLXBtn } from "@/components/clxbtn";
import { getStripBg } from "./types";
import type { StripProps } from "./types";
import { useSelectedCallsign, useSelectStrip } from "@/store/store-hooks";

const RUBIK = "'Arial', sans-serif";
const CELL_BORDER = "border-r border-[#85b4af]";
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
  const selectedCallsign = useSelectedCallsign();
  const selectStrip = useSelectStrip();
  const isSelected = selectable && selectedCallsign === callsign;

  const handleClick = selectable
    ? () => selectStrip(isSelected ? null : callsign)
    : undefined;

  return (
    <div
      className={`select-none${isSelected ? " outline outline-2 outline-[#FF00F5]" : ""}${selectable ? " cursor-pointer" : ""}`}
      style={{
        height: FULL_H,
        width: "80%",
        backgroundColor: "#85b4af",
        padding: "1px",
        borderLeft: "2px solid white",
        borderRight: "2px solid white",
        borderTop: "2px solid white",
        borderBottom: "1px solid white",
        boxShadow: "1px 0 0 0 #2F2F2F, 0 -1px 0 0 #2F2F2F",
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
          className={`flex items-center justify-start overflow-hidden active:bg-[#F237AA] ${CELL_BORDER}`}
          style={{ flex: "2 0 0%", height: "100%", minWidth: 0, fontFamily: RUBIK, fontWeight: "bold", fontSize: 24, textAlign: "left", paddingLeft: "4px" }}
        >
          <span className="truncate w-full">{callsign}</span>
        </button>

        {/* Dest / Stand — 1/3 of left half, top=dest bottom=stand, no line */}
        <div
          className={`flex flex-col overflow-hidden ${CELL_BORDER}`}
          style={{ flex: "1 0 0%", height: "100%", minWidth: 0 }}
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
          className={`flex flex-row overflow-hidden ${CELL_BORDER}`}
          style={{ flex: "3 0 0%", height: "100%", minWidth: 0 }}
        >
          {/* EOBT — left half */}
          <div className={`flex flex-col justify-start overflow-hidden ${CELL_BORDER}`} style={{ flex: "1 0 0%", height: "100%", minWidth: 0 }}>
            <div className="flex items-center justify-between px-1 overflow-hidden" style={{ height: HALF_H, fontFamily: RUBIK, fontWeight: 400, fontSize: 14 }}>
              <span className="text-black shrink-0">EOBT</span>
              <span>{eobt}</span>
            </div>
          </div>

          {/* TOBT / TSAT — right half, stacked with line between */}
          <div className="flex flex-col" style={{ flex: "1 0 0%", height: "100%" }}>
            <div className="flex items-center justify-between px-1 border-b border-[#85b4af] overflow-hidden" style={{ height: HALF_H, fontFamily: RUBIK, fontWeight: 400, fontSize: 14 }}>
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
