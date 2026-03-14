import type { HalfStripVariant, StripProps } from "./types";
import { useStripSelection, getCellBorderColor, getFlatStripBorderStyle, SELECTION_COLOR, COLOR_ARR_YELLOW, COLOR_ARR_STRIP_BG, COLOR_BTN_BLUE, COLOR_BTN_ORANGE, COLOR_UNEXPECTED_YELLOW } from "./shared";
import { useWebSocketStore } from "@/store/store-hooks";

// Variant-specific background colours
const COLOR_HALF_PUSH_BG  = "#bfbfbf"; // compact APN-PUSH half strip (lighter grey)
const COLOR_MESSAGES_BG   = "#285A5C"; // teal — matches MessageStrip / primary theme
const COLOR_CROSSING_BG   = "#FFF500"; // bright yellow for crossing tactical strip

/** Background colour per half-strip variant. */
const VARIANT_BG: Record<HalfStripVariant, string> = {
  "APN-PUSH":   COLOR_HALF_PUSH_BG,
  "APN-ARR":    COLOR_ARR_YELLOW,
  "LOCKED-DEP": COLOR_ARR_STRIP_BG,
  "LOCKED-ARR": COLOR_ARR_YELLOW,
  "MESSAGES":   COLOR_MESSAGES_BG,
  "MEM-AID":    COLOR_BTN_BLUE,
  "LAND-START": COLOR_BTN_ORANGE,
  "CROSSING":   COLOR_CROSSING_BG,
};

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
  runway,
  taxiway,
  holdingPoint,
  stand,
  selectable,
  halfStripVariant = "APN-PUSH",
  marked = false,
  unexpectedChangeFields,
}: StripProps) {
  const isLocked = LOCKED_VARIANTS.includes(halfStripVariant);
  const isFreeText = FREE_TEXT_VARIANTS.includes(halfStripVariant);
  // Locked variants are never selectable regardless of the prop
  const isSelectable = selectable && !isLocked;
  const { isSelected, handleClick } = useStripSelection(callsign, isSelectable);
  const acknowledgeUnexpectedChange = useWebSocketStore(s => s.acknowledgeUnexpectedChange);
  const standYellow = unexpectedChangeFields?.includes("stand");

  const cellBorderColor = getCellBorderColor(marked, HALF_CELL_BASE);

  const bg = VARIANT_BG[halfStripVariant];
  const label = VARIANT_LABEL[halfStripVariant];

  // Use light text on dark backgrounds for readability
  const darkBg = ["MESSAGES", "MEM-AID", "LAND-START"].includes(halfStripVariant);
  const textColor = darkBg ? "text-white" : "text-black";

  return (
    <div
      className={`w-fit flex text-sm select-none${isSelectable ? " cursor-pointer" : ""}`}
      style={{
        height: "21px",
        backgroundColor: bg,
        ...getFlatStripBorderStyle({ borderBottom: "1px solid white" }),
      }}
      onClick={handleClick}
    >
      {/* Left identifier box */}
      <div
        className={`h-full w-8 border-r-2 flex items-center justify-center font-bold text-xs ${textColor}`}
        style={{ borderRightColor: cellBorderColor }}
      >
        {label}
      </div>

      {isFreeText ? (
        /* Free-text variants: single flexible content area */
        <div className={`h-full w-[394px] flex items-center pl-2 text-xs ${textColor} truncate`}>
          {callsign}
        </div>
      ) : (
        /* Structured variants: callsign + flight data cells */
        <>
          <div
            className={`h-full w-[130px] border-r-2 flex items-center pl-2 font-bold truncate ${textColor}`}
            style={{ borderRightColor: cellBorderColor, backgroundColor: isSelected ? SELECTION_COLOR : undefined }}
          >
            {callsign}
          </div>
          <div
            className={`h-full w-14 border-r-2 flex items-center justify-center text-xs ${textColor}`}
            style={{ borderRightColor: cellBorderColor }}
          >
            {aircraftType}
          </div>
          <div
            className={`h-full w-14 border-r-2 flex items-center justify-center font-bold ${textColor}`}
            style={{ borderRightColor: cellBorderColor }}
          >
            {runway}
          </div>
          <div
            className={`h-full w-14 border-r-2 flex items-center justify-center font-bold ${textColor}`}
            style={{ borderRightColor: cellBorderColor }}
          >
            {taxiway}
          </div>
          <div
            className={`h-full w-10 border-r-2 flex items-center justify-center text-xs ${textColor}`}
            style={{ borderRightColor: cellBorderColor }}
          >
            {holdingPoint}
          </div>
          <div
            className={`h-full w-14 flex items-center justify-center font-bold ${textColor}`}
            style={{ backgroundColor: standYellow ? COLOR_UNEXPECTED_YELLOW : undefined, cursor: standYellow ? "pointer" : undefined }}
            onClick={standYellow ? (e) => { e.stopPropagation(); acknowledgeUnexpectedChange(callsign, "stand"); } : undefined}
          >
            {stand}
          </div>
        </>
      )}
    </div>
  );
}
