import type { CSSProperties } from "react";

import { Bay, type FrontendStrip } from "@/api/models";
import { cn } from "@/lib/utils";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { computeCDMColors, computeCTOTColors } from "@/lib/cdmColors";

import {
  EST_CELL_HEIGHT,
  EST_CELL_WIDTH,
  formatTimeLabel,
  getBridgeStatus,
  getVgdsStatus,
  type EstCanvasStand,
} from "@/components/est/metadata";

const LABEL_TOP = EST_CELL_HEIGHT * 0.14;
const CALLSIGN_TOP = EST_CELL_HEIGHT * 0.31;
const TOBT_ROW_TOP = EST_CELL_HEIGHT * 0.555;
const TSAT_ROW_TOP = EST_CELL_HEIGHT * 0.673;
const CTOT_ROW_TOP = EST_CELL_HEIGHT * 0.797;
const ROW_HEIGHT = EST_CELL_HEIGHT * 0.108;
const LABEL_FONT_SIZE = 20;
const CONTENT_FONT_SIZE = 13;

interface EstStandCellProps {
  stand: { label: string; column?: number; row?: number } | EstCanvasStand;
  strip?: FrontendStrip;
  blocked: boolean;
  actionActive: boolean;
  blinking: boolean;
  ctotImproved: boolean;
  nowMs: number;
  containerStyle?: CSSProperties;
  onClick: (stand: string, strip: FrontendStrip | undefined, element: HTMLButtonElement) => void;
}

