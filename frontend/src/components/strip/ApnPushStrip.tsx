import { useState } from "react";
import type { StripProps } from "./types";
import {
  useStripSelection,
  getFramedStripStyle,
  getCellBorderColor,
  SELECTION_COLOR,
  FONT,
  COLOR_ARR_STRIP_BG,
  COLOR_UNEXPECTED_YELLOW,
  COLOR_MANUAL_BLUE,
  getCellTextColor,
} from "./shared";
import { useWebSocketStore } from "@/store/store-hooks";
import { SIBox } from "./SIBox";
import { useStripTransfers } from "@/store/store-hooks";
import { useCDMColors } from "@/hooks/useCDMColors";
import { useCTOTColor } from "@/hooks/useCTOTColor";
import { Bay } from "@/api/models";
import { PushbackMapDialog } from "@/components/map-dialogs/PushbackMapDialog";
import { ApronTaxiMapDialog } from "@/components/map-dialogs/ApronTaxiMapDialog";
import { RunwayDialog } from "./RunwayDialog";

// Height: 45px fixed (intentional — matches APN push strip spec)
const HALF_H = "2.22vh";    // half of 4.44vh for TSAT/CTOT split
const TOP_H  = "2.96vh";    // 2/3 of 4.44vh
const BOT_H  = "1.48vh";    // 1/3 of 4.44vh

// Flex-grow proportions (flex-basis: 0 so space is shared proportionally).
// Base flex unit. Each cell is a fraction of this base.
const F_BASE     = 25;
const F_CALLSIGN = F_BASE;                   // full width
const F_TYPE     = F_BASE * (2 / 3);         // 2/3 of callsign width  ~16.67
const F_STAND    = F_BASE * (2 / 3);         // 2/3 of callsign width  ~16.67
const F_TSAT     = F_BASE * (2 / 3);         // 2/3 of callsign width  ~16.67
const F_RWY      = F_BASE * (2 / 3) * (2 / 3); // 4/9 of callsign width ~11.11

/**
 * ApnPushStrip — APNPUSH strip for STARTUP, PUSH BACK and DE-ICE bays (status="PUSH").
 *
 * Width: 90% of bay. Cells use flex proportions:
 *   SI 8 | Callsign 25 | Type+Reg 25*(2/3) | Stand 25*(2/3) | TSAT/CTOT 25*(2/3) | RWY 25*(2/3)*(2/3)
 *
 * Background: cyan (#bef5ef).
 */
