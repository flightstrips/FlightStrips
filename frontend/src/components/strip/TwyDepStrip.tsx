import { useState } from "react";
import { getAircraftTypeWithWtc } from "@/lib/utils";
import { useControllers, useStripTransfers, useTransitionAltitude, useWebSocketStore } from "@/store/store-hooks";
import { formatAltitude } from "@/lib/utils";
import FlightPlanDialog from "@/components/FlightPlanDialog";
import { useCTOTColor } from "@/hooks/useCTOTColor";
import { COLOR_UNEXPECTED_YELLOW, COLOR_TYPE_HEAVY, getValidationBlockedCursor } from "./shared";
import { getStripBg } from "./types";
import type { StripProps } from "./types";
import { useStripCallsignInteraction, getCellBorderColor, getFlatStripBorderStyle, SELECTION_COLOR, FONT, getStripOwnership, getCellTextColor, useStripBg, getValidationBlinkStyle } from "./shared";
import { TaxiMapDialog } from "../map-dialogs/TaxiMapDialog";
import { HoldingPointDialog } from "../map-dialogs/HoldingPointDialog";
import { SIBox } from "./SIBox";
import { TAXI_MAP_POINTS } from "@/config/ekch";
import { Bay } from "@/api/models";
import { ValidationStatusDialog } from "./ValidationStatusDialog";

// Heights— 4.72dvh total (51px at 1080p), 2/3 top / 1/3 bottom (used by callsign and SID/dest)
const TOP_H      = "3.15dvh";  // 2/3 of 4.72dvh
const BOT_H      = "1.57dvh";  // 1/3 of 4.72dvh
const TOP_HALF_H = "1.575dvh"; // half of TOP_H — used by SID/dest two-line split

// Equal halves — used by type/squawk, stand/ctot, TWY, runway/HP, FL/heading
const HALF_H = "2.36dvh"; // 1/2 of 4.72dvh

// Flex-grow proportions (flex-basis: 0 so space is shared proportionally).
// Values match the original pixel widths: SI=40, Callsign=120, Type/Squawk=60, Stand/CTOT=60, Small×3=53, SID/Dest=80
const F_SI         = 40;
const F_CALLSIGN   = 120;
const F_TYPE_SQ    = 60;
const F_STAND_CTOT = 60;
const F_SMALL      = 53;
const F_SID_DEST   = 80;

// -----------------------------------------------------------------------------
// TwyDepStrip — TWY-DEP strip for the TETW tower view (status="TWY-DEP").
//
// 4.72dvh height (51px at 1080p), scales to 95% of bay width. Cells left → right:
//   [SI] | [callsign + :freq] | [type / squawk] |
//   [stand / ctot] | [TWY label] |
//   [runway / HP] | [FL / heading] | [SID / dest]
//
// Background: cyan (var(--color-strip-dep-bg)).
// -----------------------------------------------------------------------------

