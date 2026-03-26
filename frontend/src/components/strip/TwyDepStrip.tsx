import { useState } from "react";
import { getSimpleAircraftType } from "@/lib/utils";
import { useControllers, useStripTransfers, useWebSocketStore } from "@/store/store-hooks";
import { useCTOTColor } from "@/hooks/useCTOTColor";
import { COLOR_UNEXPECTED_YELLOW } from "./shared";
import { getStripBg } from "./types";
import type { StripProps } from "./types";
import { useStripSelection, getCellBorderColor, getFlatStripBorderStyle, SELECTION_COLOR, FONT, getStripOwnership, resolveStripBg, getCellTextColor } from "./shared";
import { TaxiMapDialog } from "../map-dialogs/TaxiMapDialog";
import { HoldingPointDialog } from "../map-dialogs/HoldingPointDialog";
import { SIBox } from "./SIBox";
import { TAXI_MAP_POINTS } from "@/config/ekch";
import { Bay } from "@/api/models";

// Heights — 4.72vh total (51px at 1080p), 2/3 top / 1/3 bottom (used by callsign and SID/dest)
const TOP_H      = "3.15vh";  // 2/3 of 4.72vh
const BOT_H      = "1.57vh";  // 1/3 of 4.72vh
const TOP_HALF_H = "1.575vh"; // half of TOP_H — used by SID/dest two-line split

// Equal halves — used by type/squawk, stand/ctot, TWY, runway/HP, FL/heading
const HALF_H = "2.36vh"; // 1/2 of 4.72vh

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
// 4.72vh height (51px at 1080p), scales to 95% of bay width. Cells left → right:
//   [SI] | [callsign + :freq] | [type / squawk] |
//   [stand / ctot] | [TWY label] |
//   [runway / HP] | [FL / heading] | [SID / dest]
//
// Background: cyan (#bef5ef).
// -----------------------------------------------------------------------------

