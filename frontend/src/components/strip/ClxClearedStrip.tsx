import { useState, useEffect, useRef } from "react";
import { CLXBtn } from "@/components/clxbtn";
import { getStripBg } from "./types";
import type { StripProps } from "./types";
import {
  useStripCallsignInteraction,
  getFramedStripStyle,
  getCellBorderColor,
  SELECTION_COLOR,
  FONT,
  CLS_CALLSIGN_ACTIVE,
  COLOR_UNEXPECTED_YELLOW,
  COLOR_MANUAL_BLUE,
  getStripOwnership,
  getCellTextColor,
  useStripBg,
} from "./shared";
import { SIBox } from "./SIBox";
import { useStripTransfers, useWebSocketStore } from "@/store/store-hooks";
import { useCDMColors } from "@/hooks/useCDMColors";
import { useCTOTColor } from "@/hooks/useCTOTColor";
import { Bay } from "@/api/models";
import { ValidationStatusDialog } from "./ValidationStatusDialog";

// Height: 4.44vh viewport-relative (intentional — matches DelStrip height)
const FULL_H  = "4.72vh";
const HALF_H  = "2.36vh";

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
  runway,
  owner,
  nextControllers,
  previousControllers,
  myPosition,
  selectable,
  marked = false,
  fullWidth = false,
  unexpectedChangeFields,
  controllerModifiedFields,
  isManual = false,
}: StripProps) {
  const { isSelected, handleClick, handleContextMenu, showActivePress, validationDialogOpen, setValidationDialogOpen, validationStatus } = useStripCallsignInteraction({ callsign, selectable, bay, owner, myPosition });
  const stripTransfers = useStripTransfers();
  const isTagRequest = !!stripTransfers[callsign]?.isTagRequest;
  const { isUnconcerned } = getStripOwnership(myPosition, owner, nextControllers, previousControllers);
  const { bg, textWhite } = useStripBg(runway, getStripBg(pdcStatus, arrival, bay), isTagRequest, isUnconcerned, pdcStatus, bay);
  const cdmReady = useWebSocketStore(s => s.cdmReady);
  const acknowledgeUnexpectedChange = useWebSocketStore(s => s.acknowledgeUnexpectedChange);
  const standYellow = unexpectedChangeFields?.includes("stand");
  const { tobtBg, tsatBg } = useCDMColors({ bay: bay ?? Bay.Unknown, tsat: tsat ?? "", tobt: tobt ?? "" });
  const { ctotBg, ctotColor, showCtot } = useCTOTColor(ctot ?? "");

  // Blink logic: fires once for 5 seconds when pdc_state transitions into CLEARED.
  // JS interval alternates phase every 500ms so text + borders alternate in sync with background.
  const [blinkPhase, setBlinkPhase] = useState<"off" | "dark" | "light">("off");
  const prevPdcStatus = useRef(pdcStatus);

  useEffect(() => {
    if (pdcStatus === "CLEARED" && prevPdcStatus.current !== "CLEARED") {
      setBlinkPhase("dark");
      let phase: "dark" | "light" = "dark";
      const interval = setInterval(() => {
        phase = phase === "dark" ? "light" : "dark";
        setBlinkPhase(phase);
      }, 500);
      const timeout = setTimeout(() => {
        clearInterval(interval);
        setBlinkPhase("off");
      }, 5000);
      prevPdcStatus.current = pdcStatus;
      return () => { clearInterval(interval); clearTimeout(timeout); };
    }
    prevPdcStatus.current = pdcStatus;
  }, [pdcStatus]);

  // During blink: phase controls colors (dark=navy/white, light=cyan/black).
  // After blink: pdcStatus=CLEARED → always navy.
  const isBlinking = blinkPhase !== "off";
  const isNavyBg = isBlinking ? blinkPhase === "dark" : pdcStatus === "CLEARED";
  const cellBorderColor = isNavyBg ? "white" : getCellBorderColor(marked);
  const blinkBg = blinkPhase === "dark" ? "var(--color-pdc-cleared)" : blinkPhase === "light" ? "var(--color-strip-dep-bg)" : undefined;
  const manualBlue = isManual && !isNavyBg ? COLOR_MANUAL_BLUE : undefined;

  return (
    <div
      className="select-none"
      style={{
        height: FULL_H,
        width: fullWidth ? "100%" : CLX_CLEARED_STRIP_WIDTH,
        ...getFramedStripStyle(marked),
      }}
    >
      <div
        className={`flex ${isNavyBg || textWhite ? "text-white" : "text-black"}`}
        style={{ height: "100%", overflow: "hidden", backgroundColor: blinkBg ?? bg }}
      >
        {/* SI / ownership — 8.44% */}
        <SIBox
          callsign={callsign}
          owner={owner}
          nextControllers={nextControllers}
          previousControllers={previousControllers}
          myPosition={myPosition}
          flexGrow={8.44}
          transferringTo={stripTransfers[callsign]?.to ?? ""}
          isTagRequest={isTagRequest}
        />

        {/* ── Left half of 80% ── */}

        {/* Callsign — 2/3 of left half */}
        <button
          className={`flex items-center justify-start overflow-hidden ${showActivePress ? CLS_CALLSIGN_ACTIVE : ""} border-r-2 cursor-pointer`}
          style={{ flex: `${F_CALLSIGN} 0 0%`, height: "100%", minWidth: 0, fontFamily: FONT, fontWeight: "bold", fontSize: "1.25vw", textAlign: "left", paddingLeft: "0.21vw", borderRightColor: cellBorderColor, backgroundColor: isSelected ? SELECTION_COLOR : undefined, color: manualBlue, ...(validationStatus?.active && validationStatus.owning_position === myPosition && { animation: "validation-blink 1s step-start infinite" }) }}
          onClick={handleClick}
          onContextMenu={handleContextMenu}
        >
          <span className="truncate w-full">{callsign}</span>
        </button>

        {/* Dest / Stand — 1/3 of left half, top=dest bottom=stand, no line */}
        <div
          className="flex flex-col overflow-hidden border-r-2"
          style={{ flex: `${F_DEST} 0 0%`, height: "100%", minWidth: 0, borderRightColor: cellBorderColor }}
        >
          <CLXBtn callsign={callsign}>
            <div className="flex items-center justify-center border-b-2 overflow-hidden" style={{ height: HALF_H, fontFamily: FONT, fontWeight: "bold", fontSize: "0.73vw", color: manualBlue, borderBottomColor: "transparent" }}>
              {destination}
            </div>
            <div
              className="flex items-center justify-center overflow-hidden"
              style={{ height: HALF_H, fontFamily: FONT, fontWeight: "bold", fontSize: "0.73vw", backgroundColor: standYellow ? COLOR_UNEXPECTED_YELLOW : undefined, cursor: standYellow ? "pointer" : undefined, color: manualBlue ?? getCellTextColor("stand", controllerModifiedFields) }}
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
            <div className="flex items-center justify-between px-[0.21vw] border-b-2 overflow-hidden" style={{ height: HALF_H, fontFamily: FONT, fontSize: "0.73vw", borderBottomColor: "transparent" }}>
              <span className={`${isNavyBg ? "text-white" : "text-black"} shrink-0`}>EOBT</span>
              <span style={{ color: manualBlue }}>{eobt}</span>
            </div>
            <div className="flex items-center justify-between px-[0.21vw] overflow-hidden" style={{ height: HALF_H, fontFamily: FONT, fontSize: "0.73vw", backgroundColor: ctotBg, color: ctotColor }}>
              <span className="shrink-0">{showCtot ? "CTOT" : ""}</span>
              <span>{showCtot ? ctot : ""}</span>
            </div>
          </div>

          {/* TOBT / TSAT — right half, stacked with line between */}
          <div className="flex flex-col" style={{ flex: "1 0 0%", height: "100%" }}>
            <div className="flex items-center justify-between px-[0.21vw] border-b-2 overflow-hidden" style={{ height: HALF_H, fontFamily: FONT, fontSize: "0.73vw", borderBottomColor: cellBorderColor, backgroundColor: tobtBg }}>
              <span className={`${isNavyBg ? "text-white" : "text-black"} shrink-0`}>TOBT</span>
              <span>{tobt}</span>
            </div>
            <div className="flex items-center justify-between px-[0.21vw] overflow-hidden" style={{ height: HALF_H, fontFamily: FONT, fontSize: "0.73vw", backgroundColor: tsatBg, cursor: "pointer" }}
              onClick={(e) => { e.stopPropagation(); cdmReady(callsign); }}
            >
              <span className={`${isNavyBg ? "text-white" : "text-black"} shrink-0`}>TSAT</span>
              <span>{tsat}</span>
            </div>
          </div>
        </div>
      </div>
      {validationStatus && (
        <ValidationStatusDialog callsign={callsign} status={validationStatus} open={validationDialogOpen} onOpenChange={setValidationDialogOpen} />
      )}
    </div>
  );
}
