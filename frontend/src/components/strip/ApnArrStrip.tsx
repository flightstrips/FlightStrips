import { useState } from "react";
import { getSimpleAircraftType } from "@/lib/utils";
import type { StripProps } from "./types";
import { useStripSelection, getCellBorderColor, getFlatStripBorderStyle, SELECTION_COLOR, COLOR_ARR_YELLOW, COLOR_UNEXPECTED_YELLOW, COLOR_MANUAL_BLUE, getStripOwnership, resolveStripBg, getCellTextColor } from "./shared";
import { useStripTransfers, useWebSocketStore } from "@/store/store-hooks";
import { RunwayDialog } from "./RunwayDialog";
import { ArrStandDialog } from "./ArrStandDialog";
import { ApronTaxiMapDialog } from "../map-dialogs/ApronTaxiMapDialog";
import { SIBox } from "./SIBox";

// Height: 48px fixed (intentional — matches FinalArrStrip ATC arrival strip spec)
const TOP_H = 32; // 2/3 of 48px
const BOT_H = 16; // 1/3 of 48px

/** Gold cell borders — matches the yellow arrival strip design (same as FinalArrStrip). */
const CELL_BORDER = "#FFD100";

/**
 * ApnArrStrip — APN-TAXI-ARR strip used in TWY ARR and STAND bays (status="ARR").
 *
 * 48px strip with 2/3 (32px) top row / 1/3 (16px) bottom row vertical layout:
 *   [40px SI] | [120px callsign] | [80px actype↑ / reg↓] | [54px RWY] | [54px HS] | [80px stand]
 *
 * Background: yellow (#fff28e).
 */
export function ApnArrStrip({
  callsign,
  aircraftType,
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
  const acknowledgeUnexpectedChange = useWebSocketStore(s => s.acknowledgeUnexpectedChange);
  const openStripContextMenu = useWebSocketStore(s => s.openStripContextMenu);
  const standYellow = unexpectedChangeFields?.includes("stand");
  const runwayYellow = unexpectedChangeFields?.includes("runway");

  const { isUnconcerned } = getStripOwnership(myPosition, owner, nextControllers, previousControllers);

  return (
    <>
    <div
      className={`flex text-black select-none${selectable ? " cursor-pointer" : ""}`}
      style={{
        height: 48, // 48px fixed — intentional ATC arrival strip height
        width: 428,
        backgroundColor: resolveStripBg(COLOR_ARR_YELLOW, isTagRequest, isUnconcerned),
        ...getFlatStripBorderStyle({}, CELL_BORDER),
      }}
      onClick={handleClick}
      onContextMenu={(e) => { e.preventDefault(); openStripContextMenu(callsign, { x: e.clientX, y: e.clientY }); }}
    >
      <SIBox
        callsign={callsign}
        owner={owner}
        nextControllers={nextControllers}
        previousControllers={previousControllers}
        myPosition={myPosition}
        marked={marked}
        transferringTo={stripTransfers[callsign]?.to ?? ""}
        isTagRequest={isTagRequest}
        baseBorderColor={CELL_BORDER}
      />

      {/* Callsign — 120px */}
      <div className="flex-shrink-0 flex flex-col border-r-2" style={{ width: 120, height: "100%", borderRightColor: cellBorderColor }}>
        <div className="flex items-center pl-2" style={{ height: TOP_H, backgroundColor: isSelected ? SELECTION_COLOR : undefined }}>
          <span className="font-bold text-xl truncate w-full" style={{ color: manualBlue }}>{callsign}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* A/C type / Registration — 80px */}
      <div className="flex-shrink-0 flex flex-col border-r-2" style={{ width: 80, height: "100%", borderRightColor: cellBorderColor }}>
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="text-xs font-semibold truncate px-1">{getSimpleAircraftType(aircraftType)}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* RWY — 54px */}
      <div
        className="flex-shrink-0 flex flex-col overflow-hidden border-r-2 cursor-pointer hover:brightness-95"
        style={{ width: 54, height: "100%", borderRightColor: cellBorderColor, backgroundColor: runwayYellow ? COLOR_UNEXPECTED_YELLOW : undefined }}
        onClick={(e) => { e.stopPropagation(); if (runwayYellow) { acknowledgeUnexpectedChange(callsign, "runway"); } else { setRunwayOpen(true); } }}
      >
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="font-bold text-xl truncate">{runway}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* HS / Taxiway — 54px */}
      <div
        className="flex-shrink-0 flex flex-col overflow-hidden border-r-2 cursor-pointer hover:brightness-95"
        style={{ width: 54, height: "100%", borderRightColor: cellBorderColor }}
        onClick={(e) => { e.stopPropagation(); setTaxiMapOpen(true); }}
      >
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          {(() => { const twy = taxiway ?? holdingPoint; return <span className="font-bold text-xl truncate" style={{ opacity: twy ? 1 : 0.2 }}>{twy || "TWY"}</span>; })()}
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* Stand — 80px */}
      <div
        className="flex-shrink-0 flex flex-col overflow-hidden cursor-pointer hover:brightness-95"
        style={{ width: 80, height: "100%", backgroundColor: standYellow ? COLOR_UNEXPECTED_YELLOW : undefined }}
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
    </>
  );
}
