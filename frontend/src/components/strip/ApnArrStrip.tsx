import { useState } from "react";
import { getAircraftTypeWithWtc } from "@/lib/utils";
import type { StripProps } from "./types";
import FlightPlanDialog from "@/components/FlightPlanDialog";
import { useStripSelection, getCellBorderColor, getFlatStripBorderStyle, SELECTION_COLOR, COLOR_ARR_YELLOW, COLOR_UNEXPECTED_YELLOW, COLOR_MANUAL_BLUE, COLOR_TYPE_HEAVY, getStripOwnership, getCellTextColor, useStripBg } from "./shared";
import { useStripTransfers, useWebSocketStore } from "@/store/store-hooks";
import { RunwayDialog } from "./RunwayDialog";
import { ArrStandDialog } from "./ArrStandDialog";
import { ApronTaxiMapDialog } from "../map-dialogs/ApronTaxiMapDialog";
import { SIBox } from "./SIBox";

// Height: 4.72vh (51px at 1080p), matches FinalArrStrip ATC arrival strip spec
const TOP_H = "3.15vh"; // 2/3 of 4.72vh
const BOT_H = "1.57vh"; // 1/3 of 4.72vh

/** Gold cell borders — matches the yellow arrival strip design (same as FinalArrStrip). */
const CELL_BORDER = "var(--color-cell-border-arr)";

// Flex-grow proportions (flex-basis: 0 so space is shared proportionally).
// Values match the original pixel widths: SI=40, Callsign=120, Type=80, RWY=54, HS=54, Stand=80
const F_SI       = 40;
const F_CALLSIGN = 120;
const F_TYPE     = 80;
const F_RWY      = 54;
const F_HS       = 54;
const F_STAND    = 80;

/**
 * ApnArrStrip — APN-TAXI-ARR strip used in TWY ARR and STAND bays (status="ARR").
 *
 * 4.72vh strip (51px at 1080p), 90% of bay width, with 2/3 top row / 1/3 bottom row:
 *   [SI] | [callsign] | [actype↑ / reg↓] | [RWY] | [HS] | [stand]
 *
 * Background: yellow (var(--color-strip-arr-bg)).
 */
