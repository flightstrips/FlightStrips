/**
 * Shared strip sub-components and helpers used across multiple strip variants.
 */

import { useMarkArmed, useRunwaySetup, useSelectStrip, useSelectedCallsign, useStripTransfers, useTagRequestArmed, useWebSocketStore } from "@/store/store-hooks";
import { useEffect, useRef, useState } from "react";
import type { CSSProperties, MouseEvent as ReactMouseEvent } from "react";
import { Bay } from "@/api/models";
import type { PdcStatus } from "@/api/models";
import type { ValidationStatus } from "@/api/models";

export const SELECTION_COLOR = "var(--color-strip-selection)";
export const STRIP_FRAME_COLOR = "var(--color-strip-frame)";
const VALIDATION_BLINK_CYCLE_MS = 1000;
const PDC_CLEARED_CALLSIGN_BLINK_INTERVAL_MS = 500;
const PDC_CLEARED_CALLSIGN_BLINK_DURATION_MS = 7000;
const PDC_VALIDATION_ISSUE_TYPES = new Set(["PDC INVALID", "CUSTOM PDC"]);

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

export function canOpenStripContextMenu(bay?: string, owner?: string, myPosition?: string): boolean {
  return bay === Bay.NotCleared || owner !== myPosition;
}

export function canRequestTagForStrip({
  owner,
  myPosition,
  hasActiveCoordination,
}: {
  owner?: string;
  myPosition?: string;
  hasActiveCoordination: boolean;
}): boolean {
  return !!owner && !!myPosition && owner !== myPosition && !hasActiveCoordination;
}

export function canForceAssumeStrip({
  owner,
  myPosition,
  isClrDel,
  hasActiveCoordination,
}: {
  owner?: string;
  myPosition?: string;
  isClrDel: boolean;
  hasActiveCoordination: boolean;
}): boolean {
  return !!myPosition && !!owner && owner !== myPosition && !isClrDel && !hasActiveCoordination;
}

export function isPdcValidationStatus(validationStatus: ValidationStatus | undefined): boolean {
  return validationStatus != null && PDC_VALIDATION_ISSUE_TYPES.has(validationStatus.issue_type);
}

export function isValidationActiveForPosition(validationStatus: ValidationStatus | undefined, myPosition?: string): boolean {
  return validationStatus?.active === true
    && (isPdcValidationStatus(validationStatus) || validationStatus.owning_position === myPosition);
}

export function getValidationBlinkStyle(validationStatus: ValidationStatus | undefined, myPosition?: string): CSSProperties {
  if (!isValidationActiveForPosition(validationStatus, myPosition)) {
    return {};
  }

  return {
    animation: `validation-blink ${VALIDATION_BLINK_CYCLE_MS}ms step-start infinite`,
  };
}

export function getValidationBlockedCursor(
  isValidationActive: boolean,
  defaultCursor: CSSProperties["cursor"] = "pointer",
): CSSProperties["cursor"] {
  return isValidationActive ? "not-allowed" : defaultCursor;
}

export function usePdcClearedCallsignBlink(pdcStatus?: PdcStatus): boolean {
  const [isHighlighted, setIsHighlighted] = useState(pdcStatus === "CLEARED");
  const prevPdcStatus = useRef<PdcStatus | undefined>(pdcStatus);

  useEffect(() => {
    if (pdcStatus === "CLEARED" && prevPdcStatus.current !== "CLEARED") {
      setIsHighlighted(true);
      let highlighted = true;
      const interval = setInterval(() => {
        highlighted = !highlighted;
        setIsHighlighted(highlighted);
      }, PDC_CLEARED_CALLSIGN_BLINK_INTERVAL_MS);
      const timeout = setTimeout(() => {
        clearInterval(interval);
        setIsHighlighted(true);
      }, PDC_CLEARED_CALLSIGN_BLINK_DURATION_MS);

      prevPdcStatus.current = pdcStatus;
      return () => {
        clearInterval(interval);
        clearTimeout(timeout);
      };
    }

    prevPdcStatus.current = pdcStatus;
  }, [pdcStatus]);

  return pdcStatus === "CLEARED" && isHighlighted;
}

