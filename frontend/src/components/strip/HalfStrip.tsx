import type { HalfStripVariant, StripProps } from "./types";
import { useSelectedCallsign, useSelectStrip } from "@/store/store-hooks";

/** Background colour per half-strip variant. */
const VARIANT_BG: Record<HalfStripVariant, string> = {
  "APN-PUSH":   "#bfbfbf",
  "APN-ARR":    "#fff28e",
  "LOCKED-DEP": "#bef5ef",
  "LOCKED-ARR": "#fff28e",
  "MESSAGES":   "#285A5C",
  "MEM-AID":    "#004FD6",
  "LAND-START": "#DD6A12",
  "CROSSING":   "#FFF500",
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
}: StripProps) {
  const selectedCallsign = useSelectedCallsign();
  const selectStrip = useSelectStrip();

  const isLocked = LOCKED_VARIANTS.includes(halfStripVariant);
  const isFreeText = FREE_TEXT_VARIANTS.includes(halfStripVariant);
  // Locked variants are never selectable regardless of the prop
  const isSelectable = selectable && !isLocked;
  const isSelected = isSelectable && selectedCallsign === callsign;

  const handleClick = isSelectable
    ? () => selectStrip(isSelected ? null : callsign)
    : undefined;

  const bg = VARIANT_BG[halfStripVariant];
  const label = VARIANT_LABEL[halfStripVariant];

  // Use light text on dark backgrounds for readability
  const darkBg = ["MESSAGES", "MEM-AID", "LAND-START"].includes(halfStripVariant);
  const textColor = darkBg ? "text-white" : "text-black";

  return (
    <div
      className={`w-fit flex text-sm select-none${isSelected ? " outline outline-2 outline-[#FF00F5]" : ""}${isSelectable ? " cursor-pointer" : ""}`}
      style={{
        height: "21px",
        backgroundColor: bg,
        borderLeft: "2px solid white",
        borderRight: "2px solid white",
        borderTop: "2px solid white",
        borderBottom: "1px solid white",
        boxShadow: "1px 0 0 0 #2F2F2F, 0 -1px 0 0 #2F2F2F",
      }}
      onClick={handleClick}
    >
      {/* Left identifier box */}
      <div className={`h-full w-8 border-r border-[#d9d9d9] flex items-center justify-center font-bold text-xs ${textColor}`}>
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
          <div className={`h-full w-[130px] border-r border-[#d9d9d9] flex items-center pl-2 font-bold truncate ${textColor}`}>
            {callsign}
          </div>
          <div className={`h-full w-14 border-r border-[#d9d9d9] flex items-center justify-center text-xs ${textColor}`}>
            {aircraftType}
          </div>
          <div className={`h-full w-14 border-r border-[#d9d9d9] flex items-center justify-center font-bold ${textColor}`}>
            {runway}
          </div>
          <div className={`h-full w-14 border-r border-[#d9d9d9] flex items-center justify-center font-bold ${textColor}`}>
            {taxiway}
          </div>
          <div className={`h-full w-10 border-r border-[#d9d9d9] flex items-center justify-center text-xs ${textColor}`}>
            {holdingPoint}
          </div>
          <div className={`h-full w-14 flex items-center justify-center font-bold ${textColor}`}>
            {stand}
          </div>
        </>
      )}
    </div>
  );
}
