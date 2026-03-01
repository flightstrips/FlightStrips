import { CLXBtn } from "@/components/clxbtn";
import { getStripBg } from "./types";
import type { StripProps } from "./types";
import {
  useStripSelection,
  getFramedStripStyle,
  getCellBorderColor,
  SIBox,
  SELECTION_COLOR,
} from "./shared";
import { useStripTransfers } from "@/store/store-hooks";

const ARIAL = "'Arial', sans-serif";
const FULL_H  = "4.44vh";
const HALF_H  = "2.22vh";

/**
 * ClxClearedStrip — used in the CLEARED bay on the CLX layout (status="CLROK").
 *
 * Same layout as DelStrip but with:
 *  - SI box (8.44% extra width → total 88.44% of bay)
 *  - CTOT added below EOBT
 *
 * Width: 88.44% of bay.
 *   SI (8.44) | Left 50% of 80: Callsign (2/3) + Dest/Stand (1/3)
 *             | Right 50% of 80: EOBT/CTOT (left) + TOBT/TSAT (right)
 */
export function ClxClearedStrip({
  callsign,
  pdcStatus,
  destination,
  stand,
  eobt,
  tobt,
  tsat,
  ctot,
  arrival,
  owner,
  nextControllers,
  previousControllers,
  myPosition,
  selectable,
  marked = false,
}: StripProps) {
  const { isSelected, handleClick } = useStripSelection(callsign, selectable);
  const cellBorderColor = getCellBorderColor(marked);
  const stripTransfers = useStripTransfers();

  return (
    <div
      className={`select-none${selectable ? " cursor-pointer" : ""}`}
      style={{
        height: FULL_H,
        width: "88.44%",
        ...getFramedStripStyle(marked),
      }}
      onClick={handleClick}
    >
      <div
        className="flex text-black"
        style={{ height: "100%", overflow: "hidden", backgroundColor: getStripBg(pdcStatus, arrival) }}
      >
        {/* SI / ownership — 8.44% */}
        <SIBox
          callsign={callsign}
          owner={owner}
          nextControllers={nextControllers}
          previousControllers={previousControllers}
          myPosition={myPosition}
          flexGrow={8.44}
          transferringTo={stripTransfers[callsign] ?? ""}
        />

        {/* ── Left half of 80% ── */}

        {/* Callsign — 2/3 of left half */}
        <button
          className="flex items-center justify-start overflow-hidden active:bg-[#F237AA] border-r-2"
          style={{ flex: "26.667 0 0%", height: "100%", minWidth: 0, fontFamily: ARIAL, fontWeight: "bold", fontSize: 24, textAlign: "left", paddingLeft: "4px", borderRightColor: cellBorderColor, backgroundColor: isSelected ? SELECTION_COLOR : undefined }}
        >
          <span className="truncate w-full">{callsign}</span>
        </button>

        {/* Dest / Stand — 1/3 of left half, top=dest bottom=stand, no line */}
        <div
          className="flex flex-col overflow-hidden border-r-2"
          style={{ flex: "13.333 0 0%", height: "100%", minWidth: 0, borderRightColor: cellBorderColor }}
        >
          <CLXBtn callsign={callsign}>
            <div className="flex items-center justify-center overflow-hidden" style={{ height: HALF_H, fontFamily: ARIAL, fontWeight: "bold", fontSize: 14 }}>
              {destination}
            </div>
            <div className="flex items-center justify-center overflow-hidden" style={{ height: HALF_H, fontFamily: ARIAL, fontWeight: "bold", fontSize: 14 }}>
              {stand}
            </div>
          </CLXBtn>
        </div>

        {/* ── Right half of 80% ── */}

        <div
          className="flex flex-row overflow-hidden"
          style={{ flex: "40 0 0%", height: "100%", minWidth: 0 }}
        >
          {/* EOBT / CTOT — left half, stacked */}
          <div className="flex flex-col overflow-hidden border-r-2" style={{ flex: "1 0 0%", height: "100%", minWidth: 0, borderRightColor: cellBorderColor }}>
            <div className="flex items-center justify-between px-1 overflow-hidden" style={{ height: HALF_H, fontFamily: ARIAL, fontSize: 14 }}>
              <span className="text-black shrink-0">EOBT</span>
              <span>{eobt}</span>
            </div>
            <div className="flex items-center justify-between px-1 overflow-hidden" style={{ height: HALF_H, fontFamily: ARIAL, fontSize: 14 }}>
              <span className="text-black shrink-0">CTOT</span>
              <span>{ctot}</span>
            </div>
          </div>

          {/* TOBT / TSAT — right half, stacked with line between */}
          <div className="flex flex-col" style={{ flex: "1 0 0%", height: "100%" }}>
            <div className="flex items-center justify-between px-1 border-b-2 overflow-hidden" style={{ height: HALF_H, fontFamily: ARIAL, fontSize: 14, borderBottomColor: cellBorderColor }}>
              <span className="text-black shrink-0">TOBT</span>
              <span>{tobt}</span>
            </div>
            <div className="flex items-center justify-between px-1 overflow-hidden" style={{ height: HALF_H, fontFamily: ARIAL, fontSize: 14 }}>
              <span className="text-black shrink-0">TSAT</span>
              <span>{tsat}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
