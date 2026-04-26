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
  getValidationBlinkStyle,
  usePdcClearedCallsignBlink,
  getValidationBlockedCursor,
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
  const stripTransfers = useStripTransfers();
  const isTagRequest = !!stripTransfers[callsign]?.isTagRequest;
  const { isUnconcerned } = getStripOwnership(myPosition, owner, nextControllers, previousControllers);
  const stripBackgroundPdcStatus = pdcStatus === "CLEARED" || pdcStatus === "REQUESTED_WITH_FAULTS" ? "NONE" : pdcStatus;
  const { bg, textWhite } = useStripBg(
    runway,
    getStripBg(stripBackgroundPdcStatus, arrival, bay),
    isTagRequest,
    isUnconcerned,
    stripBackgroundPdcStatus,
    bay,
  );
  const showClearedCallsignHighlight = usePdcClearedCallsignBlink(pdcStatus);
  const cdmReady = useWebSocketStore(s => s.cdmReady);
  const acknowledgeUnexpectedChange = useWebSocketStore(s => s.acknowledgeUnexpectedChange);
  const standYellow = unexpectedChangeFields?.includes("stand");
  const { tobtBg, tsatBg } = useCDMColors({ bay: bay ?? Bay.Unknown, tsat: tsat ?? "", tobt: tobt ?? "" });
  const { ctotBg, ctotColor, showCtot } = useCTOTColor(ctot ?? "");
  const cellBorderColor = getCellBorderColor(marked);
  const manualBlue = isManual && !textWhite ? COLOR_MANUAL_BLUE : undefined;
  const callsignBackgroundColor = showClearedCallsignHighlight ? "var(--color-pdc-cleared)" : isSelected ? SELECTION_COLOR : undefined;
  const callsignTextColor = showClearedCallsignHighlight ? "white" : manualBlue;

  return (
    <div
      className="select-none"
      style={{
        height: FULL_H,
        width: fullWidth ? "100%" : CLX_CLEARED_STRIP_WIDTH,
        cursor: isValidationActive ? "not-allowed" : undefined,
        ...getFramedStripStyle(marked),
      }}
    >
      <div
        className={`flex ${textWhite ? "text-white" : "text-black"}`}
        style={{ height: "100%", overflow: "hidden", backgroundColor: bg }}
      >
        {/* SI / ownership — 8.44% */}
        <SIBox
          callsign={callsign}
          bay={bay}
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
          style={{ flex: `${F_CALLSIGN} 0 0%`, height: "100%", minWidth: 0, fontFamily: FONT, fontWeight: "bold", fontSize: "1.25vw", textAlign: "left", paddingLeft: "0.21vw", borderRightColor: cellBorderColor, backgroundColor: callsignBackgroundColor, color: callsignTextColor, cursor: getValidationBlockedCursor(isValidationActive), ...getValidationBlinkStyle(validationStatus, myPosition) }}
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
              style={{ height: HALF_H, fontFamily: FONT, fontWeight: "bold", fontSize: "0.73vw", backgroundColor: standYellow ? COLOR_UNEXPECTED_YELLOW : undefined, cursor: standYellow ? getValidationBlockedCursor(isValidationActive) : undefined, color: manualBlue ?? getCellTextColor("stand", controllerModifiedFields) }}
              onClick={standYellow ? (e) => guardValidationAction(e, () => acknowledgeUnexpectedChange(callsign, "stand")) : undefined}
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
              <span className={`${textWhite ? "text-white" : "text-black"} shrink-0`}>EOBT</span>
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
              <span className={`${textWhite ? "text-white" : "text-black"} shrink-0`}>TOBT</span>
              <span>{tobt}</span>
            </div>
            <div className="flex items-center justify-between px-[0.21vw] overflow-hidden" style={{ height: HALF_H, fontFamily: FONT, fontSize: "0.73vw", backgroundColor: tsatBg, cursor: getValidationBlockedCursor(isValidationActive) }}
              onClick={(e) => guardValidationAction(e, () => cdmReady(callsign))}
            >
              <span className={`${textWhite ? "text-white" : "text-black"} shrink-0`}>TSAT</span>
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
