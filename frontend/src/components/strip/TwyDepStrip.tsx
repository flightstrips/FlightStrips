import { useState } from "react";
import { useControllers, useStripTransfers } from "@/store/store-hooks";
import { getStripBg } from "./types";
import type { StripProps } from "./types";
import { useStripSelection, getCellBorderColor, getFlatStripBorderStyle, SELECTION_COLOR } from "./shared";
import { TaxiMapDialog } from "../map-dialogs/TaxiMapDialog";
import { SIBox } from "./SIBox";

const FONT = "'Arial', sans-serif";

// Heights — 48px total, 2/3 top / 1/3 bottom (used by callsign and SID/dest)
const TOP_H = 32; // 2/3 of 48px
const BOT_H = 16; // 1/3 of 48px

// Equal halves — used by type/squawk, stand/ctot, empty/TWR, runway/HP, FL/heading
const HALF_H = 24; // 1/2 of 48px

// Fixed cell widths
const W_SI         = 40;
const W_CALLSIGN   = 120;
const W_TYPE_SQ    = 60;  // 120 / 2
const W_STAND_CTOT = 60;  // 120 / 2
const W_SID_DEST   = 80;  // 120 * 2/3
const W_SMALL      = 53;  // 80 * 2/3 (≈53.33) — used by empty/TWR, runway/HP, FL/heading

const TOTAL_W = W_SI + W_CALLSIGN + W_TYPE_SQ + W_STAND_CTOT + W_SMALL + W_SMALL + W_SMALL + W_SID_DEST;

// -----------------------------------------------------------------------------
// TwyDepStrip — TWY-DEP strip for the TETW tower view (status="TWY-DEP").
//
// 48px height, fixed width. Cells left → right:
//   [40px SI] | [120px callsign + :freq] | [60px type / squawk] |
//   [60px stand / ctot] | [53px empty / TWR label] |
//   [53px runway / HP] | [53px FL / heading] | [80px SID / dest]
//
// Background: cyan (#bef5ef).
// -----------------------------------------------------------------------------

