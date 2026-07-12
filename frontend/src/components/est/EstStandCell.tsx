import type { CSSProperties } from "react";

import { Bay, type FrontendStandAssignmentEntry, type FrontendStrip } from "@/api/models";
import { COLOR_BTN_YELLOW, SELECTION_COLOR } from "@/components/strip/shared";
import { cn } from "@/lib/utils";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { CTOT_BLUE, computeCDMColors, hasManualTobtSource } from "@/lib/cdmColors";

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
  assignment?: FrontendStandAssignmentEntry;
  selected: boolean;
  blocked: boolean;
  blockReason?: string;
  actionActive: boolean;
  blinking: boolean;
  startReqActive: boolean;
  ctotImproved: boolean;
  nowMs: number;
  containerStyle?: CSSProperties;
  onClick: (stand: string, strip: FrontendStrip | undefined, element: HTMLButtonElement) => void;
}

export default function EstStandCell({
  stand,
  strip,
  assignment,
  blocked,
  blockReason,
  actionActive,
  blinking,
  startReqActive,
  ctotImproved,
  nowMs,
  containerStyle,
  onClick,
}: EstStandCellProps) {
  const vgdsStatus = getVgdsStatus(stand.label);
  const bridgeStatus = getBridgeStatus(stand.label);
  const assignmentTimes = assignment
    ? [assignment.eta ? `ETA ${formatTimeLabel(assignment.eta)}` : "", assignment.expires_at ? `expires ${formatTimeLabel(assignment.expires_at)}` : ""].filter(Boolean).join(", ")
    : "";
  const assignmentSummary = assignment
    ? `${assignment.stage} · ${assignment.source}${assignmentTimes ? ` · ${assignmentTimes}` : ""}`
    : "";
  const tooltipContent = [vgdsStatus, bridgeStatus, assignmentSummary, blocked ? blockReason : undefined].filter(Boolean).join(" \u2022 ");
  const gridStyle =
    "column" in stand && "row" in stand && stand.column !== undefined && stand.row !== undefined
      ? { gridColumn: stand.column, gridRow: stand.row }
      : undefined;

  const isDeparture = !!strip && (strip.bay === Bay.NotCleared || strip.bay === Bay.Cleared);
  const isClearedDeparture = isDeparture && strip.bay !== Bay.NotCleared;
  const isPushing = strip?.bay === Bay.Push;
  const isArrival = strip?.bay === Bay.Stand;

  let backgroundClass = "bg-[#D9D9D9]";
  let textClass = "text-[#333333]";

  if (blocked) {
    backgroundClass = "bg-[#4A4A4A]";
    textClass = "text-white";
  } else if (startReqActive || actionActive) {
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

  const { tobtBg: tobtBarColor, tsatBg: tsatBarColor } = strip && isDeparture
    ? computeCDMColors(strip.tsat, strip.tobt, nowMs, strip.bay as Bay, strip.phase)
    : { tobtBg: "", tsatBg: "" };
  const emphasizeTobtTime = strip ? hasManualTobtSource(strip.req_tobt_type, strip.tobt_set_by) : false;

  const showTobt = isDeparture && !!strip && strip.tobt !== "";
  const showTsat = isDeparture && !!strip && strip.tsat !== "";
  const showCtot = isClearedDeparture && !!strip?.ctot.trim();

  const ctotLabel =
    showCtot && strip?.ctot
      ? ctotImproved
        ? `NEW: ${formatTimeLabel(strip.ctot).replace(":", "")}`
        : `CTOT: ${formatTimeLabel(strip.ctot).replace(":", "")}`
      : "";

  const showMark = isClearedDeparture && !!strip?.marked;
  const showCtotText = ctotLabel !== "";
  const boxShadows: string[] = [];
  if (startReqActive) {
    boxShadows.push(`0 0 0 4px ${COLOR_BTN_YELLOW}`);
  }
  if (showMark) {
    boxShadows.push(`0 0 0 ${startReqActive ? 8 : 4}px ${SELECTION_COLOR}`);
  }
  const buttonStyle: CSSProperties = {
    width: EST_CELL_WIDTH,
    height: EST_CELL_HEIGHT,
    boxShadow: boxShadows.length > 0 ? boxShadows.join(", ") : undefined,
  };

  return (
    <div style={containerStyle ?? gridStyle} className="relative">
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            onClick={(event) => onClick(stand.label, strip, event.currentTarget)}
            className={cn(
              "relative overflow-hidden rounded-xl border-2 border-black/15 transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-white",
              backgroundClass,
              textClass,
              blinking && "animate-pulse",
            )}
            style={buttonStyle}
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
            {showCtot && (
              <div
                className="absolute left-0 right-0"
                style={{ top: CTOT_ROW_TOP, height: ROW_HEIGHT, backgroundColor: CTOT_BLUE }}
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
            {(assignment || (strip && !blocked)) && (
               <div
                 className="absolute left-0 right-0 flex items-center justify-center overflow-hidden px-0.5 text-center"
                 style={{
                  top: CALLSIGN_TOP,
                  height: ROW_HEIGHT,
                  fontSize: CONTENT_FONT_SIZE,
                  backgroundColor: showMark ? SELECTION_COLOR : undefined,
                  color: showMark ? "#000000" : undefined,
                }}
              >
                <span className="block truncate">{assignment?.callsign ?? strip?.callsign}</span>
              </div>
            )}

            {assignment && !showTobt && (
              <div
                className="absolute left-0 right-0 flex items-center justify-center truncate px-1 uppercase"
                style={{ top: TOBT_ROW_TOP, height: ROW_HEIGHT, fontSize: 10 }}
              >
                {assignment.stage} · {assignment.source}
              </div>
            )}

            {assignment?.eta && !showTsat && (
              <div
                className="absolute left-0 right-0 flex items-center justify-center"
                style={{ top: TSAT_ROW_TOP, height: ROW_HEIGHT, fontSize: 10 }}
              >
                ETA {formatTimeLabel(assignment.eta).replace(":", "")}
              </div>
            )}

            {assignment?.expires_at && !showCtot && (
              <div
                className="absolute left-0 right-0 flex items-center justify-center"
                style={{ top: CTOT_ROW_TOP, height: ROW_HEIGHT, fontSize: 10 }}
              >
                EXP {formatTimeLabel(assignment.expires_at).replace(":", "")}
              </div>
            )}

            {/* TOBT row */}
             {showTobt && (
                <div
                  className="absolute left-0 right-0 flex items-center justify-center gap-1"
                  style={{ top: TOBT_ROW_TOP, height: ROW_HEIGHT, fontSize: CONTENT_FONT_SIZE }}
                >
                  <span>TOBT:</span>
                  <span style={{ fontWeight: emphasizeTobtTime ? 700 : undefined }}>
                    {formatTimeLabel(strip!.tobt).replace(":", "")}
                  </span>
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

            {/* CTOT text */}
            {showCtotText && (
              <div
                className="absolute left-0 right-0 flex items-center justify-center font-bold"
                style={{ top: CTOT_ROW_TOP, height: ROW_HEIGHT, fontSize: CONTENT_FONT_SIZE, color: "#FFFFFF" }}
              >
                {ctotLabel}
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
