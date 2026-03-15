import { useState, useEffect, useRef } from "react";
import { CLXBtn } from "@/components/clxbtn";
import { getStripBg } from "./types";
import type { StripProps } from "./types";
import {
  useStripSelection,
  getFramedStripStyle,
  getCellBorderColor,
  SELECTION_COLOR,
  FONT,
  CLS_CALLSIGN_ACTIVE,
  COLOR_UNEXPECTED_YELLOW,
  getCellTextColor,
} from "./shared";
import { SIBox } from "./SIBox";
import { useStripTransfers, useWebSocketStore } from "@/store/store-hooks";
import { useCDMColors } from "@/hooks/useCDMColors";
import { useCTOTColor } from "@/hooks/useCTOTColor";
import { Bay } from "@/api/models";

// Height: 4.44vh viewport-relative (intentional — matches DelStrip height)
const FULL_H  = "4.44vh";
const HALF_H  = "2.22vh";

// Flex-grow proportions for the 80% content area (flex-basis: 0).
// Base unit is 40 (right half width). Left half uses 2/3 and 1/3 splits.
const F_RIGHT    = 40;                    // right half (EOBT/CTOT + TOBT/TSAT)
const F_CALLSIGN = F_RIGHT * (2 / 3);    // 2/3 of left half  ~26.667
const F_DEST     = F_RIGHT * (1 / 3);    // 1/3 of left half  ~13.333

export const CLX_CLEARED_STRIP_WIDTH = "88.44%";

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
  bay,
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
  fullWidth = false,
  unexpectedChangeFields,
  controllerModifiedFields,
}: StripProps) {
  const { isSelected, handleClick } = useStripSelection(callsign, selectable);
  const isNavyBg = pdcStatus === "CLEARED";
  const cellBorderColor = isNavyBg ? "white" : getCellBorderColor(marked);
  const stripTransfers = useStripTransfers();
  const cdmReady = useWebSocketStore(s => s.cdmReady);
  const acknowledgeUnexpectedChange = useWebSocketStore(s => s.acknowledgeUnexpectedChange);
  const openStripContextMenu = useWebSocketStore(s => s.openStripContextMenu);
  const standYellow = unexpectedChangeFields?.includes("stand");
  const { tobtBg, tsatBg } = useCDMColors({ bay: bay ?? Bay.Unknown, tsat: tsat ?? "", tobt: tobt ?? "" });
  const { ctotBg, ctotColor, showCtot } = useCTOTColor(ctot ?? "");

  const [blinkOn, setBlinkOn] = useState(false);
  const prevPdcStatus = useRef(pdcStatus);

  useEffect(() => {
    if (pdcStatus === "CLEARED" && prevPdcStatus.current !== "CLEARED") {
      setBlinkOn(true);
      const timer = setTimeout(() => setBlinkOn(false), 5000);
      prevPdcStatus.current = pdcStatus;
      return () => clearTimeout(timer);
    }
    prevPdcStatus.current = pdcStatus;
  }, [pdcStatus]);

  return (
    <div
      className={`select-none${selectable ? " cursor-pointer" : ""}`}
      style={{
        height: FULL_H,
        width: fullWidth ? "100%" : CLX_CLEARED_STRIP_WIDTH,
        ...getFramedStripStyle(marked),
      }}
      onClick={handleClick}
      onContextMenu={(e) => { e.preventDefault(); openStripContextMenu(callsign, { x: e.clientX, y: e.clientY }); }}
    >
      <div
        className={`flex ${isNavyBg ? "text-white" : "text-black"}${blinkOn ? " pdc-cleared-blink" : ""}`}
        style={{ height: "100%", overflow: "hidden", ...(blinkOn ? {} : { backgroundColor: getStripBg(pdcStatus, arrival) }) }}
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
          className={`flex items-center justify-start overflow-hidden ${CLS_CALLSIGN_ACTIVE} border-r-2`}
          style={{ flex: `${F_CALLSIGN} 0 0%`, height: "100%", minWidth: 0, fontFamily: FONT, fontWeight: "bold", fontSize: 24, textAlign: "left", paddingLeft: "4px", borderRightColor: cellBorderColor, backgroundColor: isSelected ? SELECTION_COLOR : undefined }}
        >
          <span className="truncate w-full">{callsign}</span>
        </button>

        {/* Dest / Stand — 1/3 of left half, top=dest bottom=stand, no line */}
        <div
          className="flex flex-col overflow-hidden border-r-2"
          style={{ flex: `${F_DEST} 0 0%`, height: "100%", minWidth: 0, borderRightColor: cellBorderColor }}
        >
          <CLXBtn callsign={callsign}>
            <div className="flex items-center justify-center overflow-hidden" style={{ height: HALF_H, fontFamily: FONT, fontWeight: "bold", fontSize: 14 }}>
              {destination}
            </div>
            <div
              className="flex items-center justify-center overflow-hidden"
              style={{ height: HALF_H, fontFamily: FONT, fontWeight: "bold", fontSize: 14, backgroundColor: standYellow ? COLOR_UNEXPECTED_YELLOW : undefined, cursor: standYellow ? "pointer" : undefined, color: getCellTextColor("stand", controllerModifiedFields) }}
              onClick={standYellow ? (e) => { e.stopPropagation(); acknowledgeUnexpectedChange(callsign, "stand"); } : undefined}
            >
              {stand}
            </div>
          </CLXBtn>
        </div>

        {/* ── Right half of 80% ── */}

        <div
          className="flex flex-row overflow-hidden"
          style={{ flex: `${F_RIGHT} 0 0%`, height: "100%", minWidth: 0 }}
        >
          {/* EOBT / CTOT — left half, stacked */}
          <div className="flex flex-col overflow-hidden border-r-2" style={{ flex: "1 0 0%", height: "100%", minWidth: 0, borderRightColor: cellBorderColor }}>
            <div className="flex items-center justify-between px-1 overflow-hidden" style={{ height: HALF_H, fontFamily: FONT, fontSize: 14 }}>
              <span className={`${isNavyBg ? "text-white" : "text-black"} shrink-0`}>EOBT</span>
              <span>{eobt}</span>
            </div>
            <div className="flex items-center justify-between px-1 overflow-hidden" style={{ height: HALF_H, fontFamily: FONT, fontSize: 14, backgroundColor: ctotBg, color: ctotColor }}>
              <span className="shrink-0">{showCtot ? "CTOT" : ""}</span>
              <span>{showCtot ? ctot : ""}</span>
            </div>
          </div>

          {/* TOBT / TSAT — right half, stacked with line between */}
          <div className="flex flex-col" style={{ flex: "1 0 0%", height: "100%" }}>
            <div className="flex items-center justify-between px-1 border-b-2 overflow-hidden" style={{ height: HALF_H, fontFamily: FONT, fontSize: 14, borderBottomColor: cellBorderColor, backgroundColor: tobtBg }}>
              <span className={`${isNavyBg ? "text-white" : "text-black"} shrink-0`}>TOBT</span>
              <span>{tobt}</span>
            </div>
            <div className="flex items-center justify-between px-1 overflow-hidden" style={{ height: HALF_H, fontFamily: FONT, fontSize: 14, backgroundColor: tsatBg, cursor: "pointer" }}
              onClick={(e) => { e.stopPropagation(); cdmReady(callsign); }}
            >
              <span className={`${isNavyBg ? "text-white" : "text-black"} shrink-0`}>TSAT</span>
              <span>{tsat}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