export function TwyDepStrip({
  callsign,
  bay,
  pdcStatus,
  aircraftType,
  aircraftCategory,
  squawk,
  assignedSquawk,
  stand,
  ctot,
  runway,
  holdingPoint,
  clearedAltitude,
  heading,
  sid,
  destination,
  owner,
  nextControllers,
  previousControllers,
  myPosition,
  selectable,
  marked = false,
  runwayCleared = false,
  runwayConfirmed = false,
  unexpectedChangeFields,
  controllerModifiedFields,
}: StripProps) {
  const { isSelected, isValidationActive, handleClick, handleContextMenu, guardValidationAction, validationDialogOpen, setValidationDialogOpen, validationStatus } = useStripCallsignInteraction({ callsign, selectable, bay, owner, myPosition });
  const stripTransfers = useStripTransfers();
  const { ctotBg, ctotColor, showCtot } = useCTOTColor(ctot ?? "");
  const isTagRequest = !!stripTransfers[callsign]?.isTagRequest;
  const cellBorderColor = getCellBorderColor(marked);
  const { isUnconcerned, isAssumed } = getStripOwnership(myPosition, owner, nextControllers, previousControllers);
  const { bg, textWhite } = useStripBg(runway, getStripBg(pdcStatus, undefined, bay), isTagRequest, isUnconcerned, pdcStatus, bay);
  const controllers = useControllers();
  const transitionAltitude = useTransitionAltitude();
  const [showTaxiMap, setShowTaxiMap] = useState(false);
  const [showHpMap, setShowHpMap] = useState(false);
  const [fplOpen, setFplOpen] = useState(false);
  const runwayClearance = useWebSocketStore(s => s.runwayClearance);
  const runwayConfirmation = useWebSocketStore(s => s.runwayConfirmation);
  const acknowledgeUnexpectedChange = useWebSocketStore(s => s.acknowledgeUnexpectedChange);
  const standYellow = unexpectedChangeFields?.includes("stand");
  const releasePointYellow = unexpectedChangeFields?.includes("release_point");
  const isCoordinationMode = (!!owner && !!myPosition && owner !== myPosition) || !!releasePointYellow;

  // RWY cell background color (only when strip is in DEPART bay):
  // - runway_cleared = false: blue/cyan (in bay, awaiting clearance)
  // - runway_cleared = true, runway_confirmed = true: green (controller acknowledged)
  // - runway_cleared = true, runway_confirmed = false: red (new/incoming, needs attention)
  let rwyColor: string | undefined;
  if (bay === Bay.Depart) {
    if (!runwayCleared) {
      rwyColor = "var(--color-strip-dep-bg)";
    } else if (runwayConfirmed) {
      rwyColor = "var(--color-runway-confirmed)";
    } else {
      rwyColor = "var(--color-runway-closed)";
    }
  }

  // Determine display slot for the release point:
  // - "hp"-typed points → HP cell
  // - "cl"-typed or untyped points → TWY cell
  const hpLabels = new Set(TAXI_MAP_POINTS.filter(p => p.type === "hp").map(p => p.label));
  const isHp = holdingPoint ? hpLabels.has(holdingPoint) : false;
  const hpDisplay = isHp ? (holdingPoint ?? "") : "";
  const twyDisplay = !isHp ? (holdingPoint ?? "") : "";

  // Next position frequency — controller.position IS the frequency string (e.g. "118.105")
  const nextPosition = nextControllers?.find(pos => pos !== myPosition);
  const nextController = controllers.find(c => c.position === nextPosition);
  const nextFreq = nextController ? `:${nextController.position}` : "";

  // Cleared altitude display: use FL notation above transition altitude, feet below.
  const fl = clearedAltitude ? formatAltitude(clearedAltitude, transitionAltitude) : "";
  const hdg = heading ? String(heading) : "";

  return (
    <>
    <div
      className={`flex ${textWhite ? "text-white" : "text-black"} select-none`}
      style={{
        height: "4.72dvh",
        width: "95%",
        backgroundColor: bg,
        cursor: isValidationActive ? "not-allowed" : undefined,
        ...getFlatStripBorderStyle({ borderBottom: "1px solid white" }),
      }}
    >
      {/* SI / ownership */}
      <SIBox
        callsign={callsign}
        bay={bay}
        owner={owner}
        nextControllers={nextControllers}
        previousControllers={previousControllers}
        myPosition={myPosition}
        flexGrow={F_SI}
        transferringTo={stripTransfers[callsign]?.to ?? ""}
        isTagRequest={isTagRequest}
      />

      {/* Callsign; top 2/3 = callsign, bottom 1/3 = :freq */}
      <div
        className="flex flex-col border-r-2 min-w-0 cursor-pointer"
        style={{ flexGrow: F_CALLSIGN, flexBasis: 0, height: "100%", borderRightColor: cellBorderColor, cursor: getValidationBlockedCursor(isValidationActive), ...getValidationBlinkStyle(validationStatus, myPosition) }}
        onClick={handleClick}
        onContextMenu={handleContextMenu}
      >
        <div
          className="flex items-center pl-[0.42vw] overflow-hidden"
          style={{ height: TOP_H, backgroundColor: isSelected ? SELECTION_COLOR : undefined }}
        >
          <span className="truncate w-full" style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "1.04vw" }}>
            {callsign}
          </span>
        </div>
        <div className="flex items-center pl-[0.42vw]" style={{ height: BOT_H }}>
          <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "0.57vw" }}>{nextFreq}</span>
        </div>
      </div>

      {/* Type / Squawk; bold type, light squawk */}
      <div
        className="flex flex-col border-r-2 min-w-0"
        style={{ flexGrow: F_TYPE_SQ, flexBasis: 0, height: "100%", borderRightColor: cellBorderColor }}
      >
        <div className="flex items-center justify-center overflow-hidden" style={{ height: HALF_H }}>
          <span className="truncate px-[0.21vw]" style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "0.68vw", color: aircraftCategory === "H" ? COLOR_TYPE_HEAVY : undefined }}>
            {getAircraftTypeWithWtc(aircraftType, aircraftCategory)}
          </span>
        </div>
        <div
          className="flex items-center justify-center overflow-hidden"
          style={{ height: HALF_H, backgroundColor: assignedSquawk && squawk && assignedSquawk !== squawk ? "var(--color-runway-closed)" : undefined }}
        >
          <span className="truncate px-[0.21vw]" style={{ fontFamily: FONT, fontWeight: 300, fontSize: "0.68vw" }}>
            {assignedSquawk ?? squawk}
          </span>
        </div>
      </div>

      {/* Stand / CTOT; bold stand, light ctot */}
      <div
        className="flex flex-col border-r-2 min-w-0"
        style={{ flexGrow: F_STAND_CTOT, flexBasis: 0, height: "100%", borderRightColor: cellBorderColor }}
      >
        <div
          className="flex items-center justify-center overflow-hidden"
          style={{ height: HALF_H, backgroundColor: standYellow ? COLOR_UNEXPECTED_YELLOW : undefined, cursor: standYellow ? getValidationBlockedCursor(isValidationActive) : undefined }}
          onClick={standYellow ? (e) => guardValidationAction(e, () => acknowledgeUnexpectedChange(callsign, "stand")) : undefined}
        >
          <span className="truncate px-[0.21vw]" style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "0.68vw", color: getCellTextColor("stand", controllerModifiedFields) }}>
            {stand}
          </span>
        </div>
        <div className="flex items-center justify-center overflow-hidden" style={{ height: HALF_H, backgroundColor: ctotBg || undefined, color: ctotColor }}>
          <span className="truncate px-[0.21vw]" style={{ fontFamily: FONT, fontWeight: 300, fontSize: "0.68vw" }}>
            {showCtot ? ctot : ""}
          </span>
        </div>
      </div>

      {/* TWY label; whole cell clickable → taxi map */}
      <div
        className="flex flex-col border-r-2 min-w-0 cursor-pointer"
        style={{
          flexGrow: F_SMALL,
          flexBasis: 0,
          height: "100%",
          borderRightColor: cellBorderColor,
          cursor: getValidationBlockedCursor(isValidationActive),
          backgroundColor: releasePointYellow && !isHp ? COLOR_UNEXPECTED_YELLOW : undefined,
        }}
        onClick={(e) => guardValidationAction(e, () => setShowTaxiMap(true))}
      >
        <div className="flex items-center justify-center h-full">
          <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "0.73vw", opacity: twyDisplay ? 1 : 0.2, color: getCellTextColor("release_point", controllerModifiedFields) }}>
            {twyDisplay || "TWY"}
          </span>
        </div>
      </div>

      {/* Runway / HP; dividing line between; bold runway, plain holding point */}
      <div
        className="flex flex-col border-r-2 min-w-0"
        style={{ flexGrow: F_SMALL, flexBasis: 0, height: "100%", borderRightColor: cellBorderColor }}
      >
        <div
          className="flex items-center justify-center border-b-2 cursor-pointer"
          style={{ height: HALF_H, borderBottomColor: cellBorderColor, backgroundColor: rwyColor, cursor: isAssumed ? getValidationBlockedCursor(isValidationActive) : undefined }}
          onClick={isAssumed ? (e) => guardValidationAction(e, () => {
            if (runwayCleared && !runwayConfirmed) {
              runwayConfirmation(callsign);
            } else {
              runwayClearance(callsign);
            }
          }) : undefined}
        >
          <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "0.73vw", color: getCellTextColor("runway", controllerModifiedFields) }}>{runway}</span>
        </div>
        <div
          className="flex items-center justify-center cursor-pointer"
          style={{ height: HALF_H, backgroundColor: releasePointYellow && isHp ? COLOR_UNEXPECTED_YELLOW : undefined, cursor: getValidationBlockedCursor(isValidationActive) }}
          onClick={(e) => guardValidationAction(e, () => setShowHpMap(true))}
        >
          <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "0.73vw", opacity: hpDisplay ? 1 : 0.2, color: getCellTextColor("release_point", controllerModifiedFields) }}>
            {hpDisplay || "HP"}
          </span>
        </div>
      </div>

      {/* Cleared FL / Heading; both bold */}
      <div
        className="flex flex-col border-r-2 min-w-0"
        style={{ flexGrow: F_SMALL, flexBasis: 0, height: "100%", borderRightColor: cellBorderColor }}
      >
        <div className="flex items-center justify-center" style={{ height: HALF_H }}>
          <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "0.68vw" }}>{fl}</span>
        </div>
        <div className="flex items-center justify-center" style={{ height: HALF_H }}>
          <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "0.68vw" }}>{hdg}</span>
        </div>
      </div>

      {/* SID / Destination; two lines in top 2/3, bottom 1/3 empty */}
      <div
        className="flex flex-col overflow-hidden min-w-0 cursor-pointer hover:brightness-95"
        style={{ flexGrow: F_SID_DEST, flexBasis: 0, height: "100%", cursor: "pointer" }}
        onClick={(e) => { e.stopPropagation(); setFplOpen(true); }}
      >
        <div className="flex items-center justify-center pl-[0.21vw] overflow-hidden" style={{ height: TOP_HALF_H }}>
          <span className="truncate" style={{ fontFamily: FONT, fontWeight: "normal", fontSize: "0.63vw", color: getCellTextColor("sid", controllerModifiedFields) }}>
            {sid}
          </span>
        </div>
        <div className="flex items-center justify-center pl-[0.21vw] overflow-hidden" style={{ height: TOP_HALF_H }}>
          <span className="truncate" style={{ fontFamily: FONT, fontWeight: "normal", fontSize: "0.63vw" }}>
            {destination}
          </span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>
    </div>

    <TaxiMapDialog
      open={showTaxiMap}
      onOpenChange={setShowTaxiMap}
      callsign={callsign}
    />
    <HoldingPointDialog
      open={showHpMap}
      onOpenChange={setShowHpMap}
      callsign={callsign}
      runway={runway}
      coordinationMode={isCoordinationMode}
    />
    <FlightPlanDialog callsign={callsign} open={fplOpen} onOpenChange={setFplOpen} mode="view" />
    {validationStatus && (
      <ValidationStatusDialog callsign={callsign} status={validationStatus} open={validationDialogOpen} onOpenChange={setValidationDialogOpen} />
    )}
    </>
  );
}