export function useStripCallsignInteraction({
  callsign,
  selectable,
  bay,
  owner,
  myPosition,
}: {
  callsign: string;
  selectable?: boolean;
  bay?: string;
  owner?: string;
  myPosition?: string;
}) {
  const selectedCallsign = useSelectedCallsign();
  const selectStrip = useSelectStrip();
  const tagRequestArmed = useTagRequestArmed();
  const markArmed = useMarkArmed();
  const stripTransfers = useStripTransfers();
  const openStripContextMenu = useWebSocketStore((state) => state.openStripContextMenu);
  const requestTag = useWebSocketStore((state) => state.requestTag);
  const toggleMarked = useWebSocketStore((state) => state.toggleMarked);
  const marked = useWebSocketStore((state) => state.strips.find((strip) => strip.callsign === callsign)?.marked ?? false);
  const validationStatus = useWebSocketStore((state) => state.strips.find((s) => s.callsign === callsign)?.validation_status);
  const isValidationActive = isValidationActiveForPosition(validationStatus, myPosition);

  const [validationDialogOpen, setValidationDialogOpen] = useState(false);

  const isSelected = !!selectable && selectedCallsign === callsign;
  const openContextMenuOnClick = canOpenStripContextMenu(bay, owner, myPosition);
  const canRequestTag = canRequestTagForStrip({
    owner,
    myPosition,
    hasActiveCoordination: !!stripTransfers[callsign],
  });

  const handleClick = (event: ReactMouseEvent<HTMLElement>) => {
    event.stopPropagation();

    if (isValidationActive) {
      setValidationDialogOpen(true);
      return;
    }

    if (tagRequestArmed) {
      if (canRequestTag) {
        requestTag(callsign);
      }
      return;
    }

    if (markArmed) {
      toggleMarked(callsign, !marked);
      return;
    }

    if (openContextMenuOnClick) {
      openStripContextMenu(callsign, { x: event.clientX, y: event.clientY });
      return;
    }

    if (!selectable) {
      return;
    }

    selectStrip(isSelected ? null : callsign);
  };

  const handleContextMenu = (event: ReactMouseEvent<HTMLElement>) => {
    event.preventDefault();
  };

  const guardValidationAction = (event: ReactMouseEvent<HTMLElement>, action: () => void) => {
    event.stopPropagation();
    if (isValidationActive) {
      setValidationDialogOpen(true);
      return;
    }
    action();
  };

  return {
    isSelected,
    isValidationActive,
    handleClick,
    handleContextMenu,
    guardValidationAction,
    showActivePress: !!selectable && !tagRequestArmed && !markArmed && !openContextMenuOnClick,
    validationDialogOpen,
    setValidationDialogOpen,
    validationStatus,
  };
}

/**
 * Returns a CSS animation style for the callsign cell when a validation is active.
 * Uses a CSS keyframe animation so no interval timers are needed.
 */
