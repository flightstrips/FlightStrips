import { useState } from "react";
import type { StripProps } from "./types";
import FlightPlanDialog from "@/components/FlightPlanDialog";
import {
  useStripSelection,
  getFramedStripStyle,
  getCellBorderColor,
  SELECTION_COLOR,
  FONT,
  COLOR_DEP_STRIP_BG,
  COLOR_UNEXPECTED_YELLOW,
  COLOR_TYPE_HEAVY,
  getStripOwnership,
  getCellTextColor,
  useStripBg,
} from "./shared";
import { SIBox } from "./SIBox";
import { getAircraftTypeWithWtc } from "@/lib/utils";
import { useStripTransfers, useWebSocketStore } from "@/store/store-hooks";
import { ApronTaxiMapDialog } from "@/components/map-dialogs/ApronTaxiMapDialog";
import { useCTOTColor } from "@/hooks/useCTOTColor";
const TOP_H  = "3.15vh";  // 2/3 of 4.72vh
const BOT_H  = "1.57vh";  // 1/3 of 4.72vh
const HALF_H = "2.08vh";  // 1/2 of inner content height (4.72vh - 2px border - 1px padding each side)

// Flex-grow proportions (flex-basis: 0 so space is shared proportionally)
const F_CALLSIGN = 25;
const F_TYPE     = 25 * (2 / 3);            // ~16.67
const F_STAND    = 25 * (2 / 3);            // ~16.67
const F_HP       = 25 * (2 / 3) * (2 / 3); // ~11.11
const F_RWY      = 25 * (2 / 3);            // ~16.67

export const APN_TAXI_DEP_STRIP_WIDTH = "90%";

/**
 * ApnTaxiDepStrip — APN-TAXI-DEP strip (status="TAXI-DEP").
 *
 * Width: 90% of bay. Cells use flex proportions:
 *   SI 8 | Callsign 25 | Type+Reg 25*(2/3) | Stand 25*(2/3) | HP 25*(2/3)*(2/3) | RWY 25*(2/3)
 *
 * Background: cyan (var(--color-strip-dep-bg)).
 */
