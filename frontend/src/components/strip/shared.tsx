/**
 * Shared strip sub-components and helpers used across multiple strip variants.
 */

import { useSelectedCallsign, useSelectStrip, useControllers, useWebSocketStore } from "@/store/store-hooks";
import type { CSSProperties } from "react";

export const SELECTION_COLOR = "#FF00F5";
export const STRIP_FRAME_COLOR = "#85b4af";

const F_SI = 8;

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

/** SI / ownership indicator. Purple = unassumed, white = assumed, orange = transferred away. */
export function SIBox({
  callsign,
  owner,
  nextControllers,
  previousControllers,
  myPosition,
  marked,
  flexGrow = F_SI,
  transferringTo,
}: {
  callsign: string;
  owner?: string;
  nextControllers?: string[];
  previousControllers?: string[];
  myPosition?: string;
  /** Pass true when the strip is in the marked state (not yet wired). */
  marked?: boolean;
  /** flex-grow value (defaults to 8). Override when the strip layout requires a different proportion. */
  flexGrow?: number;
  /** Position string of the controller being transferred to. Empty string or undefined means no transfer. */
  transferringTo?: string;
}) {
  const controllers = useControllers();
  const transferStrip = useWebSocketStore(s => s.transferStrip);
  const assumeStrip = useWebSocketStore(s => s.assumeStrip);
  const freeStrip = useWebSocketStore(s => s.freeStrip);

  const isAssumed = !!myPosition && owner === myPosition;
  const isTransferredAway = !!myPosition && !!previousControllers?.includes(myPosition);
  const isConcerned = !!myPosition && !!nextControllers?.includes(myPosition);

  const isSendingTransfer = isAssumed && !!transferringTo;
  const isReceivingTransfer = !!myPosition && !!transferringTo && transferringTo === myPosition && !isAssumed;

  const nextPosition = nextControllers?.find(pos => pos !== myPosition);

  let nextLabel = "";
  if (isSendingTransfer) {
    const targetController = controllers.find(c => c.position === transferringTo);
    nextLabel = targetController ? targetController.identifier : "";
  } else if (isAssumed) {
    const nextController = controllers.find(c => c.position === nextPosition);
    nextLabel = nextController ? nextController.identifier : "";
  }

  const handleClick = () => {
    if (isReceivingTransfer) {
      assumeStrip(callsign);
    } else if (isSendingTransfer) {
      freeStrip(callsign);
    } else if (isAssumed && nextPosition) {
      transferStrip(callsign, nextPosition);
    }
  };

  const isClickable = isReceivingTransfer || isSendingTransfer || (isAssumed && !!nextPosition);

  let background: string;
  if (isSendingTransfer) {
    background = "linear-gradient(to right, #F0F0F0 50%, #DD6A12 50%)";
  } else if (isReceivingTransfer) {
    background = "linear-gradient(to right, #E082E7 50%, #F0F0F0 50%)";
  } else if (isAssumed) {
    background = "#F0F0F0";
  } else if (isTransferredAway) {
    background = "#DD6A12";
  } else if (isConcerned) {
    background = "#E082E7";
  } else {
    background = "#808080";
  }

  return (
    <div
      className="flex items-center justify-center text-sm font-bold border-r-2"
      style={{
        flex: `${flexGrow} 0 0%`,
        height: "100%",
        background: background,
        minWidth: 0,
        borderRightColor: getCellBorderColor(!!marked),
        fontFamily: "'Arial', sans-serif",
        fontSize: 22,
        color: "#8F8F8F",
        cursor: isClickable ? "pointer" : "default",
      }}
      onClick={isClickable ? handleClick : undefined}
    >
      {nextLabel}
    </div>
  );
}