export function TwyDepStrip({
  callsign,
  bay,
  pdcStatus,
  aircraftType,
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
  const { isSelected, handleClick } = useStripSelection(callsign, selectable);
  const stripTransfers = useStripTransfers();
  const { ctotBg, ctotColor, showCtot } = useCTOTColor(ctot ?? "");
  const isTagRequest = !!stripTransfers[callsign]?.isTagRequest;
  const cellBorderColor = getCellBorderColor(marked);
  const { isUnconcerned } = getStripOwnership(myPosition, owner, nextControllers, previousControllers);
  const controllers = useControllers();
  const [showTaxiMap, setShowTaxiMap] = useState(false);
  const [showHpMap, setShowHpMap] = useState(false);
  const runwayClearance = useWebSocketStore(s => s.runwayClearance);
  const runwayConfirmation = useWebSocketStore(s => s.runwayConfirmation);
  const acknowledgeUnexpectedChange = useWebSocketStore(s => s.acknowledgeUnexpectedChange);
  const openStripContextMenu = useWebSocketStore(s => s.openStripContextMenu);
  const standYellow = unexpectedChangeFields?.includes("stand");
  const releasePointYellow = unexpectedChangeFields?.includes("release_point");

  // RWY cell background color (only when strip is in DEPART bay):
  // - runway_cleared = false: blue/cyan (in bay, awaiting clearance)
  // - runway_cleared = true, runway_confirmed = true: green (controller acknowledged)
  // - runway_cleared = true, runway_confirmed = false: red (new/incoming, needs attention)
  let rwyColor: string | undefined;
  if (bay === Bay.Depart) {
    if (!runwayCleared) {
      rwyColor = "#BEF5EF";
    } else if (runwayConfirmed) {
      rwyColor = "#70ED45";
    } else {
      rwyColor = "#F43A3A";
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

  // Cleared FL — altitude in feet → FL (e.g. 12000 → "FL120")
  const fl = clearedAltitude ? `FL${Math.floor(clearedAltitude / 100)}` : "";
  const hdg = heading ? String(heading) : "";

  return (
    <>
    <div
      className={`flex text-black select-none${selectable ? " cursor-pointer" : ""}`}
      style={{
        height: "4.72vh",
        width: "95%",
        backgroundColor: resolveStripBg(getStripBg(pdcStatus), isTagRequest, isUnconcerned),
        ...getFlatStripBorderStyle({ borderBottom: "1px solid white" }),
      }}
      onClick={handleClick}
      onContextMenu={(e) => { e.preventDefault(); openStripContextMenu(callsign, { x: e.clientX, y: e.clientY }); }}
    >
      {/* SI / ownership */}
      <SIBox
        callsign={callsign}
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
        className="flex flex-col border-r-2 min-w-0"
        style={{ flexGrow: F_CALLSIGN, flexBasis: 0, height: "100%", borderRightColor: cellBorderColor }}
      >
        <div
          className="flex items-center pl-2 overflow-hidden"
          style={{ height: TOP_H, backgroundColor: isSelected ? SELECTION_COLOR : undefined }}
        >
          <span className="truncate w-full" style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 20 }}>
            {callsign}
          </span>
        </div>
        <div className="flex items-center pl-2" style={{ height: BOT_H }}>
          <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 11 }}>{nextFreq}</span>
        </div>
      </div>

      {/* Type / Squawk; bold type, light squawk */}
      <div
        className="flex flex-col border-r-2 min-w-0"
        style={{ flexGrow: F_TYPE_SQ, flexBasis: 0, height: "100%", borderRightColor: cellBorderColor }}
      >
        <div className="flex items-center justify-center overflow-hidden" style={{ height: HALF_H }}>
          <span className="truncate px-1" style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 13 }}>
            {getSimpleAircraftType(aircraftType)}
          </span>
        </div>
        <div
          className="flex items-center justify-center overflow-hidden"
          style={{ height: HALF_H, backgroundColor: assignedSquawk && squawk && assignedSquawk !== squawk ? "#F43A3A" : undefined }}
        >
          <span className="truncate px-1" style={{ fontFamily: FONT, fontWeight: 300, fontSize: 13 }}>
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
          style={{ height: HALF_H, backgroundColor: standYellow ? COLOR_UNEXPECTED_YELLOW : undefined, cursor: standYellow ? "pointer" : undefined }}
          onClick={standYellow ? (e) => { e.stopPropagation(); acknowledgeUnexpectedChange(callsign, "stand"); } : undefined}
        >
          <span className="truncate px-1" style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 13, color: getCellTextColor("stand", controllerModifiedFields) }}>
            {stand}
          </span>
        </div>
        <div className="flex items-center justify-center overflow-hidden" style={{ height: HALF_H, backgroundColor: ctotBg || undefined, color: ctotColor }}>
          <span className="truncate px-1" style={{ fontFamily: FONT, fontWeight: 300, fontSize: 13 }}>
            {showCtot ? ctot : ""}
          </span>
        </div>
      </div>

      {/* TWY label; whole cell clickable → taxi map */}
      <div
        className="flex flex-col border-r-2 min-w-0 cursor-pointer"
        style={{ flexGrow: F_SMALL, flexBasis: 0, height: "100%", borderRightColor: cellBorderColor, backgroundColor: releasePointYellow ? COLOR_UNEXPECTED_YELLOW : undefined }}
        onClick={(e) => { e.stopPropagation(); if (releasePointYellow) { acknowledgeUnexpectedChange(callsign, "release_point"); } else { setShowTaxiMap(true); } }}
      >
        <div className="flex items-center justify-center h-full">
          <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 14, opacity: twyDisplay ? 1 : 0.2, color: getCellTextColor("release_point", controllerModifiedFields) }}>
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
          style={{ height: HALF_H, borderBottomColor: cellBorderColor, backgroundColor: rwyColor }}
          onClick={(e) => { e.stopPropagation(); if (runwayCleared && !runwayConfirmed) { runwayConfirmation(callsign); } else { runwayClearance(callsign); } }}
        >
          <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 14, color: getCellTextColor("runway", controllerModifiedFields) }}>{runway}</span>
        </div>
        <div
          className="flex items-center justify-center cursor-pointer"
          style={{ height: HALF_H }}
          onClick={(e) => { e.stopPropagation(); setShowHpMap(true); }}
        >
          <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 14, opacity: hpDisplay ? 1 : 0.2, color: getCellTextColor("release_point", controllerModifiedFields) }}>
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
          <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 13 }}>{fl}</span>
        </div>
        <div className="flex items-center justify-center" style={{ height: HALF_H }}>
          <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 13 }}>{hdg}</span>
        </div>
      </div>

      {/* SID / Destination; two lines in top 2/3, bottom 1/3 empty */}
      <div
        className="flex flex-col overflow-hidden min-w-0"
        style={{ flexGrow: F_SID_DEST, flexBasis: 0, height: "100%" }}
      >
        <div className="flex items-center justify-center pl-1 overflow-hidden" style={{ height: TOP_HALF_H }}>
          <span className="truncate" style={{ fontFamily: FONT, fontWeight: "normal", fontSize: 12, color: getCellTextColor("sid", controllerModifiedFields) }}>
            {sid}
          </span>
        </div>
        <div className="flex items-center justify-center pl-1 overflow-hidden" style={{ height: TOP_HALF_H }}>
          <span className="truncate" style={{ fontFamily: FONT, fontWeight: "normal", fontSize: 12 }}>
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
    />
    </>
  );
}
