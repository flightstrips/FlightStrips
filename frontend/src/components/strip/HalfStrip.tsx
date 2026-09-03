import type { HalfStripVariant, StripProps } from "./types";
import { AircraftTypeLabel, useStripSelection, getCellBorderColor, getFlatStripBorderStyle, getSIBoxBorderStyle, SELECTION_COLOR, COLOR_UNEXPECTED_YELLOW, COLOR_MANUAL_BLUE, getCellTextColor, useStripBg } from "./shared";
import { useStripTransfers, useWebSocketStore } from "@/store/store-hooks";
import { getStripBg } from "./types";
import { HALF_STRIP_VARIANT_BG, getHalfStripFrameColor } from "./halfStripFrame";

/** Short label shown in the left identifier box. */
const VARIANT_LABEL: Record<HalfStripVariant, string> = {
  "APN-PUSH":   "OB",
  "APN-ARR":    "AR",
  "LOCKED-DEP": "LD",
  "LOCKED-ARR": "LA",
  "MESSAGES":   "MS",
  "MEM-AID":    "MA",
  "LAND-START": "LS",
  "CROSSING":   "CX",
};

/** Variants that use free-text content rather than structured flight data. */
const FREE_TEXT_VARIANTS: HalfStripVariant[] = [
  "MESSAGES", "MEM-AID", "LAND-START", "CROSSING",
];

/** Variants that are locked (read-only, never selectable). */
const LOCKED_VARIANTS: HalfStripVariant[] = ["LOCKED-DEP", "LOCKED-ARR"];

/** Base cell border colour for HalfStrip (lighter grey). */
const HALF_CELL_BASE = "#d9d9d9";

/**
 * HalfStrip - compact single-row strip (height: 21px) used in pushback/taxi bays (status="HALF").
 * Supports multiple visual variants with different background colours and content layouts.
 */
export function HalfStrip({
  callsign,
  aircraftType,
  aircraftCategory,
  runway,
  taxiway,
  holdingPoint,
  stand,
  selectable,
  halfStripVariant = "APN-PUSH",
  marked = false,
  unexpectedChangeFields,
  controllerModifiedFields,
  isManual = false,
  arrival,
  pdcStatus,
  bay,
}: StripProps) {
  const isLocked = LOCKED_VARIANTS.includes(halfStripVariant);
  const isFreeText = FREE_TEXT_VARIANTS.includes(halfStripVariant);
  // Locked variants are never selectable regardless of the prop
  const isSelectable = selectable && !isLocked;
  const { isSelected, handleClick } = useStripSelection(callsign, isSelectable);
  const acknowledgeUnexpectedChange = useWebSocketStore(s => s.acknowledgeUnexpectedChange);
  const stripTransfers = useStripTransfers();
  const isTagRequest = !!stripTransfers[callsign]?.isTagRequest;
  const standYellow = unexpectedChangeFields?.includes("stand");
  const isArrivalVariant = halfStripVariant === "APN-ARR" || halfStripVariant === "LOCKED-ARR";
  const stripFrameColor = getHalfStripFrameColor(halfStripVariant, arrival ?? isArrivalVariant);
  const baseCellBorderColor = isFreeText ? HALF_CELL_BASE : stripFrameColor;

  const cellBorderColor = getCellBorderColor(marked, baseCellBorderColor);
  const manualBlue = isManual ? COLOR_MANUAL_BLUE : undefined;

  const label = VARIANT_LABEL[halfStripVariant];

  // Use light text on dark backgrounds for readability
  const darkBg = ["MESSAGES", "MEM-AID", "LAND-START"].includes(halfStripVariant);
  const normalBackground = isFreeText ? HALF_STRIP_VARIANT_BG[halfStripVariant] : getStripBg(pdcStatus, arrival, bay);
  const { bg, textWhite } = useStripBg(runway, normalBackground, isTagRequest, false, pdcStatus, bay);
  const textColor = (darkBg || textWhite) ? "text-white" : "text-black";

  return (
    <div
      className={`w-fit flex text-[0.73vw] select-none${isSelectable ? " cursor-pointer" : ""}`}
      style={{
        height: "2.36dvh",
        backgroundColor: isTagRequest ? SELECTION_COLOR : bg,
        ...getFlatStripBorderStyle(isTagRequest ? SELECTION_COLOR : bg, stripFrameColor),
      }}
      onClick={handleClick}
    >
      {/* Left identifier box */}
      <div
        className={`h-full w-[1.67vw] flex items-center justify-center font-bold text-[0.63vw] ${textColor}`}
        style={getSIBoxBorderStyle(marked, baseCellBorderColor)}
      >
        {label}
      </div>

      {isFreeText ? (
        /* Free-text variants: single flexible content area */
        <div className={`h-full w-[20.5vw] flex items-center pl-[0.42vw] text-[0.63vw] ${textColor} truncate`}>
          {callsign}
        </div>
      ) : (
        /* Structured variants: callsign + flight data cells */
        <>
          <div
            className={`h-full w-[6.77vw] border-r-2 flex items-center pl-[0.42vw] font-bold truncate ${textColor}`}
            style={{ borderRightColor: cellBorderColor, backgroundColor: isSelected ? SELECTION_COLOR : undefined, color: manualBlue }}
          >
            {callsign}
          </div>
          <div
            className={`h-full w-[2.92vw] border-r-2 flex items-center justify-center text-[0.63vw] ${textColor}`}
            style={{ borderRightColor: cellBorderColor }}
          >
            <AircraftTypeLabel aircraftType={aircraftType} aircraftCategory={aircraftCategory} />
          </div>
          <div
            className={`h-full w-[2.92vw] border-r-2 flex items-center justify-center font-bold ${textColor}`}
            style={{ borderRightColor: cellBorderColor }}
          >
            {runway}
          </div>
          <div
            className={`h-full w-[2.92vw] border-r-2 flex items-center justify-center font-bold ${textColor}`}
            style={{ borderRightColor: cellBorderColor }}
          >
            {taxiway}
          </div>
          <div
            className={`h-full w-[2.08vw] border-r-2 flex items-center justify-center text-[0.63vw] ${textColor}`}
            style={{ borderRightColor: cellBorderColor }}
          >
            {holdingPoint}
          </div>
          <div
            className={`h-full w-[2.92vw] flex items-center justify-center font-bold ${textColor}`}
            style={{ backgroundColor: standYellow ? COLOR_UNEXPECTED_YELLOW : undefined, cursor: standYellow ? "pointer" : undefined, color: getCellTextColor("stand", controllerModifiedFields) }}
            onClick={standYellow ? (e) => { e.stopPropagation(); acknowledgeUnexpectedChange(callsign, "stand"); } : undefined}
          >
            {stand}
          </div>
        </>
      )}
    </div>
  );
}