export function ApnPushStrip({
  callsign,
  bay,
  aircraftType,
  registration,
  stand,
  holdingPoint,
  tsat,
  tobt,
  ctot,
  runway,
  owner,
  nextControllers,
  previousControllers,
  myPosition,
  selectable,
  marked = false,
  fullWidth = false,
  unexpectedChangeFields,
  controllerModifiedFields,
  isManual = false,
}: StripProps) {
  const { isSelected, handleClick } = useStripSelection(callsign, selectable);
  const cellBorderColor = getCellBorderColor(marked);
  const manualBlue = isManual ? COLOR_MANUAL_BLUE : undefined;
  const stripTransfers = useStripTransfers();
  const isTagRequest = !!stripTransfers[callsign]?.isTagRequest;
  const [pushbackOpen, setPushbackOpen] = useState(false);
  const [apronTaxiOpen, setApronTaxiOpen] = useState(false);
  const [runwayOpen, setRunwayOpen] = useState(false);
  const acknowledgeUnexpectedChange = useWebSocketStore(s => s.acknowledgeUnexpectedChange);
  const openStripContextMenu = useWebSocketStore(s => s.openStripContextMenu);
  const standYellow = unexpectedChangeFields?.includes("stand");
  const runwayYellow = unexpectedChangeFields?.includes("runway");
  const { tsatBg } = useCDMColors({ bay: bay ?? Bay.Unknown, tsat: tsat ?? "", tobt: tobt ?? "" });
  const { ctotBg, ctotColor, showCtot } = useCTOTColor(ctot ?? "");

  return (
    <div
      className={`select-none${selectable ? " cursor-pointer" : ""}`}
      style={{
        height: 45, // 45px fixed — intentional APN push strip height
        width: fullWidth ? "100%" : "90%",
        ...getFramedStripStyle(marked),
      }}
      onClick={handleClick}
      onContextMenu={(e) => { e.preventDefault(); openStripContextMenu(callsign, { x: e.clientX, y: e.clientY }); }}
    >
      <div className="flex text-black" style={{ height: "100%", overflow: "hidden", backgroundColor: isTagRequest ? SELECTION_COLOR : COLOR_ARR_STRIP_BG }}>
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
          className="flex flex-col overflow-hidden border-r-2"
          style={{ flex: `${F_CALLSIGN} 0 0%`, height: "100%", minWidth: 0, borderRightColor: cellBorderColor }}
        >
          <div className="flex items-center pl-2" style={{ height: TOP_H, backgroundColor: isSelected ? SELECTION_COLOR : undefined }}>
            <span className="truncate w-full" style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 20, color: manualBlue }}>
              {callsign}
            </span>
          </div>
          <div style={{ height: BOT_H }} />
        </div>

        {/* A/C type / Registration — 25%*(2/3), stacked in top 2/3 */}
        <div
          className="flex flex-col items-center justify-center overflow-hidden border-r-2"
          style={{ flex: `${F_TYPE} 0 0%`, height: "100%", paddingBottom: "1.48vh", minWidth: 0, borderRightColor: cellBorderColor }}
        >
          <span className="truncate px-1 leading-tight w-full text-center" style={{ fontFamily: FONT, fontSize: 10 }}>{aircraftType?.split("/")[0]}</span>
          <span className="truncate px-1 leading-tight w-full text-center" style={{ fontFamily: FONT, fontSize: 10 }}>{registration}</span>
        </div>

        {/* Stand / Release Point — 25%*(2/3) */}
        <div
          className="flex items-center justify-center overflow-hidden border-r-2 cursor-pointer hover:bg-cyan-200"
          style={{ flex: `${F_STAND} 0 0%`, height: "100%", paddingBottom: "1.48vh", minWidth: 0, borderRightColor: cellBorderColor, backgroundColor: standYellow ? COLOR_UNEXPECTED_YELLOW : undefined }}
          onClick={(e) => { e.stopPropagation(); if (standYellow) { acknowledgeUnexpectedChange(callsign, "stand"); } else if (holdingPoint) { setApronTaxiOpen(true); } else { setPushbackOpen(true); } }}
        >
          <span style={{ fontFamily: FONT, fontWeight: 600, fontSize: 20, color: getCellTextColor("stand", controllerModifiedFields) }}>
            {holdingPoint || stand}
          </span>
        </div>

        <PushbackMapDialog
          open={pushbackOpen}
          onOpenChange={setPushbackOpen}
          callsign={callsign}
          initialReleasePoint={holdingPoint}
        />
        <ApronTaxiMapDialog
          open={apronTaxiOpen}
          onOpenChange={setApronTaxiOpen}
          callsign={callsign}
        />

        {/* TSAT / CTOT — 25%*(2/3), split in half with border */}
        <div
          className="flex flex-col overflow-hidden border-r-2"
          style={{ flex: `${F_TSAT} 0 0%`, height: "100%", minWidth: 0, borderRightColor: cellBorderColor }}
        >
          <div className="flex items-center gap-1 px-1 border-b-2" style={{ height: HALF_H, borderBottomColor: cellBorderColor, backgroundColor: tsatBg || undefined }}>
            <span className="shrink-0" style={{ fontFamily: FONT, fontSize: 12 }}>TSAT</span>
            <span className="truncate" style={{ fontFamily: FONT, fontSize: 12 }}>{tsat}</span>
          </div>
          <div className="flex items-center gap-1 px-1" style={{ height: HALF_H, backgroundColor: ctotBg || undefined, color: ctotColor }}>
            <span className="shrink-0" style={{ fontFamily: FONT, fontSize: 12 }}>{showCtot ? "CTOT" : ""}</span>
            <span className="truncate" style={{ fontFamily: FONT, fontSize: 12 }}>{showCtot ? ctot : ""}</span>
          </div>
        </div>

        {/* RWY — 25%*(2/3)*(2/3) */}
        <div
          className="flex items-center justify-center overflow-hidden cursor-pointer hover:bg-cyan-200"
          style={{ flex: `${F_RWY} 0 0%`, height: "100%", paddingBottom: "1.48vh", minWidth: 0, backgroundColor: runwayYellow ? COLOR_UNEXPECTED_YELLOW : undefined }}
          onClick={(e) => { e.stopPropagation(); if (runwayYellow) { acknowledgeUnexpectedChange(callsign, "runway"); } else { setRunwayOpen(true); } }}
        >
          <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 20, color: getCellTextColor("runway", controllerModifiedFields) }}>{runway}</span>
        </div>
      </div>

      <RunwayDialog
        open={runwayOpen}
        onOpenChange={setRunwayOpen}
        mode="ASSIGN"
        callsign={callsign}
        direction="departure"
        currentRunway={runway}
      />
    </div>
  );
}