export function useValidationBlink(callsign: string): CSSProperties {
  const validationStatus = useWebSocketStore((state) => state.strips.find((s) => s.callsign === callsign)?.validation_status);
  const myPosition = useWebSocketStore((state) => state.position);
  return getValidationBlinkStyle(validationStatus, myPosition);
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
export const COLOR_PAGE_BG       = "var(--color-bay-page)";
/** Bay header — locked / dark state. */
export const COLOR_HEADER_LOCKED = "#393939";
/** Bay header — active / light state. */
export const COLOR_HEADER_ACTIVE = "#b3b3b3";
/** Standard panel / column background. */
export const COLOR_PANEL_BG      = "var(--color-bay-panel)";
/** Dark panel background (e.g. RWY DEP bay). */
export const COLOR_PANEL_DARK    = "#212121";
/** Default button background (grey). */
export const COLOR_BTN_DEFAULT   = "#646464";
/** Orange accent button background. */
export const COLOR_BTN_ORANGE    = "var(--color-si-unconcerned)";
/** Blue accent button background. */
export const COLOR_BTN_BLUE      = "var(--color-half-mem-aid)";
/** Heavy WTC aircraft type text color. */
export const COLOR_TYPE_HEAVY    = "var(--color-type-heavy)";
/** Yellow accent button background. */
export const COLOR_BTN_YELLOW    = "#F3EA1F";
/** Cyan background for departure / push strips (ApnPushStrip, ApnTaxiDepStrip, HalfStrip). */
export const COLOR_DEP_STRIP_BG  = "var(--color-strip-dep-bg)";
/** Yellow background for arrival strips (FinalArrStrip, ApnArrStrip). */
export const COLOR_ARR_YELLOW    = "var(--color-strip-arr-bg)";
/** Yellow background for unexpected/overwritten field cells. */
export const COLOR_UNEXPECTED_YELLOW = "var(--color-field-warning)";
/** Blue text for fields intentionally modified by the controller. */
export const COLOR_CONTROLLER_MODIFIED_BLUE = "var(--color-field-modified)";
/** Blue text/background for fields on manually-created strips (is_manual = true). */
export const COLOR_MANUAL_BLUE = "var(--color-field-manual)";
/** Strip background when the strip is unconcerned (not assumed, concerned, or transferred). */
export const COLOR_UNCONCERNED_BG = "var(--color-strip-unconcerned)";

// ── Ownership helpers ─────────────────────────────────────────────────────────

/**
 * Derives strip ownership state from controller position data.
 * Use this instead of repeating the same four boolean computations in every strip.
 */
export function getStripOwnership(
  myPosition: string | undefined,
  owner: string | undefined,
  nextControllers: string[] | undefined,
  previousControllers: string[] | undefined,
) {
  const isAssumed       = !!myPosition && owner === myPosition;
  const isTransferredAway = !!myPosition && !!previousControllers?.includes(myPosition);
  const isConcerned     = !!myPosition && !!nextControllers?.includes(myPosition);
  const isUnconcerned   = !!myPosition && !isAssumed && !isTransferredAway && !isConcerned;
  return { isAssumed, isTransferredAway, isConcerned, isUnconcerned };
}

// Maps each runway pair label → constituent runway designators.
const RUNWAY_PAIR_MAP: Record<string, string[]> = {
  "04L-22R": ["04L", "22R"],
  "04R-22L": ["04R", "22L"],
  "12-30":   ["12",  "30"],
};

/**
 * Returns true if the given runway belongs to a pair whose status is "CLOSED".
 * Pass `runwayStatus` from `useRunwaySetup().runway_status`.
 */
export function isRunwayClosed(runway: string | undefined, runwayStatus: Record<string, string> | undefined): boolean {
  if (!runway || !runwayStatus) return false;
  for (const [pair, status] of Object.entries(runwayStatus)) {
    if (status === "CLOSED" && RUNWAY_PAIR_MAP[pair]?.includes(runway)) return true;
  }
  return false;
}

/**
 * Resolves the final strip background colour, applying overrides in priority order:
 *   tag-request (pink) → unconcerned (grey) → caller-supplied normal colour.
 */
/** Red used for strips whose assigned runway is CLOSED — matches the CLOSED button in the runway status dialog. */
export const COLOR_CLOSED_RWY = "var(--color-runway-closed)";

export function resolveStripBg(normalBg: string, isTagRequest: boolean, isUnconcerned: boolean, isClosedRunway = false): string {
  if (isTagRequest)   return SELECTION_COLOR;
  if (isClosedRunway) return COLOR_CLOSED_RWY;
  if (isUnconcerned)  return COLOR_UNCONCERNED_BG;
  return normalBg;
}

/**
 * Hook that resolves strip background colour and whether text should be white.
 * Encapsulates runway-status lookup, closed-runway check, and resolveStripBg — one call per strip.
 *
 * Priority: tag-request (pink) → closed runway (red) → unconcerned (grey) → normalBg.
 * `textWhite` is true only when the closed-runway override is active.
 */
export function useStripBg(
  runway: string | undefined,
  normalBg: string,
  isTagRequest: boolean,
  isUnconcerned: boolean,
  pdcStatus?: PdcStatus,
  bay?: Bay,
): { bg: string; textWhite: boolean } {
  const runwaySetup = useRunwaySetup();
  const closedRwy = isRunwayClosed(runway, runwaySetup.runway_status);
  const bg = resolveStripBg(normalBg, isTagRequest, isUnconcerned, closedRwy);
  const pdcAllowed = !bay || bay === Bay.NotCleared || bay === Bay.Cleared;
  const pdcDarkBg = pdcAllowed && pdcStatus === "CLEARED"
    && !isTagRequest && !closedRwy && !isUnconcerned;
  return { bg, textWhite: (closedRwy && !isTagRequest) || pdcDarkBg };
}

/** Returns the text color for a cell if the field was controller-modified, otherwise undefined. */
export function getCellTextColor(fieldName: string, controllerModifiedFields?: string[]): string | undefined {
  if (controllerModifiedFields?.includes(fieldName)) return COLOR_CONTROLLER_MODIFIED_BLUE;
  return undefined;
}

// ── SI ownership indicator colours ───────────────────────────────────────────
/** SI box — strip assumed by the current position. */
export const COLOR_SI_ASSUMED     = "var(--color-si-assumed)";
/** SI box — strip not relevant to current position. */
export const COLOR_SI_UNCONCERNED = "var(--color-si-unconcerned)";
/** SI box — strip in the current position's concern list. */
export const COLOR_SI_CONCERNED   = "var(--color-si-concerned)";

// ── Shadow colour ─────────────────────────────────────────────────────────────
/** Drop-shadow colour used on strip outer borders. */
export const COLOR_SHADOW = "var(--color-strip-shadow)";

// ── Shared column layout ──────────────────────────────────────────────────────
/** Standard bay column — full height, panel background, vertical flex. Fixed-width variant. */
export const CLS_COL = "h-full bg-bay-panel flex flex-col";
/** Flex-grow bay column — fills available space. Used in views with equal-width columns. */
export const CLS_COL_FLEX = "flex-1 h-full bg-bay-panel flex flex-col min-w-0";

/** Shadow applied to every bay section header — controls the depth effect.
 *  Change here to update all views at once. */
export const CLS_HEADER_SHADOW = "shadow-[inset_6px_0_8px_rgba(0,0,0,0.4),inset_0_4px_8px_rgba(0,0,0,0.4),0_1px_0_rgba(0,0,0,0.9)] relative z-10";

/** Dark section header bar. */
export const CLS_HEADER = `bg-bay-header h-[3.7vh] flex items-center px-[0.42vw] shrink-0 ${CLS_HEADER_SHADOW}`;
/** Standard header label text. */
export const CLS_LABEL = "text-white font-bold text-[0.94vw]";

/** Horizontal separator between sections within a column. */
export const CLS_COL_SEP = "border-t-[6px] border-bay-border";

/** Full-width page wrapper for all bay views. */
export const CLS_PAGE_WRAPPER = "bg-bay-border w-screen h-[95.28vh] flex divide-x-[6px] divide-bay-border border-x-2 border-t-2 border-bay-border";

// ── Scrollbar utility ─────────────────────────────────────────────────────────

/** Webkit scrollbar styling used in every bay scroll container. */
export const CLS_SCROLLBAR =
  "[&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary";

/** Standard bay scroll area — strips stack top-to-bottom. */
export const CLS_SCROLL_AREA = `w-full bg-bay-panel shadow-[inset_2px_2px_4px_rgba(0,0,0,0.55),inset_-1px_-1px_2px_rgba(255,255,255,0.07)] p-0.5 flex flex-col gap-px overflow-y-auto ${CLS_SCROLLBAR}`;
/** Bay scroll area — strips stack from bottom (newest at bottom). */
export const CLS_SCROLL_AREA_BOTTOM = `w-full bg-bay-panel shadow-[inset_2px_2px_4px_rgba(0,0,0,0.55),inset_-1px_-1px_2px_rgba(255,255,255,0.07)] p-0.5 flex flex-col justify-end gap-px overflow-y-auto ${CLS_SCROLLBAR}`;
/** Dark scroll area (e.g. de-ice bay). */
export const CLS_SCROLL_AREA_DARK = `w-full bg-bay-dark shadow-[inset_3px_3px_7px_rgba(0,0,0,0.85),inset_-1px_-1px_3px_rgba(255,255,255,0.05)] p-0.5 flex flex-col justify-end gap-px overflow-y-auto ${CLS_SCROLLBAR}`;

/** Tab bar shown below bay columns. */
export const CLS_TAB_BAR = "flex shrink-0 border-t-8 border-bay-border";
/** Individual tab button within a tab bar. */
export const CLS_TAB_BTN = "flex-1 bg-bay-header text-white font-bold text-[0.73vw] border border-white hover:bg-[#4a4a4a]";

// ── Callsign button active-press colour ───────────────────────────────────────
/** Tailwind active-state class for the callsign button press highlight. */
export const CLS_CALLSIGN_ACTIVE = "active:bg-[var(--color-strip-callsign)]";

// ── Button class variants ─────────────────────────────────────────────────────

/** Large variant used in the CommandBar toolbar. */
export const CLS_CMDBTN = "bg-bay-btn text-[1.41vw] font-bold h-[3.42vh] my-[0.65vh] w-[3.52vw] flex items-center justify-center shadow-[inset_2px_0_0_var(--color-bay-shadow),_inset_0_2px_0_var(--color-bay-shadow)] outline-none";
const CLS_HEADER_BTN_BASE = "inline-flex h-[2.22vh] items-center justify-center whitespace-nowrap border-2 px-[0.625vw] text-[0.73vw] leading-[1.04vw] font-bold";
export const CLS_BTN        = `${CLS_HEADER_BTN_BASE} bg-bay-btn text-white border-white active:bg-[#424242]`;
export const CLS_BTN_ORANGE = `${CLS_HEADER_BTN_BASE} bg-runway-low-vis text-white border-white active:bg-[#424242]`;
export const CLS_BTN_BLUE   = `${CLS_HEADER_BTN_BASE} bg-[var(--color-half-mem-aid)] text-white border-white active:bg-[#424242]`;
export const CLS_BTN_YELLOW = `${CLS_HEADER_BTN_BASE} bg-btn-yellow text-black border-white active:bg-[#424242]`;
