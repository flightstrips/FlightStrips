import { useState } from "react";
import { getAircraftTypeWithWtc } from "@/lib/utils";
import type { StripProps } from "./types";
import {
  useStripCallsignInteraction,
  getFramedStripStyle,
  getCellBorderColor,
  SELECTION_COLOR,
  FONT,
  COLOR_UNEXPECTED_YELLOW,
  COLOR_MANUAL_BLUE,
  COLOR_TYPE_HEAVY,
  getStripOwnership,
  getCellTextColor,
  useStripBg,
  getValidationBlinkStyle,
} from "./shared";
import { getStripBg } from "./types";
import { useWebSocketStore, useIsTwr } from "@/store/store-hooks";
import { SIBox } from "./SIBox";
import { useStripTransfers } from "@/store/store-hooks";
import { useCDMColors } from "@/hooks/useCDMColors";
import { useCTOTColor } from "@/hooks/useCTOTColor";
import { Bay } from "@/api/models";
import { PushbackMapDialog } from "@/components/map-dialogs/PushbackMapDialog";
import { ApronTaxiMapDialog } from "@/components/map-dialogs/ApronTaxiMapDialog";
import { TaxiMapDialog } from "@/components/map-dialogs/TaxiMapDialog";
import { RunwayDialog } from "./RunwayDialog";
import FlightPlanDialog from "@/components/FlightPlanDialog";
import { ValidationStatusDialog } from "./ValidationStatusDialog";

// Height: 4.72vh(51px at 1080p)
const HALF_H = "2.36vh";    // half of 4.72vh for TSAT/CTOT split
const TOP_H  = "3.15vh";    // 2/3 of 4.72vh
const BOT_H  = "1.57vh";    // 1/3 of 4.72vh

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
 * Background: cyan (var(--color-strip-dep-bg)).
 */
