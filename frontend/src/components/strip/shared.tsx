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
      boxShadow: "1px 0 0 0 #2F2F2F, 0 -1px 0 0 #2F2F2F",
    };
  }
  return {
    backgroundColor: STRIP_FRAME_COLOR,
    padding: "1px",
    borderLeft: "2px solid white",
    borderRight: "2px solid white",
    borderTop: "2px solid white",
    borderBottom: "2px solid white",
    boxShadow: "1px 0 0 0 #2F2F2F, 0 -1px 0 0 #2F2F2F",
  };
}

/**
 * Outer border/shadow style for flat strips (no teal padding frame).
 * The white border is always 2px — only cell border colours change on selection.
 */
export function getFlatStripBorderStyle(overrides?: Pick<CSSProperties, "borderBottom">): CSSProperties {
  return {
    borderLeft: "2px solid white",
    borderRight: "2px solid white",
    borderTop: "2px solid white",
    borderBottom: "2px solid white",
    boxShadow: "1px 0 0 0 #2F2F2F, 0 -1px 0 0 #2F2F2F",
    ...overrides,
  };
}