export function ApnArrStrip({
  callsign,
  aircraftType,
  aircraftCategory,
  runway,
  taxiway,
  holdingPoint,
  stand,
  owner,
  nextControllers,
  previousControllers,
  myPosition,
  selectable,
  marked = false,
  unexpectedChangeFields,
  controllerModifiedFields,
  isManual = false,
}: StripProps) {
  const { isSelected, handleClick } = useStripSelection(callsign, selectable);
  const cellBorderColor = getCellBorderColor(marked, CELL_BORDER);
  const manualBlue = isManual ? COLOR_MANUAL_BLUE : undefined;
  const stripTransfers = useStripTransfers();
  const isTagRequest = !!stripTransfers[callsign]?.isTagRequest;
  const [runwayOpen, setRunwayOpen] = useState(false);
  const [standOpen, setStandOpen] = useState(false);
  const [taxiMapOpen, setTaxiMapOpen] = useState(false);
  const [fplOpen, setFplOpen] = useState(false);
  const acknowledgeUnexpectedChange = useWebSocketStore(s => s.acknowledgeUnexpectedChange);
  const openStripContextMenu = useWebSocketStore(s => s.openStripContextMenu);
  const standYellow = unexpectedChangeFields?.includes("stand");
  const runwayYellow = unexpectedChangeFields?.includes("runway");

  const { isUnconcerned } = getStripOwnership(myPosition, owner, nextControllers, previousControllers);
  const { bg, textWhite } = useStripBg(runway, COLOR_ARR_YELLOW, isTagRequest, isUnconcerned);

  return (
    <>
    <div
      className={`flex ${textWhite ? "text-white" : "text-black"} select-none`}
      style={{
        height: "4.72vh",
        width: "90%",
        backgroundColor: bg,
        ...getFlatStripBorderStyle({}, CELL_BORDER),
      }}
    >
      <SIBox
        callsign={callsign}
        owner={owner}
        nextControllers={nextControllers}
        previousControllers={previousControllers}
        myPosition={myPosition}
        marked={marked}
        flexGrow={F_SI}
        transferringTo={stripTransfers[callsign]?.to ?? ""}
        isTagRequest={isTagRequest}
        baseBorderColor={CELL_BORDER}
      />

      {/* Callsign */}
      <div className="flex flex-col border-r-2 min-w-0 cursor-pointer" style={{ flexGrow: F_CALLSIGN, flexBasis: 0, height: "100%", borderRightColor: cellBorderColor }}
        onClick={handleClick}
        onContextMenu={(e) => { e.preventDefault(); openStripContextMenu(callsign, { x: e.clientX, y: e.clientY }); }}
      >
        <div className="flex items-center pl-2" style={{ height: TOP_H, backgroundColor: isSelected ? SELECTION_COLOR : undefined }}>
          <span className="font-bold text-xl truncate w-full" style={{ color: manualBlue }}>{callsign}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* A/C type / Registration */}
      <div className="flex flex-col border-r-2 min-w-0" style={{ flexGrow: F_TYPE, flexBasis: 0, height: "100%", borderRightColor: cellBorderColor }}>
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="text-xs font-semibold truncate px-1" style={{ color: aircraftCategory === "H" ? COLOR_TYPE_HEAVY : undefined }}>{getAircraftTypeWithWtc(aircraftType, aircraftCategory)}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* RWY */}
      <div
        className="flex flex-col overflow-hidden border-r-2 min-w-0 cursor-pointer hover:brightness-95"
        style={{ flexGrow: F_RWY, flexBasis: 0, height: "100%", borderRightColor: cellBorderColor, backgroundColor: runwayYellow ? COLOR_UNEXPECTED_YELLOW : undefined }}
        onClick={(e) => { e.stopPropagation(); if (runwayYellow) { acknowledgeUnexpectedChange(callsign, "runway"); } else { setRunwayOpen(true); } }}
      >
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="font-bold text-xl truncate">{runway}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* HS / Taxiway */}
      <div
        className="flex flex-col overflow-hidden border-r-2 min-w-0 cursor-pointer hover:brightness-95"
        style={{ flexGrow: F_HS, flexBasis: 0, height: "100%", borderRightColor: cellBorderColor }}
        onClick={(e) => { e.stopPropagation(); setTaxiMapOpen(true); }}
      >
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          {(() => { const twy = taxiway ?? holdingPoint; return <span className="font-bold text-xl truncate" style={{ opacity: twy ? 1 : 0.2 }}>{twy || "TWY"}</span>; })()}
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* Stand */}
      <div
        className="flex flex-col overflow-hidden min-w-0 cursor-pointer hover:brightness-95"
        style={{ flexGrow: F_STAND, flexBasis: 0, height: "100%", backgroundColor: standYellow ? COLOR_UNEXPECTED_YELLOW : undefined }}
        onClick={(e) => {
          e.stopPropagation();
          if (standYellow) {
            acknowledgeUnexpectedChange(callsign, "stand");
          } else {
            setStandOpen(true);
          }
        }}
      >
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="font-bold text-xl truncate" style={{ color: getCellTextColor("stand", controllerModifiedFields) }}>{stand}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>
    </div>

    <RunwayDialog
      open={runwayOpen}
      onOpenChange={setRunwayOpen}
      mode="ASSIGN"
      callsign={callsign}
      direction="arrival"
      currentRunway={runway}
    />
    <ArrStandDialog
      open={standOpen}
      onOpenChange={setStandOpen}
      callsign={callsign}
      currentStand={stand}
    />
    <ApronTaxiMapDialog
      open={taxiMapOpen}
      onOpenChange={setTaxiMapOpen}
      callsign={callsign}
      noMove
    />
    <FlightPlanDialog callsign={callsign} open={fplOpen} onOpenChange={setFplOpen} mode="view" />
    </>
  );
}