export function TwyDepStrip({
  callsign,
  pdcStatus,
  aircraftType,
  squawk,
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
}: StripProps) {
  const { isSelected, handleClick } = useStripSelection(callsign, selectable);
  const stripTransfers = useStripTransfers();
  const cellBorderColor = getCellBorderColor(marked);
  const controllers = useControllers();
  const [showTaxiMap, setShowTaxiMap] = useState(false);

  const isAssumed = !!myPosition && owner === myPosition;

  // Next position frequency — controller.position IS the frequency string (e.g. "118.105")
  const nextPosition = nextControllers?.find(pos => pos !== myPosition);
  const nextController = controllers.find(c => c.position === nextPosition);
  const nextFreq = isAssumed && nextController ? `:${nextController.position}` : "";

  // Cleared FL — altitude in feet → FL (e.g. 12000 → "FL120")
  const fl = clearedAltitude ? `FL${Math.floor(clearedAltitude / 100)}` : "";
  const hdg = heading ? String(heading) : "";

  return (
    <>
    <div
      className={`flex text-black select-none${selectable ? " cursor-pointer" : ""}`}
      style={{
        height: 48,
        width: TOTAL_W,
        backgroundColor: getStripBg(pdcStatus),
        ...getFlatStripBorderStyle({ borderBottom: "1px solid white" }),
      }}
      onClick={handleClick}
    >
      {/* SI / ownership — 40px */}
      <SIBox
        callsign={callsign}
        owner={owner}
        nextControllers={nextControllers}
        previousControllers={previousControllers}
        myPosition={myPosition}
        transferringTo={stripTransfers[callsign] ?? ""}
      />

      {/* Callsign — 120px; top 2/3 = callsign, bottom 1/3 = :freq */}
      <div
        className="flex-shrink-0 flex flex-col border-r-2"
        style={{ width: W_CALLSIGN, height: "100%", borderRightColor: cellBorderColor }}
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

      {/* Type / Squawk — 60px, no dividing line; bold type, light squawk */}
      <div
        className="flex-shrink-0 flex flex-col border-r-2"
        style={{ width: W_TYPE_SQ, height: "100%", borderRightColor: cellBorderColor }}
      >
        <div className="flex items-center justify-center overflow-hidden" style={{ height: HALF_H }}>
          <span className="truncate px-1" style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 13 }}>
            {aircraftType}
          </span>
        </div>
        <div className="flex items-center justify-center overflow-hidden" style={{ height: HALF_H }}>
          <span className="truncate px-1" style={{ fontFamily: FONT, fontWeight: 300, fontSize: 13 }}>
            {squawk}
          </span>
        </div>
      </div>

      {/* Stand / CTOT — 60px, no dividing line; bold stand, light ctot */}
      <div
        className="flex-shrink-0 flex flex-col border-r-2"
        style={{ width: W_STAND_CTOT, height: "100%", borderRightColor: cellBorderColor }}
      >
        <div className="flex items-center justify-center overflow-hidden" style={{ height: HALF_H }}>
          <span className="truncate px-1" style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 13 }}>
            {stand}
          </span>
        </div>
        <div className="flex items-center justify-center overflow-hidden" style={{ height: HALF_H }}>
          <span className="truncate px-1" style={{ fontFamily: FONT, fontWeight: 300, fontSize: 13 }}>
            {ctot}
          </span>
        </div>
      </div>

      {/* Empty / TWR label — 53px; whole cell clickable → taxi map; top half empty, bottom half faint "TWR" */}
      <div
        className="flex-shrink-0 flex flex-col border-r-2 cursor-pointer"
        style={{ width: W_SMALL, height: "100%", borderRightColor: cellBorderColor }}
        onClick={(e) => { e.stopPropagation(); setShowTaxiMap(true); }}
      >
        <div style={{ height: HALF_H }} />
        <div className="flex items-center justify-center" style={{ height: HALF_H }}>
          <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 14, opacity: 0.2 }}>TWY</span>
        </div>
      </div>

      {/* Runway / HP — 53px, dividing line between; bold runway, plain holding point */}
      <div
        className="flex-shrink-0 flex flex-col border-r-2"
        style={{ width: W_SMALL, height: "100%", borderRightColor: cellBorderColor }}
      >
        <div
          className="flex items-center justify-center border-b-2"
          style={{ height: HALF_H, borderBottomColor: cellBorderColor }}
        >
          <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 14 }}>{runway}</span>
        </div>
        <div
          className="flex items-center justify-center cursor-pointer"
          style={{ height: HALF_H }}
          onClick={(e) => { e.stopPropagation(); setShowTaxiMap(true); }}
        >
          <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 14, opacity: holdingPoint ? 1 : 0.2 }}>
            {holdingPoint || "HP"}
          </span>
        </div>
      </div>

      {/* Cleared FL / Heading — 53px, no dividing line; both bold */}
      <div
        className="flex-shrink-0 flex flex-col border-r-2"
        style={{ width: W_SMALL, height: "100%", borderRightColor: cellBorderColor }}
      >
        <div className="flex items-center justify-center" style={{ height: HALF_H }}>
          <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 13 }}>{fl}</span>
        </div>
        <div className="flex items-center justify-center" style={{ height: HALF_H }}>
          <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 13 }}>{hdg}</span>
        </div>
      </div>

      {/* SID / Destination — 80px; two lines in top 2/3 (16px each), bottom 1/3 empty */}
      <div
        className="flex-shrink-0 flex flex-col overflow-hidden"
        style={{ width: W_SID_DEST, height: "100%" }}
      >
        <div className="flex items-center justify-center pl-1 overflow-hidden" style={{ height: TOP_H / 2 }}>
          <span className="truncate" style={{ fontFamily: FONT, fontWeight: "normal", fontSize: 12 }}>
            {sid}
          </span>
        </div>
        <div className="flex items-center justify-center pl-1 overflow-hidden" style={{ height: TOP_H / 2 }}>
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
    </>
  );
}
