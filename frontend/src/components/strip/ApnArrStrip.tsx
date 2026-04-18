import { useState } from "react";
import { getAircraftTypeWithWtc } from "@/lib/utils";
import type { StripProps } from "./types";
import FlightPlanDialog from "@/components/FlightPlanDialog";
import { useStripCallsignInteraction, getCellBorderColor, getFlatStripBorderStyle, SELECTION_COLOR, COLOR_ARR_YELLOW, COLOR_UNEXPECTED_YELLOW, COLOR_MANUAL_BLUE, COLOR_TYPE_HEAVY, getStripOwnership, getCellTextColor, useStripBg, getValidationBlinkStyle } from "./shared";
import { useStripTransfers, useWebSocketStore } from "@/store/store-hooks";
import { RunwayDialog } from "./RunwayDialog";
import { ArrStandDialog } from "./ArrStandDialog";
import { ApronTaxiMapDialog } from "../map-dialogs/ApronTaxiMapDialog";
import { SIBox } from "./SIBox";
import { ValidationStatusDialog } from "./ValidationStatusDialog";

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
  bay,
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
  const { isSelected, handleClick, handleContextMenu, validationDialogOpen, setValidationDialogOpen, validationStatus } = useStripCallsignInteraction({ callsign, selectable, bay, owner, myPosition });
  const cellBorderColor = getCellBorderColor(marked, CELL_BORDER);
  const manualBlue = isManual ? COLOR_MANUAL_BLUE : undefined;
  const stripTransfers = useStripTransfers();
  const isTagRequest = !!stripTransfers[callsign]?.isTagRequest;
  const [runwayOpen, setRunwayOpen] = useState(false);
  const [standOpen, setStandOpen] = useState(false);
  const [taxiMapOpen, setTaxiMapOpen] = useState(false);
  const [fplOpen, setFplOpen] = useState(false);
  const acknowledgeUnexpectedChange = useWebSocketStore(s => s.acknowledgeUnexpectedChange);
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
      <div className="flex flex-col border-r-2 min-w-0 cursor-pointer" style={{ flexGrow: F_CALLSIGN, flexBasis: 0, height: "100%", borderRightColor: cellBorderColor, ...getValidationBlinkStyle(validationStatus, myPosition) }}
        onClick={handleClick}
        onContextMenu={handleContextMenu}
      >
        <div className="flex items-center pl-[0.42vw]" style={{ height: TOP_H, backgroundColor: isSelected ? SELECTION_COLOR : undefined }}>
          <span className="truncate w-full" style={{ fontWeight: "bold", fontSize: "1.04vw", color: manualBlue }}>{callsign}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* A/C type / Registration */}
      <div className="flex flex-col border-r-2 min-w-0" style={{ flexGrow: F_TYPE, flexBasis: 0, height: "100%", borderRightColor: cellBorderColor }}>
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="truncate px-[0.21vw]" style={{ fontWeight: 600, fontSize: "0.63vw", color: aircraftCategory === "H" ? COLOR_TYPE_HEAVY : undefined }}>{getAircraftTypeWithWtc(aircraftType, aircraftCategory)}</span>
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
          <span className="truncate" style={{ fontWeight: "bold", fontSize: "1.04vw" }}>{runway}</span>
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
          {(() => { const twy = taxiway ?? holdingPoint; return <span className="truncate" style={{ fontWeight: "bold", fontSize: "1.04vw", opacity: twy ? 1 : 0.2 }}>{twy || "TWY"}</span>; })()}
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
          <span className="truncate" style={{ fontWeight: "bold", fontSize: "1.04vw", color: getCellTextColor("stand", controllerModifiedFields) }}>{stand}</span>
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
    {validationStatus && (
      <ValidationStatusDialog callsign={callsign} status={validationStatus} open={validationDialogOpen} onOpenChange={setValidationDialogOpen} />
    )}
    </>
  );
}
