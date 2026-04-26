import { useState } from "react";
import { getAircraftTypeWithWtc } from "@/lib/utils";
import { useStripTransfers, useWebSocketStore } from "@/store/store-hooks";
import FlightPlanDialog from "@/components/FlightPlanDialog";
import { Bay } from "@/api/models";
import type { StripProps } from "./types";
import { useStripCallsignInteraction, getCellBorderColor, getFlatStripBorderStyle, SELECTION_COLOR, FONT, COLOR_ARR_YELLOW, COLOR_TYPE_HEAVY, getStripOwnership, useStripBg, getValidationBlinkStyle, getValidationBlockedCursor } from "./shared";
import { SIBox } from "./SIBox";
import { ArrStandDialog } from "./ArrStandDialog";
import { TaxiMapDialog } from "@/components/map-dialogs/TaxiMapDialog";
import { ValidationStatusDialog } from "./ValidationStatusDialog";

/** Gold cell borders — matches the yellow arrival strip design. */
const CELL_BORDER = "var(--color-cell-border-arr)";

// Heights — 4.72vh total (51px at 1080p), 2/3 top / 1/3 bottom
const TOP_H = "3.15vh";
const BOT_H = "1.57vh";

// Flex-grow proportions (flex-basis: 0 so space is shared proportionally).
// Values match the original pixel widths: SI=40, Callsign=120, Type=80, Taxiway=80, RWY=54, Stand=80
const F_SI       = 40;
const F_CALLSIGN = 120;
const F_TYPE     = 80;
const F_TAXIWAY  = 80;
const F_RWY      = 54;
const F_STAND    = 80;

/**
 * FinalArrStrip — strip for FINAL, RWY-ARR, and TWY-ARR bays (status="FINAL-ARR").
 *
 * 4.72vh strip (51px at 1080p), 95% of bay width, with 2/3 top row / 1/3 bottom row:
 *   [SI] | [callsign] | [type↑ / squawk↓] |
 *   [stand] | [runway↑ / holding point↓] | [stand (reserved)]
 *
 * Background: yellow (var(--color-strip-arr-bg)). Cell borders: gold (var(--color-cell-border-arr)).
 */
