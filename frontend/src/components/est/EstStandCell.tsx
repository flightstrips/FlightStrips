import type { CSSProperties } from "react";

import { Bay, type FrontendStandAssignmentEntry, type FrontendStrip } from "@/api/models";
import { SELECTION_COLOR } from "@/components/strip/shared";
import { cn, getSimpleAircraftType } from "@/lib/utils";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { CDM_GREEN, CDM_ORANGE, CTOT_BLUE, computeCDMColors, computeCTOTColors, hasManualTobtSource } from "@/lib/cdmColors";

import {
  EST_CELL_HEIGHT,
  EST_CELL_WIDTH,
  formatTimeLabel,
  getBridgeStatus,
  getVgdsStatus,
  type EstCanvasStand,
} from "@/components/est/metadata";

const STAND_ROW_TOP = 0;
const STAND_ROW_HEIGHT = 27.5;
const CALLSIGN_ROW_TOP = 27.5;
const CALLSIGN_ROW_HEIGHT = 21;
const DETAILS_ROW_TOP = 50.5;
const DETAILS_ROW_HEIGHT = 20;
const READY_ROW_TOP = 72;
const TOBT_ROW_TOP = 88;
const TSAT_ROW_TOP = 105;
const CTOT_ROW_TOP = 121;
const ROW_HEIGHT = 15;
const LABEL_FONT_SIZE = 20;
const DETAILS_FONT_SIZE = 14;
const CONTENT_FONT_SIZE = 12;
const CONTENT_FONT = "Rubik, sans-serif";

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
  departureTransferActive?: boolean;
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
  departureTransferActive = false,
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
  const ctotColors = strip ? computeCTOTColors(strip.ctot, nowMs) : null;
  const emphasizeTobtTime = strip ? hasManualTobtSource(strip.req_tobt_type, strip.tobt_set_by) : false;

  const showTobt = isDeparture && !departureTransferActive && !!strip && strip.tobt !== "";
  const showTsat = isDeparture && !departureTransferActive && !!strip && strip.tsat !== "";
  const showCtot = isClearedDeparture && !!strip?.ctot.trim() && (ctotImproved || !!ctotColors?.showCtot);
  const showReady = isDeparture && !!strip?.start_req;
  const showReleasePoint = isPushing && !!strip?.release_point.trim();
  const ctotBarColor = ctotImproved ? CTOT_BLUE : ctotColors?.ctotBg;
  const ctotTextColor = ctotImproved ? "#FFFFFF" : ctotColors?.ctotColor;

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
    boxShadows.push(`inset 0 0 0 2px ${tobtBarColor === CDM_GREEN ? CDM_GREEN : CDM_ORANGE}`);
  }
  if (showMark) {
    boxShadows.push(`0 0 0 4px ${SELECTION_COLOR}`);
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
              "relative overflow-hidden rounded-xl transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-white",
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
                data-testid="est-tobt-background"
                style={{ top: TOBT_ROW_TOP, height: ROW_HEIGHT, backgroundColor: tobtBarColor }}
              />
            )}
            {tsatBarColor && (
              <div
                className="absolute left-0 right-0"
                data-testid="est-tsat-background"
                style={{ top: TSAT_ROW_TOP, height: ROW_HEIGHT, backgroundColor: tsatBarColor }}
              />
            )}
            {showCtot && (
              <div
                className="absolute left-0 right-0"
                data-testid="est-ctot-background"
                style={{ top: CTOT_ROW_TOP, height: ROW_HEIGHT, backgroundColor: ctotBarColor }}
              />
            )}
            {showReady && (
              <div
                className="absolute left-0 right-0"
                data-testid="est-ready-background"
                style={{ top: READY_ROW_TOP, height: ROW_HEIGHT, backgroundColor: CDM_GREEN }}
              />
            )}
            {/* Stand label */}
            <div
              className="absolute left-0 right-0 flex items-center justify-center text-center font-bold"
              style={{
                top: STAND_ROW_TOP,
                height: STAND_ROW_HEIGHT,
                fontFamily: "Roboto, sans-serif",
                fontSize: LABEL_FONT_SIZE,
              }}
            >
              {stand.label}
            </div>

            {/* Callsign */}
            {(assignment || (strip && !blocked)) && (
               <div
                 className="absolute left-0 right-0 flex items-center justify-center overflow-hidden px-0.5 text-center font-bold"
                 style={{
                  top: CALLSIGN_ROW_TOP,
                  height: CALLSIGN_ROW_HEIGHT,
                  fontFamily: CONTENT_FONT,
                  fontSize: DETAILS_FONT_SIZE,
                  backgroundColor: showMark ? SELECTION_COLOR : undefined,
                  color: showMark ? "#000000" : undefined,
                }}
              >
                <span className="block truncate">{assignment?.callsign ?? strip?.callsign}</span>
              </div>
            )}

            {showReleasePoint && (
              <div
                className="absolute left-0 right-0 flex items-center justify-center"
                style={{ top: READY_ROW_TOP, height: ROW_HEIGHT, fontFamily: CONTENT_FONT, fontSize: DETAILS_FONT_SIZE }}
              >
                {strip!.release_point}
              </div>
            )}

            {strip && !blocked && (
              <div
                className="absolute left-0 right-0 flex items-center justify-between overflow-hidden px-1"
                style={{
                  top: DETAILS_ROW_TOP,
                  height: DETAILS_ROW_HEIGHT,
                  fontFamily: CONTENT_FONT,
                  fontSize: DETAILS_FONT_SIZE,
                }}
              >
                <span className="truncate text-left">{getSimpleAircraftType(strip.aircraft_type)}</span>
                <span className="truncate text-right">{strip.runway}</span>
              </div>
            )}

            {showReady && (
              <div
                className="absolute left-0 right-0 flex items-center justify-center"
                style={{ top: READY_ROW_TOP, height: ROW_HEIGHT, fontFamily: CONTENT_FONT, fontSize: CONTENT_FONT_SIZE, color: "#000000" }}
              >
                READY
              </div>
            )}

            {assignment && !showTobt && (
              <div
                className="absolute left-0 right-0 flex items-center justify-center truncate px-1 uppercase"
                style={{ top: TOBT_ROW_TOP, height: ROW_HEIGHT, fontFamily: CONTENT_FONT, fontSize: 10 }}
              >
                {assignment.stage} · {assignment.source}
              </div>
            )}

            {assignment?.eta && !showTsat && (
              <div
                className="absolute left-0 right-0 flex items-center justify-center"
                style={{ top: TSAT_ROW_TOP, height: ROW_HEIGHT, fontFamily: CONTENT_FONT, fontSize: 10 }}
              >
                ETA {formatTimeLabel(assignment.eta).replace(":", "")}
              </div>
            )}

            {assignment?.expires_at && !showCtot && (
              <div
                className="absolute left-0 right-0 flex items-center justify-center"
                style={{ top: CTOT_ROW_TOP, height: ROW_HEIGHT, fontFamily: CONTENT_FONT, fontSize: 10 }}
              >
                EXP {formatTimeLabel(assignment.expires_at).replace(":", "")}
              </div>
            )}

            {/* TOBT row */}
             {showTobt && (
                <div
                  className="absolute left-0 right-0 flex items-center justify-center gap-1"
                  style={{ top: TOBT_ROW_TOP, height: ROW_HEIGHT, fontFamily: CONTENT_FONT, fontSize: CONTENT_FONT_SIZE, color: tobtBarColor === CDM_GREEN ? "#000000" : undefined }}
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
                 style={{ top: TSAT_ROW_TOP, height: ROW_HEIGHT, fontFamily: CONTENT_FONT, fontSize: CONTENT_FONT_SIZE, color: tsatBarColor === CDM_GREEN ? "#000000" : undefined }}
               >
                 {`TSAT: ${formatTimeLabel(strip!.tsat).replace(":", "")}`}
               </div>
            )}

            {/* CTOT text */}
            {showCtotText && (
              <div
                className="absolute left-0 right-0 flex items-center justify-center font-bold"
                style={{ top: CTOT_ROW_TOP, height: ROW_HEIGHT, fontFamily: CONTENT_FONT, fontSize: CONTENT_FONT_SIZE, color: ctotTextColor }}
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