export function ApnPushStrip({
  callsign,
  bay,
  pdcStatus,
  aircraftType,
  aircraftCategory,
  registration,
  stand,
  holdingPoint,
  tsat,
  tobt,
  ctot,
  runway,
  arrival,
  owner,
  nextControllers,
  previousControllers,
  myPosition,
  selectable,
  delegateCallsignClick = false,
  onStripMoved,
  marked = false,
  fullWidth = false,
  unexpectedChangeFields,
  controllerModifiedFields,
  isManual = false,
}: StripProps) {
  const { isSelected, handleClick, handleContextMenu, validationDialogOpen, setValidationDialogOpen, validationStatus } = useStripCallsignInteraction({ callsign, selectable, bay, owner, myPosition });
  const cellBorderColor = getCellBorderColor(marked);
  const manualBlue = isManual ? COLOR_MANUAL_BLUE : undefined;
  const stripTransfers = useStripTransfers();
  const isTagRequest = !!stripTransfers[callsign]?.isTagRequest;
  const { isUnconcerned } = getStripOwnership(myPosition, owner, nextControllers, previousControllers);
  const { bg, textWhite } = useStripBg(runway, getStripBg(pdcStatus, arrival, bay), isTagRequest, isUnconcerned, pdcStatus, bay);
  const isTwr = useIsTwr();
  const [pushbackOpen, setPushbackOpen] = useState(false);
  const [apronTaxiOpen, setApronTaxiOpen] = useState(false);
  const [taxiMapOpen, setTaxiMapOpen] = useState(false);
  const [runwayOpen, setRunwayOpen] = useState(false);
  const [fplOpen, setFplOpen] = useState(false);
  const acknowledgeUnexpectedChange = useWebSocketStore(s => s.acknowledgeUnexpectedChange);
  const standYellow = unexpectedChangeFields?.includes("stand");
  const runwayYellow = unexpectedChangeFields?.includes("runway");
  const { tsatBg } = useCDMColors({ bay: bay ?? Bay.Unknown, tsat: tsat ?? "", tobt: tobt ?? "" });
  const { ctotBg, ctotColor, showCtot } = useCTOTColor(ctot ?? "");

  return (
    <div
      className="select-none"
      style={{
        height: "4.72vh",
        width: fullWidth ? "100%" : "90%",
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
          style={{ flex: `${F_CALLSIGN} 0 0%`, height: "100%", minWidth: 0, borderRightColor: cellBorderColor, ...getValidationBlinkStyle(validationStatus, myPosition) }}
          onClick={delegateCallsignClick ? undefined : handleClick}
          onContextMenu={delegateCallsignClick ? undefined : handleContextMenu}
        >
          <div className="flex items-center pl-[0.42vw]" style={{ height: TOP_H, backgroundColor: isSelected ? SELECTION_COLOR : undefined }}>
            <span className="truncate w-full" style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "1.04vw", color: manualBlue }}>
              {callsign}
            </span>
          </div>
          <div style={{ height: BOT_H }} />
        </div>

        {/* A/C type / Registration — 25%*(2/3), stacked in top 2/3 */}
        <div
          className="flex flex-col items-center justify-center overflow-hidden border-r-2 cursor-pointer hover:brightness-95"
          style={{ flex: `${F_TYPE} 0 0%`, height: "100%", paddingBottom: "1.48vh", minWidth: 0, borderRightColor: cellBorderColor }}
          onClick={(e) => { e.stopPropagation(); setFplOpen(true); }}
        >
          <span className="truncate px-[0.21vw] leading-tight w-full text-center" style={{ fontFamily: FONT, fontSize: "0.52vw", color: aircraftCategory === "H" ? COLOR_TYPE_HEAVY : undefined }}>{getAircraftTypeWithWtc(aircraftType, aircraftCategory)}</span>
          <span className="truncate px-[0.21vw] leading-tight w-full text-center" style={{ fontFamily: FONT, fontSize: "0.52vw" }}>{registration}</span>
        </div>

        {/* Stand / Release Point — 25%*(2/3) */}
        <div
          className="flex items-center justify-center overflow-hidden border-r-2 cursor-pointer hover:bg-cyan-200"
          style={{ flex: `${F_STAND} 0 0%`, height: "100%", paddingBottom: "1.48vh", minWidth: 0, borderRightColor: cellBorderColor, backgroundColor: standYellow ? COLOR_UNEXPECTED_YELLOW : undefined }}
          onClick={(e) => { e.stopPropagation(); if (standYellow) { acknowledgeUnexpectedChange(callsign, "stand"); } else if (holdingPoint) { if (isTwr) { setTaxiMapOpen(true); } else { setApronTaxiOpen(true); } } else { setPushbackOpen(true); } }}
        >
          <span style={{ fontFamily: FONT, fontWeight: 600, fontSize: "1.04vw", color: getCellTextColor("stand", controllerModifiedFields) }}>
            {holdingPoint || stand}
          </span>
        </div>

        <PushbackMapDialog
          open={pushbackOpen}
          onOpenChange={setPushbackOpen}
          callsign={callsign}
          initialReleasePoint={holdingPoint}
          onStripMoved={onStripMoved}
        />
        <ApronTaxiMapDialog
          open={apronTaxiOpen}
          onOpenChange={setApronTaxiOpen}
          callsign={callsign}
        />
        <TaxiMapDialog
          open={taxiMapOpen}
          onOpenChange={setTaxiMapOpen}
          callsign={callsign}
        />

        {/* TSAT / CTOT — 25%*(2/3), split in half with border */}
        <div
          className="flex flex-col overflow-hidden border-r-2"
          style={{ flex: `${F_TSAT} 0 0%`, height: "100%", minWidth: 0, borderRightColor: cellBorderColor }}
        >
          <div className="flex items-center gap-[0.21vw] px-[0.21vw] border-b-2" style={{ height: HALF_H, borderBottomColor: cellBorderColor, backgroundColor: tsatBg || undefined }}>
            <span className="shrink-0" style={{ fontFamily: FONT, fontSize: "0.63vw" }}>TSAT</span>
            <span className="truncate" style={{ fontFamily: FONT, fontSize: "0.63vw" }}>{tsat}</span>
          </div>
          <div className="flex items-center gap-[0.21vw] px-[0.21vw]" style={{ height: HALF_H, backgroundColor: ctotBg || undefined, color: ctotColor }}>
            <span className="shrink-0" style={{ fontFamily: FONT, fontSize: "0.63vw" }}>{showCtot ? "CTOT" : ""}</span>
            <span className="truncate" style={{ fontFamily: FONT, fontSize: "0.63vw" }}>{showCtot ? ctot : ""}</span>
          </div>
        </div>

        {/* RWY — 25%*(2/3)*(2/3) */}
        <div
          className="flex items-center justify-center overflow-hidden cursor-pointer hover:bg-cyan-200"
          style={{ flex: `${F_RWY} 0 0%`, height: "100%", paddingBottom: "1.48vh", minWidth: 0, backgroundColor: runwayYellow ? COLOR_UNEXPECTED_YELLOW : undefined }}
          onClick={(e) => { e.stopPropagation(); if (runwayYellow) { acknowledgeUnexpectedChange(callsign, "runway"); } else { setRunwayOpen(true); } }}
        >
          <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "1.04vw", color: getCellTextColor("runway", controllerModifiedFields) }}>{runway}</span>
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
      <FlightPlanDialog callsign={callsign} open={fplOpen} onOpenChange={setFplOpen} mode="view" />
      {validationStatus && (
        <ValidationStatusDialog callsign={callsign} status={validationStatus} open={validationDialogOpen} onOpenChange={setValidationDialogOpen} />
      )}
    </div>
  );
}