export default function EstStandCell({
  stand,
  strip,
  blocked,
  actionActive,
  blinking,
  ctotImproved,
  nowMs,
  containerStyle,
  onClick,
}: EstStandCellProps) {
  const vgdsStatus = getVgdsStatus(stand.label);
  const bridgeStatus = getBridgeStatus(stand.label);
  const tooltipContent = [vgdsStatus, bridgeStatus].filter(Boolean).join(" \u2022 ");
  const gridStyle =
    "column" in stand && "row" in stand && stand.column !== undefined && stand.row !== undefined
      ? { gridColumn: stand.column, gridRow: stand.row }
      : undefined;

  const isDeparture = !!strip && (strip.bay === Bay.NotCleared || strip.bay === Bay.Cleared);
  const isClearedDeparture = isDeparture && strip.bay !== Bay.NotCleared;
  const isPushing = strip?.bay === Bay.Push;
  const isArrival = strip?.bay === Bay.Stand;
  const isMoving = !!strip && !isDeparture && !isPushing && !isArrival;

  let backgroundClass = "bg-[#9E9E9E]";
  let textClass = "text-[#333333]";

  if (blocked) {
    backgroundClass = "bg-[#4A4A4A]";
    textClass = "text-white";
  } else if (actionActive || isMoving) {
    backgroundClass = "bg-[#131376]";
    textClass = "text-white";
  } else if (isPushing) {
    backgroundClass = "bg-[#DD6A12]";
    textClass = "text-white";
  } else if (isClearedDeparture) {
    backgroundClass = "bg-[#73BCF8]";
    textClass = "text-black";
  } else if (isArrival) {
    backgroundClass = "bg-[#FFF28E]";
    textClass = "text-black";
  } else if (isDeparture) {
    backgroundClass = "bg-[#D9D9D9]";
    textClass = "text-black";
  }

  const { tobtBg: tobtBarColor, tsatBg: tsatBarColor } = strip && isClearedDeparture
    ? computeCDMColors(strip.tsat, strip.tobt, nowMs, strip.bay as Bay)
    : { tobtBg: "", tsatBg: "" };

  const { ctotBg: ctotBarColor } = strip && isClearedDeparture
    ? computeCTOTColors(strip.ctot, nowMs)
    : { ctotBg: "" };

  const showTobt = isDeparture && !!strip && strip.tobt !== "";
  const showTsat = isDeparture && !!strip && strip.tsat !== "";

  const ctotLabel =
    isClearedDeparture && strip?.ctot
      ? ctotImproved
        ? `NEW: ${formatTimeLabel(strip.ctot).replace(":", "")}`
        : `CTOT: ${formatTimeLabel(strip.ctot).replace(":", "")}`
      : "";

  const showMark = isClearedDeparture && !!strip?.marked;
  const showBottomBar = showMark || (isClearedDeparture && !!ctotLabel);
  const bottomBarColor = showMark ? "#EB01FB" : ctotBarColor;

  return (
    <div style={containerStyle ?? gridStyle} className="relative">
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            onClick={(event) => onClick(stand.label, strip, event.currentTarget)}
            className={cn(
              "relative overflow-hidden rounded-xl border-2 border-black/15 shadow-sm transition-transform hover:scale-[1.02] focus-visible:outline focus-visible:outline-2 focus-visible:outline-white",
              backgroundClass,
              textClass,
              blinking && "animate-pulse ring-4 ring-[#EB01FB]",
            )}
            style={{ width: EST_CELL_WIDTH, height: EST_CELL_HEIGHT }}
          >
            {/* Indicator bars (rendered behind text via DOM order) */}
            {tobtBarColor && (
              <div
                className="absolute left-0 right-0"
                style={{ top: TOBT_ROW_TOP, height: ROW_HEIGHT, backgroundColor: tobtBarColor }}
              />
            )}
            {tsatBarColor && (
              <div
                className="absolute left-0 right-0"
                style={{ top: TSAT_ROW_TOP, height: ROW_HEIGHT, backgroundColor: tsatBarColor }}
              />
            )}
            {showBottomBar && (
              <div
                className="absolute left-0 right-0"
                style={{ top: CTOT_ROW_TOP, height: ROW_HEIGHT, backgroundColor: bottomBarColor }}
              />
            )}

            {/* Stand label */}
             <div
               className="absolute left-0 right-0 text-center font-bold"
               style={{ top: LABEL_TOP, fontSize: LABEL_FONT_SIZE, lineHeight: `${LABEL_FONT_SIZE}px` }}
             >
               {stand.label}
             </div>

            {/* Callsign */}
            {strip && !blocked && (
               <div
                 className="absolute left-0 right-0 truncate px-0.5 text-center"
                 style={{ top: CALLSIGN_TOP, fontSize: CONTENT_FONT_SIZE, lineHeight: `${CONTENT_FONT_SIZE}px` }}
               >
                 {strip.callsign}
               </div>
            )}

            {/* TOBT row */}
            {showTobt && (
               <div
                 className="absolute left-0 right-0 flex items-center justify-center"
                 style={{ top: TOBT_ROW_TOP, height: ROW_HEIGHT, fontSize: CONTENT_FONT_SIZE }}
               >
                 {`TOBT: ${formatTimeLabel(strip!.tobt).replace(":", "")}`}
               </div>
            )}

            {/* TSAT row */}
            {showTsat && (
               <div
                 className="absolute left-0 right-0 flex items-center justify-center"
                 style={{ top: TSAT_ROW_TOP, height: ROW_HEIGHT, fontSize: CONTENT_FONT_SIZE }}
               >
                 {`TSAT: ${formatTimeLabel(strip!.tsat).replace(":", "")}`}
               </div>
            )}

            {/* CTOT / MARK bottom bar text */}
            {showBottomBar && (
               <div
                 className="absolute left-0 right-0 flex items-center justify-center text-white"
                 style={{ top: CTOT_ROW_TOP, height: ROW_HEIGHT, fontSize: CONTENT_FONT_SIZE }}
               >
                 {!showMark ? ctotLabel : null}
               </div>
            )}
          </button>
        </TooltipTrigger>
        {tooltipContent ? (
          <TooltipContent sideOffset={6} className="max-w-48 text-center">
            {tooltipContent}
          </TooltipContent>
        ) : null}
      </Tooltip>
    </div>
  );
}
