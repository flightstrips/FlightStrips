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
  getCellTextColor,
  useStripBg,
  getValidationBlinkStyle,
  usePdcClearedCallsignBlink,
  getValidationBlockedCursor,
} from "./shared";
import { useStripTransfers, useWebSocketStore } from "@/store/store-hooks";
import { useCDMColors } from "@/hooks/useCDMColors";
import { useCTOTColor } from "@/hooks/useCTOTColor";
import { Bay } from "@/api/models";
import { ValidationStatusDialog } from "./ValidationStatusDialog";
const FULL_H  = "4.72vh";
const HALF_H  = "2.36vh";

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
  ctot,
  arrival,
  runway,
  owner,
  myPosition,
  selectable,
  marked = false,
  fullWidth = false,
  unexpectedChangeFields,
  controllerModifiedFields,
  isManual = false,
}: StripProps) {
  const {
    isSelected,
    isValidationActive,
    handleClick,
    handleContextMenu,
    guardValidationAction,
    showActivePress,
    validationDialogOpen,
    setValidationDialogOpen,
    validationStatus,
  } = useStripCallsignInteraction({ callsign, selectable, bay, owner, myPosition });
  const cdmReady = useWebSocketStore(s => s.cdmReady);
  const acknowledgeUnexpectedChange = useWebSocketStore(s => s.acknowledgeUnexpectedChange);
  const stripTransfers = useStripTransfers();
  const isTagRequest = !!stripTransfers[callsign]?.isTagRequest;
  const stripBackgroundPdcStatus = pdcStatus === "CLEARED" || pdcStatus === "REQUESTED_WITH_FAULTS" ? "NONE" : pdcStatus;
  const { bg, textWhite } = useStripBg(
    runway,
    getStripBg(stripBackgroundPdcStatus, arrival, bay),
    isTagRequest,
    false,
    stripBackgroundPdcStatus,
    bay,
  );
  const showClearedCallsignHighlight = usePdcClearedCallsignBlink(pdcStatus);
  const { tobtBg, tsatBg } = useCDMColors({ bay: bay ?? Bay.Unknown, tsat: tsat ?? "", tobt: tobt ?? "" });
  const { ctotBg, ctotColor, showCtot } = useCTOTColor(ctot ?? "");
  const standYellow = unexpectedChangeFields?.includes("stand");
  const cellBorderColor = getCellBorderColor(marked);
  const manualBlue = isManual && !textWhite ? COLOR_MANUAL_BLUE : undefined;
  const callsignBackgroundColor = showClearedCallsignHighlight ? "var(--color-pdc-cleared)" : isSelected ? SELECTION_COLOR : undefined;
  const callsignTextColor = showClearedCallsignHighlight ? "white" : manualBlue;

  return (
    <div
      className="select-none"
      style={{
        height: FULL_H,
        width: fullWidth ? "100%" : "80%",
        cursor: isValidationActive ? "not-allowed" : undefined,
        ...getFramedStripStyle(marked),
        borderBottom: "1px solid white",
      }}
    >
      <div
        className={`flex ${textWhite ? "text-white" : "text-black"}`}
        style={{ height: "100%", overflow: "hidden", backgroundColor: bg }}
      >
        {/* ── Left 50% ── */}

        {/* Callsign — 2/3 of left half */}
        <button
          className={`flex items-center justify-start overflow-hidden ${showActivePress ? CLS_CALLSIGN_ACTIVE : ""} border-r-2 cursor-pointer`}
          style={{ flex: "2 0 0%", height: "100%", minWidth: 0, fontFamily: FONT, fontWeight: "bold", fontSize: "1.25vw", textAlign: "left", paddingLeft: "0.21vw", borderRightColor: cellBorderColor, backgroundColor: callsignBackgroundColor, color: callsignTextColor, cursor: getValidationBlockedCursor(isValidationActive), ...getValidationBlinkStyle(validationStatus, myPosition) }}
          onClick={handleClick}
          onContextMenu={handleContextMenu}
        >
          <span className="truncate w-full">{callsign}</span>
        </button>

        {/* Dest / Stand — 1/3 of left half, top=dest bottom=stand, no line */}
        <div
          className="flex flex-col overflow-hidden border-r-2"
          style={{ flex: "1 0 0%", height: "100%", minWidth: 0, borderRightColor: cellBorderColor }}
        >
          <CLXBtn callsign={callsign}>
            <div className="flex items-center justify-center border-b-2 overflow-hidden" style={{ height: HALF_H, fontFamily: FONT, fontWeight: "bold", fontSize: "0.73vw", color: manualBlue, borderBottomColor: "transparent" }}>
              {destination}
            </div>
            <div
              className="flex items-center justify-center overflow-hidden"
              style={{ height: HALF_H, fontFamily: FONT, fontWeight: "bold", fontSize: "0.73vw", backgroundColor: standYellow ? COLOR_UNEXPECTED_YELLOW : undefined, cursor: standYellow ? getValidationBlockedCursor(isValidationActive) : undefined, color: manualBlue ?? getCellTextColor("stand", controllerModifiedFields) }}
              onClick={standYellow ? (e) => guardValidationAction(e, () => acknowledgeUnexpectedChange(callsign, "stand")) : undefined}
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
          {/* EOBT / CTOT — left half, stacked */}
          <div className="flex flex-col overflow-hidden border-r-2" style={{ flex: "1 0 0%", height: "100%", minWidth: 0, borderRightColor: cellBorderColor }}>
            <div className="flex items-center justify-between px-[0.21vw] border-b-2 overflow-hidden" style={{ height: HALF_H, fontFamily: FONT, fontSize: "0.73vw", borderBottomColor: "transparent" }}>
              <span className="shrink-0">EOBT</span>
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
              <span className="shrink-0">TOBT</span>
              <span>{tobt}</span>
            </div>
            <div className="flex items-center justify-between px-[0.21vw] overflow-hidden" style={{ height: HALF_H, fontFamily: FONT, fontSize: "0.73vw", backgroundColor: tsatBg, cursor: getValidationBlockedCursor(isValidationActive) }}
              onClick={(e) => guardValidationAction(e, () => cdmReady(callsign))}
            >
              <span className="shrink-0">TSAT</span>
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
