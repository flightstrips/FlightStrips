/**
 * Shared strip sub-components and helpers used across multiple strip variants.
 */

import { useSelectedCallsign, useSelectStrip } from "@/store/store-hooks";
import type { CSSProperties } from "react";

export const SELECTION_COLOR = "#FF00F5";
export const STRIP_FRAME_COLOR = "#85b4af";

/** Returns the border color for cell dividers within a strip. Pass `marked` when that state is available. */
export function getCellBorderColor(marked: boolean, baseColor = STRIP_FRAME_COLOR): string {
  return marked ? SELECTION_COLOR : baseColor;
}

/** Selection state and click handler for a strip. */
export function useStripSelection(callsign: string, selectable?: boolean) {
  const selectedCallsign = useSelectedCallsign();
  const selectStrip = useSelectStrip();
  const isSelected = !!selectable && selectedCallsign === callsign;
  const handleClick = selectable
    ? () => selectStrip(isSelected ? null : callsign)
    : undefined;
  return { isSelected, handleClick };
}

/**
 * Outer style for framed strips (teal padding frame).
 *
 * Unmarked: 2px white border + 1px teal padding = 3px total visible frame.
 * Marked:   1px white border + 2px pink padding = 3px total visible frame.
 * The colored portion doubles by overwriting one white pixel, keeping strip size identical.
 *
 * Width, height, and borderBottom overrides must be applied by the caller.
 * Pass `marked` when that state is available.
 */
export function getFramedStripStyle(marked: boolean): CSSProperties {
  if (marked) {
    return {
      backgroundColor: SELECTION_COLOR,
      padding: "2px",
      borderLeft: "1px solid white",
      borderRight: "1px solid white",
      borderTop: "1px solid white",
      borderBottom: "1px solid white",
      boxShadow: `1px 0 0 0 ${COLOR_SHADOW}, 0 -1px 0 0 ${COLOR_SHADOW}`,
    };
  }
  return {
    backgroundColor: STRIP_FRAME_COLOR,
    padding: "1px",
    borderLeft: "2px solid white",
    borderRight: "2px solid white",
    borderTop: "2px solid white",
    borderBottom: "2px solid white",
    boxShadow: `1px 0 0 0 ${COLOR_SHADOW}, 0 -1px 0 0 ${COLOR_SHADOW}`,
  };
}

/**
 * Outer border/shadow style for flat strips.
 * 2px white outer border + 1px colored outline (painted on top of all children, matching the
 * visual frame of framed strips without changing the box model or requiring DOM restructuring).
 * Pass `frameColor` to override the default teal frame (e.g. gold for arrival strips).
 */
export function getFlatStripBorderStyle(overrides?: Pick<CSSProperties, "borderBottom">, frameColor = STRIP_FRAME_COLOR): CSSProperties {
  return {
    borderLeft: "2px solid white",
    borderRight: "2px solid white",
    borderTop: "2px solid white",
    borderBottom: "2px solid white",
    outline: `1px solid ${frameColor}`,
    outlineOffset: "-2px",
    boxShadow: `1px 0 0 0 ${COLOR_SHADOW}, 0 -1px 0 0 ${COLOR_SHADOW}`,
    ...overrides,
  };
}

// ── Shared font ───────────────────────────────────────────────────────────────

export const FONT = "'Arial', sans-serif";

// ── Shared palette ────────────────────────────────────────────────────────────

/** Outer background of the ATC view pages. */
export const COLOR_PAGE_BG       = "#A9A9A9";
/** Bay header — locked / dark state. */
export const COLOR_HEADER_LOCKED = "#393939";
/** Bay header — active / light state. */
export const COLOR_HEADER_ACTIVE = "#b3b3b3";
/** Standard panel / column background. */
export const COLOR_PANEL_BG      = "#555355";
/** Dark panel background (e.g. RWY DEP bay). */
export const COLOR_PANEL_DARK    = "#212121";
/** Default button background (grey). */
export const COLOR_BTN_DEFAULT   = "#646464";
/** Orange accent button background. */
export const COLOR_BTN_ORANGE    = "#DD6A12";
/** Blue accent button background. */
export const COLOR_BTN_BLUE      = "#004FD6";
/** Yellow accent button background. */
export const COLOR_BTN_YELLOW    = "#F3EA1F";
/** Cyan background for push / startup strips (ApnPushStrip). */
export const COLOR_ARR_STRIP_BG  = "#bef5ef";
/** Yellow background for arrival strips (FinalArrStrip, ApnArrStrip). */
export const COLOR_ARR_YELLOW    = "#fff28e";
/** Yellow background for unexpected/overwritten field cells. */
export const COLOR_UNEXPECTED_YELLOW = "#FFD700";
/** Blue text for fields intentionally modified by the controller. */
export const COLOR_CONTROLLER_MODIFIED_BLUE = "#2751A3";
/** Blue text/background for fields on manually-created strips (is_manual = true). */
export const COLOR_MANUAL_BLUE = "#21326A";

/** Returns the text color for a cell if the field was controller-modified, otherwise undefined. */
export function getCellTextColor(fieldName: string, controllerModifiedFields?: string[]): string | undefined {
  if (controllerModifiedFields?.includes(fieldName)) return COLOR_CONTROLLER_MODIFIED_BLUE;
  return undefined;
}

// ── SI ownership indicator colours ───────────────────────────────────────────
/** SI box — strip assumed by the current position. */
export const COLOR_SI_ASSUMED     = "#F0F0F0";
/** SI box — strip not relevant to current position. */
export const COLOR_SI_UNCONCERNED = "#808080";
/** SI box — strip in the current position's concern list. */
export const COLOR_SI_CONCERNED   = "#E082E7";

// ── Shadow colour ─────────────────────────────────────────────────────────────
/** Drop-shadow colour used on strip outer borders. */
export const COLOR_SHADOW = "#2F2F2F";

// ── Shared column layout ──────────────────────────────────────────────────────
/** Standard bay column — full height, panel background, vertical flex. */
export const CLS_COL = "h-full bg-[#555355] flex flex-col";

// ── Scrollbar utility ─────────────────────────────────────────────────────────

/** Webkit scrollbar styling used in every bay scroll container. */
export const CLS_SCROLLBAR =
  "[&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary";

// ── Callsign button active-press colour ───────────────────────────────────────
/** Tailwind active-state class for the callsign button press highlight. */
export const CLS_CALLSIGN_ACTIVE = "active:bg-[#F237AA]";

// ── Button class variants ─────────────────────────────────────────────────────

/** Large variant used in the CommandBar toolbar. */
export const CLS_CMDBTN = "bg-[#646464] text-xl font-bold p-2 border-2";
export const CLS_BTN        = "bg-[#646464] text-white font-bold text-sm px-3 border-2 border-white active:bg-[#424242]";
export const CLS_BTN_ORANGE = "bg-[#DD6A12] text-white font-bold text-sm px-3 border-2 border-white active:bg-[#424242]";
export const CLS_BTN_BLUE   = "bg-[#004FD6] text-white font-bold text-sm px-3 border-2 border-white active:bg-[#424242]";
export const CLS_BTN_YELLOW = "bg-[#F3EA1F] text-black font-bold text-sm px-3 border-2 border-white active:bg-[#424242]";
