import { useState } from "react";
import type { StripProps } from "./types";
import { useStripSelection, getCellBorderColor, getFlatStripBorderStyle, SELECTION_COLOR, COLOR_ARR_YELLOW, COLOR_BTN_ORANGE, COLOR_UNEXPECTED_YELLOW, getCellTextColor } from "./shared";
import { useControllers, useWebSocketStore } from "@/store/store-hooks";
import { RunwayDialog } from "./RunwayDialog";
import { ArrStandDialog } from "./ArrStandDialog";

// Height: 48px fixed (intentional — matches FinalArrStrip ATC arrival strip spec)
const TOP_H = 32; // 2/3 of 48px
const BOT_H = 16; // 1/3 of 48px

// SI box background colors indicating strip ownership state
const COLOR_SI_UNCONCERNED    = "#808080";
const COLOR_SI_ASSUMED        = "#F0F0F0";
const COLOR_SI_TRANSFERRED    = COLOR_BTN_ORANGE;  // same orange as accent buttons
const COLOR_SI_CONCERNED      = "#E082E7";

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
}: StripProps) {
  const { isSelected, handleClick } = useStripSelection(callsign, selectable);
  const cellBorderColor = getCellBorderColor(marked);
  const controllers = useControllers();
  const [runwayOpen, setRunwayOpen] = useState(false);
  const [standOpen, setStandOpen] = useState(false);
  const acknowledgeUnexpectedChange = useWebSocketStore(s => s.acknowledgeUnexpectedChange);
  const openStripContextMenu = useWebSocketStore(s => s.openStripContextMenu);
  const standYellow = unexpectedChangeFields?.includes("stand");
  const runwayYellow = unexpectedChangeFields?.includes("runway");

  const isAssumed = !!myPosition && owner === myPosition;
  const isTransferredAway = !!myPosition && !!previousControllers?.includes(myPosition);
  const isConcerned = !!myPosition && !!nextControllers?.includes(myPosition);

  let siBg = COLOR_SI_UNCONCERNED;
  if (isAssumed) siBg = COLOR_SI_ASSUMED;
  else if (isTransferredAway) siBg = COLOR_SI_TRANSFERRED;
  else if (isConcerned) siBg = COLOR_SI_CONCERNED;

  const nextPosition = nextControllers?.find(pos => pos !== myPosition);
  const nextController = controllers.find(c => c.position === nextPosition);
  const nextLabel = isAssumed && nextController ? nextController.identifier : "";

  return (
    <>
    <div
      className={`flex text-black select-none${selectable ? " cursor-pointer" : ""}`}
      style={{
        height: 48, // 48px fixed — intentional ATC arrival strip height
        width: 428,
        backgroundColor: COLOR_ARR_YELLOW,
        ...getFlatStripBorderStyle(),
      }}
      onClick={handleClick}
      onContextMenu={(e) => { e.preventDefault(); openStripContextMenu(callsign, { x: e.clientX, y: e.clientY }); }}
    >
      {/* SI / ownership — 40px */}
      <div
        className="flex-shrink-0 flex items-center justify-center text-sm font-bold border-r-2"
        style={{ width: 40, height: "100%", backgroundColor: siBg, borderRightColor: cellBorderColor }}
      >
        {nextLabel}
      </div>

      {/* Callsign — 120px */}
      <div className="flex-shrink-0 flex flex-col border-r-2" style={{ width: 120, height: "100%", borderRightColor: cellBorderColor }}>
        <div className="flex items-center pl-2" style={{ height: TOP_H, backgroundColor: isSelected ? SELECTION_COLOR : undefined }}>
          <span className="font-bold text-xl truncate w-full">{callsign}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* A/C type / Registration — 80px */}
      <div className="flex-shrink-0 flex flex-col border-r-2" style={{ width: 80, height: "100%", borderRightColor: cellBorderColor }}>
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="text-xs font-semibold truncate px-1">{aircraftType}</span>
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
      <div className="flex-shrink-0 flex flex-col overflow-hidden border-r-2" style={{ width: 54, height: "100%", borderRightColor: cellBorderColor }}>
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="font-bold text-xl truncate">{taxiway ?? holdingPoint}</span>
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
    </>
  );
}