export function FinalArrStrip({
  callsign,
  bay,
  aircraftType,
  aircraftCategory,
  squawk,
  assignedSquawk,
  runway,
  holdingPoint,
  stand,
  owner,
  nextControllers,
  previousControllers,
  myPosition,
  selectable,
  marked = false,
  runwayCleared = false,
  runwayConfirmed = false,
}: StripProps) {
  const { isSelected, isValidationActive, handleClick, handleContextMenu, guardValidationAction, validationDialogOpen, setValidationDialogOpen, validationStatus } = useStripCallsignInteraction({ callsign, selectable, bay, owner, myPosition });
  const cellBorderColor = getCellBorderColor(marked, CELL_BORDER);
  const stripTransfers = useStripTransfers();
  const isTagRequest = !!stripTransfers[callsign]?.isTagRequest;
  const { isUnconcerned, isAssumed } = getStripOwnership(myPosition, owner, nextControllers, previousControllers);
  const { bg, textWhite } = useStripBg(runway, COLOR_ARR_YELLOW, isTagRequest, isUnconcerned);
  const runwayClearance = useWebSocketStore(s => s.runwayClearance);
  const runwayConfirmation = useWebSocketStore(s => s.runwayConfirmation);
  const [standOpen, setStandOpen] = useState(false);
  const [taxiMapOpen, setTaxiMapOpen] = useState(false);
  const [fplOpen, setFplOpen] = useState(false);

  // RWY cell color — only when cleared in RWY_ARR bay:
  // - runway_confirmed = true: green (controller acknowledged)
  // - runway_cleared = true, runway_confirmed = false: red (new/incoming, needs attention)
  // - not yet cleared: no background (default strip color)
  let rwyColor: string | undefined;
  if (bay === Bay.RwyArr && runwayCleared) {
    rwyColor = runwayConfirmed ? "var(--color-runway-confirmed)" : "var(--color-runway-closed)";
  }

  return (
    <>
    <div
      className={`flex ${textWhite ? "text-white" : "text-black"} select-none`}
      style={{
        height: "4.72vh",
        width: "95%",
        backgroundColor: bg,
        cursor: isValidationActive ? "not-allowed" : undefined,
        ...getFlatStripBorderStyle({}, CELL_BORDER),
      }}
    >
      {/* SI / ownership */}
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

      {/* Callsign; top 2/3 = callsign */}
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
        <div style={{ height: BOT_H }} />
      </div>

      {/* Type / Squawk */}
      <div
        className="flex flex-col border-r-2 min-w-0"
        style={{ flexGrow: F_TYPE, flexBasis: 0, height: "100%", borderRightColor: cellBorderColor }}
      >
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="truncate px-[0.21vw]" style={{ fontFamily: FONT, fontWeight: 600, fontSize: "0.63vw", color: aircraftCategory === "H" ? COLOR_TYPE_HEAVY : undefined }}>
            {getAircraftTypeWithWtc(aircraftType, aircraftCategory)}
          </span>
        </div>
        <div
          className="flex items-center justify-center"
          style={{ height: BOT_H, backgroundColor: assignedSquawk && squawk && assignedSquawk !== squawk ? "var(--color-runway-closed)" : undefined }}
        >
          <span className="truncate px-[0.21vw]" style={{ fontFamily: FONT, fontSize: "0.63vw" }}>
            {assignedSquawk ?? squawk}
          </span>
        </div>
      </div>

      {/* Stand (left of runway); stand in top 2/3, bottom 1/3 empty */}
      <div
        className="flex flex-col border-r-2 min-w-0 cursor-pointer hover:brightness-95"
        style={{ flexGrow: F_TAXIWAY, flexBasis: 0, height: "100%", borderRightColor: cellBorderColor, cursor: isAssumed ? getValidationBlockedCursor(isValidationActive) : undefined }}
        onClick={isAssumed ? (e) => guardValidationAction(e, () => setStandOpen(true)) : undefined}
      >
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="truncate px-[0.21vw]" style={{ fontFamily: FONT, fontWeight: 600, fontSize: "0.83vw" }}>
            {stand}
          </span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* Runway / Holding Point */}
      <div
        className="flex flex-col border-r-2 min-w-0"
        style={{ flexGrow: F_RWY, flexBasis: 0, height: "100%", borderRightColor: cellBorderColor, backgroundColor: rwyColor }}
      >
        <div
          className={`flex items-center justify-center${bay === Bay.Final || bay === Bay.RwyArr ? " cursor-pointer" : ""}`}
          style={{ height: TOP_H, cursor: (bay === Bay.Final || bay === Bay.RwyArr) && isAssumed ? getValidationBlockedCursor(isValidationActive) : undefined }}
          onClick={(bay === Bay.Final || bay === Bay.RwyArr) && isAssumed ? (e) => guardValidationAction(e, () => {
            if (runwayCleared && !runwayConfirmed) {
              runwayConfirmation(callsign);
            } else {
              runwayClearance(callsign);
            }
          }) : undefined}
        >
          <span className="truncate" style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "0.94vw" }}>
            {runway}
          </span>
        </div>
        <div
          className="flex items-center justify-center cursor-pointer"
          style={{ height: BOT_H, cursor: getValidationBlockedCursor(isValidationActive) }}
          onClick={(e) => guardValidationAction(e, () => setTaxiMapOpen(true))}
        >
          <span style={{ fontFamily: FONT, fontSize: "0.63vw", opacity: holdingPoint ? 1 : 0.2 }}>
            {holdingPoint || "TWY"}
          </span>
        </div>
      </div>

      {/* Stand (reserved) — clickable to open flight plan */}
      <div
        className="flex flex-col overflow-hidden min-w-0 cursor-pointer hover:brightness-95"
        style={{ flexGrow: F_STAND, flexBasis: 0, height: "100%", cursor: "pointer" }}
        onClick={(e) => { e.stopPropagation(); setFplOpen(true); }}
      />
    </div>

    <ArrStandDialog
      open={standOpen}
      onOpenChange={setStandOpen}
      callsign={callsign}
      currentStand={stand}
    />
    <TaxiMapDialog
      open={taxiMapOpen}
      onOpenChange={setTaxiMapOpen}
      callsign={callsign}
    />
    <FlightPlanDialog callsign={callsign} open={fplOpen} onOpenChange={setFplOpen} mode="view" />
    {validationStatus && (
      <ValidationStatusDialog callsign={callsign} status={validationStatus} open={validationDialogOpen} onOpenChange={setValidationDialogOpen} />
    )}
    </>
  );
}