export function ApnTaxiDepStrip({
  callsign,
  aircraftType,
  aircraftCategory,
  registration,
  stand,
  holdingPoint,
  runway,
  ctot,
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
  const stripTransfers = useStripTransfers();
  const isTagRequest = !!stripTransfers[callsign]?.isTagRequest;
  const { isUnconcerned } = getStripOwnership(myPosition, owner, nextControllers, previousControllers);
  const { bg, textWhite } = useStripBg(runway, COLOR_DEP_STRIP_BG, isTagRequest, isUnconcerned);
  const [showTaxiMap, setShowTaxiMap] = useState(false);
  const [fplOpen, setFplOpen] = useState(false);
  const { ctotBg, ctotColor, showCtot } = useCTOTColor(ctot ?? "");
  const acknowledgeUnexpectedChange = useWebSocketStore(s => s.acknowledgeUnexpectedChange);
  const openStripContextMenu = useWebSocketStore(s => s.openStripContextMenu);
  const standYellow = unexpectedChangeFields?.includes("stand");
  const runwayYellow = unexpectedChangeFields?.includes("runway");
  const releasePointYellow = unexpectedChangeFields?.includes("release_point");
  const isCoordinationMode = (!!owner && !!myPosition && owner !== myPosition) || !!releasePointYellow;

  const hpValue = holdingPoint ?? "";
  const hasTwy = hpValue.includes("/");

  return (
    <div
      className="select-none"
      style={{
        height: "4.44vh",
        width: APN_TAXI_DEP_STRIP_WIDTH,
        ...getFramedStripStyle(marked),
      }}
    >
      <div className={`flex ${textWhite ? "text-white" : "text-black"}`} style={{ height: "100%", overflow: "hidden", backgroundColor: bg }}>

        {/* SI / ownership — 8% */}
        <SIBox
          callsign={callsign}
          owner={owner}
          nextControllers={nextControllers}
          previousControllers={previousControllers}
          myPosition={myPosition}
          transferringTo={stripTransfers[callsign]?.to ?? ""}
          isTagRequest={isTagRequest}
        />

        {/* Callsign — 25%, FONT medium 20, top 2/3 highlighted when selected */}
        <div
          className="flex flex-col overflow-hidden border-r-2 cursor-pointer"
          style={{ flex: `${F_CALLSIGN} 0 0%`, height: "100%", minWidth: 0, borderRightColor: cellBorderColor }}
          onClick={handleClick}
          onContextMenu={(e) => { e.preventDefault(); openStripContextMenu(callsign, { x: e.clientX, y: e.clientY }); }}
        >
          <div className="flex items-center pl-[0.42vw]" style={{ height: TOP_H, backgroundColor: isSelected ? SELECTION_COLOR : undefined }}>
            <span className="truncate w-full" style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "1.04vw" }}>{callsign}</span>
          </div>
          <div style={{ height: BOT_H }} />
        </div>

        {/* A/C type / Registration — 25%*(2/3), stacked in top 2/3 */}
        <div
          className="flex flex-col items-center justify-center overflow-hidden border-r-2 cursor-pointer hover:brightness-95"
          style={{ flex: `${F_TYPE} 0 0%`, height: "100%", paddingBottom: BOT_H, minWidth: 0, borderRightColor: cellBorderColor }}
          onClick={(e) => { e.stopPropagation(); setFplOpen(true); }}
        >
          <span className="truncate px-[0.21vw] leading-tight w-full text-center" style={{ fontFamily: FONT, fontSize: "0.52vw", color: aircraftCategory === "H" ? COLOR_TYPE_HEAVY : undefined }}>
            {getAircraftTypeWithWtc(aircraftType, aircraftCategory)}
          </span>
          <span className="truncate px-[0.21vw] leading-tight w-full text-center" style={{ fontFamily: FONT, fontSize: "0.52vw" }}>
            {registration}
          </span>
        </div>

        {/* Stand — 25%*(2/3), stand in top 2/3, ctot in bottom 1/3 */}
        <div
          className="flex flex-col overflow-hidden border-r-2"
          style={{ flex: `${F_STAND} 0 0%`, height: "100%", minWidth: 0, borderRightColor: cellBorderColor, backgroundColor: standYellow ? COLOR_UNEXPECTED_YELLOW : undefined, cursor: standYellow ? "pointer" : undefined }}
          onClick={standYellow ? (e) => { e.stopPropagation(); acknowledgeUnexpectedChange(callsign, "stand"); } : undefined}
        >
          <div className="flex items-center justify-center" style={{ height: TOP_H }}>
            <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "1.04vw", color: getCellTextColor("stand", controllerModifiedFields) }}>{stand}</span>
          </div>
          <div className="flex items-center justify-center" style={{ height: BOT_H }}>
            {showCtot && <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "0.52vw", backgroundColor: ctotBg, color: ctotColor, padding: "0 0.1vw" }}>{ctot}</span>}
          </div>
        </div>

        {/* HP / TWY — 25%*(2/3)*(2/3) */}
        <div
          className="flex flex-col overflow-hidden border-r-2"
          style={{ flex: `${F_HP} 0 0%`, height: "100%", minWidth: 0, borderRightColor: cellBorderColor, cursor: "pointer", backgroundColor: releasePointYellow ? COLOR_UNEXPECTED_YELLOW : undefined }}
          onClick={(e) => { e.stopPropagation(); setShowTaxiMap(true); }}
        >
          <div className="flex items-center justify-center border-b-2" style={{ height: HALF_H, borderBottomColor: cellBorderColor }}>
            <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "0.57vw", opacity: hasTwy ? 1 : 0.15, color: getCellTextColor("release_point", controllerModifiedFields) }}>
              {hasTwy ? hpValue : "TWY"}
            </span>
          </div>
          <div className="flex items-center justify-center" style={{ height: HALF_H }}>
            <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "0.57vw", opacity: hasTwy ? 0.15 : 1, color: getCellTextColor("release_point", controllerModifiedFields) }}>
              {hasTwy ? "HP" : hpValue || "HP"}
            </span>
          </div>
        </div>

        {/* Runway — 25%*(2/3) */}
        <div
          className="flex flex-col overflow-hidden border-r-2"
          style={{ flex: `${F_RWY} 0 0%`, height: "100%", minWidth: 0, borderRight: 0, backgroundColor: runwayYellow ? COLOR_UNEXPECTED_YELLOW : undefined, cursor: runwayYellow ? "pointer" : undefined }}
          onClick={runwayYellow ? (e) => { e.stopPropagation(); acknowledgeUnexpectedChange(callsign, "runway"); } : undefined}
        >
          <div className="flex" style={{ height: HALF_H }}>
            <div className="flex items-center justify-center" style={{ flex: "2 0 0%", height: "100%" }}>
              <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "0.73vw", color: getCellTextColor("runway", controllerModifiedFields) }}>{runway}</span>
            </div>
            <div style={{ flexShrink: 0, width: HALF_H, height: "100%", borderLeft: `1px solid ${cellBorderColor}`, borderBottom: `1px solid ${cellBorderColor}` }} />
          </div>
          <div style={{ height: HALF_H }} />
        </div>

      </div>

      <ApronTaxiMapDialog
        open={showTaxiMap}
        onOpenChange={setShowTaxiMap}
        callsign={callsign}
        coordinationMode={isCoordinationMode}
      />
      <FlightPlanDialog callsign={callsign} open={fplOpen} onOpenChange={setFplOpen} mode="view" />
    </div>
  );
}
