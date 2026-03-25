import { useState, useEffect, useRef, type MouseEvent } from "react";
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
  COLOR_MANUAL_BLUE,
  getCellTextColor,
} from "./shared";
import { useIsClrDel, useStripTransfers, useWebSocketStore } from "@/store/store-hooks";
import { useCDMColors } from "@/hooks/useCDMColors";
import { Bay } from "@/api/models";
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
  bay,
  pdcStatus,
  destination,
  stand,
  eobt,
  tobt,
  tsat,
  arrival,
  selectable,
  marked = false,
  fullWidth = false,
  unexpectedChangeFields,
  controllerModifiedFields,
  isManual = false,
}: StripProps) {
  const { isSelected, handleClick } = useStripSelection(callsign, selectable);
  const isClrDel = useIsClrDel();
  const cdmReady = useWebSocketStore(s => s.cdmReady);
  const acknowledgeUnexpectedChange = useWebSocketStore(s => s.acknowledgeUnexpectedChange);
  const openStripContextMenu = useWebSocketStore(s => s.openStripContextMenu);
  const stripTransfers = useStripTransfers();
  const isTagRequest = !!stripTransfers[callsign]?.isTagRequest;
  const { tobtBg, tsatBg } = useCDMColors({ bay: bay ?? Bay.Unknown, tsat: tsat ?? "", tobt: tobt ?? "" });
  const standYellow = unexpectedChangeFields?.includes("stand");

  // Blink logic: fires once for 5 seconds when pdc_state transitions into REQUESTED_WITH_FAULTS.
  // JS interval alternates phase every 500ms so text + borders alternate in sync with background.
  const [faultBlinkPhase, setFaultBlinkPhase] = useState<"off" | "dark" | "light">("off");
  const prevPdcStatus = useRef(pdcStatus);

  useEffect(() => {
    if (pdcStatus === "REQUESTED_WITH_FAULTS" && prevPdcStatus.current !== "REQUESTED_WITH_FAULTS") {
      setFaultBlinkPhase("dark");
      let phase: "dark" | "light" = "dark";
      const interval = setInterval(() => {
        phase = phase === "dark" ? "light" : "dark";
        setFaultBlinkPhase(phase);
      }, 500);
      const timeout = setTimeout(() => {
        clearInterval(interval);
        setFaultBlinkPhase("off");
      }, 5000);
      prevPdcStatus.current = pdcStatus;
      return () => { clearInterval(interval); clearTimeout(timeout); };
    }
    prevPdcStatus.current = pdcStatus;
  }, [pdcStatus]);

  // During blink: phase controls colors (dark=navy/white, light=cyan/black).
  // After blink: CLEARED stays navy; REQUESTED_WITH_FAULTS shows yellow with normal colors.
  const isBlinking = faultBlinkPhase !== "off";
  const isNavyBg = isBlinking ? faultBlinkPhase === "dark" : pdcStatus === "CLEARED";
  const cellBorderColor = isNavyBg ? "white" : getCellBorderColor(marked);
  const blinkBg = faultBlinkPhase === "dark" ? "#00154A" : faultBlinkPhase === "light" ? "#bef5ef" : undefined;
  const manualBlue = isManual && !isNavyBg ? COLOR_MANUAL_BLUE : undefined;

  return (
    <div
      className={`select-none${(selectable || isClrDel) ? " cursor-pointer" : ""}`}
      style={{
        height: FULL_H,
        width: fullWidth ? "100%" : "80%",
        ...getFramedStripStyle(marked),
        borderBottom: "1px solid white",
      }}
      onClick={isClrDel
        ? (e: MouseEvent) => { openStripContextMenu(callsign, { x: e.clientX, y: e.clientY }); }
        : handleClick}
      onContextMenu={(e) => { e.preventDefault(); openStripContextMenu(callsign, { x: e.clientX, y: e.clientY }); }}
    >
      <div
        className={`flex ${isNavyBg ? "text-white" : "text-black"}`}
        style={{ height: "100%", overflow: "hidden", backgroundColor: blinkBg ?? (isTagRequest ? SELECTION_COLOR : getStripBg(pdcStatus, arrival)) }}
      >
        {/* ── Left 50% ── */}

        {/* Callsign — 2/3 of left half */}
        <button
          className={`flex items-center justify-start overflow-hidden ${CLS_CALLSIGN_ACTIVE} border-r-2`}
          style={{ flex: "2 0 0%", height: "100%", minWidth: 0, fontFamily: FONT, fontWeight: "bold", fontSize: 24, textAlign: "left", paddingLeft: "4px", borderRightColor: cellBorderColor, backgroundColor: isSelected ? SELECTION_COLOR : undefined, color: manualBlue }}
        >
          <span className="truncate w-full">{callsign}</span>
        </button>

        {/* Dest / Stand — 1/3 of left half, top=dest bottom=stand, no line */}
        <div
          className="flex flex-col overflow-hidden border-r-2"
          style={{ flex: "1 0 0%", height: "100%", minWidth: 0, borderRightColor: cellBorderColor }}
        >
          <CLXBtn callsign={callsign}>
            <div className="flex items-center justify-center border-b-2 overflow-hidden" style={{ height: HALF_H, fontFamily: FONT, fontWeight: "bold", fontSize: 14, color: manualBlue, borderBottomColor: "transparent" }}>
              {destination}
            </div>
            <div
              className="flex items-center justify-center overflow-hidden"
              style={{ height: HALF_H, fontFamily: FONT, fontWeight: "bold", fontSize: 14, backgroundColor: standYellow ? COLOR_UNEXPECTED_YELLOW : undefined, cursor: standYellow ? "pointer" : undefined, color: manualBlue ?? getCellTextColor("stand", controllerModifiedFields) }}
              onClick={standYellow ? (e) => { e.stopPropagation(); acknowledgeUnexpectedChange(callsign, "stand"); } : undefined}
            >
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
          <div className="flex flex-col overflow-hidden border-r-2" style={{ flex: "1 0 0%", height: "100%", minWidth: 0, borderRightColor: cellBorderColor }}>
            <div className="flex items-center justify-between px-1 border-b-2 overflow-hidden" style={{ height: "50%", fontFamily: FONT, fontSize: 14, borderBottomColor: "transparent" }}>
              <span className={`${isNavyBg ? "text-white" : "text-black"} shrink-0`}>EOBT</span>
              <span style={{ color: manualBlue }}>{eobt}</span>
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
